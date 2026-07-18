package models

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/renovisaude/renovi-care/internal/adapters/agenda"
	"github.com/renovisaude/renovi-care/internal/models/careline"
)

// Erros da jornada. Use errors.Is (errors.As para o ErrNotEligible, que carrega
// os blocks do motor).
var (
	// ErrItemNotFound: o item não existe, ou não pertence a nenhuma matrícula não
	// terminal do paciente. Os dois respondem igual (404) — a rota não deve virar
	// oráculo de ids de itens de linha.
	ErrItemNotFound = errors.New("jornada: item não encontrado")
	// ErrCareAppointmentNotFound: a consulta da jornada não existe, ou não é do
	// portador da sessão. Mesma resposta (404) pelos mesmos motivos.
	ErrCareAppointmentNotFound = errors.New("jornada: consulta não encontrada")
	// ErrIdemKeyRequired: POST /me/appointments sem Idempotency-Key. A key é a
	// rede de proteção contra o duplo-clique num POST que fala com a DAV.
	ErrIdemKeyRequired = errors.New("jornada: Idempotency-Key é obrigatória")
	// ErrCareCancelNotAllowed: a consulta existe e é do paciente, mas o status não
	// é cancelável (só agendada/confirmada cancelam). Vira 409.
	ErrCareCancelNotAllowed = errors.New("jornada: consulta não pode ser cancelada")
	// ErrSpecialtyNotFound: a especialidade do item sumiu do catálogo do legado.
	// O publish validou que existia, então isto é o legado tendo mudado por baixo:
	// 404 com a listagem viva; indisponibilidade da listagem NÃO cai aqui (o
	// agenda.ErrUnavailable sobe intacto e vira 503).
	ErrSpecialtyNotFound = errors.New("jornada: especialidade do item não encontrada no legado")
	// ErrBadCursor: cursor de auditoria que não decodifica. 400.
	ErrBadCursor = errors.New("jornada: cursor inválido")
	// ErrBadDateRange: intervalo de disponibilidade invertido ou maior que o teto.
	ErrBadDateRange = errors.New("jornada: intervalo de datas inválido")
	// ErrInvalidForceStatus: o status forçado não é realizada nem falta.
	ErrInvalidForceStatus = errors.New("jornada: status forçado inválido (use realizada ou falta)")
	// ErrForceStatusNotAllowed: a consulta já está num estado terminal
	// (realizada/falta/cancelada) e não aceita forçar status. 409.
	ErrForceStatusNotAllowed = errors.New("jornada: consulta em estado terminal não aceita forçar status")
)

// errCareIdemRace é o sinal INTERNO de que o CreateScheduled perdeu a corrida no
// índice ux_care_appt_idem: outro request com a MESMA key gravou primeiro. Quem o
// devolve é o journeyStorage; quem o resolve (compensando o booking e devolvendo
// o vencedor) é o JourneyStore.Schedule.
var errCareIdemRace = errors.New("jornada: corrida de idempotency key")

// ErrNotEligible: o motor barrou o agendamento. Carrega TODOS os blocks para o
// controller montar o 422 com `blocks[]` — o paciente merece a lista inteira de
// motivos, não só o primeiro.
type ErrNotEligible struct {
	Blocks []careline.Block
}

func (e ErrNotEligible) Error() string {
	return fmt.Sprintf("jornada: agendamento bloqueado pelo motor (%d regra(s))", len(e.Blocks))
}

// maxAvailabilityWindow espelha o teto de /professionals/{id}/slots: a
// disponibilidade agrega VÁRIOS profissionais, então o mesmo cliente distraído
// custaria N vezes mais caro ao MySQL de terceiro.
const maxAvailabilityWindow = 60 * 24 * time.Hour

// ---------------------------------------------------------------------------
// Interfaces no consumidor (ADR-012)
// ---------------------------------------------------------------------------

