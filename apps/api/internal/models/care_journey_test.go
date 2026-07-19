package models

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/adapters/agenda"
	"github.com/renovisaude/renovi-care/internal/models/careline"
)

// O teste é do MESMO pacote (models) de propósito: o journeyStorage é interface
// não exportada — declarada no consumidor — e o fake precisa implementá-la.

var jLoc = func() *time.Location {
	l, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		panic(err)
	}
	return l
}()

var (
	jNow       = time.Date(2026, 7, 20, 8, 0, 0, 0, jLoc)
	jSlotStart = time.Date(2026, 7, 22, 9, 0, 0, 0, jLoc)
	jAccount   = Account{ID: uuid.MustParse("019820e2-0000-7000-8000-000000000001"), FullName: "Maria de Teste"}
)

// ---------------------------------------------------------------------------
// Fakes (structs à mão, padrão do repo — sem framework de mock)
// ---------------------------------------------------------------------------

type fakeJourneyStorage struct {
	// SnapshotByItem
	snap     EnrollmentSnapshot
	snapItem CareLineItem
	snapErr  error

	// ListEnrollmentsByPatient / SnapshotEnrollment
	listSnaps []EnrollmentSnapshot
	snapshots map[uuid.UUID]EnrollmentSnapshot

	// Expire
	expireCalls []uuid.UUID
	expireErr   error

	// FindByIdemKey
	byIdem    map[string]CareAppointment
	findCalls int

	// CreateScheduled
	createCalls  []CreateScheduledInput
	createErr    error
	createResult CareAppointment
	// raceWinner simula o vencedor da corrida: quando createErr == errCareIdemRace,
	// ele "aparece" no índice como no banco de verdade.
	raceWinner *CareAppointment

	// CancelScheduled
	cancelCalls []CancelScheduledInput
	cancelErr   error

	// GetForPatient
	getAppt CareAppointment
	getErr  error

	// ListForPatient
	listResult []CareAppointment
	listStatus []*string

	// Audit / RecentEvents
	auditCursors []*AuditCursor
	auditLimits  []int
	auditResult  []JourneyEvent
	recent       []JourneyEvent
	recentLimits []int
}

func (f *fakeJourneyStorage) ListEnrollmentsByPatient(context.Context, uuid.UUID) ([]EnrollmentSnapshot, error) {
	return f.listSnaps, nil
}

func (f *fakeJourneyStorage) SnapshotEnrollment(_ context.Context, id uuid.UUID) (EnrollmentSnapshot, error) {
	if s, ok := f.snapshots[id]; ok {
		return s, nil
	}
	return f.snap, nil
}

func (f *fakeJourneyStorage) SnapshotByItem(context.Context, uuid.UUID, uuid.UUID) (EnrollmentSnapshot, CareLineItem, error) {
	if f.snapErr != nil {
		return EnrollmentSnapshot{}, CareLineItem{}, f.snapErr
	}
	return f.snap, f.snapItem, nil
}

func (f *fakeJourneyStorage) Expire(_ context.Context, id uuid.UUID, _ time.Time) error {
	f.expireCalls = append(f.expireCalls, id)
	return f.expireErr
}

func (f *fakeJourneyStorage) FindByIdemKey(_ context.Context, _ uuid.UUID, key string) (CareAppointment, bool, error) {
	f.findCalls++
	appt, ok := f.byIdem[key]
	return appt, ok, nil
}

func (f *fakeJourneyStorage) GetForPatient(context.Context, uuid.UUID, uuid.UUID) (CareAppointment, error) {
	return f.getAppt, f.getErr
}

func (f *fakeJourneyStorage) ListForPatient(_ context.Context, _ uuid.UUID, status *string) ([]CareAppointment, error) {
	f.listStatus = append(f.listStatus, status)
	return f.listResult, nil
}

func (f *fakeJourneyStorage) CreateScheduled(_ context.Context, in CreateScheduledInput) (CareAppointment, error) {
	f.createCalls = append(f.createCalls, in)
	if f.createErr != nil {
		if f.raceWinner != nil {
			if f.byIdem == nil {
				f.byIdem = map[string]CareAppointment{}
			}
			f.byIdem[in.IdemKey] = *f.raceWinner
		}
		return CareAppointment{}, f.createErr
	}
	if f.createResult.ID != uuid.Nil {
		return f.createResult, nil
	}
	return CareAppointment{
		ID: uuid.New(), EnrollmentID: in.EnrollmentID, CareLineItemID: in.CareLineItemID,
		ItemRef: in.ItemRef, Label: in.Label, BookingID: in.BookingID,
		ScheduledAt: in.ScheduledAt, Status: careline.StatusAgendada,
	}, nil
}

func (f *fakeJourneyStorage) CancelScheduled(_ context.Context, in CancelScheduledInput) (CareAppointment, error) {
	f.cancelCalls = append(f.cancelCalls, in)
	if f.cancelErr != nil {
		return CareAppointment{}, f.cancelErr
	}
	out := f.getAppt
	out.Status = careline.StatusCancelada
	now := in.Now
	out.CancelledAt = &now
	return out, nil
}

func (f *fakeJourneyStorage) ForceStatus(_ context.Context, in ForceStatusInput) (CareAppointment, error) {
	out := f.getAppt
	out.Status = in.Status
	return out, f.getErr
}

