package controllers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/controllers"
	"github.com/renovisaude/renovi-care/internal/models"
	"github.com/renovisaude/renovi-care/internal/models/careline"
)

// fakeJourneys implementa a JourneyService que o CONTROLLER declara (ADR-012).
type fakeJourneys struct {
	journey    []models.JourneyEnrollment
	journeyErr error

	elig    careline.Eligibility
	eligErr error

	avail    models.CareAvailability
	availErr error

	scheduleAppt     models.CareAppointment
	scheduleReplayed bool
	scheduleErr      error
	scheduleCalls    []models.ScheduleInput

	cancelAppt models.CareAppointment
	cancelErr  error

	list    []models.CareAppointment
	listErr error

	audit    models.CareAuditPage
	auditErr error
}

func (f *fakeJourneys) Journey(context.Context, models.Account, time.Time) ([]models.JourneyEnrollment, error) {
	return f.journey, f.journeyErr
}

func (f *fakeJourneys) Eligibility(context.Context, models.Account, uuid.UUID, *time.Time, time.Time) (careline.Eligibility, error) {
	return f.elig, f.eligErr
}

func (f *fakeJourneys) Availability(context.Context, models.Account, uuid.UUID, *time.Time, *time.Time, time.Time) (models.CareAvailability, error) {
	return f.avail, f.availErr
}

func (f *fakeJourneys) Schedule(_ context.Context, in models.ScheduleInput) (models.CareAppointment, bool, error) {
	f.scheduleCalls = append(f.scheduleCalls, in)
	if f.scheduleErr != nil {
		return models.CareAppointment{}, false, f.scheduleErr
	}
	return f.scheduleAppt, f.scheduleReplayed, nil
}

func (f *fakeJourneys) CancelCare(context.Context, models.Account, uuid.UUID, time.Time) (models.CareAppointment, error) {
	return f.cancelAppt, f.cancelErr
}

func (f *fakeJourneys) ListCare(context.Context, models.Account, *string) ([]models.CareAppointment, error) {
	return f.list, f.listErr
}

func (f *fakeJourneys) Audit(context.Context, models.Account, *string, int) (models.CareAuditPage, error) {
	return f.audit, f.auditErr
}

func (f *fakeJourneys) Location() *time.Location { return spLoc }