// BookingService é o módulo de booking existente (saga MySQL+DAV) visto pela
// jornada. Implementado por *BookingStore; as assinaturas são as REAIS do store
// — a jornada não pede formato próprio, pede o que já existe.
type BookingService interface {
	Book(ctx context.Context, in BookInput) (Appointment, error)
	Cancel(ctx context.Context, account Account, id uuid.UUID, now time.Time) (CancelBookingResult, error)
	ListSpecialties(ctx context.Context, now time.Time) ([]agenda.Specialty, error)
	ListProfessionals(ctx context.Context, specialtyID string, now time.Time) ([]agenda.Professional, error)
	ListSlotPage(ctx context.Context, professionalID string, from, to, now time.Time) (SlotPage, error)
	SlotInfo(ctx context.Context, slotID, specialtyID string) (agenda.Booking, error)
	Location() *time.Location
}

var _ BookingService = (*BookingStore)(nil)

// journeyStorage é o acesso a dados da jornada. Implementado pelo JourneyRepo
// (pool+gen, care_journey_repo.go) e por um fake em memória nos testes.
type journeyStorage interface {
	ListEnrollmentsByPatient(ctx context.Context, patientID uuid.UUID) ([]EnrollmentSnapshot, error)
	SnapshotEnrollment(ctx context.Context, enrollmentID uuid.UUID) (EnrollmentSnapshot, error)
	// SnapshotByItem acha a matrícula NÃO terminal do paciente dona do item
	// (encerradas/concluídas ficam de fora — item de linha antiga é 404).
	SnapshotByItem(ctx context.Context, patientID, itemID uuid.UUID) (EnrollmentSnapshot, CareLineItem, error)
	// Expire transiciona ativa->expirada e grava o evento matricula_expirada
	// (actor=sistema) na MESMA transação. Idempotente: já expirada = no-op.
	Expire(ctx context.Context, enrollmentID uuid.UUID, now time.Time) error
	FindByIdemKey(ctx context.Context, enrollmentID uuid.UUID, key string) (CareAppointment, bool, error)
	GetForPatient(ctx context.Context, patientID, careApptID uuid.UUID) (CareAppointment, error)
	ListForPatient(ctx context.Context, patientID uuid.UUID, status *string) ([]CareAppointment, error)
	// CreateScheduled grava a consulta + evento consulta_agendada numa TX só.
	// Corrida no ux_care_appt_idem devolve errCareIdemRace.
	CreateScheduled(ctx context.Context, in CreateScheduledInput) (CareAppointment, error)
	// CancelScheduled grava o cancelamento + evento consulta_cancelada numa TX só.
	CancelScheduled(ctx context.Context, in CancelScheduledInput) (CareAppointment, error)
	// ForceStatus grava o status forçado + evento consulta_status_forcado numa TX só.
	ForceStatus(ctx context.Context, in ForceStatusInput) (CareAppointment, error)
	AuditPage(ctx context.Context, patientID uuid.UUID, cursor *AuditCursor, limit int) ([]JourneyEvent, error)
	RecentEvents(ctx context.Context, enrollmentID uuid.UUID, limit int) ([]JourneyEvent, error)
}

// ---------------------------------------------------------------------------
// Tipos de domínio da jornada
// ---------------------------------------------------------------------------

// EnrollmentSnapshot carrega TUDO que o motor precisa para decidir sobre uma
// matrícula: a matrícula (com períodos e versão resolvidos), os itens da linha
// congelada (com regras) e as consultas da jornada. Nada é buscado fora dele.
type EnrollmentSnapshot struct {
	Enrollment   Enrollment
	CareLineName string
	Items        []CareLineItem
	Appointments []CareAppointment
}

// CareAppointment é a consulta da jornada como o produto a enxerga (a projeção
// clínica do booking, amarrada ao item da linha).
type CareAppointment struct {
	ID             uuid.UUID
	EnrollmentID   uuid.UUID
	CareLineItemID uuid.UUID
	ItemRef        string
	Label          string
	BookingID      uuid.UUID
	ScheduledAt    time.Time
	Status         string
	CancelledAt    *time.Time
	// TimeZone é o fuso de exibição (o da agenda). Preenchido pelo JourneyStore,
	// que é quem conhece o booking; o storage devolve vazio.
	TimeZone string
}

