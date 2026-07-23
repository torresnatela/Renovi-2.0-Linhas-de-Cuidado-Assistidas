package controllers

import (
	"context"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/renovisaude/renovi-care/internal/http/api"
	"github.com/renovisaude/renovi-care/internal/models"
)

// GestaoIngestion é o que o controller precisa do model (interface no consumidor,
// ADR-012). O GestaoIngestionStore a implementa.
type GestaoIngestion interface {
	RecordContract(ctx context.Context, in models.ContractPush) (models.RecordResult, error)
	ResendInvite(ctx context.Context, cpfHmac []byte) (models.ResendResult, error)
}

// IngestionController expõe as rotas de integração da Gestão (push, ADR-043).
type IngestionController struct {
	Ingestion GestaoIngestion
}

// RecordContract recebe o push de um contrato (POST /integration/gestao/contracts).
func (c IngestionController) RecordContract(w http.ResponseWriter, r *http.Request) {
	var body api.GestaoContractPush
	if !decodeJSON(w, r, &body) {
		return
	}
	cpfHmac, ok := decodeCPFHmac(w, body.Employee.CpfHmac)
	if !ok {
		return
	}

	var startedAt time.Time
	if body.StartedAt != nil {
		startedAt = *body.StartedAt
	}
	in := models.ContractPush{
		ContractID: body.ContractId,
		Status:     string(body.Status),
		StartedAt:  startedAt,
		EndedAt:    body.EndedAt,
		Employee: models.EmployeePush{
			ID: body.Employee.Id, CPFHmac: cpfHmac, Name: body.Employee.Name,
			Email: deref(body.Employee.Email), Phone: deref(body.Employee.Phone),
		},
		Company: models.CompanyPush{ID: body.Company.Id, DisplayName: body.Company.DisplayName},
	}

	res, err := c.Ingestion.RecordContract(r.Context(), in)
	if err != nil {
		if errors.Is(err, models.ErrInvalidContractPush) {
			WriteProblem(w, http.StatusBadRequest, "requisição inválida",
				"o contrato enviado não passou na validação")
			return
		}
		slog.ErrorContext(r.Context(), "falha ao registrar contrato da Gestão", "error", err)
		WriteProblem(w, http.StatusInternalServerError, "erro interno",
			"não foi possível registrar o contrato")
		return
	}

	out := api.GestaoContractPushResult{
		PersonStatus:   api.GestaoContractPushResultPersonStatus(res.PersonStatus),
		ContractStatus: api.GestaoContractPushResultContractStatus(res.ContractStatus),
		InviteSent:     res.InviteSent,
	}
	if res.InviteURL != "" {
		out.InviteUrl = &res.InviteURL
		out.InviteExpiresAt = res.InviteExpiresAt
	}
	WriteJSON(w, http.StatusOK, out)
}

// ResendInvite reenvia o convite de um colaborador
// (POST /integration/gestao/employees/{cpf_hmac}/resend-invite).
func (c IngestionController) ResendInvite(w http.ResponseWriter, r *http.Request) {
	cpfHmac, ok := decodeCPFHmac(w, chi.URLParam(r, "cpf_hmac"))
	if !ok {
		return
	}

	res, err := c.Ingestion.ResendInvite(r.Context(), cpfHmac)
	if err != nil {
		switch {
		case errors.Is(err, models.ErrEmployeeUnknown):
			WriteProblem(w, http.StatusNotFound, "não encontrado", "colaborador desconhecido")
		case errors.Is(err, models.ErrAlreadyHasAccount):
			WriteProblemReason(w, http.StatusConflict, "conflito",
				"o colaborador já tem conta; o vínculo passa pelo consentimento, não por convite",
				Reason{Code: "ALREADY_HAS_ACCOUNT"})
		case errors.Is(err, models.ErrInvalidContractPush):
			WriteProblem(w, http.StatusBadRequest, "requisição inválida", "cpf_hmac inválido")
		default:
			slog.ErrorContext(r.Context(), "falha ao reenviar convite da Gestão", "error", err)
			WriteProblem(w, http.StatusInternalServerError, "erro interno",
				"não foi possível reenviar o convite")
		}
		return
	}

	WriteJSON(w, http.StatusOK, api.ResendInviteResult{
		InviteUrl: res.InviteURL, ExpiresAt: res.ExpiresAt,
	})
}

// decodeCPFHmac decodifica o cpf_hmac de hex para 32 bytes, respondendo 400 se
// malformado. Aceita hex em qualquer caixa (os bytes são os mesmos); a validação
// de tamanho é o que importa para casar com a coluna bytea de 32 bytes.
func decodeCPFHmac(w http.ResponseWriter, s string) ([]byte, bool) {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 32 {
		WriteProblem(w, http.StatusBadRequest, "requisição inválida",
			"cpf_hmac deve ser hex de 64 caracteres (32 bytes)")
		return nil, false
	}
	return b, true
}
