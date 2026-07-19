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
	"github.com/renovisaude/renovi-care/internal/models/careline"
)

// Erros da matrícula. Use errors.Is.
var (
	// ErrEnrollmentAlive: o paciente já tem uma matrícula viva (ativa/pausada)
	// nesta linha. A trava é o índice parcial ux_enrollment_viva.
	ErrEnrollmentAlive = errors.New("matrícula: paciente já tem matrícula viva nesta linha")
	// ErrEnrollmentNotFound: a matrícula não existe.
	ErrEnrollmentNotFound = errors.New("matrícula: não encontrada")
	// ErrEnrollmentClosed: a matrícula está em desfecho final (concluida/encerrada)
	// e não aceita renovação nem novo encerramento.
	ErrEnrollmentClosed = errors.New("matrícula: em desfecho final")
	// ErrPatientNotFound: o paciente informado não existe.
	ErrPatientNotFound = errors.New("matrícula: paciente não encontrado")
	// ErrCareLineNotPublished: a linha não tem nenhuma versão publicada para
	// matricular.
	ErrCareLineNotPublished = errors.New("matrícula: linha sem versão publicada")
	// ErrInvalidMonths: months fora de {1,2,3}.
	ErrInvalidMonths = errors.New("matrícula: meses inválidos (use 1, 2 ou 3)")
	// ErrInvalidEndStatus: status de encerramento fora de {concluida, encerrada}.
	ErrInvalidEndStatus = errors.New("matrícula: status de encerramento inválido")
)

// Enrollment é o espelho amigável da matrícula, com a versão da linha resolvida e
// os períodos de vigência.
type Enrollment struct {
	ID              uuid.UUID
	PatientID       uuid.UUID
	CareLineCode    string
	CareLineVersion int
	Status          string
	ValidFrom       time.Time
	ValidUntil      time.Time
	Periods         []EnrollmentPeriod
}

// EnrollmentPeriod é uma janela de vigência concedida.
type EnrollmentPeriod struct {
	ID       uuid.UUID
	StartsAt time.Time
	EndsAt   time.Time
	Source   string
}

// EnrollmentStore é a camada de dados + regra da matrícula.
type EnrollmentStore struct {
	pool *pgxpool.Pool
	q    *gen.Queries
}

// NewEnrollmentStore monta o store.
func NewEnrollmentStore(pool *pgxpool.Pool) *EnrollmentStore {
	return &EnrollmentStore{pool: pool, q: gen.New(pool)}
}

// payloads da jornada (JSONB). Structs em vez de map: o formato é contrato de
// auditoria e merece ser explícito.
type enrollCreatedPayload struct {
	Months          int       `json:"months"`
	ValidFrom       time.Time `json:"valid_from"`
	ValidUntil      time.Time `json:"valid_until"`
	CareLineID      uuid.UUID `json:"care_line_id"`
	CareLineVersion int32     `json:"care_line_version"`
	PeriodID        uuid.UUID `json:"period_id"`
}

type enrollRenewedPayload struct {
	Months      int       `json:"months"`
	PeriodID    uuid.UUID `json:"period_id"`
	StartsAt    time.Time `json:"starts_at"`
	ValidUntil  time.Time `json:"valid_until"`
	Reactivated bool      `json:"reactivated"`
}

type enrollEndedPayload struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