// JourneyEvent é um fato do event log da jornada.
type JourneyEvent struct {
	ID           uuid.UUID
	EnrollmentID uuid.UUID
	EventType    string
	Actor        string
	OccurredAt   time.Time
	Payload      json.RawMessage
}

// JourneyItem é um item da linha com o veredito do motor para AGORA.
type JourneyItem struct {
	Item        CareLineItem
	Eligibility careline.Eligibility
}

// JourneyEnrollment é uma matrícula pronta para a tela de jornada.
type JourneyEnrollment struct {
	Enrollment   Enrollment
	CareLineName string
	Items        []JourneyItem
	RecentEvents []JourneyEvent
}

// AvailabilitySlot é um horário anotado com o profissional e o veredito do motor
// para AQUELE instante.
type AvailabilitySlot struct {
	Slot         agenda.Slot
	Professional agenda.Professional
	Eligibility  careline.Eligibility
}

// CareAvailability é a página de disponibilidade de um item, com o intervalo
// EFETIVO usado (para o controller ecoar from/to).
type CareAvailability struct {
	ItemID   uuid.UUID
	From     time.Time // primeiro dia efetivo, no fuso da agenda
	To       time.Time // último dia efetivo, INCLUSIVO
	TimeZone string
	Slots    []AvailabilitySlot
}

// ScheduleInput são os dados do agendamento pela jornada.
type ScheduleInput struct {
	Account Account
	ItemID  uuid.UUID
	SlotID  string
	IdemKey string
	Now     time.Time
}

// CreateScheduledInput são os dados que o storage grava atomicamente (consulta +
// evento consulta_agendada). Label não persiste — serve à view devolvida.
type CreateScheduledInput struct {
	EnrollmentID   uuid.UUID
	PatientID      uuid.UUID
	CareLineItemID uuid.UUID
	ItemRef        string
	Label          string
	BookingID      uuid.UUID
	SlotID         string
	ScheduledAt    time.Time
	IdemKey        string
	Now            time.Time
}

// CancelScheduledInput são os dados do cancelamento + o bookkeeping que vai no
// payload do evento consulta_cancelada.
type CancelScheduledInput struct {
	ID           uuid.UUID
	EnrollmentID uuid.UUID
	PatientID    uuid.UUID
	Now          time.Time
	// HoursBefore: com quanta antecedência (em horas, 1 casa) o paciente cancelou.
	HoursBefore float64
	// CountsForQuota: se o cancelamento foi TARDIO (dentro do threshold) e a
	// consulta continua consumindo a cota — a MESMA semântica do motor.
	CountsForQuota bool
	DAVCancelled   bool
	DAVError       string
}

// ForceStatusInput são os dados do status forçado (rota interna de teste).
type ForceStatusInput struct {
	ID     uuid.UUID
	Status string
	Now    time.Time
}

// AuditCursor é o cursor DECODIFICADO do keyset de auditoria.
type AuditCursor struct {
	OccurredAt time.Time
	ID         uuid.UUID
}

// CareAuditPage é uma página do event log. NextCursor presente sse pode haver
// mais (a página veio cheia).
type CareAuditPage struct {
	Events     []JourneyEvent
	NextCursor *string
}

// ---------------------------------------------------------------------------
// JourneyStore
// ---------------------------------------------------------------------------

// JourneyStore é a camada de regra da jornada do paciente: reavalia a
// elegibilidade SEMPRE no servidor (nunca confia no front), agenda pelo booking
// existente e mantém a projeção clínica + o event log.
type JourneyStore struct {
	storage   journeyStorage
	booking   BookingService
	threshold time.Duration
	logger    *slog.Logger
}

