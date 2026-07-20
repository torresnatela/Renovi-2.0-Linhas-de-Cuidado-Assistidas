package controllers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/controllers"
	"github.com/renovisaude/renovi-care/internal/models"
)

// fakeConsents é um ConsentService controlável para os testes de HTTP.
type fakeConsents struct {
	active    models.Consent
	activeErr error
	granted   models.Consent
	grantErr  error
	revokeErr error

	grantCalls  int
	revokeCalls int
}

func (f *fakeConsents) Active(context.Context, uuid.UUID, string) (models.Consent, error) {
	return f.active, f.activeErr
}

func (f *fakeConsents) Grant(context.Context, uuid.UUID, string, string, *string, time.Time) (models.Consent, error) {
	f.grantCalls++
	return f.granted, f.grantErr
}

func (f *fakeConsents) Revoke(context.Context, uuid.UUID, string, time.Time) error {
	f.revokeCalls++
	return f.revokeErr
}

func serveConsent(t *testing.T, f *fakeConsents, method, alvo, corpo string) *httptest.ResponseRecorder {
	t.Helper()
	c := controllers.ConsentController{Consents: f, Now: func() time.Time { return time.Unix(0, 0).UTC() }}

	r := chi.NewRouter()
	// RequireSession de verdade com fake de sessões — o mesmo padrão do agendamento.
	r.Use(controllers.RequireSession(&fakeSessions{validated: models.Account{ID: uuid.New(), FullName: "Maria de Teste"}}))
	r.Get("/me/consent", c.GetConsent)
	r.Post("/me/consent", c.GrantConsent)
	r.Post("/me/consent/revoke", c.RevokeConsent)

	req := httptest.NewRequest(method, alvo, strings.NewReader(corpo))
	req.AddCookie(&http.Cookie{Name: controllers.SessionCookieName, Value: "token-de-teste"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestConsent_GetSemConsentimento_RetornaInativo(t *testing.T) {
	f := &fakeConsents{activeErr: models.ErrNoActiveConsent}
	w := serveConsent(t, f, http.MethodGet, "/me/consent?finalidade=checkin_humor", "")

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, false, body["active"])
	require.Equal(t, "checkin_humor", body["finalidade"])
}

func TestConsent_GetComConsentimento_RetornaAtivo(t *testing.T) {
	f := &fakeConsents{active: models.Consent{
		Finalidade: "checkin_humor", VersaoTermo: "v1", Status: "ativo",
		ConcedidoEm: time.Unix(100, 0).UTC(),
	}}
	w := serveConsent(t, f, http.MethodGet, "/me/consent", "")

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, true, body["active"])
	require.Equal(t, "v1", body["versao_termo"])
}

func TestConsent_Grant_OK(t *testing.T) {
	f := &fakeConsents{granted: models.Consent{
		Finalidade: "checkin_humor", VersaoTermo: "v1", Status: "ativo",
		ConcedidoEm: time.Unix(100, 0).UTC(),
	}}
	w := serveConsent(t, f, http.MethodPost, "/me/consent", `{"finalidade":"checkin_humor","versao_termo":"v1"}`)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 1, f.grantCalls)
}

func TestConsent_Grant_Invalido_400(t *testing.T) {
	f := &fakeConsents{grantErr: models.ErrConsentInvalid}
	w := serveConsent(t, f, http.MethodPost, "/me/consent", `{"finalidade":"xpto","versao_termo":"v1"}`)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestConsent_Revoke_OK(t *testing.T) {
	f := &fakeConsents{}
	w := serveConsent(t, f, http.MethodPost, "/me/consent/revoke", `{"finalidade":"checkin_humor"}`)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 1, f.revokeCalls)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, false, body["active"])
}
