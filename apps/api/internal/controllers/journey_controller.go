package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/renovisaude/renovi-care/internal/adapters/agenda"
	"github.com/renovisaude/renovi-care/internal/http/api"
	"github.com/renovisaude/renovi-care/internal/models"
	"github.com/renovisaude/renovi-care/internal/models/careline"
)

// JourneyService é o que o controller precisa da jornada (interface no
// consumidor, ADR-012). Implementada por *models.JourneyStore.
type JourneyService interface {
	Journey(ctx context.Context, account models.Account, now time.Time) ([]models.JourneyEnrollment, error)
	Eligibility(ctx context.Context, account models.Account, itemID uuid.UUID, date *time.Time, now time.Time) (careline.Eligibility, error)
	Availability(ctx context.Context, account models.Account, itemID uuid.UUID, from, to *time.Time, now time.Time) (models.CareAvailability, error)
	Schedule(ctx context.Context, in models.ScheduleInput) (models.CareAppointment, bool, error)
	CancelCare(ctx context.Context, account models.Account, careApptID uuid.UUID, now time.Time) (models.CareAppointment, error)
	ListCare(ctx context.Context, account models.Account, status *string) ([]models.CareAppointment, error)
	Audit(ctx context.Context, account models.Account, cursor *string, limit int) (models.CareAuditPage, error)
	Location() *time.Location
}

// JourneyController expõe as rotas /me/* da jornada do paciente.
type JourneyController struct {
	Journeys JourneyService
	// Now é injetável para o teste não depender do relógio da máquina.
	Now func() time.Time
	// BookDeadline é quanto o handler de agendamento pode escrever — o mesmo
	// racional (e a mesma fonte na config) do SchedulingController: a rota fala
	// com a DAV de forma síncrona. Zero cai num default seguro.
	BookDeadline time.Duration
}

func (c JourneyController) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// ---------------------------------------------------------------------------
// Leituras
// ---------------------------------------------------------------------------

func (c JourneyController) GetJourney(w http.ResponseWriter, r *http.Request) {
	account, ok := AccountFrom(r.Context())
	if !ok {
		WriteProblem(w, http.StatusUnauthorized, "Não autenticado", "Sessão ausente ou expirada.")
		return
	}
	enrollments, err := c.Journeys.Journey(r.Context(), account, c.now())
	if err != nil {
		WriteProblem(w, http.StatusInternalServerError, "Erro interno", "Não foi possível montar sua jornada.")
		return
	}
	WriteJSON(w, http.StatusOK, toAPIJourney(enrollments))
}

func (c JourneyController) GetEligibility(w http.ResponseWriter, r *http.Request) {
	account, ok := AccountFrom(r.Context())
	if !ok {
		WriteProblem(w, http.StatusUnauthorized, "Não autenticado", "Sessão ausente ou expirada.")
		return
	}
	itemID, ok := queryItemID(w, r)
	if !ok {
		return
	}
	var date *time.Time
	if raw := r.URL.Query().Get("date"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			WriteProblem(w, http.StatusBadRequest, "Requisição inválida", "O parâmetro `date` deve ser um instante RFC 3339.")
			return
		}
		date = &parsed
	}

	elig, err := c.Journeys.Eligibility(r.Context(), account, itemID, date, c.now())
	if err != nil {
		writeJourneyError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, toAPIEligibility(elig))
}

func (c JourneyController) GetAvailability(w http.ResponseWriter, r *http.Request) {
	account, ok := AccountFrom(r.Context())
	if !ok {
		WriteProblem(w, http.StatusUnauthorized, "Não autenticado", "Sessão ausente ou expirada.")
		return
	}
	itemID, ok := queryItemID(w, r)
	if !ok {
		return
	}

	// As datas são lidas no fuso da AGENDA (mesma razão do /slots): quem sabe o
	// que é "hoje" é o servidor. Os defaults ficam no model, que ecoa o efetivo.
	loc := c.Journeys.Location()
	from, ok := queryDay(w, r, "from", loc)
	if !ok {
		return
	}
	to, ok := queryDay(w, r, "to", loc)
	if !ok {
		return
	}

	page, err := c.Journeys.Availability(r.Context(), account, itemID, from, to, c.now())
	if err != nil {
		writeJourneyError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, toAPIAvailabilityPage(page))
}

