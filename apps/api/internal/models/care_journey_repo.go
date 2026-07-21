package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/renovisaude/renovi-care/internal/db/gen"
	"github.com/renovisaude/renovi-care/internal/models/careline"
)

// Vocabulário do event log (espelha o CHECK de journey_event, migration 0007).
const (
	eventConsultaAgendada      = "consulta_agendada"
	eventConsultaCancelada     = "consulta_cancelada"
	eventConsultaStatusForcado = "consulta_status_forcado"
	eventMatriculaExpirada     = "matricula_expirada"

	actorPaciente = "paciente"
	actorSistema  = "sistema"
	actorAdmin    = "admin"

	refCareAppointment = "care_appointment"
	refEnrollment      = "enrollment"
)

// Payloads dos eventos da jornada (JSONB). Structs, não map: o formato é
// contrato de auditoria e merece ser explícito (mesmo padrão do enrollment.go).
type careScheduledPayload struct {
	BookingID   uuid.UUID `json:"booking_id"`
	SlotID      string    `json:"slot_id"`
	ItemRef     string    `json:"item_ref"`
	ScheduledAt time.Time `json:"scheduled_at"`
}

type careCancelledPayload struct {
	CancelledBy    string  `json:"cancelled_by"`
	HoursBefore    float64 `json:"hours_before"`
	CountsForQuota bool    `json:"counts_for_quota"`
	DAVCancelled   bool    `json:"dav_cancelled"`
	DAVError       string  `json:"dav_error,omitempty"`
}

type careForcedPayload struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type enrollExpiredPayload struct {
	ValidUntil time.Time `json:"valid_until"`
}

// JourneyRepo implementa o journeyStorage com o Postgres próprio (pool + gen).
// Toda escrita que gera evento roda linha+evento na MESMA transação — evento
// sem fato (ou fato sem evento) é o bug que a auditoria existe para impedir.
type JourneyRepo struct {
	pool *pgxpool.Pool
	q    *gen.Queries
}

// NewJourneyRepo monta o repositório da jornada.
func NewJourneyRepo(pool *pgxpool.Pool) *JourneyRepo {
	return &JourneyRepo{pool: pool, q: gen.New(pool)}
}

var _ journeyStorage = (*JourneyRepo)(nil)

// ---------------------------------------------------------------------------
// Snapshots
// ---------------------------------------------------------------------------

func (r *JourneyRepo) ListEnrollmentsByPatient(ctx context.Context, patientID uuid.UUID) ([]EnrollmentSnapshot, error) {
	rows, err := r.q.ListEnrollmentsByPatient(ctx, patientID)
	if err != nil {
		return nil, fmt.Errorf("listar matrículas: %w", err)
	}
	out := make([]EnrollmentSnapshot, 0, len(rows))
	for _, enr := range rows {
		// A linha universal (Verificador de Humor para todos, ADR-040) NÃO é um
		// "plano": fica fora da jornada/perfil. O humor lê /me/mood/today (endpoint
		// separado), então não depende desta listagem. Filtrar só AQUI (não na query
		// gerada) mantém SnapshotByItem/labelIndex enxergando a linha para a auditoria.
		if enr.CareLineCode == UniversalMentalHealthCode {
			continue
		}
		snap, err := r.hydrateSnapshot(ctx, enr)
		if err != nil {
			return nil, err
		}
		out = append(out, snap)
	}
	return out, nil
}

func (r *JourneyRepo) SnapshotEnrollment(ctx context.Context, enrollmentID uuid.UUID) (EnrollmentSnapshot, error) {
	enr, err := r.q.GetEnrollment(ctx, enrollmentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EnrollmentSnapshot{}, ErrEnrollmentNotFound
		}
		return EnrollmentSnapshot{}, fmt.Errorf("carregar matrícula: %w", err)
	}
	return r.hydrateSnapshot(ctx, enr)
}

