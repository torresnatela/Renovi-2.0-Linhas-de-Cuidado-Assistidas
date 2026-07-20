package controllers

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/renovisaude/renovi-care/internal/http/api"
	"github.com/renovisaude/renovi-care/internal/models"
)

// ConsentService é o que o controller precisa do model de consentimento. A
// interface vive no consumidor (ADR-012); o teste injeta um fake.
type ConsentService interface {
	Active(ctx context.Context, patientID uuid.UUID, finalidade string) (models.Consent, error)
	Grant(ctx context.Context, patientID uuid.UUID, finalidade, versaoTermo string, gestaoContractID *string, now time.Time) (models.Consent, error)
	Revoke(ctx context.Context, patientID uuid.UUID, finalidade string, now time.Time) error
}

// ConsentController expõe /me/consent, atrás de sessão (cookieAuth). `Now` é
// injetável para teste.
type ConsentController struct {
	Consents ConsentService
	Now      func() time.Time
}

func (c ConsentController) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// GetConsent devolve o status do consentimento do paciente para a finalidade
// (default: checkin_humor). Sem consentimento ativo, responde active=false.
func (c ConsentController) GetConsent(w http.ResponseWriter, r *http.Request) {
	account, ok := AccountFrom(r.Context())
	if !ok {
		WriteProblem(w, http.StatusUnauthorized, "Não autenticado", "sessão ausente ou inválida")
		return
	}
	finalidade := strings.TrimSpace(r.URL.Query().Get("finalidade"))
	if finalidade == "" {
		finalidade = models.ConsentCheckinHumor
	}
	consent, err := c.Consents.Active(r.Context(), account.ID, finalidade)
	if err != nil {
		if errors.Is(err, models.ErrNoActiveConsent) {
			WriteJSON(w, http.StatusOK, api.ConsentStatus{Finalidade: finalidade, Active: false})
			return
		}
		WriteProblem(w, http.StatusInternalServerError, "Erro ao consultar consentimento", "")
		return
	}
	WriteJSON(w, http.StatusOK, toConsentStatus(consent))
}

// GrantConsent concede consentimento (versionado). Idempotente para o mesmo termo.
func (c ConsentController) GrantConsent(w http.ResponseWriter, r *http.Request) {
	account, ok := AccountFrom(r.Context())
	if !ok {
		WriteProblem(w, http.StatusUnauthorized, "Não autenticado", "sessão ausente ou inválida")
		return
	}
	var body api.GrantConsentRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	consent, err := c.Consents.Grant(r.Context(), account.ID, body.Finalidade, body.VersaoTermo, nil, c.now())
	if err != nil {
		if errors.Is(err, models.ErrConsentInvalid) {
			WriteProblem(w, http.StatusBadRequest, "Consentimento inválido", "finalidade desconhecida ou versão do termo ausente")
			return
		}
		WriteProblem(w, http.StatusInternalServerError, "Erro ao registrar consentimento", "")
		return
	}
	WriteJSON(w, http.StatusOK, toConsentStatus(consent))
}

// RevokeConsent revoga o consentimento ativo da finalidade. Idempotente.
func (c ConsentController) RevokeConsent(w http.ResponseWriter, r *http.Request) {
	account, ok := AccountFrom(r.Context())
	if !ok {
		WriteProblem(w, http.StatusUnauthorized, "Não autenticado", "sessão ausente ou inválida")
		return
	}
	var body api.RevokeConsentRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := c.Consents.Revoke(r.Context(), account.ID, body.Finalidade, c.now()); err != nil {
		if errors.Is(err, models.ErrConsentInvalid) {
			WriteProblem(w, http.StatusBadRequest, "Consentimento inválido", "finalidade desconhecida")
			return
		}
		WriteProblem(w, http.StatusInternalServerError, "Erro ao revogar consentimento", "")
		return
	}
	WriteJSON(w, http.StatusOK, api.ConsentStatus{Finalidade: strings.TrimSpace(body.Finalidade), Active: false})
}

func toConsentStatus(c models.Consent) api.ConsentStatus {
	versao := c.VersaoTermo
	concedido := c.ConcedidoEm
	return api.ConsentStatus{
		Finalidade:  c.Finalidade,
		Active:      c.Status == "ativo",
		VersaoTermo: &versao,
		ConcedidoEm: &concedido,
	}
}