func (c JourneyController) ListCareAppointments(w http.ResponseWriter, r *http.Request) {
	account, ok := AccountFrom(r.Context())
	if !ok {
		WriteProblem(w, http.StatusUnauthorized, "Não autenticado", "Sessão ausente ou expirada.")
		return
	}
	var status *string
	if raw := r.URL.Query().Get("status"); raw != "" {
		if !api.ListMyCareAppointmentsParamsStatus(raw).Valid() {
			WriteProblem(w, http.StatusBadRequest, "Requisição inválida",
				"status deve ser um de: agendada, confirmada, em_andamento, realizada, falta, cancelada.")
			return
		}
		status = &raw
	}

	items, err := c.Journeys.ListCare(r.Context(), account, status)
	if err != nil {
		WriteProblem(w, http.StatusInternalServerError, "Erro interno", "Não foi possível listar suas consultas.")
		return
	}
	out := make([]api.CareAppointment, 0, len(items))
	for _, a := range items {
		out = append(out, toAPICareAppointment(a))
	}
	WriteJSON(w, http.StatusOK, api.CareAppointmentList{Items: out})
}

func (c JourneyController) GetAudit(w http.ResponseWriter, r *http.Request) {
	account, ok := AccountFrom(r.Context())
	if !ok {
		WriteProblem(w, http.StatusUnauthorized, "Não autenticado", "Sessão ausente ou expirada.")
		return
	}
	var cursor *string
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		cursor = &raw
	}
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			WriteProblem(w, http.StatusBadRequest, "Requisição inválida", "O parâmetro `limit` deve ser um inteiro.")
			return
		}
		limit = n
	}

	page, err := c.Journeys.Audit(r.Context(), account, cursor, limit)
	if err != nil {
		writeJourneyError(w, err)
		return
	}
	events := make([]api.JourneyEvent, 0, len(page.Events))
	for _, ev := range page.Events {
		events = append(events, toAPIJourneyEvent(ev))
	}
	WriteJSON(w, http.StatusOK, api.AuditPage{Items: events, NextCursor: page.NextCursor})
}

// ---------------------------------------------------------------------------
// Escritas
// ---------------------------------------------------------------------------

func (c JourneyController) CreateCareAppointment(w http.ResponseWriter, r *http.Request) {
	account, ok := AccountFrom(r.Context())
	if !ok {
		WriteProblem(w, http.StatusUnauthorized, "Não autenticado", "Sessão ausente ou expirada.")
		return
	}

	// Mesma disciplina do POST /appointments: a DAV é lenta e imprevisível, e o
	// deadline de escrita precisa cobrir a chamada síncrona com folga.
	deadline := c.BookDeadline
	if deadline <= 0 {
		deadline = defaultBookDeadline
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(deadline))

	// A key é validada JÁ AQUI (além do model): recusar antes de ler o corpo é
	// mais barato e a mensagem sai com o reason certo.
	idemKey := r.Header.Get("Idempotency-Key")
	if strings.TrimSpace(idemKey) == "" {
		writeIdemKeyRequired(w)
		return
	}

	var body api.CreateCareAppointmentRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.SlotId == "" || body.ItemId == uuid.Nil {
		WriteProblem(w, http.StatusBadRequest, "Requisição inválida", "Informe o item e o horário.")
		return
	}

	appt, replayed, err := c.Journeys.Schedule(r.Context(), models.ScheduleInput{
		Account: account, ItemID: body.ItemId, SlotID: body.SlotId,
		IdemKey: idemKey, Now: c.now(),
	})
	if err != nil {
		writeJourneyError(w, err)
		return
	}
	status := http.StatusCreated
	if replayed {
		// Replay da mesma key: a MESMA consulta, sem criar outra — 200, não 201.
		status = http.StatusOK
	}
	WriteJSON(w, status, toAPICareAppointment(appt))
}

func (c JourneyController) CancelCareAppointment(w http.ResponseWriter, r *http.Request) {
	account, ok := AccountFrom(r.Context())
	if !ok {
		WriteProblem(w, http.StatusUnauthorized, "Não autenticado", "Sessão ausente ou expirada.")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "care_appointment_id"))
	if err != nil {
		// Id malformado responde igual a id de terceiro: 404 (mesma razão do
		// scheduling — um 400 diria "este formato existe").
		WriteProblem(w, http.StatusNotFound, "Não encontrado", "Consulta não encontrada.")
		return
	}

	appt, err := c.Journeys.CancelCare(r.Context(), account, id, c.now())
	if err != nil {
		writeJourneyError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, toAPICareAppointment(appt))
}

// ---------------------------------------------------------------------------
// Erros
// ---------------------------------------------------------------------------