// NewJourneyStore monta o store. cancelThreshold vem da config
// (CancelCountThreshold); zero cai no default do motor.
func NewJourneyStore(storage journeyStorage, booking BookingService, cancelThreshold time.Duration, logger *slog.Logger) *JourneyStore {
	if cancelThreshold <= 0 {
		cancelThreshold = careline.DefaultCancelCountThreshold
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &JourneyStore{storage: storage, booking: booking, threshold: cancelThreshold, logger: logger}
}

// Location expõe o fuso da agenda (o controller precisa dele para interpretar
// datas de query string).
func (s *JourneyStore) Location() *time.Location { return s.booking.Location() }

// Journey monta a jornada inteira do paciente: cada matrícula (expirando as
// vencidas de forma LAZY), os itens já avaliados pelo motor para AGORA e os
// eventos recentes.
func (s *JourneyStore) Journey(ctx context.Context, account Account, now time.Time) ([]JourneyEnrollment, error) {
	snaps, err := s.storage.ListEnrollmentsByPatient(ctx, account.ID)
	if err != nil {
		return nil, fmt.Errorf("listar matrículas: %w", err)
	}

	out := make([]JourneyEnrollment, 0, len(snaps))
	for _, snap := range snaps {
		snap, err = s.freshSnapshot(ctx, snap, now)
		if err != nil {
			return nil, err
		}
		j := engineJourney(snap, s.threshold)
		items := make([]JourneyItem, 0, len(snap.Items))
		for _, it := range snap.Items {
			engineItem, rules := engineItemOf(it)
			items = append(items, JourneyItem{
				Item:        it,
				Eligibility: careline.Evaluate(j, engineItem, rules, now, now),
			})
		}
		events, err := s.storage.RecentEvents(ctx, snap.Enrollment.ID, 10)
		if err != nil {
			return nil, fmt.Errorf("eventos recentes: %w", err)
		}
		out = append(out, JourneyEnrollment{
			Enrollment:   snap.Enrollment,
			CareLineName: snap.CareLineName,
			Items:        items,
			RecentEvents: events,
		})
	}
	return out, nil
}

// Eligibility responde "posso agendar ESTE item?" sem tocar na agenda do legado.
// date nil avalia agora; presente, simula o veredito naquele instante.
func (s *JourneyStore) Eligibility(ctx context.Context, account Account, itemID uuid.UUID, date *time.Time, now time.Time) (careline.Eligibility, error) {
	snap, item, err := s.freshSnapshotByItem(ctx, account.ID, itemID, now)
	if err != nil {
		return careline.Eligibility{}, err
	}
	intendedAt := now
	if date != nil {
		intendedAt = *date
	}
	j := engineJourney(snap, s.threshold)
	engineItem, rules := engineItemOf(item)
	return careline.Evaluate(j, engineItem, rules, intendedAt, now), nil
}

// Availability agrega os horários livres dos profissionais da especialidade do
// item, cada um anotado com o veredito do motor para o INSTANTE do slot — é o
// passo que casa a agenda do legado com a elegibilidade.
func (s *JourneyStore) Availability(ctx context.Context, account Account, itemID uuid.UUID, from, to *time.Time, now time.Time) (CareAvailability, error) {
	snap, item, err := s.freshSnapshotByItem(ctx, account.ID, itemID, now)
	if err != nil {
		return CareAvailability{}, err
	}

	// Defaults no fuso da AGENDA: "hoje" só existe num fuso, e quem o sabe é o
	// servidor (mesma razão do SlotPage). `to` é inclusivo; a janela consultada é
	// semiaberta [from, to+1d), e o teto de 60 dias mede a janela DE FATO.
	loc := s.booking.Location()
	f := dayStart(now, loc)
	if from != nil {
		f = dayStart(*from, loc)
	}
	t := f.AddDate(0, 0, 30)
	if to != nil {
		t = dayStart(*to, loc)
	}
	if t.Before(f) {
		return CareAvailability{}, ErrBadDateRange
	}
	upper := t.AddDate(0, 0, 1)
	if upper.Sub(f) > maxAvailabilityWindow {
		return CareAvailability{}, ErrBadDateRange
	}

	specialtyID, err := s.resolveSpecialty(ctx, item.SpecialtyCode, now)
	if err != nil {
		return CareAvailability{}, err
	}
	professionals, err := s.booking.ListProfessionals(ctx, specialtyID, now)
	if err != nil {
		return CareAvailability{}, err
	}

	j := engineJourney(snap, s.threshold)
	engineItem, rules := engineItemOf(item)

	var slots []AvailabilitySlot
	for _, p := range professionals {
		page, err := s.booking.ListSlotPage(ctx, p.ID, f, upper, now)
		if errors.Is(err, ErrProfessionalNotFound) {
			// Sumiu entre a listagem e a agenda (legado é de terceiro): a página
			// segue com os demais em vez de negar a disponibilidade inteira.
			continue
		}
		if err != nil {
			return CareAvailability{}, err
		}
		for _, sl := range page.Slots {
			slots = append(slots, AvailabilitySlot{
				Slot:         sl,
				Professional: page.Professional,
				Eligibility:  careline.Evaluate(j, engineItem, rules, sl.StartsAt, now),
			})
		}
	}
	sort.Slice(slots, func(i, k int) bool {
		if !slots[i].Slot.StartsAt.Equal(slots[k].Slot.StartsAt) {
			return slots[i].Slot.StartsAt.Before(slots[k].Slot.StartsAt)
		}
		return slots[i].Slot.ID < slots[k].Slot.ID
	})

	return CareAvailability{
		ItemID: itemID, From: f, To: t, TimeZone: loc.String(), Slots: slots,
	}, nil
}

// Schedule agenda a consulta de um item: motor primeiro, booking depois,
// projeção por último — idempotente pela Idempotency-Key.
//
// O replay devolve a MESMA consulta (replayed=true) sem tocar em nada. A corrida
// de dois requests com a mesma key é decidida pelo índice único do banco; o
// perdedor COMPENSA o booking que criou e devolve o vencedor.
func (s *JourneyStore) Schedule(ctx context.Context, in ScheduleInput) (CareAppointment, bool, error) {
	key := strings.TrimSpace(in.IdemKey)
	if key == "" {
		return CareAppointment{}, false, ErrIdemKeyRequired
	}

	snap, item, err := s.freshSnapshotByItem(ctx, in.Account.ID, in.ItemID, in.Now)
	if err != nil {
		return CareAppointment{}, false, err
	}

	// Replay: a key já criou uma consulta nesta matrícula — devolve a mesma.
	if appt, ok, err := s.storage.FindByIdemKey(ctx, snap.Enrollment.ID, key); err != nil {
		return CareAppointment{}, false, fmt.Errorf("consultar idempotência: %w", err)
	} else if ok {
		return s.view(appt), true, nil
	}

	specialtyID, err := s.resolveSpecialty(ctx, item.SpecialtyCode, in.Now)
	if err != nil {
		return CareAppointment{}, false, err
	}
	booking, err := s.booking.SlotInfo(ctx, in.SlotID, specialtyID)
	if err != nil {
		return CareAppointment{}, false, err
	}
	// Falha cedo e SEM escrever nada, como o loadBooking do Book: quem decide a
	// corrida de verdade é o CAS lá dentro.
	if booking.Booked {
		return CareAppointment{}, false, ErrSlotTaken
	}
	if !booking.Slot.StartsAt.After(in.Now) {
		return CareAppointment{}, false, ErrSlotExpired
	}

	// O motor decide ANTES de qualquer escrita. intendedAt é o INSTANTE do slot.
	j := engineJourney(snap, s.threshold)
	engineItem, rules := engineItemOf(item)
	if verdict := careline.Evaluate(j, engineItem, rules, booking.Slot.StartsAt, in.Now); !verdict.Allowed {
		return CareAppointment{}, false, ErrNotEligible{Blocks: verdict.Blocks}
	}

	booked, err := s.booking.Book(ctx, BookInput{
		Account: in.Account, SlotID: in.SlotID, SpecialtyID: specialtyID, Now: in.Now,
	})
	if err != nil {
		// Os erros do booking sobem INTACTOS: o controller já sabe mapeá-los
		// (slot tomado 409, unconfirmed 502 etc.).
		return CareAppointment{}, false, err
	}
	bookingID, err := uuid.Parse(booked.ID)
	if err != nil {
		return CareAppointment{}, false, fmt.Errorf("id do booking não é uuid: %w", err)
	}

	appt, err := s.storage.CreateScheduled(ctx, CreateScheduledInput{
		EnrollmentID: snap.Enrollment.ID, PatientID: in.Account.ID,
		CareLineItemID: item.ID, ItemRef: item.Ref, Label: item.Label,
		BookingID: bookingID, SlotID: in.SlotID, ScheduledAt: booked.StartsAt,
		IdemKey: key, Now: in.Now,
	})
	if errors.Is(err, errCareIdemRace) {
		// Dois requests com a MESMA key passaram juntos pelo replay: o índice
		// único escolheu o vencedor. O booking do perdedor é uma consulta REAL e
		// precisa ser desfeito — se o cancel falhar, fica o log para operação.
		if _, cancelErr := s.booking.Cancel(ctx, in.Account, bookingID, in.Now); cancelErr != nil {
			s.logger.ErrorContext(ctx, "jornada: compensação do booking falhou após corrida de idempotência",
				"booking_id", bookingID, "error", cancelErr.Error())
		}
		winner, ok, findErr := s.storage.FindByIdemKey(ctx, snap.Enrollment.ID, key)
		if findErr != nil || !ok {
			return CareAppointment{}, false, fmt.Errorf("jornada: corrida de idempotência sem vencedor (find=%v): %w", findErr, err)
		}
		return s.view(winner), true, nil
	}
	if err != nil {
		// O booking existe (consulta real na DAV) e a projeção não gravou. Não há
		// compensação segura genérica aqui — log ERROR com os ids para operação.
		s.logger.ErrorContext(ctx, "jornada: booking criado mas projeção da jornada não gravada",
			"booking_id", bookingID, "enrollment_id", snap.Enrollment.ID, "error", err.Error())
		return CareAppointment{}, false, fmt.Errorf("gravar consulta da jornada: %w", err)
	}
	return s.view(appt), false, nil
}

// CancelCare cancela uma consulta da jornada do próprio paciente: cancela o
// booking (legado + DAV) e grava o cancelamento + evento com o bookkeeping de
// cota (a MESMA semântica do motor).
func (s *JourneyStore) CancelCare(ctx context.Context, account Account, careApptID uuid.UUID, now time.Time) (CareAppointment, error) {
	appt, err := s.storage.GetForPatient(ctx, account.ID, careApptID)
	if err != nil {
		return CareAppointment{}, err
	}
	if appt.Status != careline.StatusAgendada && appt.Status != careline.StatusConfirmada {
		return CareAppointment{}, ErrCareCancelNotAllowed
	}

	// Bookkeeping ANTES do efeito: com quanta antecedência, e se ainda consome a
	// cota. counts_for_quota espelha o counts() do motor — cancelamento tardio
	// (dentro do threshold) segue ocupando a vaga.
	hoursBefore := math.Round(appt.ScheduledAt.Sub(now).Hours()*10) / 10
	countsForQuota := now.After(appt.ScheduledAt.Add(-s.threshold))

	res, err := s.booking.Cancel(ctx, account, appt.BookingID, now)
	davError := res.DAVError
	if err != nil {
		if errors.Is(err, ErrAppointmentNotFound) || errors.Is(err, ErrCancelNotAllowed) {
			// Dessincronia jornada×booking (o booking sumiu ou já não está
			// CONFIRMED). O paciente não pode ficar preso por isso: registra e
			// SEGUE o cancelamento local.
			s.logger.ErrorContext(ctx, "jornada: booking fora de sincronia no cancelamento — cancelando só a jornada",
				"care_appointment_id", careApptID, "booking_id", appt.BookingID, "error", err.Error())
			davError = err.Error()
		} else {
			return CareAppointment{}, err
		}
	}

	out, err := s.storage.CancelScheduled(ctx, CancelScheduledInput{
		ID: appt.ID, EnrollmentID: appt.EnrollmentID, PatientID: account.ID, Now: now,
		HoursBefore: hoursBefore, CountsForQuota: countsForQuota,
		DAVCancelled: res.DAVCancelled, DAVError: davError,
	})
	if err != nil {
		return CareAppointment{}, err
	}
	return s.view(out), nil
}

// ListCare lista as consultas da jornada do paciente, com filtro opcional por
// status (validado no controller contra o enum do contrato).
func (s *JourneyStore) ListCare(ctx context.Context, account Account, status *string) ([]CareAppointment, error) {
	rows, err := s.storage.ListForPatient(ctx, account.ID, status)
	if err != nil {
		return nil, fmt.Errorf("listar consultas da jornada: %w", err)
	}
	out := make([]CareAppointment, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.view(r))
	}
	return out, nil
}

