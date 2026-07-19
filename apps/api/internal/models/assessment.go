package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/renovisaude/renovi-care/internal/db/gen"
	"github.com/renovisaude/renovi-care/internal/models/careline"
	"github.com/renovisaude/renovi-care/internal/models/mood/scoring"
)

// Códigos e refs dos instrumentos periódicos.
const (
	Who5Codigo  = "WHO5"
	Phq4Codigo  = "PHQ4"
	Who5ItemRef = "who5-semanal"
	Phq4ItemRef = "phq4-gatilhado"
)

// assessmentSpec descreve um instrumento periódico: item da linha + formato.
type assessmentSpec struct {
	itemRef   string
	itemCount int
	valueMin  int
	valueMax  int
}

var assessmentSpecs = map[string]assessmentSpec{
	Who5Codigo: {itemRef: Who5ItemRef, itemCount: 5, valueMin: 0, valueMax: 5},
	Phq4Codigo: {itemRef: Phq4ItemRef, itemCount: 4, valueMin: 0, valueMax: 3},
}

var (
	// ErrUnknownInstrument: código de instrumento não suportado.
	ErrUnknownInstrument = errors.New("assessment: instrumento desconhecido")
	// ErrAssessmentInvalid: quantidade/valores de respostas inválidos.
	ErrAssessmentInvalid = errors.New("assessment: respostas inválidas")
)

// ErrAssessmentBlocked indica que a cadência (motor) não permite responder agora.
// Carrega os Blocks para o controller devolver reason/available_from ao front.
type ErrAssessmentBlocked struct {
	Blocks []careline.Block
}

func (e ErrAssessmentBlocked) Error() string {
	return "assessment: cadência não permite responder ainda"
}

// AssessmentAvailability é a disponibilidade + o descritor do instrumento.
type AssessmentAvailability struct {
	Codigo      string
	Eligibility careline.Eligibility
	ItemCount   int
	ValueMin    int
	ValueMax    int
}

// AssessmentResult é o resultado pontuado de um instrumento periódico.
type AssessmentResult struct {
	Codigo         string
	RawScore       float64
	IndexScore     *float64
	Subscores      map[string]int
	Faixa          string
	FlagEncaminhar bool
	RespondidoEm   time.Time
}

// AssessmentStore aplica e pontua os instrumentos periódicos (WHO-5, PHQ-4),
// reusando o motor puro de linhas de cuidado para a cadência (MIN_INTERVAL).
type AssessmentStore struct {
	pool *pgxpool.Pool
	q    *gen.Queries
}

func NewAssessmentStore(pool *pgxpool.Pool) *AssessmentStore {
	return &AssessmentStore{pool: pool, q: gen.New(pool)}
}

// Availability diz se o instrumento pode ser respondido agora — VIGENCIA e
// MIN_INTERVAL avaliados pelo MOTOR sobre o histórico imutável de aplicações.
func (s *AssessmentStore) Availability(ctx context.Context, patientID uuid.UUID, codigo string, now time.Time) (AssessmentAvailability, error) {
	spec, ok := assessmentSpecs[codigo]
	if !ok {
		return AssessmentAvailability{}, ErrUnknownInstrument
	}
	_, elig, _, err := s.resolve(ctx, patientID, spec.itemRef, now)
	if err != nil {
		return AssessmentAvailability{}, err
	}
	return AssessmentAvailability{
		Codigo: codigo, Eligibility: elig,
		ItemCount: spec.itemCount, ValueMin: spec.valueMin, ValueMax: spec.valueMax,
	}, nil
}

