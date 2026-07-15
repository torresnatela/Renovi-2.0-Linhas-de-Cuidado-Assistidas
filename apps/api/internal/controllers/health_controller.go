package controllers

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/renovisaude/renovi-care/internal/http/api"
)

const serviceName = "renovi-care"

// HealthController expõe liveness (/healthz) e readiness (/readyz).
type HealthController struct {
	// Version é a versão do build (injetada em cmd/api).
	Version string
	// Ready checa dependências (ex.: ping no Postgres). Se nil, considera-se
	// sempre pronto — útil em testes e quando o banco ainda não está acoplado.
	Ready func(ctx context.Context) error
}

// ok monta o corpo de sucesso a partir do tipo do contrato (api.HealthStatus),
// garantindo que a forma na rede acompanhe o OpenAPI (fonte da verdade).
func (c HealthController) ok() api.HealthStatus {
	now := time.Now().UTC()
	return api.HealthStatus{
		Status:  api.Ok,
		Service: serviceName,
		Version: c.Version,
		Time:    &now,
	}
}

// Live responde 200 enquanto o processo está de pé (não checa dependências).
func (c HealthController) Live(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, c.ok())
}

// Readyz responde 200 se as dependências estão acessíveis; 503 caso contrário.
//
// O erro real é logado no servidor, mas NÃO vai no corpo da resposta: /readyz é
// público e o erro do driver expõe host/usuário/topologia do banco (LGPD/segurança).
func (c HealthController) Readyz(w http.ResponseWriter, r *http.Request) {
	if c.Ready != nil {
		if err := c.Ready(r.Context()); err != nil {
			slog.WarnContext(r.Context(), "readiness check falhou", "error", err)
			WriteProblem(w, http.StatusServiceUnavailable, "not ready",
				"uma dependência do serviço está indisponível")
			return
		}
	}
	WriteJSON(w, http.StatusOK, c.ok())
}
