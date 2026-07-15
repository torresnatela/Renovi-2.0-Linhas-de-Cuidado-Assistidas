package http

import (
	"context"
	"log/slog"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/renovisaude/renovi-care/internal/controllers"
)

// Deps são as dependências injetadas no roteador.
type Deps struct {
	Logger  *slog.Logger
	Version string
	// Ready checa dependências para o /readyz (ex.: ping no Postgres). Pode ser nil.
	Ready func(ctx context.Context) error
}

// NewRouter monta o *chi.Mux com middleware e rotas.
//
// Convenção: /healthz e /readyz ficam na raiz (para o proxy/infra) e também sob
// /api/v1 (para bater com o OpenAPI). As rotas de negócio vivem em /api/v1.
func NewRouter(d Deps) *chi.Mux {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(requestLogger(d.Logger))

	health := controllers.HealthController{Version: d.Version, Ready: d.Ready}

	r.Get("/healthz", health.Live)
	r.Get("/readyz", health.Readyz)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/healthz", health.Live)
		r.Get("/readyz", health.Readyz)
		// TODO(mvp): montar aqui as rotas de negócio (/me, /me/eligibility,
		// /slots, /appointments...) junto com o middleware de auth.
		// Ver packages/contracts/openapi.yaml e docs/PROGRESSO.md.
	})

	return r
}
