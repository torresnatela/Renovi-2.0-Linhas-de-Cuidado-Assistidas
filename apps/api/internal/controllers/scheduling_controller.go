package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/renovisaude/renovi-care/internal/adapters/agenda"
	"github.com/renovisaude/renovi-care/internal/models"
)

// maxSlotRange é o teto do intervalo pedido em /professionals/{id}/slots. Existe
// para que um cliente distraído não peça a agenda do ano inteiro ao MySQL de
// terceiro.
const maxSlotRange = 60 * 24 * time.Hour

// Booking é o que o controller precisa do model (interface no consumidor,
// ADR-012).
type Booking interface {
	ListSpecialties(ctx context.Context, now time.Time) ([]agenda.Specialty, error)
	ListProfessionals(ctx context.Context, specialtyID string, now time.Time) ([]agenda.Professional, error)
	ListSlots(ctx context.Context, professionalID string, from, to, now time.Time) ([]agenda.Slot, error)
	Book(ctx context.Context, in models.BookInput) (models.Appointment, error)
	ListForAccount(ctx context.Context, accountID uuid.UUID, now time.Time) ([]models.Appointment, error)
	GetForAccount(ctx context.Context, id, accountID uuid.UUID, now time.Time) (models.Appointment, error)
	JoinURL(ctx context.Context, id, accountID uuid.UUID, now time.Time) (string, error)
	Location() *time.Location
}

// SchedulingController expõe o agendamento.
type SchedulingController struct {
	Bookings Booking
	// Now é injetável para o teste não depender do relógio da máquina.
	Now func() time.Time
}

func (c SchedulingController) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// ---------------------------------------------------------------------------
// Catálogo
// ---------------------------------------------------------------------------

func (c SchedulingController) ListSpecialties(w http.ResponseWriter, r *http.Request) {
	items, err := c.Bookings.ListSpecialties(r.Context(), c.now())
	if err != nil {
		writeLegacyError(w, err)
		return
	}

	out := make([]specialtyDTO, 0, len(items))
	for _, s := range items {
		out = append(out, specialtyDTO{ID: s.ID, Name: s.Name})
	}
	WriteJSON(w, http.StatusOK, listDTO[specialtyDTO]{Items: out})
}

func (c SchedulingController) ListProfessionals(w http.ResponseWriter, r *http.Request) {
	specialtyID := chi.URLParam(r, "specialty_id")
	if specialtyID == "" {
		WriteProblem(w, http.StatusBadRequest, "Requisição inválida", "Informe a especialidade.")
		return
	}

	items, err := c.Bookings.ListProfessionals(r.Context(), specialtyID, c.now())
	if err != nil {
		writeLegacyError(w, err)
		return
	}

	out := make([]professionalDTO, 0, len(items))
	for _, p := range items {
		out = append(out, toProfessionalDTO(p))
	}
	WriteJSON(w, http.StatusOK, listDTO[professionalDTO]{Items: out})
}

func (c SchedulingController) ListSlots(w http.ResponseWriter, r *http.Request) {
	professionalID := chi.URLParam(r, "professional_id")
	if professionalID == "" {
		WriteProblem(w, http.StatusBadRequest, "Requisição inválida", "Informe o profissional.")
		return
	}

	loc := c.Bookings.Location()
	now := c.now()

	// `from`/`to` são opcionais: quem sabe o fuso da agenda é o servidor, então é
	// ele que resolve o "hoje". A resposta ecoa o intervalo usado.
	from, err := parseDay(r.URL.Query().Get("from"), loc, startOfDay(now, loc))
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "Requisição inválida", "O parâmetro `from` deve ser uma data (AAAA-MM-DD).")
		return
	}
	to, err := parseDay(r.URL.Query().Get("to"), loc, from.AddDate(0, 0, 30))
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "Requisição inválida", "O parâmetro `to` deve ser uma data (AAAA-MM-DD).")
		return
	}
	if to.Before(from) {
		WriteProblem(w, http.StatusBadRequest, "Requisição inválida", "O fim do intervalo é anterior ao início.")
		return
	}
	if to.Sub(from) > maxSlotRange {
		WriteProblem(w, http.StatusBadRequest, "Requisição inválida", "O intervalo não pode passar de 60 dias.")
		return
	}

	// `to` é inclusivo no contrato, mas a query é semiaberta: somamos um dia.
	items, err := c.Bookings.ListSlots(r.Context(), professionalID, from, to.AddDate(0, 0, 1), now)
	if err != nil {
		writeLegacyError(w, err)
		return
	}

	out := make([]slotDTO, 0, len(items))
	for _, s := range items {
		out = append(out, slotDTO{
			ID:       s.ID,
			StartsAt: s.StartsAt.Format(time.RFC3339),
			EndsAt:   s.EndsAt.Format(time.RFC3339),
			TimeZone: loc.String(),
		})
	}
	WriteJSON(w, http.StatusOK, slotPageDTO{
		From:  from.Format(dayLayout),
		To:    to.Format(dayLayout),
		Items: out,
	})
}

