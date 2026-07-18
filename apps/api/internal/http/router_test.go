package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/controllers"
	"github.com/renovisaude/renovi-care/internal/models"
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