func writeIdemKeyRequired(w http.ResponseWriter) {
	WriteProblemReason(w, http.StatusBadRequest, "Requisição inválida",
		"Envie o header Idempotency-Key (um UUID novo por tentativa).",
		Reason{Code: "IDEMPOTENCY_KEY_REQUIRED"})
}

// writeJourneyError traduz os erros da jornada. O que não é dela cai no
// writeBookError — o mapeamento do booking (slot tomado 409, unconfirmed 502,
// legado 503 etc.) vale IGUAL aqui, e duplicá-lo divergiria na primeira mudança.
func writeJourneyError(w http.ResponseWriter, err error) {
	var notEligible models.ErrNotEligible
	switch {
	case errors.As(err, &notEligible):
		// O horário existe e está livre — quem não pode é ESTE paciente agendar
		// ESTE item agora. 422 com a lista inteira de motivos.
		WriteProblemFull(w, Problem{
			Title:  "Agendamento bloqueado",
			Status: http.StatusUnprocessableEntity,
			Detail: "As regras da sua linha de cuidado não permitem agendar este horário.",
			Reason: &Reason{Code: "ELIGIBILITY_BLOCKED"},
			Blocks: toProblemBlocks(notEligible.Blocks),
		})
	case errors.Is(err, models.ErrIdemKeyRequired):
		writeIdemKeyRequired(w)
	case errors.Is(err, models.ErrIdemKeyReused):
		// A key já agendou OUTRO item nesta matrícula: reúso indevido, não replay.
		WriteProblemReason(w, http.StatusUnprocessableEntity, "Idempotency-Key reutilizada",
			"Esta Idempotency-Key já foi usada para agendar outro item. Gere uma key nova para este agendamento.",
			Reason{Code: "IDEMPOTENCY_KEY_REUSE"})
	case errors.Is(err, models.ErrItemNotFound):
		WriteProblem(w, http.StatusNotFound, "Não encontrado", "Item da linha de cuidado não encontrado.")
	case errors.Is(err, models.ErrCareAppointmentNotFound):
		WriteProblem(w, http.StatusNotFound, "Não encontrado", "Consulta não encontrada.")
	case errors.Is(err, models.ErrSpecialtyNotFound):
		WriteProblem(w, http.StatusNotFound, "Não encontrado",
			"A especialidade deste item não está mais disponível no catálogo.")
	case errors.Is(err, models.ErrCareCancelNotAllowed):
		WriteProblemReason(w, http.StatusConflict, "Não é possível cancelar",
			"Esta consulta não está num estado cancelável.",
			Reason{Code: "CANCEL_NOT_ALLOWED"})
	case errors.Is(err, models.ErrBadCursor):
		WriteProblemReason(w, http.StatusBadRequest, "Requisição inválida",
			"O cursor informado é inválido. Recomece sem cursor.",
			Reason{Code: "AUDIT_CURSOR_INVALID"})
	case errors.Is(err, models.ErrBadDateRange):
		WriteProblem(w, http.StatusBadRequest, "Requisição inválida",
			"Intervalo de datas inválido (fim antes do início, ou janela acima de 60 dias).")
	case errors.Is(err, agenda.ErrUnavailable):
		WriteProblemReason(w, http.StatusServiceUnavailable, "Agenda indisponível",
			"Não conseguimos consultar a agenda agora. Tente novamente em instantes.",
			Reason{Code: "LEGACY_UNAVAILABLE"})
	case isBookingError(err):
		// Só os sentinelas do BOOKING delegam ao mapeamento do scheduling: o
		// default de lá diz "Não foi possível agendar.", que mentiria num GET de
		// elegibilidade ou de auditoria que falhou.
		writeBookError(w, err)
	default:
		WriteProblem(w, http.StatusInternalServerError, "Erro interno",
			"Não foi possível processar a solicitação.")
	}
}

// isBookingError reconhece os erros que o writeBookError sabe mapear — a lista
// dos sentinelas do módulo de booking que o Schedule deixa subir intactos.
func isBookingError(err error) bool {
	return errors.Is(err, models.ErrSlotTaken) ||
		errors.Is(err, models.ErrSlotExpired) ||
		errors.Is(err, models.ErrSlotNotFound) ||
		errors.Is(err, models.ErrSpecialtyMismatch) ||
		errors.Is(err, models.ErrSpecialtyInactive) ||
		errors.Is(err, models.ErrAccountNotLinked) ||
		errors.Is(err, models.ErrBookingRejected) ||
		errors.Is(err, models.ErrBookingUnconfirmed)
}

