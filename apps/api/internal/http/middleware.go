// Package http monta o roteador chi, o middleware e conecta os controllers.
package http

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// requestLogger registra cada requisição de forma estruturada (slog).
//
// LGPD: NUNCA logamos corpo de request/response (autenticação e dados de saúde
// são sensíveis) — apenas metadados de tráfego.
func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			logger.Info("http_request",
				"method", r.Method,
				"path", redactSensitivePath(r.URL.Path),
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", middleware.GetReqID(r.Context()),
				"remote", r.RemoteAddr,
			)
		})
	}
}

// redactSensitivePath oculta credenciais que viajam NO PATH antes de logar. Hoje só o
// token do convite de onboarding: /onboarding/<token>[/complete|/decline]. O token é
// credencial portadora (igual ao cookie de sessão) — a URL segue sendo o meio de
// entrega do convite, mas log não é canal para credencial (mesma disciplina do
// LogNotifier, que nunca loga a invite_url). Preserva o sufixo de ação (/complete,
// /decline) para não perder observabilidade.
func redactSensitivePath(path string) string {
	const marker = "/onboarding/"
	i := strings.Index(path, marker)
	if i < 0 {
		return path
	}
	rest := path[i+len(marker):]
	if rest == "" {
		return path // ".../onboarding/" sem token: nada a redigir
	}
	suffix := ""
	if j := strings.IndexByte(rest, '/'); j >= 0 {
		suffix = rest[j:] // "/complete", "/decline", etc.
	}
	return path[:i+len(marker)] + "<redacted>" + suffix
}