// SnapshotByItem acha a matrícula NÃO terminal do paciente cuja linha congelada
// contém o item. Encerradas/concluídas ficam de fora: item de linha antiga
// responde 404 — nunca 403, para a rota não virar oráculo de ids.
func (r *JourneyRepo) SnapshotByItem(ctx context.Context, patientID, itemID uuid.UUID) (EnrollmentSnapshot, CareLineItem, error) {
	enrollments, err := r.q.ListEnrollmentsByPatient(ctx, patientID)
	if err != nil {
		return EnrollmentSnapshot{}, CareLineItem{}, fmt.Errorf("listar matrículas: %w", err)
	}
	for _, enr := range enrollments {
		if isClosed(enr.Status) {
			continue
		}
		items, err := r.q.ListItemsByCareLine(ctx, enr.CareLineID)
		if err != nil {
			return EnrollmentSnapshot{}, CareLineItem{}, fmt.Errorf("listar itens: %w", err)
		}
		for _, it := range items {
			if it.ID != itemID {
				continue
			}
			snap, err := r.hydrateSnapshot(ctx, enr)
			if err != nil {
				return EnrollmentSnapshot{}, CareLineItem{}, err
			}
			for _, domainItem := range snap.Items {
				if domainItem.ID == itemID {
					return snap, domainItem, nil
				}
			}
			// Item na listagem crua mas não no snapshot: impossível de verdade,
			// mas melhor um erro alto que um item sem regras.
			return EnrollmentSnapshot{}, CareLineItem{}, fmt.Errorf("item %s sumiu na hidratação", itemID)
		}
	}
	return EnrollmentSnapshot{}, CareLineItem{}, ErrItemNotFound
}

// hydrateSnapshot monta o EnrollmentSnapshot completo de uma matrícula: linha
// congelada (nome/versão), itens com regras, períodos e as consultas da jornada.
func (r *JourneyRepo) hydrateSnapshot(ctx context.Context, enr gen.Enrollment) (EnrollmentSnapshot, error) {
	line, err := r.q.GetCareLine(ctx, enr.CareLineID)
	if err != nil {
		return EnrollmentSnapshot{}, fmt.Errorf("carregar linha da matrícula: %w", err)
	}
	items, err := r.q.ListItemsByCareLine(ctx, enr.CareLineID)
	if err != nil {
		return EnrollmentSnapshot{}, fmt.Errorf("listar itens: %w", err)
	}
	ruleRows, err := r.q.ListRulesByCareLine(ctx, enr.CareLineID)
	if err != nil {
		return EnrollmentSnapshot{}, fmt.Errorf("listar regras: %w", err)
	}
	periods, err := r.q.ListEnrollmentPeriods(ctx, enr.ID)
	if err != nil {
		return EnrollmentSnapshot{}, fmt.Errorf("listar períodos: %w", err)
	}
	appts, err := r.q.ListCareAppointmentsByEnrollment(ctx, enr.ID)
	if err != nil {
		return EnrollmentSnapshot{}, fmt.Errorf("listar consultas da matrícula: %w", err)
	}

	rulesByItem := make(map[uuid.UUID][]CareLineRule, len(items))
	for _, rr := range ruleRows {
		rulesByItem[rr.CareLineRule.CareLineItemID] = append(rulesByItem[rr.CareLineRule.CareLineItemID], CareLineRule{
			RuleType: rr.CareLineRule.RuleType,
			Params:   json.RawMessage(rr.CareLineRule.Params),
		})
	}
	domainItems := make([]CareLineItem, 0, len(items))
	labels := make(map[uuid.UUID]string, len(items))
	for _, it := range items {
		domainItems = append(domainItems, toCareLineItem(it, rulesByItem[it.ID]))
		labels[it.ID] = it.Label
	}

	domainPeriods := make([]EnrollmentPeriod, 0, len(periods))
	for _, p := range periods {
		domainPeriods = append(domainPeriods, EnrollmentPeriod{
			ID: p.ID, StartsAt: p.StartsAt, EndsAt: p.EndsAt, Source: p.Source,
		})
	}

	domainAppts := make([]CareAppointment, 0, len(appts))
	for _, a := range appts {
		domainAppts = append(domainAppts, toCareAppointment(a, labels[a.CareLineItemID]))
	}

	return EnrollmentSnapshot{
		Enrollment: Enrollment{
			ID: enr.ID, PatientID: enr.PatientID, CareLineCode: enr.CareLineCode,
			CareLineVersion: int(line.Version), Status: enr.Status,
			ValidFrom: enr.ValidFrom, ValidUntil: enr.ValidUntil, Periods: domainPeriods,
		},
		CareLineName: line.Name,
		Items:        domainItems,
		Appointments: domainAppts,
	}, nil
}

