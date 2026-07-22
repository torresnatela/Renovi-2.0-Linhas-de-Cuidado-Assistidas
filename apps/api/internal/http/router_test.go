package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/controllers"
	"github.com/renovisaude/renovi-care/internal/models"
	"github.com/renovisaude/renovi-care/internal/models/careline"
)

func TestRouter_Healthz(t *testing.T) {
	r := NewRouter(Deps{Version: "test"})
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestRouter_APIV1Healthz(t *testing.T) {
	r := NewRouter(Deps{Version: "test"})
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/healthz")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// fakeCareAdmin/fakeEnroll implementam o mínimo das interfaces do controller para o
// teste de fiação das rotas /admin (o mapeamento fino é testado no pacote controllers).
type fakeCareAdmin struct{}

func (fakeCareAdmin) Create(context.Context, string, string, string) (models.CareLine, error) {
	return models.CareLine{}, nil
}
func (fakeCareAdmin) AddItem(context.Context, uuid.UUID, models.AddItemInput) (models.CareLineItem, error) {
	return models.CareLineItem{}, nil
}
func (fakeCareAdmin) AddRule(context.Context, uuid.UUID, string, string, json.RawMessage) (models.CareLineRule, error) {
	return models.CareLineRule{}, nil
}
func (fakeCareAdmin) Publish(context.Context, uuid.UUID, time.Time) (models.CareLine, error) {
	return models.CareLine{}, nil
}
func (fakeCareAdmin) ListVersions(context.Context, string) ([]models.CareLine, error) {
	return []models.CareLine{}, nil
}

type fakeEnrollAdmin struct{}

func (fakeEnrollAdmin) Enroll(context.Context, uuid.UUID, string, int, time.Time) (models.Enrollment, error) {
	return models.Enrollment{}, nil
}
func (fakeEnrollAdmin) Renew(context.Context, uuid.UUID, int, time.Time) (models.Enrollment, error) {
	return models.Enrollment{}, nil
}
func (fakeEnrollAdmin) End(context.Context, uuid.UUID, string, string, time.Time) (models.Enrollment, error) {
	return models.Enrollment{}, nil
}

func adminRouter() *controllers.CareLineAdminController {
	return &controllers.CareLineAdminController{
		Catalog:     fakeCareAdmin{},
		Enrollments: fakeEnrollAdmin{},
	}
}

// Sem CareAdmin, as rotas /admin nem existem: 404, não 401.
func TestRouter_AdminDesligado_404(t *testing.T) {
	r := NewRouter(Deps{Version: "test"})
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/admin/care-lines")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// Com CareAdmin e o token certo, a rota responde 200.
func TestRouter_AdminLigado_ComToken_200(t *testing.T) {
	r := NewRouter(Deps{Version: "test", CareAdmin: adminRouter(), AdminToken: "s3cr3t"})
	srv := httptest.NewServer(r)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/admin/care-lines", nil)
	require.NoError(t, err)
	req.Header.Set("X-Admin-Token", "s3cr3t")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// Com CareAdmin mas sem o token, é 401 (o middleware barra antes do handler).
func TestRouter_AdminLigado_SemToken_401(t *testing.T) {
	r := NewRouter(Deps{Version: "test", CareAdmin: adminRouter(), AdminToken: "s3cr3t"})
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/admin/care-lines")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// fakeInternalJourneys é o mínimo para a fiação de /internal (o mapeamento fino
// é testado no pacote controllers).
type fakeInternalJourneys struct{}

func (fakeInternalJourneys) ForceStatus(context.Context, uuid.UUID, string, time.Time) (models.CareAppointment, error) {
	return models.CareAppointment{Status: "realizada"}, nil
}

// Por default (sem Internal), as rotas internas NEM EXISTEM: 404. É o
// comportamento de produção, onde a config proíbe RENOVI_TEST_ENDPOINTS.
func TestRouter_InternalDesligadoPorDefault_404(t *testing.T) {
	r := NewRouter(Deps{Version: "test"})
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Post(
		srv.URL+"/api/v1/internal/appointments/"+uuid.NewString()+"/force-status",
		"application/json", strings.NewReader(`{"status":"realizada"}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// Com Internal injetado (RENOVI_TEST_ENDPOINTS), a rota existe e responde SEM
// autenticação — o gate é o ambiente.
func TestRouter_InternalLigado_200(t *testing.T) {
	r := NewRouter(Deps{
		Version:  "test",
		Internal: &controllers.InternalController{Journeys: fakeInternalJourneys{}},
	})
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Post(
		srv.URL+"/api/v1/internal/appointments/"+uuid.NewString()+"/force-status",
		"application/json", strings.NewReader(`{"status":"realizada"}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// Sem Journey injetado, /me/journey não existe (404) — mesmo com Auth montado o
// grupo da jornada só sobe quando o main o construiu (exige o booking).
func TestRouter_JourneyDesligado_404(t *testing.T) {
	r := NewRouter(Deps{Version: "test"})
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/me/journey")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// fakeRouterSessions/fakeRouterJourneys são o mínimo para a fiação: /me (auth) e
// /me/journey (jornada) nascem em GROUPS diferentes do mesmo mux, e este teste
// prova que convivem na trie do chi.
type fakeRouterSessions struct{}

func (fakeRouterSessions) Create(context.Context, uuid.UUID) (string, time.Time, error) {
	return "tok", time.Now().Add(time.Hour), nil
}
func (fakeRouterSessions) Validate(context.Context, string) (models.Account, error) {
	return models.Account{ID: uuid.New(), FullName: "Maria"}, nil
}
func (fakeRouterSessions) Revoke(context.Context, string) error { return nil }

type fakeRouterJourneys struct{}

func (fakeRouterJourneys) Journey(context.Context, models.Account, time.Time) ([]models.JourneyEnrollment, error) {
	return []models.JourneyEnrollment{}, nil
}
func (fakeRouterJourneys) Eligibility(context.Context, models.Account, uuid.UUID, *time.Time, time.Time) (careline.Eligibility, error) {
	return careline.Eligibility{Allowed: true}, nil
}
func (fakeRouterJourneys) Availability(context.Context, models.Account, uuid.UUID, *time.Time, *time.Time, time.Time) (models.CareAvailability, error) {
	return models.CareAvailability{}, nil
}
func (fakeRouterJourneys) Schedule(context.Context, models.ScheduleInput) (models.CareAppointment, bool, error) {
	return models.CareAppointment{}, false, nil
}
func (fakeRouterJourneys) CancelCare(context.Context, models.Account, uuid.UUID, time.Time) (models.CareAppointment, error) {
	return models.CareAppointment{}, nil
}
func (fakeRouterJourneys) ListCare(context.Context, models.Account, *string) ([]models.CareAppointment, error) {
	return []models.CareAppointment{}, nil
}
func (fakeRouterJourneys) Audit(context.Context, models.Account, *string, int) (models.CareAuditPage, error) {
	return models.CareAuditPage{}, nil
}
func (fakeRouterJourneys) Location() *time.Location { return time.UTC }

func TestRouter_JourneyLigado_MeEJornadaConvivem(t *testing.T) {
	auth := &controllers.AuthController{Sessions: fakeRouterSessions{}}
	journey := &controllers.JourneyController{Journeys: fakeRouterJourneys{}}
	r := NewRouter(Deps{Version: "test", Auth: auth, Journey: journey})
	srv := httptest.NewServer(r)
	defer srv.Close()

	get := func(path string) int {
		req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		require.NoError(t, err)
		req.AddCookie(&http.Cookie{Name: controllers.SessionCookieName, Value: "tok"})
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		return resp.StatusCode
	}

	assert.Equal(t, http.StatusOK, get("/api/v1/me"), "/me (auth) continua vivo")
	assert.Equal(t, http.StatusOK, get("/api/v1/me/journey"), "/me/journey (jornada) montado junto")
	assert.Equal(t, http.StatusOK, get("/api/v1/me/audit"))
}

// --- Integração da Gestão (push, ADR-043) ---

type fakeIngestion struct{}

func (fakeIngestion) RecordContract(context.Context, models.ContractPush) (models.RecordResult, error) {
	return models.RecordResult{PersonStatus: "pendente", ContractStatus: "ativo"}, nil
}
func (fakeIngestion) ResendInvite(context.Context, []byte) (models.ResendResult, error) {
	return models.ResendResult{}, nil
}

func ingestionRouter() *controllers.IngestionController {
	return &controllers.IngestionController{Ingestion: fakeIngestion{}}
}

// Sem Ingestion, as rotas /integration nem existem: 404, não 401.
func TestRouter_IngestionDesligado_404(t *testing.T) {
	r := NewRouter(Deps{Version: "test"})
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/integration/gestao/contracts", "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// Com Ingestion mas sem o token, é 401 (o middleware barra antes do handler).
func TestRouter_IngestionLigado_SemToken_401(t *testing.T) {
	r := NewRouter(Deps{Version: "test", Ingestion: ingestionRouter(), GestaoIntegrationToken: "int-tok"})
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/integration/gestao/contracts", "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// Com Ingestion e o token certo, a rota chega ao handler e responde 200.
func TestRouter_IngestionLigado_ComToken_200(t *testing.T) {
	r := NewRouter(Deps{Version: "test", Ingestion: ingestionRouter(), GestaoIntegrationToken: "int-tok"})
	srv := httptest.NewServer(r)
	defer srv.Close()

	body := `{"contract_id":"C-1","status":"ativo","employee":{"id":"E-1","cpf_hmac":"` +
		strings.Repeat("0", 64) + `","name":"M"},"company":{"id":"CO","display_name":"ACME"}}`
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/integration/gestao/contracts", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Integration-Token", "int-tok")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
