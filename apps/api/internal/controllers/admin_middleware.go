package controllers

import (
	"crypto/subtle"
	"net/http"
)

// AdminTokenHeader é o header que carrega o token estático das rotas /admin. Casa
// com o securityScheme adminToken do openapi.yaml.
const AdminTokenHeader = "X-Admin-Token"

// RequireAdminToken protege as rotas /admin com um token estático (ADR: operação
// por gente do time, fora de banda, nunca a sessão do paciente).
//
// A comparação é em tempo constante (crypto/subtle) para não vazar, pelo tempo de
// resposta, quantos caracteres do token bateram. Ausente e errado respondem IGUAL
// (401 com o mesmo reason ADMIN_TOKEN_INVALID): distinguir "faltou o header" de
// "token errado" daria um oráculo ao atacante. O token NUNCA é logado.
func RequireAdminToken(token string) func(http.Handler) http.Handler {
	want := []byte(token)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := []byte(r.Header.Get(AdminTokenHeader))
			// token vazio significa admin desligado — nunca aceita (mesmo com header
			// vazio, que casaria por acaso num ConstantTimeCompare de dois vazios).
			if len(want) == 0 || subtle.ConstantTimeCompare(got, want) != 1 {
				WriteProblemReason(w, http.StatusUnauthorized, "não autorizado",
					"token de administração ausente ou inválido",
					Reason{Code: "ADMIN_TOKEN_INVALID"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
