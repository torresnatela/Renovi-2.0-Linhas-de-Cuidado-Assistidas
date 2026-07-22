package controllers

import (
	"crypto/subtle"
	"net/http"
)

// IntegrationTokenHeader carrega o token estático da integração Gestão->API (push,
// ADR-043). Casa com o securityScheme integrationToken do openapi.yaml.
const IntegrationTokenHeader = "X-Integration-Token"

// RequireIntegrationToken protege as rotas /integration/gestao com um token
// estático — a máquina do Gestão que chama, não a sessão do paciente.
//
// Mesma disciplina do RequireAdminToken: comparação em tempo constante
// (crypto/subtle) para não vazar quantos caracteres bateram; ausente e errado
// respondem IGUAL (401, mesmo reason INTEGRATION_TOKEN_INVALID) para não dar um
// oráculo; o token NUNCA é logado. Token vazio significa integração desligada e
// nunca aceita (mesmo com header vazio).
func RequireIntegrationToken(token string) func(http.Handler) http.Handler {
	want := []byte(token)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := []byte(r.Header.Get(IntegrationTokenHeader))
			if len(want) == 0 || subtle.ConstantTimeCompare(got, want) != 1 {
				WriteProblemReason(w, http.StatusUnauthorized, "não autorizado",
					"token de integração ausente ou inválido",
					Reason{Code: "INTEGRATION_TOKEN_INVALID"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
