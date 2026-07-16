package models

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/renovisaude/renovi-care/internal/adapters/agenda"
	"github.com/renovisaude/renovi-care/internal/adapters/dav"
	"github.com/renovisaude/renovi-care/internal/db/gen"
	"github.com/renovisaude/renovi-care/internal/models/scheduling"
)

// Erros do agendamento. Use errors.Is.
var (
	// ErrSlotTaken: alguém reservou o horário antes — outro paciente nosso ou o
	// app legado.
	ErrSlotTaken = errors.New("agendamento: horário já reservado")
	// ErrSlotNotFound: o horário não existe (ou não é do profissional/especialidade
	// pedidos).
	ErrSlotNotFound = errors.New("agendamento: horário não encontrado")
	// ErrSlotExpired: o horário já passou. A DAV recusaria de qualquer forma (422),
	// mas falhar aqui poupa um POST de vários segundos.
	ErrSlotExpired = errors.New("agendamento: horário no passado")
	// ErrSpecialtyMismatch: o profissional do horário não atende essa especialidade.
	ErrSpecialtyMismatch = errors.New("agendamento: profissional não atende esta especialidade")
	// ErrAccountNotLinked: a conta não tem vínculo com a DAV, então não há
	// participante PAT. Não deveria acontecer (só conta ACTIVE autentica, e ACTIVE
	// exige vínculo pelo CHECK do banco), mas é barato conferir.
	ErrAccountNotLinked = errors.New("agendamento: conta sem vínculo com a DAV")
	// ErrBookingUnconfirmed: reservamos o horário mas a DAV não confirmou, e NÃO
	// vamos descobrir sozinhos se a consulta existe. Vira 502/504 e status
	// UNCONFIRMED para o paciente. Repetir NÃO é seguro.
	ErrBookingUnconfirmed = errors.New("agendamento: a DAV não confirmou e o resultado é desconhecido")
	// ErrAppointmentNotFound: não existe, ou não é do dono da sessão. Os dois
	// respondem igual (404) — ver a resposta NotFound no openapi.yaml.
	ErrAppointmentNotFound = errors.New("agendamento: consulta não encontrada")
	// ErrJoinNotAllowed: a janela de entrada não está aberta. Carrega o motivo.
	ErrJoinNotAllowed = errors.New("agendamento: não é possível entrar agora")
)

// JoinDenied detalha o ErrJoinNotAllowed para o controller montar o 409 com
// `reason` e `opens_at`.
type JoinDenied struct {
	Reason  string
	OpensAt time.Time
}

func (e JoinDenied) Error() string { return "agendamento: entrada negada (" + e.Reason + ")" }
func (e JoinDenied) Unwrap() error { return ErrJoinNotAllowed }

// AgendaClient é o que o agendamento precisa do MySQL legado.
//
// Declarada aqui, no CONSUMIDOR, e não no adapter (ADR-012): quem usa diz o que
// precisa, e o teste troca por um fake sem subir MySQL.
type AgendaClient interface {
	ListSpecialties(ctx context.Context, now time.Time) ([]agenda.Specialty, error)
	ListProfessionalsBySpecialty(ctx context.Context, specialtyID string, now time.Time) ([]agenda.Professional, error)
	ListSlots(ctx context.Context, professionalID string, from, to, now time.Time) ([]agenda.Slot, error)
	LoadBooking(ctx context.Context, slotID, specialtyID string) (agenda.Booking, error)
	BookSlot(ctx context.Context, slotID string) error
	ReleaseSlot(ctx context.Context, slotID string) error
	Location() *time.Location
}

// DAVAppointments é o que o agendamento precisa da DAV. Separada da DAVClient do
// cadastro: cada consumidor declara só o que usa.
type DAVAppointments interface {
	CreateAppointment(ctx context.Context, in dav.CreateAppointmentInput) (dav.Appointment, error)
}

// Appointment é a consulta como o produto a enxerga.
type Appointment struct {
	ID               string
	Status           string // PROCESSING | CONFIRMED | UNCONFIRMED | CANCELLED
	StartsAt         time.Time
	EndsAt           time.Time
	TimeZone         string
	SpecialtyID      string
	SpecialtyName    string
	ProfessionalID   string
	ProfessionalName string
	CreatedAt        time.Time

	// Join é o ESTADO da janela — nunca a url. O link é credencial e sai só do
	// JoinURL(), depois de conferir a janela com o relógio do servidor. Se ele
	// morasse neste struct, mais cedo ou mais tarde alguém o serializaria numa
	// listagem e a regra dos 30 minutos viraria decoração.
	Join scheduling.Window
}