func (f *fakeJourneyStorage) AuditPage(_ context.Context, _ uuid.UUID, cursor *AuditCursor, limit int) ([]JourneyEvent, error) {
	f.auditCursors = append(f.auditCursors, cursor)
	f.auditLimits = append(f.auditLimits, limit)
	if limit < len(f.auditResult) {
		return f.auditResult[:limit], nil
	}
	return f.auditResult, nil
}

func (f *fakeJourneyStorage) RecentEvents(_ context.Context, _ uuid.UUID, limit int) ([]JourneyEvent, error) {
	f.recentLimits = append(f.recentLimits, limit)
	return f.recent, nil
}

type fakeBookingSvc struct {
	specialties   []agenda.Specialty
	specErr       error
	professionals []agenda.Professional
	profErr       error
	slotPages     map[string]SlotPage
	slotPageErr   error
	slotInfo      agenda.Booking
	slotInfoErr   error

	bookCalls  []BookInput
	bookResult Appointment
	bookErr    error

	cancelCalls  []uuid.UUID
	cancelResult CancelBookingResult
	cancelErr    error
}

func (f *fakeBookingSvc) Book(_ context.Context, in BookInput) (Appointment, error) {
	f.bookCalls = append(f.bookCalls, in)
	if f.bookErr != nil {
		return Appointment{}, f.bookErr
	}
	return f.bookResult, nil
}

func (f *fakeBookingSvc) Cancel(_ context.Context, _ Account, id uuid.UUID, _ time.Time) (CancelBookingResult, error) {
	f.cancelCalls = append(f.cancelCalls, id)
	return f.cancelResult, f.cancelErr
}

func (f *fakeBookingSvc) ListSpecialties(context.Context, time.Time) ([]agenda.Specialty, error) {
	return f.specialties, f.specErr
}

func (f *fakeBookingSvc) ListProfessionals(context.Context, string, time.Time) ([]agenda.Professional, error) {
	return f.professionals, f.profErr
}

func (f *fakeBookingSvc) ListSlotPage(_ context.Context, professionalID string, _, _, _ time.Time) (SlotPage, error) {
	if f.slotPageErr != nil {
		return SlotPage{}, f.slotPageErr
	}
	return f.slotPages[professionalID], nil
}

func (f *fakeBookingSvc) SlotInfo(context.Context, string, string) (agenda.Booking, error) {
	return f.slotInfo, f.slotInfoErr
}

func (f *fakeBookingSvc) Location() *time.Location { return jLoc }

// ---------------------------------------------------------------------------
// Cenário base: matrícula ativa numa linha com um item "acomp" (Psicologia)
// ---------------------------------------------------------------------------

func acompItem(rules ...CareLineRule) CareLineItem {
	return CareLineItem{
		ID: uuid.MustParse("019820e2-0000-7000-8000-00000000aaaa"), Ref: "acomp",
		Kind: "CONSULTA", SpecialtyCode: "Psicologia", Label: "Acompanhamento", Rules: rules,
	}
}

func rule(ruleType, params string) CareLineRule {
	return CareLineRule{RuleType: ruleType, Params: json.RawMessage(params)}
}

func snapAtiva(item CareLineItem, appts []CareAppointment) EnrollmentSnapshot {
	return EnrollmentSnapshot{
		Enrollment: Enrollment{
			ID: uuid.MustParse("019820e2-0000-7000-8000-00000000ee01"), PatientID: jAccount.ID,
			CareLineCode: "saude-mental", CareLineVersion: 1, Status: careline.EnrollmentAtiva,
			ValidFrom: jNow.Add(-10 * 24 * time.Hour), ValidUntil: jNow.Add(50 * 24 * time.Hour),
		},
		CareLineName: "Saúde Mental",
		Items:        []CareLineItem{item},
		Appointments: appts,
	}
}

func bookingFor(slotID string, startsAt time.Time) agenda.Booking {
	return agenda.Booking{
		Slot:         agenda.Slot{ID: slotID, StartsAt: startsAt, EndsAt: startsAt.Add(50 * time.Minute)},
		Professional: agenda.Professional{ID: "prof-1", FullName: "Ana Beatriz Moura"},
		Specialty:    agenda.Specialty{ID: "esp-1", Name: "Psicologia"},
	}
}

func psicologia() []agenda.Specialty {
	return []agenda.Specialty{{ID: "esp-1", Name: "Psicologia"}}
}

func confirmedBooking(id uuid.UUID, startsAt time.Time) Appointment {
	return Appointment{
		ID: id.String(), Status: "CONFIRMED", StartsAt: startsAt, EndsAt: startsAt.Add(50 * time.Minute),
		TimeZone: "America/Sao_Paulo", SpecialtyID: "esp-1", SpecialtyName: "Psicologia",
		ProfessionalID: "prof-1", ProfessionalName: "Ana Beatriz Moura",
	}
}

// ---------------------------------------------------------------------------
// Schedule
// ---------------------------------------------------------------------------

