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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/controllers"
	"github.com/renovisaude/renovi-care/internal/http/api"
	"github.com/renovisaude/renovi-care/internal/models"
)

type fakeOnboarding struct {
	info        models.OnboardingInfo
	infoErr     error
	completeAcc models.Account
	completeErr error
	declineErr  error
	gotToken    string
	gotInput    models.RegisterInput
}

func (f *fakeOnboarding) Info(_ context.Context, token string) (models.OnboardingInfo, error) {
	f.gotToken = token
	return f.info, f.infoErr
}

func (f *fakeOnboarding) Complete(_ context.Context, token string, in models.RegisterInput) (models.Account, error) {
	f.gotToken = token
	f.gotInput = in
	return f.completeAcc, f.completeErr
}

func (f *fakeOnboarding) Decline(_ context.Context, token string) error {
	f.gotToken = token
	return f.declineErr
}

func serveOnboarding(f *fakeOnboarding, s *fakeSessions) http.Handler {
	c := controllers.OnboardingController{
		Onboarding: f, Sessions: s, CookieSecure: true, SessionTTL: time.Hour,
	}
	r := chi.NewRouter()
	r.Get("/onboarding/{token}", c.GetInfo)
	r.Post("/onboarding/{token}/complete", c.Complete)
	r.Post("/onboarding/{token}/decline", c.Decline)
	return r
}

func doReq(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	h.ServeHTTP(w, req)
	return w
}

func TestOnboardingGetInfo_Ok(t *testing.T) {
	f := &fakeOnboarding{info: models.OnboardingInfo{
		InviteName: "Maria", InviteEmail: "m@e.test", InvitePhone: "11999999999",
		Companies: []string{"ACME", "Beta"},
	}}
	w := doReq(serveOnboarding(f, &fakeSessions{}), http.MethodGet, "/onboarding/tok-1", "")

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var out api.OnboardingInfo
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	assert.Equal(t, "Maria", out.InviteName)
	require.NotNil(t, out.InviteEmail)
	assert.Equal(t, "m@e.test", *out.InviteEmail)
	assert.Equal(t, []string{"ACME", "Beta"}, out.Companies)
	assert.Equal(t, "tok-1", f.gotToken, "o token da URL chega ao model")
}

func TestOnboardingGetInfo_NotFound(t *testing.T) {
	f := &fakeOnboarding{infoErr: models.ErrTokenNotFound}
	w := doReq(serveOnboarding(f, &fakeSessions{}), http.MethodGet, "/onboarding/x", "")
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "TOKEN_NOT_FOUND")
}

func TestOnboardingGetInfo_Expirado410(t *testing.T) {
	f := &fakeOnboarding{infoErr: models.ErrTokenExpired}
	w := doReq(serveOnboarding(f, &fakeSessions{}), http.MethodGet, "/onboarding/x", "")
	assert.Equal(t, http.StatusGone, w.Code)
	assert.Contains(t, w.Body.String(), "TOKEN_EXPIRED")
}

func TestOnboardingComplete_201ComCookie(t *testing.T) {
	f := &fakeOnboarding{completeAcc: contaValida}
	s := &fakeSessions{token: "sess-opaca"}
	w := doReq(serveOnboarding(f, s), http.MethodPost, "/onboarding/tok-9/complete", corpoValido)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, "renovi_session", cookies[0].Name)
	assert.Equal(t, "sess-opaca", cookies[0].Value)
	assert.True(t, cookies[0].HttpOnly)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, contaValida.Email, body["email"])
	assert.Equal(t, "tok-9", f.gotToken)
	assert.Equal(t, "948.190.898-46", f.gotInput.CPF, "o CPF cru é repassado para a verificação por HMAC")
}

func TestOnboardingComplete_CPFMismatch400(t *testing.T) {
	f := &fakeOnboarding{completeErr: models.ErrCPFMismatch}
	w := doReq(serveOnboarding(f, &fakeSessions{}), http.MethodPost, "/onboarding/t/complete", corpoValido)
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "CPF_MISMATCH")
}

func TestOnboardingComplete_JaTemConta409(t *testing.T) {
	f := &fakeOnboarding{completeErr: models.ErrAlreadyHasAccount}
	w := doReq(serveOnboarding(f, &fakeSessions{}), http.MethodPost, "/onboarding/t/complete", corpoValido)
	require.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "ALREADY_HAS_ACCOUNT")
}

func TestOnboardingComplete_TokenExpirado410(t *testing.T) {
	f := &fakeOnboarding{completeErr: models.ErrTokenExpired}
	w := doReq(serveOnboarding(f, &fakeSessions{}), http.MethodPost, "/onboarding/t/complete", corpoValido)
	require.Equal(t, http.StatusGone, w.Code)
	assert.Contains(t, w.Body.String(), "TOKEN_EXPIRED")
}

func TestOnboardingComplete_DAVIndisponivel504(t *testing.T) {
	f := &fakeOnboarding{completeErr: models.ErrDAVUnavailable}
	w := doReq(serveOnboarding(f, &fakeSessions{}), http.MethodPost, "/onboarding/t/complete", corpoValido)
	assert.Equal(t, http.StatusGatewayTimeout, w.Code)
}

func TestOnboardingDecline_204(t *testing.T) {
	f := &fakeOnboarding{}
	w := doReq(serveOnboarding(f, &fakeSessions{}), http.MethodPost, "/onboarding/tok/decline", "")
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "tok", f.gotToken)
}

func TestOnboardingDecline_JaConcluido409(t *testing.T) {
	f := &fakeOnboarding{declineErr: models.ErrOnboardingAlreadyDone}
	w := doReq(serveOnboarding(f, &fakeSessions{}), http.MethodPost, "/onboarding/tok/decline", "")
	require.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "ONBOARDING_ALREADY_DONE")
}