// Audit devolve uma página do event log, por cursor OPACO
// (base64url de "occurred_at RFC3339Nano|id"). next_cursor presente sse a
// página veio cheia.
func (s *JourneyStore) Audit(ctx context.Context, account Account, cursor *string, limit int) (CareAuditPage, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	var cur *AuditCursor
	if cursor != nil && *cursor != "" {
		c, err := parseAuditCursor(*cursor)
		if err != nil {
			return CareAuditPage{}, ErrBadCursor
		}
		cur = &c
	}

	events, err := s.storage.AuditPage(ctx, account.ID, cur, limit)
	if err != nil {
		return CareAuditPage{}, fmt.Errorf("paginar auditoria: %w", err)
	}
	page := CareAuditPage{Events: events}
	if len(events) == limit {
		last := events[len(events)-1]
		next := encodeAuditCursor(AuditCursor{OccurredAt: last.OccurredAt, ID: last.ID})
		page.NextCursor = &next
	}
	return page, nil
}

// ForceStatus força realizada/falta (rota interna de teste, gated por env).
func (s *JourneyStore) ForceStatus(ctx context.Context, careApptID uuid.UUID, status string, now time.Time) (CareAppointment, error) {
	if status != careline.StatusRealizada && status != careline.StatusFalta {
		return CareAppointment{}, ErrInvalidForceStatus
	}
	out, err := s.storage.ForceStatus(ctx, ForceStatusInput{ID: careApptID, Status: status, Now: now})
	if err != nil {
		return CareAppointment{}, err
	}
	return s.view(out), nil
}