// Bloqueado pelo motor: QUOTA total=1 já consumida por uma consulta realizada.
// O 422 nasce aqui — e o booking NÃO pode ser tocado.
func TestJourneyStore_Schedule_BloqueadoPeloMotor_NaoChamaBooking(t *testing.T) {
	item := acompItem(rule(careline.RuleQuota, `{"max":1,"period":"total"}`))
	st := &fakeJourneyStorage{
		snap: snapAtiva(item, []CareAppointment{{
			ItemRef: "acomp", Status: careline.StatusRealizada,
			ScheduledAt: jNow.Add(-5 * 24 * time.Hour),
		}}),
		snapItem: item,
	}
	bk := &fakeBookingSvc{specialties: psicologia(), slotInfo: bookingFor("slot-1", jSlotStart)}
	js := NewJourneyStore(st, bk, 24*time.Hour, nil)

	_, _, err := js.Schedule(context.Background(), ScheduleInput{
		Account: jAccount, ItemID: item.ID, SlotID: "slot-1", IdemKey: "k-1", Now: jNow,
	})

	var ne ErrNotEligible
	require.ErrorAs(t, err, &ne, "motor barrado deve virar ErrNotEligible")
	require.Len(t, ne.Blocks, 1)
	assert.Equal(t, careline.RuleQuota, ne.Blocks[0].RuleType)
	assert.NotEmpty(t, ne.Blocks[0].Reason)

	assert.Empty(t, bk.bookCalls, "o booking NÃO pode ser chamado quando o motor barra")
	assert.Empty(t, st.createCalls, "nada pode ser gravado quando o motor barra")
}

// Caminho feliz: motor libera, Book recebe a especialidade RESOLVIDA pelo nome
// normalizado, e consulta+evento saem numa única chamada atômica ao storage.
func TestJourneyStore_Schedule_Feliz(t *testing.T) {
	item := acompItem()
	st := &fakeJourneyStorage{snap: snapAtiva(item, nil), snapItem: item}
	bookingID := uuid.New()
	bk := &fakeBookingSvc{
		specialties: psicologia(),
		slotInfo:    bookingFor("slot-1", jSlotStart),
		bookResult:  confirmedBooking(bookingID, jSlotStart),
	}
	js := NewJourneyStore(st, bk, 24*time.Hour, nil)

	appt, replayed, err := js.Schedule(context.Background(), ScheduleInput{
		Account: jAccount, ItemID: item.ID, SlotID: "slot-1", IdemKey: "k-1", Now: jNow,
	})
	require.NoError(t, err)
	assert.False(t, replayed)
	assert.Equal(t, careline.StatusAgendada, appt.Status)
	assert.Equal(t, "acomp", appt.ItemRef)
	assert.Equal(t, "Acompanhamento", appt.Label)
	assert.Equal(t, bookingID, appt.BookingID)
	assert.Equal(t, "America/Sao_Paulo", appt.TimeZone)

	require.Len(t, bk.bookCalls, 1)
	assert.Equal(t, "esp-1", bk.bookCalls[0].SpecialtyID, "a especialidade vem do catálogo do legado, resolvida pelo nome")
	assert.Equal(t, "slot-1", bk.bookCalls[0].SlotID)

	require.Len(t, st.createCalls, 1, "consulta+evento saem numa ÚNICA chamada atômica")
	created := st.createCalls[0]
	assert.Equal(t, st.snap.Enrollment.ID, created.EnrollmentID)
	assert.Equal(t, item.ID, created.CareLineItemID)
	assert.Equal(t, "k-1", created.IdemKey)
	assert.Equal(t, bookingID, created.BookingID)
	assert.True(t, created.ScheduledAt.Equal(jSlotStart))
}

// Replay: a MESMA key duas vezes devolve a MESMA consulta, com Book chamado
// UMA vez e um único CreateScheduled.
func TestJourneyStore_Schedule_Replay(t *testing.T) {
	item := acompItem()
	st := &fakeJourneyStorage{snap: snapAtiva(item, nil), snapItem: item}
	bookingID := uuid.New()
	bk := &fakeBookingSvc{
		specialties: psicologia(),
		slotInfo:    bookingFor("slot-1", jSlotStart),
		bookResult:  confirmedBooking(bookingID, jSlotStart),
	}
	js := NewJourneyStore(st, bk, 24*time.Hour, nil)
	in := ScheduleInput{Account: jAccount, ItemID: item.ID, SlotID: "slot-1", IdemKey: "k-replay", Now: jNow}

	first, replayed, err := js.Schedule(context.Background(), in)
	require.NoError(t, err)
	require.False(t, replayed)

	// O fake espelha o banco: a key agora existe no índice.
	st.byIdem = map[string]CareAppointment{"k-replay": {
		ID: first.ID, EnrollmentID: first.EnrollmentID, CareLineItemID: first.CareLineItemID,
		ItemRef: first.ItemRef, Label: first.Label, BookingID: first.BookingID,
		ScheduledAt: first.ScheduledAt, Status: first.Status,
	}}

	second, replayed, err := js.Schedule(context.Background(), in)
	require.NoError(t, err)
	assert.True(t, replayed, "replay da mesma key responde replayed=true (200)")
	assert.Equal(t, first.ID, second.ID, "o replay devolve a MESMA consulta")
	assert.Len(t, bk.bookCalls, 1, "Book não pode rodar de novo no replay")
	assert.Len(t, st.createCalls, 1, "nada novo é gravado no replay")
}

