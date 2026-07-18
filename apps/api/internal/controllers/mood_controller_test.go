package controllers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/controllers"
	"github.com/renovisaude/renovi-care/internal/models"
)

// fakeInstruments é um InstrumentService controlável.
type fakeInstruments struct {
	cfg models.InstrumentConfig
	err error
}

func (f *fakeInstruments) Config(context.Context, string) (models.InstrumentConfig, error) {
	return f.cfg, f.err
}

func serveMood(t *testing.T, f *fakeInstruments, alvo string) *httptest.ResponseRecorder {
	t.Helper()
	c := controllers.MoodController{Instruments: f}
	r := chi.NewRouter()
	r.Use(controllers.RequireSession(&fakeSessions{validated: models.Account{ID: uuid.New(), FullName: "Maria"}}))
	r.Get("/me/mood/instruments/{codigo}", c.GetInstrument)

	req := httptest.NewRequest(http.MethodGet, alvo, nil)
	req.AddCookie(&http.Cookie{Name: controllers.SessionCookieName, Value: "token-de-teste"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestMood_GetInstrument_OK(t *testing.T) {
	f := &fakeInstruments{cfg: models.InstrumentConfig{
		Codigo: "GRID", Versao: "1", Anel: "diario",
		Dimensions:    []models.InstrumentDimension{{Dimensao: "valencia", Polaridade: "positiva", MinScore: 0, MaxScore: 100}},
		EmotionLabels: []models.EmotionLabel{{Quadrante: "desagradavel_calmo", Rotulo: "Triste"}},
		ContextTags:   []models.ContextTag{{Chave: "sono", Rotulo: "Sono"}},
	}}
	w := serveMood(t, f, "/me/mood/instruments/GRID")

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "GRID", body["codigo"])
	require.Equal(t, "diario", body["anel"])
	require.Len(t, body["dimensions"], 1)
	require.Len(t, body["emotion_labels"], 1)
	require.Len(t, body["context_tags"], 1)
}

func TestMood_GetInstrument_NaoEncontrado_404(t *testing.T) {
	f := &fakeInstruments{err: models.ErrInstrumentNotFound}
	w := serveMood(t, f, "/me/mood/instruments/XPTO")

	require.Equal(t, http.StatusNotFound, w.Code)
}
