package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/renovisaude/renovi-care/internal/db/gen"
	"github.com/renovisaude/renovi-care/internal/models/mood/scoring"
)

// CheckinHumorDiarioRef é o ref do item de atividade do check-in diário na linha.
const CheckinHumorDiarioRef = "checkin-humor-diario"

// instrumentGrid é o código do instrumento do anel diário.
const instrumentGrid = "GRID"

var (
	// ErrMoodCheckinInvalid: coordenadas fora de 0–100.
	ErrMoodCheckinInvalid = errors.New("mood: dados do check-in inválidos")
	// ErrNotEnrolledInActivity: sem matrícula ativa/vigente numa linha com o item.
	ErrNotEnrolledInActivity = errors.New("mood: sem matrícula elegível para o check-in")
)

// MoodCheckin é um check-in diário (visão de domínio, sem ids internos).
type MoodCheckin struct {
	Valencia     int
	Energia      int
	Quadrante    string
	EmotionLabel *string
	ContextTags  []string
	RespondidoEm time.Time
}

// MoodCheckinInput são os dados de entrada de um check-in.
type MoodCheckinInput struct {
	Valencia     int
	Energia      int
	EmotionLabel *string
	ContextTags  []string
}

// MoodToday resume o dia do paciente: elegibilidade + o check-in de hoje.
type MoodToday struct {
	Dia        time.Time // dia local (meia-noite civil)
	CanCheckin bool
	Reason     string // "" | "consent_required" | "not_enrolled"
	Checkin    *MoodCheckin
}

// Motivos de inelegibilidade (máquina-legíveis para o front).
const (
	ReasonConsentRequired = "consent_required"
	ReasonNotEnrolled     = "not_enrolled"
)

// brLocation: o "dia" do check-in é o do colaborador (America/Sao_Paulo), não o
// dia UTC — evita a fronteira de meia-noite do fuso corromper o "1 por dia".
var brLocation = loadBR()

func loadBR() *time.Location {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return time.UTC
	}
	return loc
}