// Reúso da key: a MESMA key já criou uma consulta de OUTRO item nesta matrícula.
// Não é replay — devolver a consulta do item A quando o cliente pediu o item B
// confirmaria o agendamento errado. Vira ErrIdemKeyReused, sem tocar no Book.
func TestJourneyStore_Schedule_KeyReutilizadaParaOutroItem(t *testing.T) {
	item := acompItem()
	st := &fakeJourneyStorage{snap: snapAtiva(item, nil), snapItem: item}
	// O índice já tem a key, mas amarrada a OUTRO item (outro CareLineItemID).
	st.byIdem = map[string]CareAppointment{"k-reuso": {
		ID: uuid.New(), EnrollmentID: st.snap.Enrollment.ID, CareLineItemID: uuid.New(),
		ItemRef: "outro", Status: careline.StatusAgendada,
	}}
	bk := &fakeBookingSvc{
		specialties: psicologia(),
		slotInfo:    bookingFor("slot-1", jSlotStart),
		bookResult:  confirmedBooking(uuid.New(), jSlotStart),
	}
	js := NewJourneyStore(st, bk, 24*time.Hour, nil)

	_, _, err := js.Schedule(context.Background(), ScheduleInput{
		Account: jAccount, ItemID: item.ID, SlotID: "slot-1", IdemKey: "k-reuso", Now: jNow,
	})
	require.ErrorIs(t, err, ErrIdemKeyReused)
	assert.Empty(t, bk.bookCalls, "reúso indevido não pode chamar o Book")
	assert.Empty(t, st.createCalls, "nada é gravado no reúso")
}

// Corrida de key: o índice único decide; o perdedor COMPENSA o booking que
// criou (Cancel com o bookingID recém-criado) e devolve o vencedor.
func TestJourneyStore_Schedule_CorridaDeKey_Compensa(t *testing.T) {
	item := acompItem()
	winner := CareAppointment{
		ID: uuid.New(), EnrollmentID: uuid.New(), ItemRef: "acomp", Label: "Acompanhamento",
		BookingID: uuid.New(), ScheduledAt: jSlotStart, Status: careline.StatusAgendada,
	}
	st := &fakeJourneyStorage{
		snap: snapAtiva(item, nil), snapItem: item,
		createErr: errCareIdemRace, raceWinner: &winner,
	}
	loserBookingID := uuid.New()
	bk := &fakeBookingSvc{
		specialties: psicologia(),
		slotInfo:    bookingFor("slot-1", jSlotStart),
		bookResult:  confirmedBooking(loserBookingID, jSlotStart),
	}
	js := NewJourneyStore(st, bk, 24*time.Hour, nil)

	got, replayed, err := js.Schedule(context.Background(), ScheduleInput{
		Account: jAccount, ItemID: item.ID, SlotID: "slot-1", IdemKey: "k-race", Now: jNow,
	})
	require.NoError(t, err)
	assert.True(t, replayed, "quem perde a corrida devolve o vencedor como replay")
	assert.Equal(t, winner.ID, got.ID)
	require.Len(t, bk.cancelCalls, 1, "o booking do perdedor precisa ser compensado")
	assert.Equal(t, loserBookingID, bk.cancelCalls[0], "compensa o booking RECÉM-criado, não o do vencedor")
}

// A compensação que falha não pode virar 500 para o paciente: o vencedor existe
// e é a resposta certa; a falha vira log de operação.
func TestJourneyStore_Schedule_CorridaDeKey_CompensacaoFalhaAindaDevolveVencedor(t *testing.T) {
	item := acompItem()
	winner := CareAppointment{ID: uuid.New(), ItemRef: "acomp", Status: careline.StatusAgendada, ScheduledAt: jSlotStart}
	st := &fakeJourneyStorage{
		snap: snapAtiva(item, nil), snapItem: item,
		createErr: errCareIdemRace, raceWinner: &winner,
	}
	bk := &fakeBookingSvc{
		specialties: psicologia(),
		slotInfo:    bookingFor("slot-1", jSlotStart),
		bookResult:  confirmedBooking(uuid.New(), jSlotStart),
		cancelErr:   ErrCancelNotAllowed,
	}
	js := NewJourneyStore(st, bk, 24*time.Hour, nil)

	got, replayed, err := js.Schedule(context.Background(), ScheduleInput{
		Account: jAccount, ItemID: item.ID, SlotID: "slot-1", IdemKey: "k-race", Now: jNow,
	})
	require.NoError(t, err)
	assert.True(t, replayed)
	assert.Equal(t, winner.ID, got.ID)
}

func TestJourneyStore_Schedule_SemIdemKey(t *testing.T) {
	item := acompItem()
	st := &fakeJourneyStorage{snap: snapAtiva(item, nil), snapItem: item}
	bk := &fakeBookingSvc{specialties: psicologia(), slotInfo: bookingFor("slot-1", jSlotStart)}
	js := NewJourneyStore(st, bk, 24*time.Hour, nil)

	_, _, err := js.Schedule(context.Background(), ScheduleInput{
		Account: jAccount, ItemID: item.ID, SlotID: "slot-1", IdemKey: "   ", Now: jNow,
	})
	require.ErrorIs(t, err, ErrIdemKeyRequired)
	assert.Empty(t, bk.bookCalls)
	assert.Empty(t, st.createCalls)
}

