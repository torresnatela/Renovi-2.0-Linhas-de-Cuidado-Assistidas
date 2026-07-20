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

// fakeCheckins é um MoodCheckinService controlável.
type fakeCheckins struct {
	recorded  models.MoodCheckin
	recordErr error
	today     models.MoodToday
	todayErr  error
	history   []models.MoodCheckin
	help      models.HelpChannel
}

func (f *fakeCheckins) Record(context.Context, uuid.UUID, models.MoodCheckinInput, time.Time) (models.MoodCheckin, error) {
	return f.recorded, f.recordErr
}
func (f *fakeCheckins) Today(context.Context, uuid.UUID, time.Time) (models.MoodToday, error) {
	return f.today, f.todayErr
}
func (f *fakeCheckins) History(context.Context, uuid.UUID, int) ([]models.MoodCheckin, error) {
	return f.history, nil
}

func (f *fakeCheckins) HelpNow(context.Context, uuid.UUID, time.Time) (models.HelpChannel, error) {
	return f.help, nil
}

func serveCheckin(t *testing.T, f *fakeCheckins, method, alvo, corpo string) *httptest.ResponseRecorder {
	t.Helper()
	c := controllers.MoodController{Checkins: f, Now: func() time.Time { return time.Unix(0, 0).UTC() }}
	r := chi.NewRouter()
	r.Use(controllers.RequireSession(&fakeSessions{validated: models.Account{ID: uuid.New(), FullName: "Maria"}}))
	r.Post("/me/mood/checkin", c.RecordCheckin)
	r.Get("/me/mood/today", c.GetToday)
	r.Get("/me/mood/history", c.GetHistory)
	r.Post("/me/mood/help-now", c.HelpNow)

	req := httptest.NewRequest(method, alvo, strings.NewReader(corpo))
	req.AddCookie(&http.Cookie{Name: controllers.SessionCookieName, Value: "t"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestCheckin_Record_OK(t *testing.T) {
	f := &fakeCheckins{recorded: models.MoodCheckin{Valencia: 20, Energia: 20, Quadrante: "desagradavel_calmo"}}
	w := serveCheckin(t, f, http.MethodPost, "/me/mood/checkin", `{"valencia":20,"energia":20}`)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "desagradavel_calmo", body["quadrante"])
}

func TestCheckin_Record_SemConsentimento_403(t *testing.T) {
	f := &fakeCheckins{recordErr: models.ErrNoActiveConsent}
	w := serveCheckin(t, f, http.MethodPost, "/me/mood/checkin", `{"valencia":20,"energia":20}`)

	require.Equal(t, http.StatusForbidden, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	reason := body["reason"].(map[string]any)
	require.Equal(t, "consent_required", reason["code"])
}

func TestCheckin_Record_SemMatricula_403(t *testing.T) {
	f := &fakeCheckins{recordErr: models.ErrNotEnrolledInActivity}
	w := serveCheckin(t, f, http.MethodPost, "/me/mood/checkin", `{"valencia":20,"energia":20}`)

	require.Equal(t, http.StatusForbidden, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	reason := body["reason"].(map[string]any)
	require.Equal(t, "not_enrolled", reason["code"])
}

func TestCheckin_Record_Invalido_400(t *testing.T) {
	f := &fakeCheckins{recordErr: models.ErrMoodCheckinInvalid}
	w := serveCheckin(t, f, http.MethodPost, "/me/mood/checkin", `{"valencia":200,"energia":20}`)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCheckin_HelpNow_OK(t *testing.T) {
	f := &fakeCheckins{help: models.HelpChannel{Type: "care_navigation", Label: "Falar agora", Message: "estamos com você"}}
	w := serveCheckin(t, f, http.MethodPost, "/me/mood/help-now", "")

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "care_navigation", body["type"])
}

func TestCheckin_Today_OK(t *testing.T) {
	f := &fakeCheckins{today: models.MoodToday{Dia: time.Unix(0, 0).UTC(), CanCheckin: false, Reason: "consent_required"}}
	w := serveCheckin(t, f, http.MethodGet, "/me/mood/today", "")

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, false, body["can_checkin"])
	require.Equal(t, "consent_required", body["reason"])
}