// serveJourney monta as rotas /me/* atrás do RequireSession DE VERDADE (com o
// fake de sessões), como o serve do scheduling.
func serveJourney(t *testing.T, f *fakeJourneys, method, alvo, corpo string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	c := controllers.JourneyController{Journeys: f, Now: func() time.Time { return agoraFixo }}

	r := chi.NewRouter()
	r.Use(controllers.RequireSession(&fakeSessions{
		validated: models.Account{ID: uuid.New(), FullName: "Maria de Teste"},
	}))
	r.Get("/me/journey", c.GetJourney)
	r.Get("/me/eligibility", c.GetEligibility)
	r.Get("/me/availability", c.GetAvailability)
	r.Get("/me/appointments", c.ListCareAppointments)
	r.Post("/me/appointments", c.CreateCareAppointment)
	r.Post("/me/appointments/{care_appointment_id}/cancel", c.CancelCareAppointment)
	r.Get("/me/audit", c.GetAudit)

	req := httptest.NewRequest(method, alvo, strings.NewReader(corpo))
	req.AddCookie(&http.Cookie{Name: controllers.SessionCookieName, Value: "tok"})
	if corpo != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func careApptFixa() models.CareAppointment {
	return models.CareAppointment{
		ID: uuid.New(), EnrollmentID: uuid.New(), CareLineItemID: uuid.New(),
		ItemRef: "acomp", Label: "Acompanhamento", BookingID: uuid.New(),
		ScheduledAt: inicioFix, Status: "agendada", TimeZone: "America/Sao_Paulo",
	}
}

// ---------------------------------------------------------------------------
// POST /me/appointments
// ---------------------------------------------------------------------------

// 422 com problem+json e blocks[] no corpo: rule_type, reason e available_from
// (RFC 3339) — é o contrato que o front usa para explicar o bloqueio.
func TestJourneyController_Create_BloqueadoPeloMotor422(t *testing.T) {
	af := inicioFix.Add(72 * time.Hour)
	f := &fakeJourneys{scheduleErr: models.ErrNotEligible{Blocks: []careline.Block{
		{RuleType: careline.RuleQuota, Reason: "Você atingiu o limite de 1 consulta(s) por semana. Disponível a partir de 25/07/2026", AvailableFrom: &af},
		{RuleType: careline.RulePrerequisite, Reason: "Realize primeiro: Avaliação inicial"},
	}}}

	body := `{"item_id":"` + uuid.NewString() + `","slot_id":"slot-1"}`
	rec := serveJourney(t, f, http.MethodPost, "/me/appointments", body, map[string]string{"Idempotency-Key": "k-1"})

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "application/problem+json")

	var problem struct {
		Status int `json:"status"`
		Reason struct {
			Code string `json:"code"`
		} `json:"reason"`
		Blocks []struct {
			RuleType      string  `json:"rule_type"`
			Reason        string  `json:"reason"`
			AvailableFrom *string `json:"available_from"`
		} `json:"blocks"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
	assert.Equal(t, "ELIGIBILITY_BLOCKED", problem.Reason.Code)
	require.Len(t, problem.Blocks, 2, "TODOS os blocks viajam, não só o primeiro")

	assert.Equal(t, "QUOTA", problem.Blocks[0].RuleType)
	assert.NotEmpty(t, problem.Blocks[0].Reason)
	require.NotNil(t, problem.Blocks[0].AvailableFrom)
	parsed, err := time.Parse(time.RFC3339, *problem.Blocks[0].AvailableFrom)
	require.NoError(t, err, "available_from precisa ser RFC 3339")
	assert.True(t, parsed.Equal(af))

	assert.Equal(t, "PREREQUISITE", problem.Blocks[1].RuleType)
	assert.Nil(t, problem.Blocks[1].AvailableFrom, "pré-requisito depende de ação, não de data")
}

func TestJourneyController_Create_SemIdemKey400(t *testing.T) {
	f := &fakeJourneys{}
	body := `{"item_id":"` + uuid.NewString() + `","slot_id":"slot-1"}`
	rec := serveJourney(t, f, http.MethodPost, "/me/appointments", body, nil)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	var problem struct {
		Reason struct {
			Code string `json:"code"`
		} `json:"reason"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
	assert.Equal(t, "IDEMPOTENCY_KEY_REQUIRED", problem.Reason.Code)
	assert.Empty(t, f.scheduleCalls, "sem key o model nem é chamado")
}

func TestJourneyController_Create_FelizE201(t *testing.T) {
	appt := careApptFixa()
	f := &fakeJourneys{scheduleAppt: appt}
	itemID := uuid.NewString()
	body := `{"item_id":"` + itemID + `","slot_id":"slot-1"}`
	rec := serveJourney(t, f, http.MethodPost, "/me/appointments", body, map[string]string{"Idempotency-Key": "k-1"})

	require.Equal(t, http.StatusCreated, rec.Code)
	var got struct {
		ID        string `json:"id"`
		BookingID string `json:"booking_id"`
		ItemRef   string `json:"item_ref"`
		Label     string `json:"label"`
		Status    string `json:"status"`
		TimeZone  string `json:"time_zone"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, appt.ID.String(), got.ID)
	assert.Equal(t, appt.BookingID.String(), got.BookingID, "o booking_id é a ponte para as rotas /appointments")
	assert.Equal(t, "acomp", got.ItemRef)
	assert.Equal(t, "Acompanhamento", got.Label)
	assert.Equal(t, "agendada", got.Status)
	assert.Equal(t, "America/Sao_Paulo", got.TimeZone)

	require.Len(t, f.scheduleCalls, 1)
	assert.Equal(t, "k-1", f.scheduleCalls[0].IdemKey)
	assert.Equal(t, itemID, f.scheduleCalls[0].ItemID.String())
}

func TestJourneyController_Create_Replay200(t *testing.T) {
	f := &fakeJourneys{scheduleAppt: careApptFixa(), scheduleReplayed: true}
	body := `{"item_id":"` + uuid.NewString() + `","slot_id":"slot-1"}`
	rec := serveJourney(t, f, http.MethodPost, "/me/appointments", body, map[string]string{"Idempotency-Key": "k-1"})
	require.Equal(t, http.StatusOK, rec.Code, "replay da mesma key é 200, não 201")
}

// Os erros do booking valem IGUAL aqui: slot tomado é 409 com SLOT_TAKEN.
func TestJourneyController_Create_SlotTomado409(t *testing.T) {
	f := &fakeJourneys{scheduleErr: models.ErrSlotTaken}
	body := `{"item_id":"` + uuid.NewString() + `","slot_id":"slot-1"}`
	rec := serveJourney(t, f, http.MethodPost, "/me/appointments", body, map[string]string{"Idempotency-Key": "k-1"})

	require.Equal(t, http.StatusConflict, rec.Code)
	var problem struct {
		Reason struct {
			Code string `json:"code"`
		} `json:"reason"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
	assert.Equal(t, "SLOT_TAKEN", problem.Reason.Code)
}

func TestJourneyController_Create_Unconfirmed502(t *testing.T) {
	f := &fakeJourneys{scheduleErr: models.ErrBookingUnconfirmed}
	body := `{"item_id":"` + uuid.NewString() + `","slot_id":"slot-1"}`
	rec := serveJourney(t, f, http.MethodPost, "/me/appointments", body, map[string]string{"Idempotency-Key": "k-1"})
	require.Equal(t, http.StatusBadGateway, rec.Code)
}

func TestJourneyController_Create_ItemNaoEncontrado404(t *testing.T) {
	f := &fakeJourneys{scheduleErr: models.ErrItemNotFound}
	body := `{"item_id":"` + uuid.NewString() + `","slot_id":"slot-1"}`
	rec := serveJourney(t, f, http.MethodPost, "/me/appointments", body, map[string]string{"Idempotency-Key": "k-1"})
	require.Equal(t, http.StatusNotFound, rec.Code)
}

// ---------------------------------------------------------------------------
// POST /me/appointments/{id}/cancel
// ---------------------------------------------------------------------------

func TestJourneyController_Cancel_NaoCancelavel409(t *testing.T) {
	f := &fakeJourneys{cancelErr: models.ErrCareCancelNotAllowed}
	rec := serveJourney(t, f, http.MethodPost, "/me/appointments/"+uuid.NewString()+"/cancel", "", nil)

	require.Equal(t, http.StatusConflict, rec.Code)
	var problem struct {
		Reason struct {
			Code string `json:"code"`
		} `json:"reason"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
	assert.Equal(t, "CANCEL_NOT_ALLOWED", problem.Reason.Code)
}

func TestJourneyController_Cancel_NaoEncontrada404(t *testing.T) {
	f := &fakeJourneys{cancelErr: models.ErrCareAppointmentNotFound}
	rec := serveJourney(t, f, http.MethodPost, "/me/appointments/"+uuid.NewString()+"/cancel", "", nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestJourneyController_Cancel_IdMalformado404(t *testing.T) {
	f := &fakeJourneys{}
	rec := serveJourney(t, f, http.MethodPost, "/me/appointments/nao-e-uuid/cancel", "", nil)
	require.Equal(t, http.StatusNotFound, rec.Code, "id malformado responde igual a id de terceiro")
}

func TestJourneyController_Cancel_Feliz200(t *testing.T) {
	appt := careApptFixa()
	appt.Status = "cancelada"
	cancelled := agoraFixo
	appt.CancelledAt = &cancelled
	f := &fakeJourneys{cancelAppt: appt}
	rec := serveJourney(t, f, http.MethodPost, "/me/appointments/"+appt.ID.String()+"/cancel", "", nil)

	require.Equal(t, http.StatusOK, rec.Code)
	var got struct {
		Status      string  `json:"status"`
		CancelledAt *string `json:"cancelled_at"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "cancelada", got.Status)
	require.NotNil(t, got.CancelledAt)
}

// ---------------------------------------------------------------------------
// GET /me/audit
// ---------------------------------------------------------------------------

func TestJourneyController_Audit_CursorInvalido400(t *testing.T) {
	f := &fakeJourneys{auditErr: models.ErrBadCursor}
	rec := serveJourney(t, f, http.MethodGet, "/me/audit?cursor=lixo", "", nil)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	var problem struct {
		Reason struct {
			Code string `json:"code"`
		} `json:"reason"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
	assert.Equal(t, "AUDIT_CURSOR_INVALID", problem.Reason.Code)
}

func TestJourneyController_Audit_LimitNaoInteiro400(t *testing.T) {
	f := &fakeJourneys{}
	rec := serveJourney(t, f, http.MethodGet, "/me/audit?limit=abc", "", nil)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestJourneyController_Audit_Feliz200(t *testing.T) {
	next := "cursor-opaco"
	f := &fakeJourneys{audit: models.CareAuditPage{
		Events: []models.JourneyEvent{{
			ID: uuid.New(), EventType: "consulta_agendada", Actor: "paciente",
			OccurredAt: agoraFixo, Payload: json.RawMessage(`{"slot_id":"slot-1"}`),
		}},
		NextCursor: &next,
	}}
	rec := serveJourney(t, f, http.MethodGet, "/me/audit", "", nil)

	require.Equal(t, http.StatusOK, rec.Code)
	var got struct {
		Items []struct {
			EventType string         `json:"event_type"`
			Actor     string         `json:"actor"`
			Payload   map[string]any `json:"payload"`
		} `json:"items"`
		NextCursor *string `json:"next_cursor"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got.Items, 1)
	assert.Equal(t, "consulta_agendada", got.Items[0].EventType)
	assert.Equal(t, "paciente", got.Items[0].Actor)
	assert.Equal(t, "slot-1", got.Items[0].Payload["slot_id"])
	require.NotNil(t, got.NextCursor)
	assert.Equal(t, "cursor-opaco", *got.NextCursor)
}

// ---------------------------------------------------------------------------
// GET /me/eligibility e /me/availability
// ---------------------------------------------------------------------------

func TestJourneyController_Eligibility_ItemIDInvalido400(t *testing.T) {
	f := &fakeJourneys{}
	rec := serveJourney(t, f, http.MethodGet, "/me/eligibility?item_id=nao-e-uuid", "", nil)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestJourneyController_Eligibility_ItemNaoEncontrado404(t *testing.T) {
	f := &fakeJourneys{eligErr: models.ErrItemNotFound}
	rec := serveJourney(t, f, http.MethodGet, "/me/eligibility?item_id="+uuid.NewString(), "", nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestJourneyController_Eligibility_DateInvalida400(t *testing.T) {
	f := &fakeJourneys{}
	rec := serveJourney(t, f, http.MethodGet, "/me/eligibility?item_id="+uuid.NewString()+"&date=ontem", "", nil)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

// Falha genérica em rota que NÃO é de agendamento: o 500 default precisa de
// frase neutra — "Não foi possível agendar." num GET de elegibilidade mentiria.
func TestJourneyController_ErroGenerico_500Neutro(t *testing.T) {
	boom := errors.New("banco caiu")
	cases := []struct {
		nome string
		f    *fakeJourneys
		alvo string
	}{
		{"eligibility", &fakeJourneys{eligErr: boom}, "/me/eligibility?item_id=" + uuid.NewString()},
		{"availability", &fakeJourneys{availErr: boom}, "/me/availability?item_id=" + uuid.NewString()},
		{"audit", &fakeJourneys{auditErr: boom}, "/me/audit"},
	}
	for _, tc := range cases {
		t.Run(tc.nome, func(t *testing.T) {
			rec := serveJourney(t, tc.f, http.MethodGet, tc.alvo, "", nil)
			require.Equal(t, http.StatusInternalServerError, rec.Code)
			var problem struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
			assert.Equal(t, "Erro interno", problem.Title)
			assert.Equal(t, "Não foi possível processar a solicitação.", problem.Detail)
			assert.NotContains(t, problem.Detail, "agendar", "a frase do booking não vaza para leituras")
		})
	}
}

func TestJourneyController_Eligibility_Feliz200(t *testing.T) {
	f := &fakeJourneys{elig: careline.Eligibility{Allowed: true, Blocks: []careline.Block{}}}
	rec := serveJourney(t, f, http.MethodGet, "/me/eligibility?item_id="+uuid.NewString(), "", nil)

	require.Equal(t, http.StatusOK, rec.Code)
	var got struct {
		Allowed bool  `json:"allowed"`
		Blocks  []any `json:"blocks"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.True(t, got.Allowed)
	assert.NotNil(t, got.Blocks, "blocks é lista vazia, não null")
}

func TestJourneyController_Availability_FromInvalido400(t *testing.T) {
	f := &fakeJourneys{}
	rec := serveJourney(t, f, http.MethodGet, "/me/availability?item_id="+uuid.NewString()+"&from=20-07-2026", "", nil)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestJourneyController_Availability_Feliz200(t *testing.T) {
	itemID := uuid.New()
	f := &fakeJourneys{avail: models.CareAvailability{
		ItemID:   itemID,
		From:     time.Date(2026, 7, 20, 0, 0, 0, 0, spLoc),
		To:       time.Date(2026, 8, 19, 0, 0, 0, 0, spLoc),
		TimeZone: "America/Sao_Paulo",
		Slots:    []models.AvailabilitySlot{},
	}}
	rec := serveJourney(t, f, http.MethodGet, "/me/availability?item_id="+itemID.String(), "", nil)

	require.Equal(t, http.StatusOK, rec.Code)
	var got struct {
		ItemID string `json:"item_id"`
		From   string `json:"from"`
		To     string `json:"to"`
		Items  []any  `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, itemID.String(), got.ItemID)
	assert.Equal(t, "2026-07-20", got.From, "from ecoado como data (AAAA-MM-DD)")
	assert.Equal(t, "2026-08-19", got.To)
	assert.NotNil(t, got.Items)
}

// ---------------------------------------------------------------------------
// GET /me/appointments
// ---------------------------------------------------------------------------

func TestJourneyController_List_StatusInvalido400(t *testing.T) {
	f := &fakeJourneys{}
	rec := serveJourney(t, f, http.MethodGet, "/me/appointments?status=marciana", "", nil)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestJourneyController_List_Feliz200(t *testing.T) {
	f := &fakeJourneys{list: []models.CareAppointment{careApptFixa()}}
	rec := serveJourney(t, f, http.MethodGet, "/me/appointments?status=agendada", "", nil)

	require.Equal(t, http.StatusOK, rec.Code)
	var got struct {
		Items []struct {
			ItemRef string `json:"item_ref"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got.Items, 1)
	assert.Equal(t, "acomp", got.Items[0].ItemRef)
}

// ---------------------------------------------------------------------------
// Sem sessão: tudo 401 (o RequireSession barra antes do handler)
// ---------------------------------------------------------------------------

func TestJourneyController_SemSessao401(t *testing.T) {
	c := controllers.JourneyController{Journeys: &fakeJourneys{}}
	r := chi.NewRouter()
	r.Use(controllers.RequireSession(&fakeSessions{validErr: models.ErrNoSession}))
	r.Get("/me/journey", c.GetJourney)

	req := httptest.NewRequest(http.MethodGet, "/me/journey", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}