// resolve monta a Journey a partir dos fatos imutáveis e chama o motor. É a peça
// de wiring central do módulo: os fatos de execução da ATIVIDADE entram no shape
// de JourneyAppointment (Status=realizada, ScheduledAt=respondido_em), sem tocar
// a tabela normativa T1–T19 do motor.
func (s *AssessmentStore) resolve(ctx context.Context, patientID uuid.UUID, itemRef string, now time.Time) (gen.FindActivityEnrollmentDetailRow, careline.Eligibility, bool, error) {
	det, err := s.q.FindActivityEnrollmentDetail(ctx, gen.FindActivityEnrollmentDetailParams{
		PatientID: patientID, ValidFrom: now, Ref: itemRef,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return det, careline.Eligibility{Allowed: false, Blocks: []careline.Block{{
				RuleType: careline.RuleVigencia,
				Reason:   "Você não tem uma linha de cuidado ativa com este instrumento",
			}}}, false, nil
		}
		return det, careline.Eligibility{}, false, fmt.Errorf("consultar matrícula: %w", err)
	}

	ruleRows, err := s.q.ListItemRules(ctx, det.CareLineItemID)
	if err != nil {
		return det, careline.Eligibility{}, false, fmt.Errorf("listar regras: %w", err)
	}
	rules := make([]careline.Rule, 0, len(ruleRows))
	for _, r := range ruleRows {
		rules = append(rules, careline.Rule{Type: r.RuleType, Params: json.RawMessage(r.Params)})
	}

	times, err := s.q.ListAssessmentTimes(ctx, gen.ListAssessmentTimesParams{
		PatientID: patientID, CareLineItemID: det.CareLineItemID,
	})
	if err != nil {
		return det, careline.Eligibility{}, false, fmt.Errorf("listar aplicações: %w", err)
	}
	appts := make([]careline.JourneyAppointment, 0, len(times))
	for _, t := range times {
		appts = append(appts, careline.JourneyAppointment{
			ItemRef: itemRef, Status: careline.StatusRealizada, ScheduledAt: t,
		})
	}

	journey := careline.Journey{
		Status: det.Status, ValidFrom: det.ValidFrom, ValidUntil: det.ValidUntil,
		Appointments: appts,
	}
	item := careline.Item{Ref: itemRef, Kind: careline.KindAtividade}
	// intendedAt = now: a aplicação é agora; o motor decide se a cadência permite.
	return det, careline.Evaluate(journey, item, rules, now, now), true, nil
}

