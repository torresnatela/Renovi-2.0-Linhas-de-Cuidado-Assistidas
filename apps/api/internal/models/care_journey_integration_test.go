//go:build integration

package models_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/models"
	"github.com/renovisaude/renovi-care/internal/models/careline"
)

// journeyFixture monta o cenário mínimo da jornada contra Postgres real: linha
// publicada (aval -> acomp), paciente e matrícula ativa.
type journeyFixture struct {
	pool       *pgxpool.Pool
	repo       *models.JourneyRepo
	patient    uuid.UUID
	enrollment models.Enrollment
	line       models.CareLine
	items      map[string]models.CareLineItem // por ref
}

func newJourneyFixture(t *testing.T) journeyFixture {
	t.Helper()
	ctx := context.Background()
	catalog, enroll, pool := newCareStores(t, aceitaAmbas())

	line, err := catalog.Create(ctx, "saude-mental", "Saúde Mental", "linha piloto")
	require.NoError(t, err)
	_, err = catalog.AddItem(ctx, line.ID, models.AddItemInput{
		Ref: "aval", SpecialtyCode: "Psicologia", Label: "Avaliação inicial",
	})
	require.NoError(t, err)
	_, err = catalog.AddItem(ctx, line.ID, models.AddItemInput{
		Ref: "acomp", SpecialtyCode: "Psiquiatria", Label: "Acompanhamento",
	})
	require.NoError(t, err)
	published, err := catalog.Publish(ctx, line.ID, time.Now())
	require.NoError(t, err)

	patient := insertPatient(t, pool)
	e, err := enroll.Enroll(ctx, patient, "saude-mental", 2, time.Now().UTC().Truncate(time.Microsecond))
	require.NoError(t, err)

	items := map[string]models.CareLineItem{}
	for _, it := range published.Items {
		items[it.Ref] = it
	}
	return journeyFixture{
		pool: pool, repo: models.NewJourneyRepo(pool), patient: patient,
		enrollment: e, line: published, items: items,
	}
}

func (f journeyFixture) countRows(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, f.pool.QueryRow(context.Background(), query, args...).Scan(&n))
	return n
}

// TestJourneyRepo_CreateScheduled_Atomico prova que consulta + evento nascem na
// MESMA transação: os dois juntos no sucesso, NENHUM quando o evento falha.
func TestJourneyRepo_CreateScheduled_Atomico(t *testing.T) {
	ctx := context.Background()
	f := newJourneyFixture(t)
	now := time.Now().UTC().Truncate(time.Microsecond)

	bookingID := uuid.New()
	appt, err := f.repo.CreateScheduled(ctx, models.CreateScheduledInput{
		EnrollmentID: f.enrollment.ID, PatientID: f.patient,
		CareLineItemID: f.items["aval"].ID, ItemRef: "aval", Label: "Avaliação inicial",
		BookingID: bookingID, SlotID: "slot-77", ScheduledAt: now.Add(48 * time.Hour),
		IdemKey: "key-atomica", Now: now,
	})
	require.NoError(t, err)
	require.Equal(t, careline.StatusAgendada, appt.Status)
	require.Equal(t, "Avaliação inicial", appt.Label)

	// As DUAS linhas existem.
	require.Equal(t, 1, f.countRows(t,
		`SELECT count(*) FROM care_appointment WHERE enrollment_id = $1 AND idempotency_key = 'key-atomica'`,
		f.enrollment.ID))
	evType, actor, payload := latestEvent(t, f.pool, f.enrollment.ID)
	require.Equal(t, "consulta_agendada", evType)
	require.Equal(t, "paciente", actor)
	require.Equal(t, bookingID.String(), payload["booking_id"])
	require.Equal(t, "slot-77", payload["slot_id"])
	require.Equal(t, "aval", payload["item_ref"])
	require.NotEmpty(t, payload["scheduled_at"])

	// Falha induzida NO EVENTO (patient_id inexistente viola a FK de
	// journey_event): a consulta que entrou na mesma TX precisa sumir junto.
	_, err = f.repo.CreateScheduled(ctx, models.CreateScheduledInput{
		EnrollmentID: f.enrollment.ID, PatientID: uuid.New(), // não existe
		CareLineItemID: f.items["aval"].ID, ItemRef: "aval", Label: "Avaliação inicial",
		BookingID: uuid.New(), SlotID: "slot-88", ScheduledAt: now.Add(72 * time.Hour),
		IdemKey: "key-rollback", Now: now,
	})
	require.Error(t, err, "evento com FK inválida deve falhar")
	require.Equal(t, 0, f.countRows(t,
		`SELECT count(*) FROM care_appointment WHERE enrollment_id = $1 AND idempotency_key = 'key-rollback'`,
		f.enrollment.ID), "rollback conjunto: a consulta NÃO pode persistir sem o evento")
}