// ---------------------------------------------------------------------------
// Auxiliares
// ---------------------------------------------------------------------------

// freshSnapshot aplica a expiração LAZY: matrícula ativa com a vigência vencida
// é expirada AGORA (com evento, na TX do storage) e o snapshot é recarregado —
// o motor nunca decide sobre um status que já não é verdade.
func (s *JourneyStore) freshSnapshot(ctx context.Context, snap EnrollmentSnapshot, now time.Time) (EnrollmentSnapshot, error) {
	if snap.Enrollment.Status != careline.EnrollmentAtiva || !now.After(snap.Enrollment.ValidUntil) {
		return snap, nil
	}
	if err := s.storage.Expire(ctx, snap.Enrollment.ID, now); err != nil {
		return EnrollmentSnapshot{}, fmt.Errorf("expirar matrícula: %w", err)
	}
	fresh, err := s.storage.SnapshotEnrollment(ctx, snap.Enrollment.ID)
	if err != nil {
		return EnrollmentSnapshot{}, fmt.Errorf("recarregar matrícula expirada: %w", err)
	}
	return fresh, nil
}

func (s *JourneyStore) freshSnapshotByItem(ctx context.Context, patientID, itemID uuid.UUID, now time.Time) (EnrollmentSnapshot, CareLineItem, error) {
	snap, item, err := s.storage.SnapshotByItem(ctx, patientID, itemID)
	if err != nil {
		return EnrollmentSnapshot{}, CareLineItem{}, err
	}
	snap, err = s.freshSnapshot(ctx, snap, now)
	if err != nil {
		return EnrollmentSnapshot{}, CareLineItem{}, err
	}
	return snap, item, nil
}