// BookingStore é a camada de dados + regra do agendamento.
type BookingStore struct {
	pool   *pgxpool.Pool
	q      *gen.Queries
	agenda AgendaClient
	dav    DAVAppointments
	policy scheduling.Policy
	logger *slog.Logger
}

func NewBookingStore(pool *pgxpool.Pool, ag AgendaClient, davClient DAVAppointments, policy scheduling.Policy, logger *slog.Logger) *BookingStore {
	if logger == nil {
		logger = slog.Default()
	}
	return &BookingStore{pool: pool, q: gen.New(pool), agenda: ag, dav: davClient, policy: policy, logger: logger}
}

// ---------------------------------------------------------------------------
// Leitura do catálogo (passa direto para o legado)
// ---------------------------------------------------------------------------

func (s *BookingStore) ListSpecialties(ctx context.Context, now time.Time) ([]agenda.Specialty, error) {
	return s.agenda.ListSpecialties(ctx, now)
}

func (s *BookingStore) ListProfessionals(ctx context.Context, specialtyID string, now time.Time) ([]agenda.Professional, error) {
	return s.agenda.ListProfessionalsBySpecialty(ctx, specialtyID, now)
}

func (s *BookingStore) ListSlots(ctx context.Context, professionalID string, from, to, now time.Time) ([]agenda.Slot, error) {
	return s.agenda.ListSlots(ctx, professionalID, from, to, now)
}

func (s *BookingStore) Location() *time.Location { return s.agenda.Location() }

// ---------------------------------------------------------------------------
// A saga
// ---------------------------------------------------------------------------

// BookInput são os dados do agendamento.
type BookInput struct {
	Account     Account
	DAVPersonID string
	SlotID      string
	SpecialtyID string
	Now         time.Time
}

// Book agenda a consulta: reserva o horário no legado, cria na DAV e espelha aqui.
//
// A ordem é a única que sobrevive a um crash em qualquer ponto:
//
//	TX1 (PG)    INSERT PENDING_SLOT           -- intenção registrada
//	   (MySQL)  CAS booked=1                  -- o horário é nosso
//	TX2 (PG)    status=DAV_PENDING            -- "vou fazer a escrita insondável"
//	   (sem TX) POST /appointment             -- 3-17s, NUNCA repete
//	TX3 (PG)    status=CONFIRMED + link
//
// Por que a intenção vem ANTES de reservar o horário: se travássemos o slot
// primeiro, um crash entre o commit do MySQL e o INSERT aqui deixaria booked=1
// sem nada apontando para ele — e `tb_slots` não tem coluna de dono, então nem um
// humano saberia distinguir esse resíduo de uma reserva do app legado. Ficaria
// perdido para sempre. Registrando a intenção antes, todo horário que travamos
// tem uma linha nossa que o explica.
//
// Nenhuma conexão de pool — de banco nenhum — fica presa durante o HTTP. É a
// mesma disciplina do Register (ADR-011), pelo mesmo motivo: uma transação aberta
// por 17s prende conexão e derruba a API sob carga.
func (s *BookingStore) Book(ctx context.Context, in BookInput) (Appointment, error) {
	if in.DAVPersonID == "" {
		return Appointment{}, ErrAccountNotLinked
	}

	booking, err := s.loadBooking(ctx, in)
	if err != nil {
		return Appointment{}, err
	}

	row, err := s.registerIntent(ctx, in, booking)
	if err != nil {
		return Appointment{}, err
	}

	// A partir daqui, qualquer saída precisa decidir o destino do horário.
	if err := s.holdSlot(ctx, row.ID, booking.Slot.ID); err != nil {
		return Appointment{}, err
	}

	created, err := s.createInDAV(ctx, row.ID, booking, in)
	if err != nil {
		return Appointment{}, err
	}

	if err := s.q.ConfirmAppointment(ctx, gen.ConfirmAppointmentParams{
		ID:               row.ID,
		DavAppointmentID: text(created.ID),
		PatientJoinUrl:   text(created.PatientJoinURL),
	}); err != nil {
		// A consulta EXISTE na DAV e não conseguimos gravar. Não dá para desfazer
		// (o cancel deles responde 500 — achado #20) e não dá para reencontrá-la.
		// É o mesmo desconhecido: segura o horário, chama gente.
		s.logger.ErrorContext(ctx, "agendamento: consulta criada na DAV mas não gravada",
			"appointment_id", row.ID, "error", err.Error())
		s.markUnknown(ctx, row.ID)
		return Appointment{}, fmt.Errorf("%w: gravar confirmação: %v", ErrBookingUnconfirmed, err)
	}

	out := s.toAppointment(row, booking, in.Now)
	out.Status = statusConfirmed
	out.Join = scheduling.Evaluate(s.policy, scheduling.StateConfirmed, out.StartsAt, out.EndsAt, in.Now)
	return out, nil
}