// TestJourneyRepo_CancelScheduled_Atomico espelha o teste do CreateScheduled:
// cancelamento + evento juntos, ou nada.
func TestJourneyRepo_CancelScheduled_Atomico(t *testing.T) {
	ctx := context.Background()
	f := newJourneyFixture(t)
	now := time.Now().UTC().Truncate(time.Microsecond)

	appt, err := f.repo.CreateScheduled(ctx, models.CreateScheduledInput{
		EnrollmentID: f.enrollment.ID, PatientID: f.patient,
		CareLineItemID: f.items["aval"].ID, ItemRef: "aval", Label: "Avaliação inicial",
		BookingID: uuid.New(), SlotID: "slot-1", ScheduledAt: now.Add(48 * time.Hour),
		IdemKey: "key-cancel", Now: now,
	})
	require.NoError(t, err)

	// Rollback primeiro: evento com patient_id inexistente falha a FK e o UPDATE
	// da MESMA transação não pode sobrar.
	_, err = f.repo.CancelScheduled(ctx, models.CancelScheduledInput{
		ID: appt.ID, EnrollmentID: f.enrollment.ID, PatientID: uuid.New(), // não existe
		Now: now, HoursBefore: 48, CountsForQuota: false,
	})
	require.Error(t, err)
	require.Equal(t, 1, f.countRows(t,
		`SELECT count(*) FROM care_appointment WHERE id = $1 AND status = 'agendada'`, appt.ID),
		"rollback conjunto: o cancelamento não pode persistir sem o evento")

	// Agora o caminho feliz.
	cancelled, err := f.repo.CancelScheduled(ctx, models.CancelScheduledInput{
		ID: appt.ID, EnrollmentID: f.enrollment.ID, PatientID: f.patient, Now: now,
		HoursBefore: 47.9, CountsForQuota: false, DAVCancelled: true,
	})
	require.NoError(t, err)
	require.Equal(t, careline.StatusCancelada, cancelled.Status)
	require.NotNil(t, cancelled.CancelledAt)
	require.Equal(t, "Avaliação inicial", cancelled.Label)

	evType, actor, payload := latestEvent(t, f.pool, f.enrollment.ID)
	require.Equal(t, "consulta_cancelada", evType)
	require.Equal(t, "paciente", actor)
	require.Equal(t, "paciente", payload["cancelled_by"])
	require.Equal(t, 47.9, payload["hours_before"])
	require.Equal(t, false, payload["counts_for_quota"])
	require.Equal(t, true, payload["dav_cancelled"])

	// Cancelar de novo: o guard casa 0 linhas.
	_, err = f.repo.CancelScheduled(ctx, models.CancelScheduledInput{
		ID: appt.ID, EnrollmentID: f.enrollment.ID, PatientID: f.patient, Now: now,
	})
	require.ErrorIs(t, err, models.ErrCareCancelNotAllowed)
}

// TestJourneyRepo_Expire prova a expiração lazy: transiciona, grava o evento
// actor=sistema e NÃO duplica nada na segunda chamada.
func TestJourneyRepo_Expire(t *testing.T) {
	ctx := context.Background()
	f := newJourneyFixture(t)
	now := time.Now().UTC().Truncate(time.Microsecond)

	// Força a vigência a já ter vencido (o caminho da leitura lazy). O CHECK
	// vigencia_valida exige valid_from < valid_until, então recua os dois.
	_, err := f.pool.Exec(ctx, `UPDATE enrollment SET valid_from = $2, valid_until = $3 WHERE id = $1`,
		f.enrollment.ID, now.Add(-60*24*time.Hour), now.Add(-1*time.Hour))
	require.NoError(t, err)

	require.NoError(t, f.repo.Expire(ctx, f.enrollment.ID, now))

	snap, err := f.repo.SnapshotEnrollment(ctx, f.enrollment.ID)
	require.NoError(t, err)
	require.Equal(t, careline.EnrollmentExpirada, snap.Enrollment.Status)

	evType, actor, _ := latestEvent(t, f.pool, f.enrollment.ID)
	require.Equal(t, "matricula_expirada", evType)
	require.Equal(t, "sistema", actor, "expiração automática é do SISTEMA, não do paciente")

	// Idempotente: expirar de novo não grava um segundo evento.
	require.NoError(t, f.repo.Expire(ctx, f.enrollment.ID, now.Add(time.Minute)))
	require.Equal(t, 1, f.countRows(t,
		`SELECT count(*) FROM journey_event WHERE enrollment_id = $1 AND event_type = 'matricula_expirada'`,
		f.enrollment.ID), "já expirada não gera evento duplicado")
}

