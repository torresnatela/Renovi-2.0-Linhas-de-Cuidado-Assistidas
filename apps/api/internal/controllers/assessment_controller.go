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

// AssessmentService é o que o controller precisa dos anéis periódicos (WHO-5/PHQ-4).
type AssessmentService interface {
	Availability(ctx context.Context, patientID uuid.UUID, codigo string, now time.Time) (models.AssessmentAvailability, error)
	Submit(ctx context.Context, patientID uuid.UUID, codigo string, items []int, now time.Time) (models.AssessmentResult, error)
}

// AssessmentController expõe /me/assessments, atrás de sessão. `Now` é injetável.
type AssessmentController struct {
	Assessments AssessmentService
	Now         func() time.Time
}

func (c AssessmentController) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// GetAvailability diz se o instrumento pode ser respondido agora (motor).
func (c AssessmentController) GetAvailability(w http.ResponseWriter, r *http.Request) {
	account, ok := AccountFrom(r.Context())
	if !ok {
		WriteProblem(w, http.StatusUnauthorized, "Não autenticado", "sessão ausente ou inválida")
		return
	}
	codigo := chi.URLParam(r, "codigo")
	av, err := c.Assessments.Availability(r.Context(), account.ID, codigo, c.now())
	if err != nil {
		if errors.Is(err, models.ErrUnknownInstrument) {
			WriteProblem(w, http.StatusNotFound, "Instrumento não encontrado", "código desconhecido")
			return
		}
		WriteProblem(w, http.StatusInternalServerError, "Erro ao consultar disponibilidade", "")
		return
	}
	WriteJSON(w, http.StatusOK, toAPIAssessmentAvailability(av))
}

// Submit pontua e persiste um instrumento periódico.
func (c AssessmentController) Submit(w http.ResponseWriter, r *http.Request) {
	account, ok := AccountFrom(r.Context())
	if !ok {
		WriteProblem(w, http.StatusUnauthorized, "Não autenticado", "sessão ausente ou inválida")
		return
	}
	var body api.SubmitAssessmentRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	res, err := c.Assessments.Submit(r.Context(), account.ID, string(body.Codigo), body.Items, c.now())
	if err != nil {
		var blocked models.ErrAssessmentBlocked
		switch {
		case errors.Is(err, models.ErrAssessmentInvalid):
			WriteProblem(w, http.StatusBadRequest, "Respostas inválidas", "quantidade ou valores fora do esperado")
		case errors.Is(err, models.ErrUnknownInstrument):
			WriteProblem(w, http.StatusBadRequest, "Instrumento inválido", "código desconhecido")
		case errors.Is(err, models.ErrNoActiveConsent):
			WriteProblemReason(w, http.StatusForbidden, "Consentimento necessário",
				"registre o consentimento antes de responder", Reason{Code: models.ReasonConsentRequired})
		case errors.Is(err, models.ErrNotEnrolledInActivity):
			WriteProblemReason(w, http.StatusForbidden, "Sem matrícula elegível",
				"você não tem uma linha de cuidado ativa com este instrumento", Reason{Code: models.ReasonNotEnrolled})
		case errors.As(err, &blocked):
			WriteProblemFull(w, Problem{
				Title: "Ainda não disponível", Status: http.StatusConflict,
				Detail: "responda novamente após o intervalo mínimo", Blocks: toProblemBlocks(blocked.Blocks),
			})
		default:
			WriteProblem(w, http.StatusInternalServerError, "Erro ao registrar", "")
		}
		return
	}
	WriteJSON(w, http.StatusOK, toAPIAssessmentResult(res))
}

func toAPIAssessmentAvailability(a models.AssessmentAvailability) api.AssessmentAvailability {
	return api.AssessmentAvailability{
		Codigo:      a.Codigo,
		Eligibility: toAPIEligibility(a.Eligibility),
		ItemCount:   a.ItemCount,
		ValueMin:    a.ValueMin,
		ValueMax:    a.ValueMax,
	}
}

func toAPIAssessmentResult(r models.AssessmentResult) api.AssessmentResult {
	out := api.AssessmentResult{
		Codigo:         r.Codigo,
		Faixa:          r.Faixa,
		FlagEncaminhar: r.FlagEncaminhar,
		RespondidoEm:   r.RespondidoEm,
	}
	raw := float32(r.RawScore)
	out.RawScore = &raw
	if r.IndexScore != nil {
		idx := float32(*r.IndexScore)
		out.IndexScore = &idx
	}
	if r.Subscores != nil {
		m := r.Subscores
		out.Subscores = &m
	}
	return out
}