// loadBooking resolve e valida o horário ANTES de escrever em qualquer lugar.
func (s *BookingStore) loadBooking(ctx context.Context, in BookInput) (agenda.Booking, error) {
	booking, err := s.agenda.LoadBooking(ctx, in.SlotID, in.SpecialtyID)
	switch {
	case errors.Is(err, agenda.ErrSlotNotFound):
		return agenda.Booking{}, ErrSlotNotFound
	case errors.Is(err, agenda.ErrSpecialtyMismatch):
		return agenda.Booking{}, ErrSpecialtyMismatch
	case err != nil:
		return agenda.Booking{}, err
	}

	// O `booked` que lemos aqui é só um atalho para falhar cedo e sem escrever
	// nada: quem decide de verdade é o CAS, atômico, mais abaixo.
	if booking.Booked {
		return agenda.Booking{}, ErrSlotTaken
	}
	if !booking.Slot.StartsAt.After(in.Now) {
		return agenda.Booking{}, ErrSlotExpired
	}
	return booking, nil
}

// registerIntent é a TX1. O índice único parcial decide a corrida entre dois
// pacientes NOSSOS antes de qualquer sistema externo ser tocado.
func (s *BookingStore) registerIntent(ctx context.Context, in BookInput, b agenda.Booking) (gen.Appointment, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return gen.Appointment{}, fmt.Errorf("gerar uuid v7: %w", err)
	}

	row, err := s.q.CreateAppointmentIntent(ctx, gen.CreateAppointmentIntentParams{
		ID:                   id,
		AccountID:            in.Account.ID,
		LegacySlotID:         b.Slot.ID,
		LegacyProfessionalID: b.Professional.ID,
		LegacySpecialtyID:    b.Specialty.ID,
		ProfessionalName:     b.Professional.FullName,
		SpecialtyName:        b.Specialty.Name,
		StartsAt:             b.Slot.StartsAt,
		EndsAt:               b.Slot.EndsAt,
	})
	if err != nil {
		// O índice único parcial ux_appointment_slot_vivo: outro paciente NOSSO já
		// tem reserva viva neste horário.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return gen.Appointment{}, ErrSlotTaken
		}
		return gen.Appointment{}, fmt.Errorf("registrar intenção: %w", err)
	}
	return row, nil
}

// holdSlot é o CAS no legado + a marca de que a escrita insondável vem a seguir.
func (s *BookingStore) holdSlot(ctx context.Context, appointmentID uuid.UUID, slotID string) error {
	err := s.agenda.BookSlot(ctx, slotID)
	switch {
	case errors.Is(err, agenda.ErrSlotTaken):
		// Perdemos a corrida para o app legado (ou para outro processo nosso). Não
		// travamos nada, então não há o que compensar.
		s.failQuietly(ctx, appointmentID)
		return ErrSlotTaken
	case errors.Is(err, agenda.ErrSlotNotFound):
		s.failQuietly(ctx, appointmentID)
		return ErrSlotNotFound
	case err != nil:
		s.failQuietly(ctx, appointmentID)
		return err
	}

	// O horário é nosso. Gravar DAV_PENDING antes do POST custa ~1ms e é o que
	// separa "sabemos que a DAV nunca foi chamada" de "pode ter sido".
	if err := s.q.MarkAppointmentSlotHeld(ctx, appointmentID); err != nil {
		// Travamos o horário e não conseguimos anotar. Devolvê-lo é seguro: a DAV
		// ainda não foi chamada, então não existe consulta fantasma.
		s.releaseQuietly(ctx, appointmentID, slotID)
		return fmt.Errorf("marcar horário reservado: %w", err)
	}
	return nil
}

