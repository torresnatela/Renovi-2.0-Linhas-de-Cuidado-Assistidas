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
	"github.com/renovisaude/renovi-care/internal/models/careline"
)

type fakeAssessments struct {
	avail     models.AssessmentAvailability
	availErr  error
	result    models.AssessmentResult
	submitErr error
}

func (f *fakeAssessments) Availability(context.Context, uuid.UUID, string, time.Time) (models.AssessmentAvailability, error) {
	return f.avail, f.availErr
}
func (f *fakeAssessments) Submit(context.Context, uuid.UUID, string, []int, time.Time) (models.AssessmentResult, error) {
	return f.result, f.submitErr
}

func serveAssessment(t *testing.T, f *fakeAssessments, method, alvo, corpo string) *httptest.ResponseRecorder {
	t.Helper()
	c := controllers.AssessmentController{Assessments: f, Now: func() time.Time { return time.Unix(0, 0).UTC() }}
	r := chi.NewRouter()
	r.Use(controllers.RequireSession(&fakeSessions{validated: models.Account{ID: uuid.New(), FullName: "Maria"}}))
	r.Get("/me/assessments/{codigo}", c.GetAvailability)
	r.Post("/me/assessments", c.Submit)

	req := httptest.NewRequest(method, alvo, strings.NewReader(corpo))
	req.AddCookie(&http.Cookie{Name: controllers.SessionCookieName, Value: "t"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestAssessment_Availability_OK(t *testing.T) {
	f := &fakeAssessments{avail: models.AssessmentAvailability{
		Codigo: "WHO5", Eligibility: careline.Eligibility{Allowed: true}, ItemCount: 5, ValueMin: 0, ValueMax: 5,
	}}
	w := serveAssessment(t, f, http.MethodGet, "/me/assessments/WHO5", "")

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, float64(5), body["item_count"])
	require.Equal(t, true, body["eligibility"].(map[string]any)["allowed"])
}

func TestAssessment_Submit_OK(t *testing.T) {
	idx := 100.0
	f := &fakeAssessments{result: models.AssessmentResult{Codigo: "WHO5", Faixa: "normal", IndexScore: &idx}}
	w := serveAssessment(t, f, http.MethodPost, "/me/assessments", `{"codigo":"WHO5","items":[5,5,5,5,5]}`)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "normal", body["faixa"])
}

func TestAssessment_Submit_Bloqueado_409ComBlocks(t *testing.T) {
	af := time.Unix(1000, 0).UTC()
	f := &fakeAssessments{submitErr: models.ErrAssessmentBlocked{Blocks: []careline.Block{
		{RuleType: careline.RuleMinInterval, Reason: "Muito cedo", AvailableFrom: &af},
	}}}
	w := serveAssessment(t, f, http.MethodPost, "/me/assessments", `{"codigo":"WHO5","items":[3,3,3,3,3]}`)

	require.Equal(t, http.StatusConflict, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	blocks := body["blocks"].([]any)
	require.Len(t, blocks, 1)
	require.Equal(t, "MIN_INTERVAL", blocks[0].(map[string]any)["rule_type"])
}

func TestAssessment_Submit_SemConsentimento_403(t *testing.T) {
	f := &fakeAssessments{submitErr: models.ErrNoActiveConsent}
	w := serveAssessment(t, f, http.MethodPost, "/me/assessments", `{"codigo":"WHO5","items":[0,0,0,0,0]}`)

	require.Equal(t, http.StatusForbidden, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "consent_required", body["reason"].(map[string]any)["code"])
}