// Submit valida consentimento + cadência (motor), pontua de forma determinística
// e persiste (respostas + assessment + fato na jornada), atomicamente.
func (s *AssessmentStore) Submit(ctx context.Context, patientID uuid.UUID, codigo string, items []int, now time.Time) (AssessmentResult, error) {
	spec, ok := assessmentSpecs[codigo]
	if !ok {
		return AssessmentResult{}, ErrUnknownInstrument
	}
	if len(items) != spec.itemCount {
		return AssessmentResult{}, ErrAssessmentInvalid
	}

	consent, err := s.q.GetActiveConsent(ctx, gen.GetActiveConsentParams{PatientID: patientID, Finalidade: ConsentCheckinHumor})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AssessmentResult{}, ErrNoActiveConsent
		}
		return AssessmentResult{}, fmt.Errorf("consultar consentimento: %w", err)
	}

	det, elig, enrolled, err := s.resolve(ctx, patientID, spec.itemRef, now)
	if err != nil {
		return AssessmentResult{}, err
	}
	if !enrolled {
		return AssessmentResult{}, ErrNotEnrolledInActivity
	}
	if !elig.Allowed {
		return AssessmentResult{}, ErrAssessmentBlocked{Blocks: elig.Blocks}
	}

	inst, err := s.q.GetActiveInstrument(ctx, codigo)
	if err != nil {
		return AssessmentResult{}, fmt.Errorf("carregar instrumento: %w", err)
	}

	scored, err := s.score(ctx, codigo, inst.ID, items)
	if err != nil {
		return AssessmentResult{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return AssessmentResult{}, fmt.Errorf("abrir transação: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	id, err := uuid.NewV7()
	if err != nil {
		return AssessmentResult{}, fmt.Errorf("gerar uuid v7: %w", err)
	}
	var subscoresJSON []byte
	if scored.Subscores != nil {
		if subscoresJSON, err = json.Marshal(scored.Subscores); err != nil {
			return AssessmentResult{}, fmt.Errorf("serializar subscores: %w", err)
		}
	}
	row, err := q.InsertWellbeingAssessment(ctx, gen.InsertWellbeingAssessmentParams{
		ID: id, PatientID: patientID, EnrollmentID: det.EnrollmentID,
		CareLineItemID: det.CareLineItemID, ConsentID: consent.ID, InstrumentID: inst.ID,
		RawScore: numericFromInt(int(scored.RawScore)), IndexScore: numericPtr(scored.IndexScore),
		Subscores: subscoresJSON, Faixa: scored.Faixa, FlagEncaminhar: scored.FlagEncaminhar,
		RespondidoEm: now,
	})
	if err != nil {
		return AssessmentResult{}, fmt.Errorf("gravar assessment: %w", err)
	}

	for i, v := range items {
		rid, err := uuid.NewV7()
		if err != nil {
			return AssessmentResult{}, fmt.Errorf("gerar uuid v7 da resposta: %w", err)
		}
		if err := q.InsertAssessmentItemResponse(ctx, gen.InsertAssessmentItemResponseParams{
			ID: rid, AssessmentID: row.ID, ItemOrdem: int32(i + 1), Valor: int32(v),
		}); err != nil {
			return AssessmentResult{}, fmt.Errorf("gravar resposta: %w", err)
		}
	}

	payload, err := json.Marshal(map[string]any{
		"codigo": codigo, "faixa": scored.Faixa, "flag_encaminhar": scored.FlagEncaminhar,
	})
	if err != nil {
		return AssessmentResult{}, fmt.Errorf("serializar payload: %w", err)
	}
	evID, err := uuid.NewV7()
	if err != nil {
		return AssessmentResult{}, fmt.Errorf("gerar uuid v7 do evento: %w", err)
	}
	if _, err := q.InsertJourneyEvent(ctx, gen.InsertJourneyEventParams{
		ID: evID, EnrollmentID: det.EnrollmentID, PatientID: patientID,
		EventType: "assessment_respondido", Actor: "paciente",
		RefTable: pgtype.Text{String: "wellbeing_assessment", Valid: true},
		RefID:    pgUUID(row.ID),
		Payload:  payload,
	}); err != nil {
		return AssessmentResult{}, fmt.Errorf("emitir evento: %w", err)
	}

	// Rastreio positivo => escalonamento à trilha CLÍNICA (nunca ao gestor). É um
	// fato próprio na jornada; o roteamento efetivo entra quando a trilha clínica
	// existir (ver PROGRESSO). actor=sistema: quem escala é a regra, não o paciente.
	if scored.FlagEncaminhar {
		escID, err := uuid.NewV7()
		if err != nil {
			return AssessmentResult{}, fmt.Errorf("gerar uuid v7 do escalonamento: %w", err)
		}
		escPayload, err := json.Marshal(map[string]any{
			"codigo": codigo, "faixa": scored.Faixa, "origem": "rastreio_positivo",
		})
		if err != nil {
			return AssessmentResult{}, fmt.Errorf("serializar escalonamento: %w", err)
		}
		if _, err := q.InsertJourneyEvent(ctx, gen.InsertJourneyEventParams{
			ID: escID, EnrollmentID: det.EnrollmentID, PatientID: patientID,
			EventType: "escalonamento_clinico", Actor: "sistema",
			RefTable: pgtype.Text{String: "wellbeing_assessment", Valid: true},
			RefID:    pgUUID(row.ID),
			Payload:  escPayload,
		}); err != nil {
			return AssessmentResult{}, fmt.Errorf("emitir escalonamento: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return AssessmentResult{}, fmt.Errorf("commit: %w", err)
	}

	scored.Codigo = codigo
	scored.RespondidoEm = now
	return scored, nil
}

// score pontua os itens conforme o instrumento, com os cortes do banco.
func (s *AssessmentStore) score(ctx context.Context, codigo string, instrumentID uuid.UUID, items []int) (AssessmentResult, error) {
	switch codigo {
	case Who5Codigo:
		cutoffs, err := s.who5Cutoffs(ctx, instrumentID)
		if err != nil {
			return AssessmentResult{}, err
		}
		r, err := scoring.ScoreWHO5(items, cutoffs)
		if err != nil {
			return AssessmentResult{}, fmt.Errorf("%w: %v", ErrAssessmentInvalid, err)
		}
		index := float64(r.Index)
		return AssessmentResult{
			RawScore: float64(r.Raw), IndexScore: &index,
			Faixa: r.Faixa, FlagEncaminhar: r.FlagEncaminhar,
		}, nil
	case Phq4Codigo:
		cutoffs, err := s.phq4Cutoffs(ctx, instrumentID)
		if err != nil {
			return AssessmentResult{}, err
		}
		r, err := scoring.ScorePHQ4(items, cutoffs)
		if err != nil {
			return AssessmentResult{}, fmt.Errorf("%w: %v", ErrAssessmentInvalid, err)
		}
		// PHQ-4 não tem índice 0–100; guarda as subescalas PHQ-2/GAD-2.
		return AssessmentResult{
			RawScore:       float64(r.Total),
			Subscores:      map[string]int{"phq2": r.PHQ2, "gad2": r.GAD2},
			Faixa:          r.Faixa,
			FlagEncaminhar: r.FlagEncaminhar,
		}, nil
	default:
		return AssessmentResult{}, ErrUnknownInstrument
	}
}

// phq4Cutoffs carrega os cortes do PHQ-4 do banco (validação BR versionada).
func (s *AssessmentStore) phq4Cutoffs(ctx context.Context, instrumentID uuid.UUID) (scoring.PHQ4Cutoffs, error) {
	rows, err := s.q.ListInstrumentCutoffs(ctx, instrumentID)
	if err != nil {
		return scoring.PHQ4Cutoffs{}, fmt.Errorf("listar cortes: %w", err)
	}
	c := scoring.PHQ4Cutoffs{SubescalaPositiva: 3, TotalModerado: 6} // defaults se faltar seed
	for _, r := range rows {
		v := int(numericToFloat(r.Valor))
		switch r.Faixa {
		case "subescala_positiva":
			c.SubescalaPositiva = v
		case "moderado":
			c.TotalModerado = v
		}
	}
	return c, nil
}

// who5Cutoffs carrega os cortes do WHO-5 do banco (validação BR versionada).
func (s *AssessmentStore) who5Cutoffs(ctx context.Context, instrumentID uuid.UUID) (scoring.WHO5Cutoffs, error) {
	rows, err := s.q.ListInstrumentCutoffs(ctx, instrumentID)
	if err != nil {
		return scoring.WHO5Cutoffs{}, fmt.Errorf("listar cortes: %w", err)
	}
	c := scoring.WHO5Cutoffs{Sinaliza: 50, Encaminha: 28} // defaults se faltar seed
	for _, r := range rows {
		if r.Dimensao != "bem_estar" {
			continue
		}
		v := int(numericToFloat(r.Valor))
		switch r.Faixa {
		case "sinaliza":
			c.Sinaliza = v
		case "encaminha":
			c.Encaminha = v
		}
	}
	return c, nil
}

// numericFromInt embrulha um int como pgtype.Numeric válido.
func numericFromInt(i int) pgtype.Numeric {
	return pgtype.Numeric{Int: big.NewInt(int64(i)), Valid: true}
}

// numericPtr embrulha um *float64 como pgtype.Numeric (nil -> nulo). Usa a parte
// inteira: os escores deste anexo são inteiros (índice 0–100, total 0–12).
func numericPtr(f *float64) pgtype.Numeric {
	if f == nil {
		return pgtype.Numeric{}
	}
	return numericFromInt(int(*f))
}