// ---------------------------------------------------------------------------
// Expiração lazy
// ---------------------------------------------------------------------------

// Expire transiciona ativa->expirada e grava o evento matricula_expirada
// (actor=sistema) na MESMA transação. Idempotente: se outra requisição (ou o
// worker) expirou antes, o guard do UPDATE casa 0 linhas e NADA é gravado — sem
// evento duplicado.
func (r *JourneyRepo) Expire(ctx context.Context, enrollmentID uuid.UUID, now time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("abrir transação: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)

	// FOR UPDATE serializa duas requisições expirando a mesma matrícula: só uma
	// vê 1 linha no UPDATE e grava o evento.
	enr, err := q.GetEnrollmentForUpdate(ctx, enrollmentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrEnrollmentNotFound
		}
		return fmt.Errorf("travar matrícula: %w", err)
	}

	n, err := q.ExpireEnrollment(ctx, gen.ExpireEnrollmentParams{ID: enrollmentID, UpdatedAt: now})
	if err != nil {
		return fmt.Errorf("expirar matrícula: %w", err)
	}
	if n == 0 {
		// Já expirada (ou mudou de status): nada a fazer, nada a registrar.
		return nil
	}

	payload, err := json.Marshal(enrollExpiredPayload{ValidUntil: enr.ValidUntil})
	if err != nil {
		return fmt.Errorf("montar payload: %w", err)
	}
	if err := r.insertEvent(ctx, q, eventInput{
		EnrollmentID: enrollmentID, PatientID: enr.PatientID,
		EventType: eventMatriculaExpirada, Actor: actorSistema,
		RefTable: refEnrollment, RefID: enrollmentID, Payload: payload,
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Consultas da jornada
// ---------------------------------------------------------------------------

func (r *JourneyRepo) FindByIdemKey(ctx context.Context, enrollmentID uuid.UUID, key string) (CareAppointment, bool, error) {
	row, err := r.q.GetCareAppointmentByIdemKey(ctx, gen.GetCareAppointmentByIdemKeyParams{
		EnrollmentID: enrollmentID, IdempotencyKey: text(key),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return CareAppointment{}, false, nil
	}
	if err != nil {
		return CareAppointment{}, false, fmt.Errorf("buscar por idempotency key: %w", err)
	}
	label, err := r.labelFor(ctx, r.q, row)
	if err != nil {
		return CareAppointment{}, false, err
	}
	return toCareAppointment(row, label), true, nil
}

func (r *JourneyRepo) GetForPatient(ctx context.Context, patientID, careApptID uuid.UUID) (CareAppointment, error) {
	row, err := r.q.GetCareAppointmentForPatient(ctx, gen.GetCareAppointmentForPatientParams{
		ID: careApptID, PatientID: patientID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return CareAppointment{}, ErrCareAppointmentNotFound
	}
	if err != nil {
		// Falha de infra NÃO é "não existe": propaga (vira 5xx), não 404.
		return CareAppointment{}, fmt.Errorf("buscar consulta da jornada: %w", err)
	}
	label, err := r.labelFor(ctx, r.q, row)
	if err != nil {
		return CareAppointment{}, err
	}
	return toCareAppointment(row, label), nil
}

func (r *JourneyRepo) ListForPatient(ctx context.Context, patientID uuid.UUID, status *string) ([]CareAppointment, error) {
	rows, err := r.q.ListCareAppointmentsByPatient(ctx, gen.ListCareAppointmentsByPatientParams{
		PatientID: patientID, Status: textPtr(status),
	})
	if err != nil {
		return nil, fmt.Errorf("listar consultas da jornada: %w", err)
	}
	labels, err := r.labelIndex(ctx, patientID)
	if err != nil {
		return nil, err
	}
	out := make([]CareAppointment, 0, len(rows))
	for _, row := range rows {
		out = append(out, toCareAppointment(row, labels[row.CareLineItemID]))
	}
	return out, nil
}

// CreateScheduled grava a consulta E o evento consulta_agendada numa transação
// só. A corrida de idempotência é decidida pelo índice ux_care_appt_idem — o
// perdedor recebe errCareIdemRace e o JourneyStore compensa o booking.
func (r *JourneyRepo) CreateScheduled(ctx context.Context, in CreateScheduledInput) (CareAppointment, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return CareAppointment{}, fmt.Errorf("abrir transação: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)

	id, err := uuid.NewV7()
	if err != nil {
		return CareAppointment{}, fmt.Errorf("gerar uuid v7: %w", err)
	}
	row, err := q.InsertCareAppointment(ctx, gen.InsertCareAppointmentParams{
		ID: id, EnrollmentID: in.EnrollmentID, CareLineItemID: in.CareLineItemID,
		ItemRef: in.ItemRef, BookingID: in.BookingID, ScheduledAt: in.ScheduledAt,
		Status: careline.StatusAgendada, IdempotencyKey: text(in.IdemKey),
	})
	if err != nil {
		if isIdemKeyViolation(err) {
			return CareAppointment{}, errCareIdemRace
		}
		return CareAppointment{}, fmt.Errorf("inserir consulta da jornada: %w", err)
	}

	payload, err := json.Marshal(careScheduledPayload{
		BookingID: in.BookingID, SlotID: in.SlotID,
		ItemRef: in.ItemRef, ScheduledAt: in.ScheduledAt,
	})
	if err != nil {
		return CareAppointment{}, fmt.Errorf("montar payload: %w", err)
	}
	if err := r.insertEvent(ctx, q, eventInput{
		EnrollmentID: in.EnrollmentID, PatientID: in.PatientID,
		EventType: eventConsultaAgendada, Actor: actorPaciente,
		RefTable: refCareAppointment, RefID: id, Payload: payload,
	}); err != nil {
		return CareAppointment{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return CareAppointment{}, fmt.Errorf("commit: %w", err)
	}
	return toCareAppointment(row, in.Label), nil
}

// CancelScheduled grava o cancelamento E o evento consulta_cancelada numa
// transação só. O guard do UPDATE (agendada/confirmada) decide corridas: 0
// linhas = outra requisição resolveu a consulta antes = recusa, não erro.
func (r *JourneyRepo) CancelScheduled(ctx context.Context, in CancelScheduledInput) (CareAppointment, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return CareAppointment{}, fmt.Errorf("abrir transação: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)

	n, err := q.CancelCareAppointment(ctx, gen.CancelCareAppointmentParams{
		ID: in.ID, CancelledAt: pgtype.Timestamptz{Time: in.Now, Valid: true},
	})
	if err != nil {
		return CareAppointment{}, fmt.Errorf("cancelar consulta da jornada: %w", err)
	}
	if n == 0 {
		return CareAppointment{}, ErrCareCancelNotAllowed
	}

	payload, err := json.Marshal(careCancelledPayload{
		CancelledBy: actorPaciente, HoursBefore: in.HoursBefore,
		CountsForQuota: in.CountsForQuota, DAVCancelled: in.DAVCancelled, DAVError: in.DAVError,
	})
	if err != nil {
		return CareAppointment{}, fmt.Errorf("montar payload: %w", err)
	}
	if err := r.insertEvent(ctx, q, eventInput{
		EnrollmentID: in.EnrollmentID, PatientID: in.PatientID,
		EventType: eventConsultaCancelada, Actor: actorPaciente,
		RefTable: refCareAppointment, RefID: in.ID, Payload: payload,
	}); err != nil {
		return CareAppointment{}, err
	}

	row, err := q.GetCareAppointmentForPatient(ctx, gen.GetCareAppointmentForPatientParams{
		ID: in.ID, PatientID: in.PatientID,
	})
	if err != nil {
		return CareAppointment{}, fmt.Errorf("recarregar consulta cancelada: %w", err)
	}
	label, err := r.labelFor(ctx, q, row)
	if err != nil {
		return CareAppointment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CareAppointment{}, fmt.Errorf("commit: %w", err)
	}
	return toCareAppointment(row, label), nil
}

// ForceStatus (rota interna de teste) grava o status forçado E o evento
// consulta_status_forcado (actor=admin, payload {from, to}) numa transação só.
func (r *JourneyRepo) ForceStatus(ctx context.Context, in ForceStatusInput) (CareAppointment, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return CareAppointment{}, fmt.Errorf("abrir transação: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)

	before, err := q.GetCareAppointment(ctx, in.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return CareAppointment{}, ErrCareAppointmentNotFound
	}
	if err != nil {
		return CareAppointment{}, fmt.Errorf("buscar consulta: %w", err)
	}

	n, err := q.ForceCareAppointmentStatus(ctx, gen.ForceCareAppointmentStatusParams{
		ID: in.ID, Status: in.Status, UpdatedAt: in.Now,
	})
	if err != nil {
		return CareAppointment{}, fmt.Errorf("forçar status: %w", err)
	}
	if n == 0 {
		// Existe, mas está num estado terminal — o guard não alcança.
		return CareAppointment{}, ErrForceStatusNotAllowed
	}

	enr, err := q.GetEnrollment(ctx, before.EnrollmentID)
	if err != nil {
		return CareAppointment{}, fmt.Errorf("carregar matrícula da consulta: %w", err)
	}
	payload, err := json.Marshal(careForcedPayload{From: before.Status, To: in.Status})
	if err != nil {
		return CareAppointment{}, fmt.Errorf("montar payload: %w", err)
	}
	if err := r.insertEvent(ctx, q, eventInput{
		EnrollmentID: before.EnrollmentID, PatientID: enr.PatientID,
		EventType: eventConsultaStatusForcado, Actor: actorAdmin,
		RefTable: refCareAppointment, RefID: in.ID, Payload: payload,
	}); err != nil {
		return CareAppointment{}, err
	}

	label, err := r.labelFor(ctx, q, before)
	if err != nil {
		return CareAppointment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CareAppointment{}, fmt.Errorf("commit: %w", err)
	}
	after := before
	after.Status = in.Status
	return toCareAppointment(after, label), nil
}

// ---------------------------------------------------------------------------
// Event log
// ---------------------------------------------------------------------------

// auditMax é o cursor da PRIMEIRA página: maior instante/uuid possíveis, para o
// keyset "< cursor" devolver tudo do mais novo para o mais antigo.
var auditMaxTime = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)

func (r *JourneyRepo) AuditPage(ctx context.Context, patientID uuid.UUID, cursor *AuditCursor, limit int) ([]JourneyEvent, error) {
	before := AuditCursor{OccurredAt: auditMaxTime, ID: uuid.Max}
	if cursor != nil {
		before = *cursor
	}
	rows, err := r.q.ListJourneyEventsByPatient(ctx, gen.ListJourneyEventsByPatientParams{
		PatientID:        patientID,
		BeforeOccurredAt: before.OccurredAt,
		BeforeID:         before.ID,
		Limit:            int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("paginar eventos: %w", err)
	}
	return toJourneyEvents(rows), nil
}

func (r *JourneyRepo) RecentEvents(ctx context.Context, enrollmentID uuid.UUID, limit int) ([]JourneyEvent, error) {
	rows, err := r.q.ListRecentJourneyEventsByEnrollment(ctx, gen.ListRecentJourneyEventsByEnrollmentParams{
		EnrollmentID: enrollmentID, Limit: int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("eventos recentes: %w", err)
	}
	return toJourneyEvents(rows), nil
}

// ---------------------------------------------------------------------------
// Auxiliares
// ---------------------------------------------------------------------------

type eventInput struct {
	EnrollmentID uuid.UUID
	PatientID    uuid.UUID
	EventType    string
	Actor        string
	RefTable     string
	RefID        uuid.UUID
	Payload      []byte
}

func (r *JourneyRepo) insertEvent(ctx context.Context, q *gen.Queries, in eventInput) error {
	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("gerar uuid v7: %w", err)
	}
	if _, err := q.InsertJourneyEvent(ctx, gen.InsertJourneyEventParams{
		ID: id, EnrollmentID: in.EnrollmentID, PatientID: in.PatientID,
		EventType: in.EventType, Actor: in.Actor,
		RefTable: text(in.RefTable), RefID: pgUUID(in.RefID), Payload: in.Payload,
	}); err != nil {
		return fmt.Errorf("gravar evento %s: %w", in.EventType, err)
	}
	return nil
}

// labelFor resolve o rótulo do item de UMA consulta (matrícula -> linha ->
// itens). O ref é o fallback: melhor que um label vazio na tela.
func (r *JourneyRepo) labelFor(ctx context.Context, q *gen.Queries, row gen.CareAppointment) (string, error) {
	enr, err := q.GetEnrollment(ctx, row.EnrollmentID)
	if err != nil {
		return "", fmt.Errorf("carregar matrícula da consulta: %w", err)
	}
	items, err := q.ListItemsByCareLine(ctx, enr.CareLineID)
	if err != nil {
		return "", fmt.Errorf("listar itens da linha: %w", err)
	}
	for _, it := range items {
		if it.ID == row.CareLineItemID {
			return it.Label, nil
		}
	}
	return row.ItemRef, nil
}

// labelIndex resolve os rótulos de TODOS os itens das linhas do paciente de uma
// vez (para listagens não fazerem um lookup por consulta).
func (r *JourneyRepo) labelIndex(ctx context.Context, patientID uuid.UUID) (map[uuid.UUID]string, error) {
	enrollments, err := r.q.ListEnrollmentsByPatient(ctx, patientID)
	if err != nil {
		return nil, fmt.Errorf("listar matrículas: %w", err)
	}
	idx := map[uuid.UUID]string{}
	seen := map[uuid.UUID]bool{}
	for _, enr := range enrollments {
		if seen[enr.CareLineID] {
			continue
		}
		seen[enr.CareLineID] = true
		items, err := r.q.ListItemsByCareLine(ctx, enr.CareLineID)
		if err != nil {
			return nil, fmt.Errorf("listar itens da linha: %w", err)
		}
		for _, it := range items {
			idx[it.ID] = it.Label
		}
	}
	return idx, nil
}

// isIdemKeyViolation reconhece a corrida de idempotência PELO NOME do índice:
// outra unique violation (ex.: ux_care_appt_booking) é bug, não replay, e não
// pode ser confundida com ela.
func isIdemKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == "ux_care_appt_idem"
}

func toCareAppointment(row gen.CareAppointment, label string) CareAppointment {
	return CareAppointment{
		ID:             row.ID,
		EnrollmentID:   row.EnrollmentID,
		CareLineItemID: row.CareLineItemID,
		ItemRef:        row.ItemRef,
		Label:          label,
		BookingID:      row.BookingID,
		ScheduledAt:    row.ScheduledAt,
		Status:         row.Status,
		CancelledAt:    timestamptzPtr(row.CancelledAt),
	}
}

func toJourneyEvents(rows []gen.JourneyEvent) []JourneyEvent {
	out := make([]JourneyEvent, 0, len(rows))
	for _, ev := range rows {
		out = append(out, JourneyEvent{
			ID: ev.ID, EnrollmentID: ev.EnrollmentID,
			EventType: ev.EventType, Actor: ev.Actor,
			OccurredAt: ev.OccurredAt, Payload: json.RawMessage(ev.Payload),
		})
	}
	return out
}