func TestJourneyStore_Schedule_ItemNaoEncontrado(t *testing.T) {
	st := &fakeJourneyStorage{snapErr: ErrItemNotFound}
	bk := &fakeBookingSvc{specialties: psicologia()}
	js := NewJourneyStore(st, bk, 24*time.Hour, nil)

	_, _, err := js.Schedule(context.Background(), ScheduleInput{
		Account: jAccount, ItemID: uuid.New(), SlotID: "slot-1", IdemKey: "k", Now: jNow,
	})
	require.ErrorIs(t, err, ErrItemNotFound)
}

// Os erros do booking sobem intactos (o controller já sabe mapeá-los).
func TestJourneyStore_Schedule_ErroDoBookingSobeIntacto(t *testing.T) {
	item := acompItem()
	st := &fakeJourneyStorage{snap: snapAtiva(item, nil), snapItem: item}
	bk := &fakeBookingSvc{
		specialties: psicologia(),
		slotInfo:    bookingFor("slot-1", jSlotStart),
		bookErr:     ErrSlotTaken,
	}
	js := NewJourneyStore(st, bk, 24*time.Hour, nil)

	_, _, err := js.Schedule(context.Background(), ScheduleInput{
		Account: jAccount, ItemID: item.ID, SlotID: "slot-1", IdemKey: "k", Now: jNow,
	})
	require.ErrorIs(t, err, ErrSlotTaken)
	assert.Empty(t, st.createCalls, "booking falhou: nada entra na jornada")
}

// A especialidade do item sumiu do catálogo vivo do legado: 404, não 500.
func TestJourneyStore_Schedule_EspecialidadeSumiu(t *testing.T) {
	item := acompItem()
	st := &fakeJourneyStorage{snap: snapAtiva(item, nil), snapItem: item}
	bk := &fakeBookingSvc{specialties: []agenda.Specialty{{ID: "esp-9", Name: "Nutrição"}}}
	js := NewJourneyStore(st, bk, 24*time.Hour, nil)

	_, _, err := js.Schedule(context.Background(), ScheduleInput{
		Account: jAccount, ItemID: item.ID, SlotID: "slot-1", IdemKey: "k", Now: jNow,
	})
	require.ErrorIs(t, err, ErrSpecialtyNotFound)
	assert.Empty(t, bk.bookCalls)
}

// ---------------------------------------------------------------------------
// CancelCare
// ---------------------------------------------------------------------------

func agendadaEm(scheduledAt time.Time) CareAppointment {
	return CareAppointment{
		ID: uuid.New(), EnrollmentID: uuid.New(), CareLineItemID: uuid.New(),
		ItemRef: "acomp", Label: "Acompanhamento", BookingID: uuid.New(),
		ScheduledAt: scheduledAt, Status: careline.StatusAgendada,
	}
}

// Cancelamento TARDIO (23h antes, threshold 24h): counts_for_quota=true — a
// mesma semântica do counts() do motor.
func TestJourneyStore_CancelCare_TardioContaNaCota(t *testing.T) {
	appt := agendadaEm(jNow.Add(23 * time.Hour))
	st := &fakeJourneyStorage{getAppt: appt}
	bk := &fakeBookingSvc{cancelResult: CancelBookingResult{DAVCancelled: true}}
	js := NewJourneyStore(st, bk, 24*time.Hour, nil)

	out, err := js.CancelCare(context.Background(), jAccount, appt.ID, jNow)
	require.NoError(t, err)
	assert.Equal(t, careline.StatusCancelada, out.Status)
	require.NotNil(t, out.CancelledAt)

	require.Len(t, bk.cancelCalls, 1)
	assert.Equal(t, appt.BookingID, bk.cancelCalls[0])

	require.Len(t, st.cancelCalls, 1)
	call := st.cancelCalls[0]
	assert.InDelta(t, 23.0, call.HoursBefore, 0.01)
	assert.True(t, call.CountsForQuota, "cancelar a 23h de uma janela de 24h CONSOME a cota")
	assert.True(t, call.DAVCancelled)
	assert.Empty(t, call.DAVError)
}

// Cancelamento com folga (25h antes, threshold 24h): counts_for_quota=false, e
// o dav_error do booking viaja no bookkeeping.
func TestJourneyStore_CancelCare_ComFolgaNaoConta(t *testing.T) {
	appt := agendadaEm(jNow.Add(25 * time.Hour))
	st := &fakeJourneyStorage{getAppt: appt}
	bk := &fakeBookingSvc{cancelResult: CancelBookingResult{DAVCancelled: false, DAVError: "dav respondeu 500"}}
	js := NewJourneyStore(st, bk, 24*time.Hour, nil)

	_, err := js.CancelCare(context.Background(), jAccount, appt.ID, jNow)
	require.NoError(t, err)

	require.Len(t, st.cancelCalls, 1)
	call := st.cancelCalls[0]
	assert.InDelta(t, 25.0, call.HoursBefore, 0.01)
	assert.False(t, call.CountsForQuota, "cancelar a 25h de uma janela de 24h DEVOLVE a vaga")
	assert.False(t, call.DAVCancelled)
	assert.Equal(t, "dav respondeu 500", call.DAVError)
}