// createInDAV faz a escrita que não dá para desfazer nem reconciliar.
func (s *BookingStore) createInDAV(ctx context.Context, appointmentID uuid.UUID, b agenda.Booking, in BookInput) (dav.Appointment, error) {
	created, err := s.dav.CreateAppointment(ctx, dav.CreateAppointmentInput{
		Title:          b.Specialty.Name + " com " + b.Professional.FullName,
		StartsAt:       b.Slot.StartsAt,
		EndsAt:         b.Slot.EndsAt,
		Specialty:      b.Specialty.Name,
		ProfessionalID: b.Professional.ID,
		PatientID:      in.DAVPersonID,
	})
	if err == nil {
		return created, nil
	}

	// 4xx: a DAV tem opinião firme e não houve efeito. O horário volta ao mercado.
	if errors.Is(err, dav.ErrValidation) || errors.Is(err, dav.ErrDuplicateCPF) || errors.Is(err, dav.ErrDuplicateEmail) {
		s.logger.WarnContext(ctx, "agendamento: a DAV recusou a consulta",
			"appointment_id", appointmentID, "error", err.Error())
		s.failQuietly(ctx, appointmentID)
		s.releaseQuietly(ctx, appointmentID, b.Slot.ID)
		return dav.Appointment{}, fmt.Errorf("%w: %v", ErrBookingUnconfirmed, err)
	}

	// Qualquer outra coisa é DESCONHECIDO — e aqui, ao contrário do cadastro, é
	// desconhecido para sempre: não podemos sondar (o id é deles e não há rota de
	// busca), não podemos cancelar (o cancel deles responde 500) e não podemos
	// repetir (criaria uma segunda consulta real). O horário FICA retido. Soltá-lo
	// deixaria outro paciente marcar em cima de uma consulta que talvez exista, e
	// a DAV não barra sobreposição.
	s.logger.ErrorContext(ctx, "agendamento: resultado desconhecido na DAV — horário retido e consulta em revisão",
		"appointment_id", appointmentID, "error", err.Error())
	s.markUnknown(ctx, appointmentID)
	return dav.Appointment{}, fmt.Errorf("%w: %v", ErrBookingUnconfirmed, err)
}

// ---------------------------------------------------------------------------
// Compensação
// ---------------------------------------------------------------------------

// failQuietly e releaseQuietly não propagam erro: quem os chama já está no
// caminho de falha e tem um erro melhor para contar ao paciente. O que eles
// deixam por fazer vira fila do worker (ListPendingSlotRelease), que é justamente
// o motivo de a compensação não depender de o request path dar certo.
func (s *BookingStore) failQuietly(ctx context.Context, id uuid.UUID) {
	if err := s.q.FailAppointment(ctx, id); err != nil {
		s.logger.ErrorContext(ctx, "agendamento: não consegui marcar como falha",
			"appointment_id", id, "error", err.Error())
	}
}

func (s *BookingStore) markUnknown(ctx context.Context, id uuid.UUID) {
	if err := s.q.MarkAppointmentUnknown(ctx, id); err != nil {
		s.logger.ErrorContext(ctx, "agendamento: não consegui marcar como desconhecido",
			"appointment_id", id, "error", err.Error())
	}
}