func toProblemBlocks(blocks []careline.Block) []ProblemBlock {
	out := make([]ProblemBlock, 0, len(blocks))
	for _, b := range blocks {
		out = append(out, ProblemBlock{RuleType: b.RuleType, Reason: b.Reason, AvailableFrom: b.AvailableFrom})
	}
	return out
}

// ---------------------------------------------------------------------------
// Parse de query params
// ---------------------------------------------------------------------------

// queryItemID lê o item_id obrigatório. Malformado é 400 — é um parâmetro de
// query documentado como UUID, não um recurso endereçado por path.
func queryItemID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.URL.Query().Get("item_id"))
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "Requisição inválida", "Informe o `item_id` (UUID) do item da linha.")
		return uuid.UUID{}, false
	}
	return id, true
}

// queryDay lê uma data opcional (AAAA-MM-DD) no fuso da agenda. Ausente = nil.
func queryDay(w http.ResponseWriter, r *http.Request, name string, loc *time.Location) (*time.Time, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return nil, true
	}
	parsed, err := time.ParseInLocation(dayLayout, raw, loc)
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "Requisição inválida",
			"O parâmetro `"+name+"` deve ser uma data (AAAA-MM-DD).")
		return nil, false
	}
	return &parsed, true
}

// ---------------------------------------------------------------------------
// Mapeamento domínio -> api (tipos gerados, fiéis ao contrato)
// ---------------------------------------------------------------------------

func toAPIJourney(enrollments []models.JourneyEnrollment) api.Journey {
	out := make([]api.JourneyEnrollment, 0, len(enrollments))
	for _, je := range enrollments {
		items := make([]api.JourneyItem, 0, len(je.Items))
		for _, it := range je.Items {
			items = append(items, api.JourneyItem{
				Item:        toAPICareLineItem(it.Item),
				Eligibility: toAPIEligibility(it.Eligibility),
			})
		}
		events := make([]api.JourneyEvent, 0, len(je.RecentEvents))
		for _, ev := range je.RecentEvents {
			events = append(events, toAPIJourneyEvent(ev))
		}
		out = append(out, api.JourneyEnrollment{
			CareLineName: je.CareLineName,
			Enrollment:   toAPIEnrollment(je.Enrollment),
			Items:        items,
			RecentEvents: events,
		})
	}
	return api.Journey{Enrollments: out}
}

func toAPIEligibility(e careline.Eligibility) api.Eligibility {
	blocks := make([]api.EligibilityBlock, 0, len(e.Blocks))
	for _, b := range e.Blocks {
		blocks = append(blocks, api.EligibilityBlock{
			RuleType:      api.EligibilityBlockRuleType(b.RuleType),
			Reason:        b.Reason,
			AvailableFrom: b.AvailableFrom,
		})
	}
	return api.Eligibility{Allowed: e.Allowed, Blocks: blocks}
}

func toAPICareAppointment(a models.CareAppointment) api.CareAppointment {
	return api.CareAppointment{
		Id:          a.ID,
		BookingId:   a.BookingID,
		ItemRef:     a.ItemRef,
		Label:       a.Label,
		ScheduledAt: a.ScheduledAt,
		Status:      api.CareAppointmentStatus(a.Status),
		CancelledAt: a.CancelledAt,
		TimeZone:    a.TimeZone,
	}
}

func toAPIJourneyEvent(ev models.JourneyEvent) api.JourneyEvent {
	payload := map[string]interface{}{}
	if len(ev.Payload) > 0 {
		_ = json.Unmarshal(ev.Payload, &payload)
	}
	return api.JourneyEvent{
		Id:         ev.ID,
		EventType:  api.JourneyEventEventType(ev.EventType),
		Actor:      api.JourneyEventActor(ev.Actor),
		OccurredAt: ev.OccurredAt,
		Payload:    payload,
	}
}

func toAPIAvailabilityPage(p models.CareAvailability) api.AvailabilityPage {
	items := make([]api.AnnotatedSlot, 0, len(p.Slots))
	for _, s := range p.Slots {
		items = append(items, api.AnnotatedSlot{
			Id:       s.Slot.ID,
			StartsAt: s.Slot.StartsAt,
			EndsAt:   s.Slot.EndsAt,
			TimeZone: p.TimeZone,
			Professional: api.AppointmentProfessional{
				Id:       s.Professional.ID,
				FullName: s.Professional.FullName,
			},
			Eligibility: toAPIEligibility(s.Eligibility),
		})
	}
	return api.AvailabilityPage{
		ItemId: p.ItemID,
		From:   openapi_types.Date{Time: p.From},
		To:     openapi_types.Date{Time: p.To},
		Items:  items,
	}
}