// Enroll matricula o paciente na ÚLTIMA versão publicada da linha, com vigência de
// `months` meses (1..3). Grava, atomicamente, a matrícula, o primeiro período e o
// evento matricula_criada. Uma segunda matrícula viva na mesma linha vira
// ErrEnrollmentAlive.
func (s *EnrollmentStore) Enroll(ctx context.Context, patientID uuid.UUID, careLineCode string, months int, now time.Time) (Enrollment, error) {
	if !validMonths(months) {
		return Enrollment{}, ErrInvalidMonths
	}

	// O paciente precisa existir (reusa a query de conta do auth).
	if _, err := s.q.GetAccountByID(ctx, patientID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Enrollment{}, ErrPatientNotFound
		}
		return Enrollment{}, fmt.Errorf("carregar paciente: %w", err)
	}

	line, err := s.q.GetLatestPublishedCareLine(ctx, careLineCode)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Enrollment{}, ErrCareLineNotPublished
		}
		return Enrollment{}, fmt.Errorf("carregar linha publicada: %w", err)
	}

	validFrom := now
	validUntil := now.Add(careline.MonthWindow * time.Duration(months))

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Enrollment{}, fmt.Errorf("abrir transação: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	enrollmentID, err := uuid.NewV7()
	if err != nil {
		return Enrollment{}, fmt.Errorf("gerar uuid v7: %w", err)
	}
	if _, err := q.InsertEnrollment(ctx, gen.InsertEnrollmentParams{
		ID: enrollmentID, PatientID: patientID, CareLineID: line.ID,
		CareLineCode: careLineCode, Status: careline.EnrollmentAtiva,
		ValidFrom: validFrom, ValidUntil: validUntil,
	}); err != nil {
		if isUniqueViolation(err) {
			return Enrollment{}, ErrEnrollmentAlive
		}
		return Enrollment{}, fmt.Errorf("inserir matrícula: %w", err)
	}

	periodID, err := s.insertPeriod(ctx, q, enrollmentID, validFrom, validUntil)
	if err != nil {
		return Enrollment{}, err
	}

	payload, err := json.Marshal(enrollCreatedPayload{
		Months: months, ValidFrom: validFrom, ValidUntil: validUntil,
		CareLineID: line.ID, CareLineVersion: line.Version, PeriodID: periodID,
	})
	if err != nil {
		return Enrollment{}, fmt.Errorf("montar payload: %w", err)
	}
	if err := s.insertEvent(ctx, q, enrollmentID, patientID, "matricula_criada", payload); err != nil {
		return Enrollment{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Enrollment{}, fmt.Errorf("commit: %w", err)
	}
	return s.Get(ctx, enrollmentID)
}

// Renew acrescenta `months` meses de vigência como um NOVO período. Para uma
// matrícula viva (ativa/pausada) o período é CONTÍGUO — começa no valid_until atual,
// e o paciente não perde os dias já pagos. Para uma expirada, REATIVA a partir de
// agora. Concluída/encerrada não renovam (ErrEnrollmentClosed).
func (s *EnrollmentStore) Renew(ctx context.Context, id uuid.UUID, months int, now time.Time) (Enrollment, error) {
	if !validMonths(months) {
		return Enrollment{}, ErrInvalidMonths
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Enrollment{}, fmt.Errorf("abrir transação: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	enr, err := q.GetEnrollmentForUpdate(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Enrollment{}, ErrEnrollmentNotFound
		}
		return Enrollment{}, fmt.Errorf("travar matrícula: %w", err)
	}
	if isClosed(enr.Status) {
		return Enrollment{}, ErrEnrollmentClosed
	}

	// Contíguo por padrão; reativação (a partir de agora) quando a vigência já
	// venceu. Decidir pelo TEMPO, não só pelo status: a expiração é lazy (só as
	// leituras da jornada do paciente a marcam), então uma matrícula pode estar
	// 'ativa'/'pausada' com valid_until no passado. Renovar de forma contígua a
	// partir desse valid_until velho geraria um período INTEIRO no passado — o
	// paciente pagaria por dias que já venceram. Se a vigência passou, reativa.
	startsAt := enr.ValidUntil
	reactivated := false
	if enr.Status == careline.EnrollmentExpirada || !enr.ValidUntil.After(now) {
		startsAt = now
		reactivated = true
	}
	newValidUntil := startsAt.Add(careline.MonthWindow * time.Duration(months))

	n, err := q.RenewEnrollment(ctx, gen.RenewEnrollmentParams{
		ID: id, ValidUntil: newValidUntil, UpdatedAt: now,
	})
	if err != nil {
		if isUniqueViolation(err) {
			// Reativar uma expirada (status volta a 'ativa') pode colidir com
			// ux_enrollment_viva se o paciente já rematriculou o MESMO code depois do
			// vencimento: outra matrícula viva ocupa a trava. Mesmo mapeamento do
			// Enroll — a corrida é uma matrícula viva já existente, não um 500.
			return Enrollment{}, ErrEnrollmentAlive
		}
		return Enrollment{}, fmt.Errorf("renovar matrícula: %w", err)
	}
	if n == 0 {
		// A linha estava travada e não era final: só resta uma corrida improvável.
		return Enrollment{}, ErrEnrollmentClosed
	}

	periodID, err := s.insertPeriod(ctx, q, id, startsAt, newValidUntil)
	if err != nil {
		return Enrollment{}, err
	}

	payload, err := json.Marshal(enrollRenewedPayload{
		Months: months, PeriodID: periodID, StartsAt: startsAt,
		ValidUntil: newValidUntil, Reactivated: reactivated,
	})
	if err != nil {
		return Enrollment{}, fmt.Errorf("montar payload: %w", err)
	}
	if err := s.insertEvent(ctx, q, id, enr.PatientID, "matricula_renovada", payload); err != nil {
		return Enrollment{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Enrollment{}, fmt.Errorf("commit: %w", err)
	}
	return s.Get(ctx, id)
}

// End marca a matrícula como concluida ou encerrada e registra o motivo na jornada.
// Uma matrícula já em desfecho final não é encerrada de novo (ErrEnrollmentClosed).
func (s *EnrollmentStore) End(ctx context.Context, id uuid.UUID, status, reason string, now time.Time) (Enrollment, error) {
	if status != careline.EnrollmentConcluida && status != careline.EnrollmentEncerrada {
		return Enrollment{}, ErrInvalidEndStatus
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Enrollment{}, fmt.Errorf("abrir transação: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	enr, err := q.GetEnrollmentForUpdate(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Enrollment{}, ErrEnrollmentNotFound
		}
		return Enrollment{}, fmt.Errorf("travar matrícula: %w", err)
	}

	n, err := q.EndEnrollment(ctx, gen.EndEnrollmentParams{ID: id, Status: status, UpdatedAt: now})
	if err != nil {
		return Enrollment{}, fmt.Errorf("encerrar matrícula: %w", err)
	}
	if n == 0 {
		// Já estava concluida/encerrada: fora do WHERE do EndEnrollment.
		return Enrollment{}, ErrEnrollmentClosed
	}

	payload, err := json.Marshal(enrollEndedPayload{Status: status, Reason: reason})
	if err != nil {
		return Enrollment{}, fmt.Errorf("montar payload: %w", err)
	}
	if err := s.insertEvent(ctx, q, id, enr.PatientID, "matricula_encerrada", payload); err != nil {
		return Enrollment{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Enrollment{}, fmt.Errorf("commit: %w", err)
	}
	return s.Get(ctx, id)
}

// Get devolve a matrícula com a versão da linha resolvida e os períodos.
func (s *EnrollmentStore) Get(ctx context.Context, id uuid.UUID) (Enrollment, error) {
	enr, err := s.q.GetEnrollment(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Enrollment{}, ErrEnrollmentNotFound
		}
		return Enrollment{}, fmt.Errorf("carregar matrícula: %w", err)
	}

	// A versão vem da linha congelada na matrícula (care_line_id).
	line, err := s.q.GetCareLine(ctx, enr.CareLineID)
	if err != nil {
		return Enrollment{}, fmt.Errorf("carregar versão da linha: %w", err)
	}

	periods, err := s.q.ListEnrollmentPeriods(ctx, id)
	if err != nil {
		return Enrollment{}, fmt.Errorf("listar períodos: %w", err)
	}

	out := make([]EnrollmentPeriod, 0, len(periods))
	for _, p := range periods {
		out = append(out, EnrollmentPeriod{
			ID: p.ID, StartsAt: p.StartsAt, EndsAt: p.EndsAt, Source: p.Source,
		})
	}
	return Enrollment{
		ID: enr.ID, PatientID: enr.PatientID, CareLineCode: enr.CareLineCode,
		CareLineVersion: int(line.Version), Status: enr.Status,
		ValidFrom: enr.ValidFrom, ValidUntil: enr.ValidUntil, Periods: out,
	}, nil
}

// insertPeriod grava um período de vigência e devolve seu id (para o payload).
func (s *EnrollmentStore) insertPeriod(ctx context.Context, q *gen.Queries, enrollmentID uuid.UUID, startsAt, endsAt time.Time) (uuid.UUID, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("gerar uuid v7: %w", err)
	}
	if _, err := q.InsertEnrollmentPeriod(ctx, gen.InsertEnrollmentPeriodParams{
		ID: id, EnrollmentID: enrollmentID, StartsAt: startsAt, EndsAt: endsAt,
	}); err != nil {
		return uuid.UUID{}, fmt.Errorf("inserir período: %w", err)
	}
	return id, nil
}

// insertEvent grava um evento append-only na jornada (actor=admin, ref para a
// própria matrícula).
func (s *EnrollmentStore) insertEvent(ctx context.Context, q *gen.Queries, enrollmentID, patientID uuid.UUID, eventType string, payload []byte) error {
	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("gerar uuid v7: %w", err)
	}
	if _, err := q.InsertJourneyEvent(ctx, gen.InsertJourneyEventParams{
		ID: id, EnrollmentID: enrollmentID, PatientID: patientID,
		EventType: eventType, Actor: "admin",
		RefTable: pgtype.Text{String: "enrollment", Valid: true},
		RefID:    pgUUID(enrollmentID), Payload: payload,
	}); err != nil {
		return fmt.Errorf("gravar evento %s: %w", eventType, err)
	}
	return nil
}

func validMonths(m int) bool { return m >= 1 && m <= 3 }

func isClosed(status string) bool {
	return status == careline.EnrollmentConcluida || status == careline.EnrollmentEncerrada
}