func TestJourneyStore_CancelCare_RealizadaNaoCancela(t *testing.T) {
	appt := agendadaEm(jNow.Add(-24 * time.Hour))
	appt.Status = careline.StatusRealizada
	st := &fakeJourneyStorage{getAppt: appt}
	bk := &fakeBookingSvc{}
	js := NewJourneyStore(st, bk, 24*time.Hour, nil)

	_, err := js.CancelCare(context.Background(), jAccount, appt.ID, jNow)
	require.ErrorIs(t, err, ErrCareCancelNotAllowed)
	assert.Empty(t, bk.cancelCalls, "consulta não cancelável nem chega ao booking")
	assert.Empty(t, st.cancelCalls)
}

// Booking fora de sincronia (já cancelado lá): o paciente NÃO fica preso — o
// cancel local segue, com a dessincronia registrada no bookkeeping.
func TestJourneyStore_CancelCare_BookingForaDeSincroniaSegueLocal(t *testing.T) {
	appt := agendadaEm(jNow.Add(48 * time.Hour))
	st := &fakeJourneyStorage{getAppt: appt}
	bk := &fakeBookingSvc{cancelErr: ErrCancelNotAllowed}
	js := NewJourneyStore(st, bk, 24*time.Hour, nil)

	out, err := js.CancelCare(context.Background(), jAccount, appt.ID, jNow)
	require.NoError(t, err, "dessincronia com o booking não pode travar o cancelamento da jornada")
	assert.Equal(t, careline.StatusCancelada, out.Status)
	require.Len(t, st.cancelCalls, 1)
	assert.False(t, st.cancelCalls[0].DAVCancelled)
	assert.NotEmpty(t, st.cancelCalls[0].DAVError)
}

func TestJourneyStore_CancelCare_NaoEncontrada(t *testing.T) {
	st := &fakeJourneyStorage{getErr: ErrCareAppointmentNotFound}
	js := NewJourneyStore(st, &fakeBookingSvc{}, 24*time.Hour, nil)

	_, err := js.CancelCare(context.Background(), jAccount, uuid.New(), jNow)
	require.ErrorIs(t, err, ErrCareAppointmentNotFound)
}

// ---------------------------------------------------------------------------
// Journey (lazy expire)
// ---------------------------------------------------------------------------

func TestJourneyStore_Journey_LazyExpire(t *testing.T) {
	item := acompItem()
	vencida := snapAtiva(item, nil)
	vencida.Enrollment.ValidUntil = jNow.Add(-1 * time.Hour)

	expirada := vencida
	expirada.Enrollment.Status = careline.EnrollmentExpirada

	st := &fakeJourneyStorage{
		listSnaps: []EnrollmentSnapshot{vencida},
		snapshots: map[uuid.UUID]EnrollmentSnapshot{vencida.Enrollment.ID: expirada},
	}
	js := NewJourneyStore(st, &fakeBookingSvc{}, 24*time.Hour, nil)

	out, err := js.Journey(context.Background(), jAccount, jNow)
	require.NoError(t, err)
	require.Len(t, st.expireCalls, 1, "matrícula ativa vencida expira LAZY na leitura")
	assert.Equal(t, vencida.Enrollment.ID, st.expireCalls[0])

	require.Len(t, out, 1)
	assert.Equal(t, careline.EnrollmentExpirada, out[0].Enrollment.Status, "a resposta reflete o snapshot PÓS-expiração")
	require.Len(t, out[0].Items, 1)
	require.False(t, out[0].Items[0].Eligibility.Allowed)
	assert.Equal(t, careline.RuleVigencia, out[0].Items[0].Eligibility.Blocks[0].RuleType)

	// Já expirada: o Expire NÃO roda de novo (sem evento duplicado).
	st.listSnaps = []EnrollmentSnapshot{expirada}
	_, err = js.Journey(context.Background(), jAccount, jNow)
	require.NoError(t, err)
	assert.Len(t, st.expireCalls, 1, "matrícula já expirada não expira (nem gera evento) de novo")
}

func TestJourneyStore_Journey_ItensAvaliadosEEventos(t *testing.T) {
	item := acompItem()
	snap := snapAtiva(item, nil)
	ev := JourneyEvent{ID: uuid.New(), EnrollmentID: snap.Enrollment.ID, EventType: "matricula_criada", Actor: "admin", OccurredAt: jNow}
	st := &fakeJourneyStorage{listSnaps: []EnrollmentSnapshot{snap}, recent: []JourneyEvent{ev}}
	js := NewJourneyStore(st, &fakeBookingSvc{}, 24*time.Hour, nil)

	out, err := js.Journey(context.Background(), jAccount, jNow)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "Saúde Mental", out[0].CareLineName)
	require.Len(t, out[0].Items, 1)
	assert.True(t, out[0].Items[0].Eligibility.Allowed)
	require.Len(t, out[0].RecentEvents, 1)
	assert.Equal(t, []int{10}, st.recentLimits, "eventos recentes limitados a 10")
}

// ---------------------------------------------------------------------------
// Eligibility
// ---------------------------------------------------------------------------