// ---------------------------------------------------------------------------
// Consultas
// ---------------------------------------------------------------------------

func (c SchedulingController) Create(w http.ResponseWriter, r *http.Request) {
	account, ok := AccountFrom(r.Context())
	if !ok {
		WriteProblem(w, http.StatusUnauthorized, "Não autenticado", "Sessão ausente ou expirada.")
		return
	}

	// A DAV é lenta e imprevisível (3s a 17s medidos, teto de 29s do gateway).
	// Sem estender o deadline de escrita, o servidor desiste de responder antes
	// de a DAV responder — e o paciente fica sem saber se marcou. Mesma lição do
	// cadastro.
	if rc := http.NewResponseController(w); rc != nil {
		_ = rc.SetWriteDeadline(time.Now().Add(90 * time.Second))
	}

	var body struct {
		SlotID      string `json:"slot_id"`
		SpecialtyID string `json:"specialty_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteProblem(w, http.StatusBadRequest, "Requisição inválida", "Corpo JSON inválido.")
		return
	}
	if body.SlotID == "" || body.SpecialtyID == "" {
		WriteProblem(w, http.StatusBadRequest, "Requisição inválida", "Informe o horário e a especialidade.")
		return
	}

	appt, err := c.Bookings.Book(r.Context(), models.BookInput{
		Account:     account,
		SlotID:      body.SlotID,
		SpecialtyID: body.SpecialtyID,
		Now:         c.now(),
	})
	if err != nil {
		writeBookError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, c.toAppointmentDTO(appt))
}

func (c SchedulingController) List(w http.ResponseWriter, r *http.Request) {
	account, ok := AccountFrom(r.Context())
	if !ok {
		WriteProblem(w, http.StatusUnauthorized, "Não autenticado", "Sessão ausente ou expirada.")
		return
	}

	items, err := c.Bookings.ListForAccount(r.Context(), account.ID, c.now())
	if err != nil {
		WriteProblem(w, http.StatusInternalServerError, "Erro interno", "Não foi possível listar suas consultas.")
		return
	}

	out := make([]appointmentDTO, 0, len(items))
	for _, a := range items {
		out = append(out, c.toAppointmentDTO(a))
	}
	WriteJSON(w, http.StatusOK, listDTO[appointmentDTO]{Items: out})
}

func (c SchedulingController) Get(w http.ResponseWriter, r *http.Request) {
	account, ok := AccountFrom(r.Context())
	if !ok {
		WriteProblem(w, http.StatusUnauthorized, "Não autenticado", "Sessão ausente ou expirada.")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "appointment_id"))
	if err != nil {
		// Id malformado responde igual a id de terceiro: 404. Um 400 aqui diria
		// "este formato existe", que é meio caminho para enumerar.
		writeNotFound(w)
		return
	}

	appt, err := c.Bookings.GetForAccount(r.Context(), id, account.ID, c.now())
	if err != nil {
		writeNotFound(w)
		return
	}
	WriteJSON(w, http.StatusOK, c.toAppointmentDTO(appt))
}

// Join entrega o link da sala. É a única rota que o faz.
func (c SchedulingController) Join(w http.ResponseWriter, r *http.Request) {
	account, ok := AccountFrom(r.Context())
	if !ok {
		WriteProblem(w, http.StatusUnauthorized, "Não autenticado", "Sessão ausente ou expirada.")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "appointment_id"))
	if err != nil {
		writeNotFound(w)
		return
	}

	url, err := c.Bookings.JoinURL(r.Context(), id, account.ID, c.now())
	if err != nil {
		writeJoinError(w, err)
		return
	}

	// O link é credencial: nada de cache, em lugar nenhum do caminho.
	w.Header().Set("Cache-Control", "no-store")
	WriteJSON(w, http.StatusOK, joinTicketDTO{URL: url})
}

// ---------------------------------------------------------------------------
// Erros
// ---------------------------------------------------------------------------

func writeNotFound(w http.ResponseWriter) {
	// "Não existe" e "não é seu" respondem igual, de propósito: distinguir
	// transformaria a rota num oráculo de ids válidos.
	WriteProblem(w, http.StatusNotFound, "Não encontrado", "Consulta não encontrada.")
}

func writeLegacyError(w http.ResponseWriter, err error) {
	if errors.Is(err, agenda.ErrUnavailable) {
		// 503 e não lista vazia: "não há horários" e "não conseguimos ler os
		// horários" são coisas diferentes, e confundi-las faz o paciente desistir
		// de um profissional que estava livre.
		WriteProblem(w, http.StatusServiceUnavailable, "Agenda indisponível",
			"Não conseguimos consultar a agenda agora. Tente novamente em instantes.")
		return
	}
	WriteProblem(w, http.StatusInternalServerError, "Erro interno", "Não foi possível concluir a operação.")
}

func writeBookError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, models.ErrSlotTaken):
		WriteProblemReason(w, http.StatusConflict, "Horário indisponível",
			"Este horário acabou de ser reservado por outra pessoa. Escolha outro.",
			Reason{Code: "SLOT_TAKEN"})
	case errors.Is(err, models.ErrSlotExpired):
		WriteProblemReason(w, http.StatusConflict, "Horário indisponível",
			"Este horário já passou.", Reason{Code: "SLOT_EXPIRED"})
	case errors.Is(err, models.ErrSlotNotFound):
		WriteProblem(w, http.StatusNotFound, "Não encontrado", "Horário não encontrado.")
	case errors.Is(err, models.ErrSpecialtyMismatch):
		WriteProblem(w, http.StatusBadRequest, "Requisição inválida",
			"Este profissional não atende a especialidade escolhida.")
	case errors.Is(err, models.ErrAccountNotLinked):
		WriteProblem(w, http.StatusForbidden, "Cadastro incompleto",
			"Sua conta ainda não está vinculada à Doutor ao Vivo.")
	case errors.Is(err, models.ErrBookingUnconfirmed):
		// 502 e NÃO 500: o problema é do sistema de terceiro. E o texto precisa
		// ser honesto — a consulta PODE existir, e repetir criaria uma segunda de
		// verdade, então o paciente é mandado para a lista, não para um "tentar de
		// novo".
		WriteProblemReason(w, http.StatusBadGateway, "Não conseguimos confirmar",
			"Reservamos seu horário, mas a Doutor ao Vivo não confirmou a consulta a tempo. "+
				"Ela PODE ter sido marcada — veja em Minhas consultas. Nossa equipe está verificando.",
			Reason{Code: "BOOKING_UNCONFIRMED"})
	case errors.Is(err, agenda.ErrUnavailable):
		writeLegacyError(w, err)
	default:
		WriteProblem(w, http.StatusInternalServerError, "Erro interno", "Não foi possível agendar.")
	}
}

func writeJoinError(w http.ResponseWriter, err error) {
	var denied models.JoinDenied
	if errors.As(err, &denied) {
		detail := "Ainda não é possível entrar nesta consulta."
		if denied.Reason == "JOIN_TOO_LATE" {
			detail = "Esta consulta já terminou."
		}
		WriteProblemReason(w, http.StatusConflict, "Fora da janela de entrada", detail,
			Reason{Code: denied.Reason, Detail: denied.OpensAt.Format(time.RFC3339)})
		return
	}
	writeNotFound(w)
}

// ---------------------------------------------------------------------------
// DTOs
// ---------------------------------------------------------------------------

const dayLayout = "2006-01-02"

type listDTO[T any] struct {
	Items []T `json:"items"`
}

type specialtyDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type licenseDTO struct {
	Council string  `json:"council"`
	Number  string  `json:"number"`
	Region  string  `json:"region"`
	RQE     *string `json:"rqe"`
}

type professionalDTO struct {
	ID       string     `json:"id"`
	FullName string     `json:"full_name"`
	ImageURL *string    `json:"image_url"`
	License  licenseDTO `json:"license"`
}

type slotDTO struct {
	ID       string `json:"id"`
	StartsAt string `json:"starts_at"`
	EndsAt   string `json:"ends_at"`
	TimeZone string `json:"time_zone"`
}

type slotPageDTO struct {
	From  string    `json:"from"`
	To    string    `json:"to"`
	Items []slotDTO `json:"items"`
}

type joinWindowDTO struct {
	Status   string  `json:"status"`
	OpensAt  string  `json:"opens_at"`
	ClosesAt string  `json:"closes_at"`
	Reason   *Reason `json:"reason,omitempty"`
}

// apptProfessionalDTO é mais enxuto que o professionalDTO de propósito: a
// consulta guarda uma FOTOGRAFIA do legado (nome), não o registro no conselho.
// Reusar o professionalDTO aqui devolvia `council: ""` e a tela mostraria
// "CRP/ " — campo obrigatório vazio é pior que campo ausente. Foi assim que este
// bug apareceu: rodando a API de verdade, não nos testes.
type apptProfessionalDTO struct {
	ID       string `json:"id"`
	FullName string `json:"full_name"`
}

type appointmentDTO struct {
	ID           string              `json:"id"`
	Status       string              `json:"status"`
	StartsAt     string              `json:"starts_at"`
	EndsAt       string              `json:"ends_at"`
	TimeZone     string              `json:"time_zone"`
	Specialty    specialtyDTO        `json:"specialty"`
	Professional apptProfessionalDTO `json:"professional"`
	Join         joinWindowDTO       `json:"join"`
	CreatedAt    string              `json:"created_at"`
}

type joinTicketDTO struct {
	URL string `json:"url"`
}

func toProfessionalDTO(p agenda.Professional) professionalDTO {
	return professionalDTO{
		ID:       p.ID,
		FullName: p.FullName,
		ImageURL: nilIfEmpty(p.ImageURL),
		License: licenseDTO{
			Council: p.LicenseCouncil,
			Number:  p.LicenseNumber,
			Region:  p.LicenseRegion,
			RQE:     nilIfEmpty(p.RQE),
		},
	}
}

// toAppointmentDTO. Repare no que NÃO está aqui: o link da sala, o id da consulta
// na DAV e o id do slot no legado. Id de terceiro não vaza, e o link só sai do
// /join.
func (c SchedulingController) toAppointmentDTO(a models.Appointment) appointmentDTO {
	var reason *Reason
	if a.Join.Reason != "" {
		reason = &Reason{Code: a.Join.Reason}
	}
	return appointmentDTO{
		ID:       a.ID,
		Status:   a.Status,
		StartsAt: a.StartsAt.Format(time.RFC3339),
		EndsAt:   a.EndsAt.Format(time.RFC3339),
		TimeZone: a.TimeZone,
		Specialty: specialtyDTO{
			ID:   a.SpecialtyID,
			Name: a.SpecialtyName,
		},
		Professional: apptProfessionalDTO{
			ID:       a.ProfessionalID,
			FullName: a.ProfessionalName,
		},
		Join: joinWindowDTO{
			Status:   string(a.Join.Status),
			OpensAt:  a.Join.OpensAt.Format(time.RFC3339),
			ClosesAt: a.Join.ClosesAt.Format(time.RFC3339),
			Reason:   reason,
		},
		CreatedAt: a.CreatedAt.Format(time.RFC3339),
	}
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func startOfDay(t time.Time, loc *time.Location) time.Time {
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

// parseDay lê uma data no fuso da AGENDA. Sem o loc explícito, "2026-07-20"
// viraria meia-noite UTC — 21:00 do dia anterior em São Paulo — e o primeiro dia
// do intervalo sairia errado.
func parseDay(raw string, loc *time.Location, def time.Time) (time.Time, error) {
	if raw == "" {
		return def, nil
	}
	return time.ParseInLocation(dayLayout, raw, loc)
}