// TestJourneyRepo_SnapshotByItem acha a matrícula certa do paciente e IGNORA as
// encerradas — item de linha antiga é 404, não um snapshot morto.
func TestJourneyRepo_SnapshotByItem(t *testing.T) {
	ctx := context.Background()
	f := newJourneyFixture(t)
	now := time.Now().UTC().Truncate(time.Microsecond)

	snap, item, err := f.repo.SnapshotByItem(ctx, f.patient, f.items["acomp"].ID)
	require.NoError(t, err)
	require.Equal(t, f.enrollment.ID, snap.Enrollment.ID)
	require.Equal(t, "acomp", item.Ref)
	require.Equal(t, "Acompanhamento", item.Label)
	require.Equal(t, "Saúde Mental", snap.CareLineName)
	require.Len(t, snap.Items, 2, "o snapshot carrega TODOS os itens da linha (PREREQUISITE precisa deles)")

	// Item inexistente → 404.
	_, _, err = f.repo.SnapshotByItem(ctx, f.patient, uuid.New())
	require.ErrorIs(t, err, models.ErrItemNotFound)

	// Outro paciente sem matrícula → 404 (o item existe, mas não é dele).
	outro := insertPatient(t, f.pool)
	_, _, err = f.repo.SnapshotByItem(ctx, outro, f.items["acomp"].ID)
	require.ErrorIs(t, err, models.ErrItemNotFound)

	// Matrícula encerrada é IGNORADA...
	enrollStore := models.NewEnrollmentStore(f.pool)
	_, err = enrollStore.End(ctx, f.enrollment.ID, careline.EnrollmentEncerrada, "teste", now)
	require.NoError(t, err)
	_, _, err = f.repo.SnapshotByItem(ctx, f.patient, f.items["acomp"].ID)
	require.ErrorIs(t, err, models.ErrItemNotFound)

	// ...e uma matrícula nova na mesma linha volta a ser encontrada.
	e2, err := enrollStore.Enroll(ctx, f.patient, "saude-mental", 1, now)
	require.NoError(t, err)
	snap, _, err = f.repo.SnapshotByItem(ctx, f.patient, f.items["acomp"].ID)
	require.NoError(t, err)
	require.Equal(t, e2.ID, snap.Enrollment.ID, "acha a matrícula VIVA, não a encerrada")
}

// TestJourneyRepo_AuditPage_KeysetEstavel prova a paginação com empate de
// occurred_at: o id (v7) desempata, nada some, nada duplica.
func TestJourneyRepo_AuditPage_KeysetEstavel(t *testing.T) {
	ctx := context.Background()
	f := newJourneyFixture(t)

	// 3 eventos com o MESMO occurred_at e ids v7 crescentes, no passado (antes
	// do matricula_criada do fixture).
	occurred := time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Microsecond)
	ids := make([]uuid.UUID, 3)
	for i := range ids {
		id, err := uuid.NewV7()
		require.NoError(t, err)
		ids[i] = id
		_, err = f.pool.Exec(ctx, `
			INSERT INTO journey_event (id, enrollment_id, patient_id, event_type, actor, payload, occurred_at)
			VALUES ($1, $2, $3, 'consulta_agendada', 'paciente', '{}', $4)`,
			id, f.enrollment.ID, f.patient, occurred)
		require.NoError(t, err)
	}
	// v7 gerado em sequência é crescente; garanta a premissa do teste.
	require.True(t, ids[0].String() < ids[1].String() && ids[1].String() < ids[2].String(),
		"uuids v7 sequenciais devem ser crescentes")

	// Página 1 (limite 2): o matricula_criada (mais novo) + o maior id do empate.
	page1, err := f.repo.AuditPage(ctx, f.patient, nil, 2)
	require.NoError(t, err)
	require.Len(t, page1, 2)
	require.Equal(t, "matricula_criada", page1[0].EventType)
	require.Equal(t, ids[2], page1[1].ID, "no empate de occurred_at, o id DESC decide")

	// Página 2, a partir do último visto: os dois ids restantes, em ordem.
	cursor := &models.AuditCursor{OccurredAt: page1[1].OccurredAt, ID: page1[1].ID}
	page2, err := f.repo.AuditPage(ctx, f.patient, cursor, 2)
	require.NoError(t, err)
	require.Len(t, page2, 2)
	require.Equal(t, ids[1], page2[0].ID)
	require.Equal(t, ids[0], page2[1].ID)

	// Página 3: vazia — nada sumiu, nada repetiu.
	cursor = &models.AuditCursor{OccurredAt: page2[1].OccurredAt, ID: page2[1].ID}
	page3, err := f.repo.AuditPage(ctx, f.patient, cursor, 2)
	require.NoError(t, err)
	require.Empty(t, page3)
}

