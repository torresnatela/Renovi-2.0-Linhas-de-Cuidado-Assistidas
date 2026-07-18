package controllers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/renovisaude/renovi-care/internal/http/api"
	"github.com/renovisaude/renovi-care/internal/models"
)

// InternalJourneys é o que o controller interno precisa da jornada.
// Implementada por *models.JourneyStore.
type InternalJourneys interface {
	ForceStatus(ctx context.Context, careApptID uuid.UUID, status string, now time.Time) (models.CareAppointment, error)
}

// InternalController expõe as rotas /internal/* de teste.
//
// SEM autenticação de propósito: o gate é de AMBIENTE (RENOVI_TEST_ENDPOINTS) —
// em produção o controller nem é montado e as rotas não existem. Ainda assim,
// id inexistente responde 404 e corpo inválido 400, como qualquer rota.
type InternalController struct {
	Journeys InternalJourneys
	// Now é injetável para o teste não depender do relógio da máquina.
	Now func() time.Time
}

func (c InternalController) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// ForceStatus marca a consulta como realizada ou falta sem esperar o fluxo real
// — o atalho para testar auto-conclusão da jornada e lembretes.
func (c InternalController) ForceStatus(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "care_appointment_id"))
	if err != nil {
		// Id malformado = id que não existe: 404.
		WriteProblem(w, http.StatusNotFound, "Não encontrado", "Consulta não encontrada.")
		return
	}

	var body api.ForceStatusRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	if !body.Status.Valid() {
		WriteProblem(w, http.StatusBadRequest, "Requisição inválida", "status deve ser realizada ou falta.")
		return
	}

	appt, err := c.Journeys.ForceStatus(r.Context(), id, string(body.Status), c.now())
	switch {
	case errors.Is(err, models.ErrCareAppointmentNotFound):
		WriteProblem(w, http.StatusNotFound, "Não encontrado", "Consulta não encontrada.")
		return
	case errors.Is(err, models.ErrForceStatusNotAllowed):
		WriteProblemReason(w, http.StatusConflict, "Não é possível forçar",
			"A consulta já está num estado terminal.",
			Reason{Code: "FORCE_STATUS_NOT_ALLOWED"})
		return
	case errors.Is(err, models.ErrInvalidForceStatus):
		WriteProblem(w, http.StatusBadRequest, "Requisição inválida", "status deve ser realizada ou falta.")
		return
	case err != nil:
		WriteProblem(w, http.StatusInternalServerError, "Erro interno", "Não foi possível forçar o status.")
		return
	}
	WriteJSON(w, http.StatusOK, toAPICareAppointment(appt))
}