// localDay devolve a meia-noite civil do dia local de `now`.
func localDay(now time.Time) time.Time {
	y, m, d := now.In(brLocation).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// MoodCheckinStore é a camada de dados + regra do anel diário.
type MoodCheckinStore struct {
	pool *pgxpool.Pool
	q    *gen.Queries
}

func NewMoodCheckinStore(pool *pgxpool.Pool) *MoodCheckinStore {
	return &MoodCheckinStore{pool: pool, q: gen.New(pool)}
}

// Record grava (ou atualiza) o check-in do dia. Pré-condições DERIVADAS sob
// demanda: consentimento ativo (LGPD) e matrícula elegível. Deriva o quadrante
// de forma determinística e emite o fato de execução na jornada (append-only).
func (s *MoodCheckinStore) Record(ctx context.Context, patientID uuid.UUID, in MoodCheckinInput, now time.Time) (MoodCheckin, error) {
	if in.Valencia < 0 || in.Valencia > 100 || in.Energia < 0 || in.Energia > 100 {
		return MoodCheckin{}, ErrMoodCheckinInvalid
	}

	consent, err := s.q.GetActiveConsent(ctx, gen.GetActiveConsentParams{PatientID: patientID, Finalidade: ConsentCheckinHumor})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MoodCheckin{}, ErrNoActiveConsent
		}
		return MoodCheckin{}, fmt.Errorf("consultar consentimento: %w", err)
	}

	act, err := s.q.FindActivityEnrollment(ctx, gen.FindActivityEnrollmentParams{
		PatientID: patientID, ValidFrom: now, Ref: CheckinHumorDiarioRef,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MoodCheckin{}, ErrNotEnrolledInActivity
		}
		return MoodCheckin{}, fmt.Errorf("consultar matrícula: %w", err)
	}

	grid, err := s.q.GetActiveInstrument(ctx, instrumentGrid)
	if err != nil {
		return MoodCheckin{}, fmt.Errorf("carregar instrumento GRID: %w", err)
	}

	quadrante := scoring.Quadrant(in.Valencia, in.Energia)

	var tagsJSON []byte
	if len(in.ContextTags) > 0 {
		tagsJSON, err = json.Marshal(in.ContextTags)
		if err != nil {
			return MoodCheckin{}, fmt.Errorf("serializar context_tags: %w", err)
		}
	}
	diaRef := pgtype.Date{Time: localDay(now), Valid: true}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return MoodCheckin{}, fmt.Errorf("abrir transação: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	id, err := uuid.NewV7()
	if err != nil {
		return MoodCheckin{}, fmt.Errorf("gerar uuid v7: %w", err)
	}
	row, err := q.UpsertMoodCheckin(ctx, gen.UpsertMoodCheckinParams{
		ID: id, PatientID: patientID, EnrollmentID: act.EnrollmentID,
		CareLineItemID: act.CareLineItemID, ConsentID: consent.ID, InstrumentID: grid.ID,
		Valencia: int32(in.Valencia), Energia: int32(in.Energia), Quadrante: quadrante,
		EmotionLabel: textPtr(in.EmotionLabel), ContextTags: tagsJSON,
		DiaRef: diaRef, RespondidoEm: now,
	})
	if err != nil {
		return MoodCheckin{}, fmt.Errorf("gravar check-in: %w", err)
	}

	payload, err := json.Marshal(map[string]any{"quadrante": quadrante, "instrument": grid.Codigo})
	if err != nil {
		return MoodCheckin{}, fmt.Errorf("serializar payload do evento: %w", err)
	}
	evID, err := uuid.NewV7()
	if err != nil {
		return MoodCheckin{}, fmt.Errorf("gerar uuid v7 do evento: %w", err)
	}
	if _, err := q.InsertJourneyEvent(ctx, gen.InsertJourneyEventParams{
		ID: evID, EnrollmentID: act.EnrollmentID, PatientID: patientID,
		EventType: "checkin_humor_registrado", Actor: "paciente",
		RefTable: pgtype.Text{String: "mood_checkin", Valid: true},
		RefID:    pgUUID(row.ID),
		Payload:  payload,
	}); err != nil {
		return MoodCheckin{}, fmt.Errorf("emitir evento: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return MoodCheckin{}, fmt.Errorf("commit: %w", err)
	}
	return toMoodCheckin(row), nil
}

// Today devolve a elegibilidade do paciente e o check-in de hoje (se houver).
func (s *MoodCheckinStore) Today(ctx context.Context, patientID uuid.UUID, now time.Time) (MoodToday, error) {
	dia := localDay(now)
	out := MoodToday{Dia: dia}

	_, cErr := s.q.GetActiveConsent(ctx, gen.GetActiveConsentParams{PatientID: patientID, Finalidade: ConsentCheckinHumor})
	switch {
	case errors.Is(cErr, pgx.ErrNoRows):
		out.Reason = ReasonConsentRequired
	case cErr != nil:
		return MoodToday{}, fmt.Errorf("consultar consentimento: %w", cErr)
	}
	if out.Reason == "" {
		_, eErr := s.q.FindActivityEnrollment(ctx, gen.FindActivityEnrollmentParams{PatientID: patientID, ValidFrom: now, Ref: CheckinHumorDiarioRef})
		switch {
		case errors.Is(eErr, pgx.ErrNoRows):
			out.Reason = ReasonNotEnrolled
		case eErr != nil:
			return MoodToday{}, fmt.Errorf("consultar matrícula: %w", eErr)
		}
	}
	out.CanCheckin = out.Reason == ""

	row, err := s.q.GetMoodCheckinByDay(ctx, gen.GetMoodCheckinByDayParams{PatientID: patientID, DiaRef: pgtype.Date{Time: dia, Valid: true}})
	switch {
	case err == nil:
		c := toMoodCheckin(row)
		out.Checkin = &c
	case !errors.Is(err, pgx.ErrNoRows):
		return MoodToday{}, fmt.Errorf("consultar check-in de hoje: %w", err)
	}
	return out, nil
}

// History devolve os check-ins recentes do paciente (mais novo primeiro).
func (s *MoodCheckinStore) History(ctx context.Context, patientID uuid.UUID, limit int) ([]MoodCheckin, error) {
	if limit <= 0 || limit > 120 {
		limit = 30
	}
	rows, err := s.q.ListMoodCheckins(ctx, gen.ListMoodCheckinsParams{PatientID: patientID, Limit: int32(limit)})
	if err != nil {
		return nil, fmt.Errorf("listar check-ins: %w", err)
	}
	out := make([]MoodCheckin, 0, len(rows))
	for _, r := range rows {
		out = append(out, toMoodCheckin(r))
	}
	return out, nil
}

func toMoodCheckin(row gen.MoodCheckin) MoodCheckin {
	c := MoodCheckin{
		Valencia:     int(row.Valencia),
		Energia:      int(row.Energia),
		Quadrante:    row.Quadrante,
		EmotionLabel: textToPtr(row.EmotionLabel),
		RespondidoEm: row.RespondidoEm,
	}
	if len(row.ContextTags) > 0 {
		_ = json.Unmarshal(row.ContextTags, &c.ContextTags)
	}
	return c
}