// resolveSpecialty casa o specialty_code do item com o catálogo VIVO do legado,
// pela MESMA normalização do publish. Listagem fora do ar sobe intacta (503);
// nome que sumiu é ErrSpecialtyNotFound (404) — o legado mudou por baixo.
func (s *JourneyStore) resolveSpecialty(ctx context.Context, code string, now time.Time) (string, error) {
	specs, err := s.booking.ListSpecialties(ctx, now)
	if err != nil {
		return "", err
	}
	want := careline.NormalizeSpecialty(code)
	for _, sp := range specs {
		if careline.NormalizeSpecialty(sp.Name) == want {
			return sp.ID, nil
		}
	}
	return "", ErrSpecialtyNotFound
}

// view finaliza uma consulta para consumo: instantes no fuso da agenda e o
// TimeZone de exibição preenchido.
func (s *JourneyStore) view(a CareAppointment) CareAppointment {
	loc := s.booking.Location()
	a.ScheduledAt = a.ScheduledAt.In(loc)
	if a.CancelledAt != nil {
		t := a.CancelledAt.In(loc)
		a.CancelledAt = &t
	}
	a.TimeZone = loc.String()
	return a
}

// engineJourney projeta o snapshot no formato do motor puro.
func engineJourney(snap EnrollmentSnapshot, threshold time.Duration) careline.Journey {
	items := make([]careline.Item, 0, len(snap.Items))
	for _, it := range snap.Items {
		engineItem, _ := engineItemOf(it)
		items = append(items, engineItem)
	}
	appts := make([]careline.JourneyAppointment, 0, len(snap.Appointments))
	for _, a := range snap.Appointments {
		appts = append(appts, careline.JourneyAppointment{
			ItemRef: a.ItemRef, Status: a.Status,
			ScheduledAt: a.ScheduledAt, CancelledAt: a.CancelledAt,
		})
	}
	return careline.Journey{
		Status:               snap.Enrollment.Status,
		ValidFrom:            snap.Enrollment.ValidFrom,
		ValidUntil:           snap.Enrollment.ValidUntil,
		LineItems:            items,
		Appointments:         appts,
		CancelCountThreshold: threshold,
	}
}