// TestJourneyRepo_ForceStatus exercita a query nova (GetCareAppointment) e o
// evento admin da rota interna.
func TestJourneyRepo_ForceStatus(t *testing.T) {
	ctx := context.Background()
	f := newJourneyFixture(t)
	now := time.Now().UTC().Truncate(time.Microsecond)

	appt, err := f.repo.CreateScheduled(ctx, models.CreateScheduledInput{
		EnrollmentID: f.enrollment.ID, PatientID: f.patient,
		CareLineItemID: f.items["aval"].ID, ItemRef: "aval", Label: "Avaliação inicial",
		BookingID: uuid.New(), SlotID: "slot-9", ScheduledAt: now.Add(-1 * time.Hour),
		IdemKey: "key-force", Now: now,
	})
	require.NoError(t, err)

	forced, err := f.repo.ForceStatus(ctx, models.ForceStatusInput{
		ID: appt.ID, Status: careline.StatusRealizada, Now: now,
	})
	require.NoError(t, err)
	require.Equal(t, careline.StatusRealizada, forced.Status)

	evType, actor, payload := latestEvent(t, f.pool, f.enrollment.ID)
	require.Equal(t, "consulta_status_forcado", evType)
	require.Equal(t, "admin", actor)
	require.Equal(t, "agendada", payload["from"])
	require.Equal(t, "realizada", payload["to"])

	// Terminal não força de novo (guard casa 0 linhas) — e id inexistente é 404.
	_, err = f.repo.ForceStatus(ctx, models.ForceStatusInput{ID: appt.ID, Status: careline.StatusFalta, Now: now})
	require.ErrorIs(t, err, models.ErrForceStatusNotAllowed)
	_, err = f.repo.ForceStatus(ctx, models.ForceStatusInput{ID: uuid.New(), Status: careline.StatusFalta, Now: now})
	require.ErrorIs(t, err, models.ErrCareAppointmentNotFound)
}

// TestJourneyRepo_FindByIdemKey_EViewComLabel cobre o replay real: a key acha a
// consulta e o label vem resolvido da linha congelada.
func TestJourneyRepo_FindByIdemKey_EViewComLabel(t *testing.T) {
	ctx := context.Background()
	f := newJourneyFixture(t)
	now := time.Now().UTC().Truncate(time.Microsecond)

	_, ok, err := f.repo.FindByIdemKey(ctx, f.enrollment.ID, "key-nunca-usada")
	require.NoError(t, err)
	require.False(t, ok)

	created, err := f.repo.CreateScheduled(ctx, models.CreateScheduledInput{
		EnrollmentID: f.enrollment.ID, PatientID: f.patient,
		CareLineItemID: f.items["acomp"].ID, ItemRef: "acomp", Label: "Acompanhamento",
		BookingID: uuid.New(), SlotID: "slot-2", ScheduledAt: now.Add(24 * time.Hour),
		IdemKey: "key-replay", Now: now,
	})
	require.NoError(t, err)

	found, ok, err := f.repo.FindByIdemKey(ctx, f.enrollment.ID, "key-replay")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, created.ID, found.ID)
	require.Equal(t, "Acompanhamento", found.Label, "o label vem resolvido da linha congelada")

	// GetForPatient: dono acha; terceiro não.
	got, err := f.repo.GetForPatient(ctx, f.patient, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, got.ID)
	outro := insertPatient(t, f.pool)
	_, err = f.repo.GetForPatient(ctx, outro, created.ID)
	require.ErrorIs(t, err, models.ErrCareAppointmentNotFound)

	// ListForPatient com e sem filtro de status.
	all, err := f.repo.ListForPatient(ctx, f.patient, nil)
	require.NoError(t, err)
	require.Len(t, all, 1)
	agendada := "agendada"
	filtered, err := f.repo.ListForPatient(ctx, f.patient, &agendada)
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	cancelada := "cancelada"
	empty, err := f.repo.ListForPatient(ctx, f.patient, &cancelada)
	require.NoError(t, err)
	require.Empty(t, empty)
}

// evita unused em builds parciais durante o desenvolvimento.
var _ = json.Marshal