func TestJourneyStore_Eligibility_DataSimulada(t *testing.T) {
	// MAX_ADVANCE de 30 dias: hoje o slot de daqui a 40 dias é bloqueado; a
	// simulação com date=+40d (intendedAt = date) muda o veredito.
	item := acompItem(rule(careline.RuleMaxAdvance, `{"days":30}`))
	st := &fakeJourneyStorage{snap: snapAtiva(item, nil), snapItem: item}
	js := NewJourneyStore(st, &fakeBookingSvc{}, 24*time.Hour, nil)

	// Sem date: avalia agora — agendar AGORA (intendedAt=now) está dentro do horizonte.
	got, err := js.Eligibility(context.Background(), jAccount, item.ID, nil, jNow)
	require.NoError(t, err)
	assert.True(t, got.Allowed)

	// date = daqui a 40 dias: além do horizonte de 30 → bloqueado, com available_from.
	future := jNow.Add(40 * 24 * time.Hour)
	got, err = js.Eligibility(context.Background(), jAccount, item.ID, &future, jNow)
	require.NoError(t, err)
	require.False(t, got.Allowed)
	assert.Equal(t, careline.RuleMaxAdvance, got.Blocks[0].RuleType)
	require.NotNil(t, got.Blocks[0].AvailableFrom)
}

func TestJourneyStore_Eligibility_ItemNaoEncontrado(t *testing.T) {
	st := &fakeJourneyStorage{snapErr: ErrItemNotFound}
	js := NewJourneyStore(st, &fakeBookingSvc{}, 24*time.Hour, nil)

	_, err := js.Eligibility(context.Background(), jAccount, uuid.New(), nil, jNow)
	require.ErrorIs(t, err, ErrItemNotFound)
}

// ---------------------------------------------------------------------------
// Availability
// ---------------------------------------------------------------------------

// 3 slots de 2 profissionais; um viola MIN_INTERVAL(7d) por causa de uma
// consulta já marcada — anotado bloqueado; os outros Allowed. Cada slot carrega
// o SEU profissional.
func TestJourneyStore_Availability_AnotaMotorEProfissional(t *testing.T) {
	item := acompItem(rule(careline.RuleMinInterval, `{"days":7}`))
	existente := jNow.Add(24 * time.Hour) // consulta marcada para amanhã
	st := &fakeJourneyStorage{
		snap: snapAtiva(item, []CareAppointment{{
			ItemRef: "acomp", Status: careline.StatusConfirmada, ScheduledAt: existente,
		}}),
		snapItem: item,
	}

	ana := agenda.Professional{ID: "prof-1", FullName: "Ana Beatriz Moura"}
	bruno := agenda.Professional{ID: "prof-2", FullName: "Bruno Lima"}
	perto := existente.Add(48 * time.Hour)               // a 2d da existente: viola MIN_INTERVAL
	longe1 := existente.Add(10 * 24 * time.Hour)         // a 10d: ok
	longe2 := existente.Add(12*24*time.Hour + time.Hour) // a 12d: ok
	bk := &fakeBookingSvc{
		specialties:   psicologia(),
		professionals: []agenda.Professional{ana, bruno},
		slotPages: map[string]SlotPage{
			"prof-1": {Professional: ana, Slots: []agenda.Slot{
				{ID: "s-perto", StartsAt: perto, EndsAt: perto.Add(50 * time.Minute)},
				{ID: "s-longe1", StartsAt: longe1, EndsAt: longe1.Add(50 * time.Minute)},
			}},
			"prof-2": {Professional: bruno, Slots: []agenda.Slot{
				{ID: "s-longe2", StartsAt: longe2, EndsAt: longe2.Add(50 * time.Minute)},
			}},
		},
	}
	js := NewJourneyStore(st, bk, 24*time.Hour, nil)

	page, err := js.Availability(context.Background(), jAccount, item.ID, nil, nil, jNow)
	require.NoError(t, err)
	require.Len(t, page.Slots, 3)
	assert.Equal(t, "America/Sao_Paulo", page.TimeZone)

	// Ordenados por starts_at.
	assert.Equal(t, "s-perto", page.Slots[0].Slot.ID)
	assert.Equal(t, "s-longe1", page.Slots[1].Slot.ID)
	assert.Equal(t, "s-longe2", page.Slots[2].Slot.ID)

	assert.False(t, page.Slots[0].Eligibility.Allowed, "slot a 2d da consulta existente viola MIN_INTERVAL(7d)")
	assert.Equal(t, careline.RuleMinInterval, page.Slots[0].Eligibility.Blocks[0].RuleType)
	assert.True(t, page.Slots[1].Eligibility.Allowed)
	assert.True(t, page.Slots[2].Eligibility.Allowed)

	assert.Equal(t, "Ana Beatriz Moura", page.Slots[0].Professional.FullName, "cada slot carrega o SEU profissional")
	assert.Equal(t, "Ana Beatriz Moura", page.Slots[1].Professional.FullName)
	assert.Equal(t, "Bruno Lima", page.Slots[2].Professional.FullName)

	// Defaults ecoados: from=hoje, to=from+30d, no fuso da agenda.
	wantFrom := time.Date(2026, 7, 20, 0, 0, 0, 0, jLoc)
	assert.True(t, page.From.Equal(wantFrom), "from default = hoje no fuso da agenda")
	assert.True(t, page.To.Equal(wantFrom.AddDate(0, 0, 30)), "to default = from+30d")
}

func TestJourneyStore_Availability_IntervaloInvalido(t *testing.T) {
	item := acompItem()
	st := &fakeJourneyStorage{snap: snapAtiva(item, nil), snapItem: item}
	js := NewJourneyStore(st, &fakeBookingSvc{specialties: psicologia()}, 24*time.Hour, nil)

	from := jNow
	to := jNow.Add(-24 * time.Hour)
	_, err := js.Availability(context.Background(), jAccount, item.ID, &from, &to, jNow)
	require.ErrorIs(t, err, ErrBadDateRange, "to antes de from")

	to = jNow.Add(65 * 24 * time.Hour)
	_, err = js.Availability(context.Background(), jAccount, item.ID, &from, &to, jNow)
	require.ErrorIs(t, err, ErrBadDateRange, "janela acima de 60 dias")
}

