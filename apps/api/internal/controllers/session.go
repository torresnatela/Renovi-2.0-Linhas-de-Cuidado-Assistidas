package controllers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/renovisaude/renovi-care/internal/models"
)

// sessionCookies monta o cookie de sessão. Compartilhado pelos controllers que abrem
// sessão (cadastro, login, conclusão do onboarding) para emitir SEMPRE o mesmo cookie.
type sessionCookies struct {
	secure bool
	ttl    time.Duration
}

// cookie monta o cookie de sessão.
//
// HttpOnly é fixo, não configurável: é ele que impede JavaScript (e portanto XSS) de
// ler o token. Secure é configurável só porque o desenvolvimento local roda sem TLS.
// SameSite=Lax e não Strict: com Strict, quem chega por link externo (e-mail de
// convite) apareceria deslogado mesmo tendo sessão válida.
func (sc sessionCookies) cookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   sc.secure,
		SameSite: http.SameSiteLaxMode,
	}
}

// issueSession cria a sessão no servidor e emite o cookie. Devolve false (e já
// respondeu 500) em falha de criação.
func issueSession(w http.ResponseWriter, r *http.Request, sessions Sessions, sc sessionCookies, acc models.Account) bool {
	token, _, err := sessions.Create(r.Context(), acc.ID)
	if err != nil {
		slog.ErrorContext(r.Context(), "sessão: falha ao criar", "error", err)
		WriteProblem(w, http.StatusInternalServerError, "erro interno", "não foi possível iniciar a sessão")
		return false
	}
	http.SetCookie(w, sc.cookie(token, int(sc.ttl.Seconds())))
	return true
}
