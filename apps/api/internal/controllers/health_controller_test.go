package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/http/api"
)

func TestHealthController_Live(t *testing.T) {
	c := HealthController{Version: "test"}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	c.Live(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var body api.HealthStatus
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, api.Ok, body.Status)
	assert.Equal(t, "renovi-care", body.Service)
	assert.Equal(t, "test", body.Version)
}

func TestHealthController_Readyz_OK(t *testing.T) {
	c := HealthController{
		Version: "test",
		Ready:   func(ctx context.Context) error { return nil },
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)

	c.Readyz(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHealthController_Readyz_DependencyDown(t *testing.T) {
	c := HealthController{
		Version: "test",
		Ready:   func(ctx context.Context) error { return errors.New("secret-host:5432 user=renovi") },
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)

	c.Readyz(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), "not ready")
	// O corpo NÃO pode vazar detalhes internos da dependência (host/usuário).
	assert.NotContains(t, rec.Body.String(), "secret-host")
	assert.NotContains(t, rec.Body.String(), "user=renovi")
}
