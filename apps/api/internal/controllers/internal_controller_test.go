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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/controllers"
	"github.com/renovisaude/renovi-care/internal/models"
)

type fakeInternalJourneys struct {
	appt  models.CareAppointment
	err   error
	calls []string // status forçados
}

func (f *fakeInternalJourneys) ForceStatus(_ context.Context, _ uuid.UUID, status string, _ time.Time) (models.CareAppointment, error) {
	f.calls = append(f.calls, status)
	if f.err != nil {
		return models.CareAppointment{}, f.err
	}
	out := f.appt
	out.Status = status
	return out, nil
}

func serveInternal(t *testing.T, f *fakeInternalJourneys, alvo, corpo string) *httptest.ResponseRecorder {
	t.Helper()
	c := controllers.InternalController{Journeys: f, Now: func() time.Time { return agoraFixo }}
	r := chi.NewRouter()
	r.Post("/internal/appointments/{care_appointment_id}/force-status", c.ForceStatus)

	req := httptest.NewRequest(http.MethodPost, alvo, strings.NewReader(corpo))
	if corpo != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestInternalController_ForceStatus_Feliz200(t *testing.T) {
	appt := careApptFixa()
	f := &fakeInternalJourneys{appt: appt}
	rec := serveInternal(t, f, "/internal/appointments/"+appt.ID.String()+"/force-status", `{"status":"realizada"}`)

	require.Equal(t, http.StatusOK, rec.Code)
	var got struct {
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "realizada", got.Status)
	assert.Equal(t, []string{"realizada"}, f.calls)
}

func TestInternalController_ForceStatus_BodyInvalido400(t *testing.T) {
	f := &fakeInternalJourneys{}
	id := uuid.NewString()

	rec := serveInternal(t, f, "/internal/appointments/"+id+"/force-status", `{"status":"cancelada"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code, "status fora de realizada|falta é 400")

	rec = serveInternal(t, f, "/internal/appointments/"+id+"/force-status", `{nao-e-json`)
	require.Equal(t, http.StatusBadRequest, rec.Code, "JSON quebrado é 400")

	assert.Empty(t, f.calls, "corpo inválido não chega ao model")
}

func TestInternalController_ForceStatus_NaoEncontrada404(t *testing.T) {
	f := &fakeInternalJourneys{err: models.ErrCareAppointmentNotFound}
	rec := serveInternal(t, f, "/internal/appointments/"+uuid.NewString()+"/force-status", `{"status":"falta"}`)
	require.Equal(t, http.StatusNotFound, rec.Code)

	// Id malformado também é 404 (id que não existe).
	rec = serveInternal(t, &fakeInternalJourneys{}, "/internal/appointments/nao-e-uuid/force-status", `{"status":"falta"}`)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestInternalController_ForceStatus_Terminal409(t *testing.T) {
	f := &fakeInternalJourneys{err: models.ErrForceStatusNotAllowed}
	rec := serveInternal(t, f, "/internal/appointments/"+uuid.NewString()+"/force-status", `{"status":"falta"}`)
	require.Equal(t, http.StatusConflict, rec.Code)
}