func engineItemOf(it CareLineItem) (careline.Item, []careline.Rule) {
	rules := make([]careline.Rule, 0, len(it.Rules))
	for _, r := range it.Rules {
		rules = append(rules, careline.Rule{Type: r.RuleType, Params: r.Params})
	}
	return careline.Item{Ref: it.Ref, Kind: it.Kind, SpecialtyCode: it.SpecialtyCode, Label: it.Label}, rules
}

// dayStart normaliza um instante para a meia-noite do dia dele no fuso dado.
func dayStart(t time.Time, loc *time.Location) time.Time {
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

// encodeAuditCursor/parseAuditCursor: o cursor viaja OPACO
// (base64url de "occurred_at RFC3339Nano|id"). Opaco de propósito — o cliente o
// devolve como veio; o formato interno pode mudar sem quebrar ninguém.
func encodeAuditCursor(c AuditCursor) string {
	raw := c.OccurredAt.UTC().Format(time.RFC3339Nano) + "|" + c.ID.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func parseAuditCursor(s string) (AuditCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return AuditCursor{}, ErrBadCursor
	}
	occurred, id, ok := strings.Cut(string(raw), "|")
	if !ok {
		return AuditCursor{}, ErrBadCursor
	}
	at, err := time.Parse(time.RFC3339Nano, occurred)
	if err != nil {
		return AuditCursor{}, ErrBadCursor
	}
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return AuditCursor{}, ErrBadCursor
	}
	return AuditCursor{OccurredAt: at, ID: parsedID}, nil
}