func (s *BookingStore) releaseQuietly(ctx context.Context, id uuid.UUID, slotID string) {
	if err := s.agenda.ReleaseSlot(ctx, slotID); err != nil {
		// O worker repete: a linha continua FAILED com slot_held_at e sem
		// slot_released_at, que é exatamente a fila de compensação.
		s.logger.ErrorContext(ctx, "agendamento: não consegui devolver o horário — o worker tenta de novo",
			"appointment_id", id, "slot_id", slotID, "error", err.Error())
		return
	}
	if err := s.q.MarkSlotReleased(ctx, id); err != nil {
		s.logger.ErrorContext(ctx, "agendamento: horário devolvido mas não anotado",
			"appointment_id", id, "error", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Consulta
// ---------------------------------------------------------------------------

func (s *BookingStore) ListForAccount(ctx context.Context, accountID uuid.UUID, now time.Time) ([]Appointment, error) {
	rows, err := s.q.ListAppointmentsByAccount(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("listar consultas: %w", err)
	}
	out := make([]Appointment, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.fromRow(r, now))
	}
	return out, nil
}

func (s *BookingStore) GetForAccount(ctx context.Context, id, accountID uuid.UUID, now time.Time) (Appointment, error) {
	row, err := s.q.GetAppointmentForAccount(ctx, gen.GetAppointmentForAccountParams{ID: id, AccountID: accountID})
	if err != nil {
		return Appointment{}, ErrAppointmentNotFound
	}
	if row.Status == statusRowFailed {
		return Appointment{}, ErrAppointmentNotFound
	}
	return s.fromRow(row, now), nil
}

// JoinURL entrega o link da sala — e é o ÚNICO lugar que o entrega.
//
// A janela é decidida com o relógio do SERVIDOR e com a política que vem da
// config. Se o link viajasse no payload da consulta, a regra dos 30 minutos
// viraria decoração: bastaria abrir o DevTools para entrar a qualquer hora.
func (s *BookingStore) JoinURL(ctx context.Context, id, accountID uuid.UUID, now time.Time) (string, error) {
	row, err := s.q.GetAppointmentForAccount(ctx, gen.GetAppointmentForAccountParams{ID: id, AccountID: accountID})
	if err != nil || row.Status == statusRowFailed {
		return "", ErrAppointmentNotFound
	}

	w := scheduling.Evaluate(s.policy, stateOf(row.Status), row.StartsAt, row.EndsAt, now)
	if !w.Allowed {
		return "", JoinDenied{Reason: w.Reason, OpensAt: w.OpensAt}
	}
	if !row.PatientJoinUrl.Valid || row.PatientJoinUrl.String == "" {
		// CONFIRMED sem link não passa pelo CHECK do banco, então isto é defesa em
		// profundidade — mas se acontecer, o paciente merece a frase certa.
		return "", JoinDenied{Reason: scheduling.ReasonNotConfirmed, OpensAt: w.OpensAt}
	}
	return row.PatientJoinUrl.String, nil
}

// ---------------------------------------------------------------------------
// Tradução entre o vocabulário da saga e o do paciente
// ---------------------------------------------------------------------------

// Status internos (banco) — o paciente nunca os vê.
const (
	statusRowPendingSlot = "PENDING_SLOT"
	statusRowDavPending  = "DAV_PENDING"
	statusRowConfirmed   = "CONFIRMED"
	statusRowFailed      = "FAILED"
	statusRowDavUnknown  = "DAV_UNKNOWN"
	statusRowCancelled   = "CANCELLED"
)

// Status do contrato (o que o paciente vê).
const (
	statusProcessing  = "PROCESSING"
	statusConfirmed   = "CONFIRMED"
	statusUnconfirmed = "UNCONFIRMED"
	statusCancelled   = "CANCELLED"
)

// patientStatus mapeia os estados da saga para o que o paciente entende.
//
// DAV_UNKNOWN vira UNCONFIRMED e NÃO some da lista: o paciente pode ter uma
// consulta de verdade marcada, e esconder seria pior do que dizer "não
// conseguimos confirmar; estamos verificando".
func patientStatus(rowStatus string) string {
	switch rowStatus {
	case statusRowConfirmed:
		return statusConfirmed
	case statusRowCancelled:
		return statusCancelled
	case statusRowDavUnknown:
		return statusUnconfirmed
	default: // PENDING_SLOT, DAV_PENDING
		return statusProcessing
	}
}

// stateOf traduz para o enum do pacote puro, que não conhece (nem deve conhecer)
// o vocabulário da saga.
func stateOf(rowStatus string) scheduling.State {
	switch rowStatus {
	case statusRowConfirmed:
		return scheduling.StateConfirmed
	case statusRowCancelled:
		return scheduling.StateCancelled
	default:
		return scheduling.StatePending
	}
}

func (s *BookingStore) fromRow(r gen.Appointment, now time.Time) Appointment {
	loc := s.agenda.Location()
	a := Appointment{
		ID:               r.ID.String(),
		Status:           patientStatus(r.Status),
		StartsAt:         r.StartsAt.In(loc),
		EndsAt:           r.EndsAt.In(loc),
		TimeZone:         loc.String(),
		SpecialtyID:      r.LegacySpecialtyID,
		SpecialtyName:    r.SpecialtyName,
		ProfessionalID:   r.LegacyProfessionalID,
		ProfessionalName: r.ProfessionalName,
		CreatedAt:        r.CreatedAt,
		Join:             scheduling.Evaluate(s.policy, stateOf(r.Status), r.StartsAt.In(loc), r.EndsAt.In(loc), now),
	}
	return a
}

func (s *BookingStore) toAppointment(r gen.Appointment, b agenda.Booking, now time.Time) Appointment {
	loc := s.agenda.Location()
	return Appointment{
		ID:               r.ID.String(),
		StartsAt:         b.Slot.StartsAt.In(loc),
		EndsAt:           b.Slot.EndsAt.In(loc),
		TimeZone:         loc.String(),
		SpecialtyID:      b.Specialty.ID,
		SpecialtyName:    b.Specialty.Name,
		ProfessionalID:   b.Professional.ID,
		ProfessionalName: b.Professional.FullName,
		CreatedAt:        r.CreatedAt,
	}
}