func TestJourneyStore_Availability_LegadoIndisponivelPropaga(t *testing.T) {
	item := acompItem()
	st := &fakeJourneyStorage{snap: snapAtiva(item, nil), snapItem: item}
	js := NewJourneyStore(st, &fakeBookingSvc{specErr: agenda.ErrUnavailable}, 24*time.Hour, nil)

	_, err := js.Availability(context.Background(), jAccount, item.ID, nil, nil, jNow)
	require.ErrorIs(t, err, agenda.ErrUnavailable, "legado fora sobe intacto (vira 503, não 404)")
}

// ---------------------------------------------------------------------------
// Audit
// ---------------------------------------------------------------------------

func TestAuditCursor_RoundTrip(t *testing.T) {
	c := AuditCursor{OccurredAt: jNow.Add(123 * time.Millisecond), ID: uuid.New()}
	got, err := parseAuditCursor(encodeAuditCursor(c))
	require.NoError(t, err)
	assert.True(t, got.OccurredAt.Equal(c.OccurredAt), "occurred_at sobrevive ao round-trip (com sub-segundo)")
	assert.Equal(t, c.ID, got.ID)
}

func TestJourneyStore_Audit_CursorInvalido(t *testing.T) {
	js := NewJourneyStore(&fakeJourneyStorage{}, &fakeBookingSvc{}, 24*time.Hour, nil)
	bad := "n@o-e-base64url!"
	_, err := js.Audit(context.Background(), jAccount, &bad, 10)
	require.ErrorIs(t, err, ErrBadCursor)

	// base64 válido com conteúdo inválido também é ErrBadCursor.
	bad2 := "bm9wZQ" // "nope"
	_, err = js.Audit(context.Background(), jAccount, &bad2, 10)
	require.ErrorIs(t, err, ErrBadCursor)
}

func TestJourneyStore_Audit_LimitClampENextCursor(t *testing.T) {
	events := make([]JourneyEvent, 150)
	for i := range events {
		events[i] = JourneyEvent{ID: uuid.New(), EventType: "consulta_agendada", Actor: "paciente", OccurredAt: jNow.Add(-time.Duration(i) * time.Minute)}
	}
	st := &fakeJourneyStorage{auditResult: events}
	js := NewJourneyStore(st, &fakeBookingSvc{}, 24*time.Hour, nil)

	// limit acima do teto: clamp para 100; página cheia => next_cursor presente.
	page, err := js.Audit(context.Background(), jAccount, nil, 500)
	require.NoError(t, err)
	assert.Equal(t, []int{100}, st.auditLimits, "limit acima de 100 clampa em 100")
	assert.Len(t, page.Events, 100)
	require.NotNil(t, page.NextCursor, "página cheia => next_cursor presente")

	// O next_cursor decodifica para o ÚLTIMO evento devolvido.
	cur, err := parseAuditCursor(*page.NextCursor)
	require.NoError(t, err)
	last := page.Events[len(page.Events)-1]
	assert.Equal(t, last.ID, cur.ID)
	assert.True(t, cur.OccurredAt.Equal(last.OccurredAt))

	// limit zero: default 50.
	st.auditLimits = nil
	_, err = js.Audit(context.Background(), jAccount, nil, 0)
	require.NoError(t, err)
	assert.Equal(t, []int{50}, st.auditLimits, "limit ausente => default 50")

	// Página incompleta => sem next_cursor.
	st.auditResult = events[:7]
	page, err = js.Audit(context.Background(), jAccount, nil, 10)
	require.NoError(t, err)
	assert.Len(t, page.Events, 7)
	assert.Nil(t, page.NextCursor, "página incompleta => acabou, sem next_cursor")

	// O cursor decodificado é REPASSADO ao storage.
	c := encodeAuditCursor(AuditCursor{OccurredAt: jNow, ID: jAccount.ID})
	st.auditCursors = nil
	_, err = js.Audit(context.Background(), jAccount, &c, 10)
	require.NoError(t, err)
	require.Len(t, st.auditCursors, 1)
	require.NotNil(t, st.auditCursors[0])
	assert.Equal(t, jAccount.ID, st.auditCursors[0].ID)
}

// ---------------------------------------------------------------------------
// ForceStatus
// ---------------------------------------------------------------------------

func TestJourneyStore_ForceStatus(t *testing.T) {
	appt := agendadaEm(jNow.Add(-2 * time.Hour))
	st := &fakeJourneyStorage{getAppt: appt}
	js := NewJourneyStore(st, &fakeBookingSvc{}, 24*time.Hour, nil)

	out, err := js.ForceStatus(context.Background(), appt.ID, careline.StatusRealizada, jNow)
	require.NoError(t, err)
	assert.Equal(t, careline.StatusRealizada, out.Status)

	_, err = js.ForceStatus(context.Background(), appt.ID, "cancelada", jNow)
	require.ErrorIs(t, err, ErrInvalidForceStatus, "só realizada|falta podem ser forçados")
}
