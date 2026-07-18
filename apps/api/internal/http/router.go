package http

import (
	"context"
	"log/slog"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/renovisaude/renovi-care/internal/controllers"
)

// defaultRouteTimeout é o teto de uma rota normal. O cadastro é a exceção e usa
// o seu próprio (ver Deps.RegisterTimeout).
const defaultRouteTimeout = 30 * time.Second

// Deps são as dependências injetadas no roteador.
type Deps struct {
	Logger  *slog.Logger
	Version string
	// Ready checa dependências para o /readyz (ex.: ping no Postgres). Pode ser nil.
	Ready func(ctx context.Context) error
	// Auth monta /auth/* e /me. Nil desliga essas rotas (útil em testes que só
	// exercitam saúde, e no boot sem banco).
	Auth *controllers.AuthController
	// Scheduling monta o agendamento. Nil desliga (ex.: sem DSN do legado no
	// ambiente local). Depende de Auth: tudo aqui exige sessão.
	Scheduling *controllers.SchedulingController
	// RegisterTimeout é o teto da rota de cadastro, que fala com a DAV de forma
	// síncrona. Deve vir de config.DAVBudget() + folga; zero cai num default.
	RegisterTimeout time.Duration
	// BookTimeout é o teto da rota de agendamento, que também fala com a DAV.
	// Zero cai num default.
	BookTimeout time.Duration
}

// NewRouter monta o *chi.Mux com middleware e rotas.
//
// Convenção: /healthz e /readyz ficam na raiz (para o proxy/infra) e também sob
// /api/v1 (para bater com o OpenAPI). As rotas de negócio vivem em /api/v1.
func NewRouter(d Deps) *chi.Mux {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}

	if d.RegisterTimeout <= 0 {
		d.RegisterTimeout = 75 * time.Second
	}
	// O agendamento faz UMA tentativa contra a DAV (escrita nunca repete), então o
	// orçamento é menor que o do cadastro. Mas continua bem acima do teto normal:
	// só a chamada dela já pode levar 17s, e o gateway dela corta em 29s.
	if d.BookTimeout <= 0 {
		d.BookTimeout = 45 * time.Second
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	// Sem Timeout global aqui, de propósito: ele é aplicado POR ROTA (abaixo).
	// Um Timeout global é um teto que rota nenhuma consegue estender — foi ele
	// que cortou a última tentativa do cadastro no primeiro teste real, porque
	// 3 tentativas de 10s davam exatamente os mesmos 30s do middleware.
	r.Use(requestLogger(d.Logger))

	health := controllers.HealthController{Version: d.Version, Ready: d.Ready}

	r.Group(func(r chi.Router) {
		r.Use(middleware.Timeout(defaultRouteTimeout))
		r.Get("/healthz", health.Live)
		r.Get("/readyz", health.Readyz)
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(middleware.Timeout(defaultRouteTimeout))
			r.Get("/healthz", health.Live)
			r.Get("/readyz", health.Readyz)
		})

		if d.Auth != nil {
			mountAuth(r, *d.Auth, d.RegisterTimeout)

			// O agendamento inteiro exige sessão, então depende do Auth estar
			// montado: sem ele não há RequireSession, e sem RequireSession não há
			// dono para a consulta.
			if d.Scheduling != nil {
				mountScheduling(r, *d.Scheduling, *d.Auth, d.BookTimeout)
			}
		}
		// TODO(mvp): /me/eligibility entra aqui, filtrando ANTES do agendamento.
		// Ver packages/contracts/openapi.yaml e docs/PROGRESSO.md.
	})

	return r
}

// mountScheduling monta o agendamento, todo atrás de sessão.
func mountScheduling(r chi.Router, s controllers.SchedulingController, auth controllers.AuthController, bookTimeout time.Duration) {
	r.Group(func(r chi.Router) {
		r.Use(controllers.RequireSession(auth.Sessions))

		// Catálogo e leitura: rápidos, só o MySQL legado.
		r.Group(func(r chi.Router) {
			r.Use(middleware.Timeout(defaultRouteTimeout))
			r.Get("/specialties", s.ListSpecialties)
			r.Get("/specialties/{specialty_id}/professionals", s.ListProfessionals)
			r.Get("/professionals/{professional_id}/slots", s.ListSlots)
			r.Get("/appointments", s.List)
			r.Get("/appointments/{appointment_id}", s.Get)
			r.Post("/appointments/{appointment_id}/join", s.Join)
		})

		// Agendar tem timeout próprio, pelo mesmo motivo do cadastro: fala com a
		// DAV de forma síncrona, e ela é lenta e IMPREVISÍVEL (3s a 29s medidos em
		// sondagens do mesmo dia, contra o teto do gateway dela). Com o teto normal
		// de 30s, a requisição morreria antes da DAV responder — e aí o paciente
		// fica sem saber se marcou, que é o pior desfecho possível nesta rota: não
		// dá para reconciliar depois.
		//
		// E tem rate limit, como o /auth/register — é a rota MAIS cara e perigosa
		// (escrita não idempotente e insondável, segurando até bookTimeout e
		// queimando orçamento da DAV). Sem isto, uma única sessão dispara N
		// agendamentos concorrentes sem freio. O contrato promete o 429; é aqui que
		// ele passa a existir de verdade.
		r.Group(func(r chi.Router) {
			// Por CONTA, não por IP: a rota é autenticada (RequireSession já rodou),
			// e a conta é chave justa sob NAT e imune ao spoofing de IP por header.
			r.Use(rateLimitByAccount(20, 1.0/3.0))
			r.Use(middleware.Timeout(bookTimeout))
			r.Post("/appointments", s.Create)
		})
	})
}

// mountAuth monta as rotas de autenticação.
//
// O rate limit é generoso de propósito: o NAT corporativo faz muitos
// colaboradores dividirem um IP, e a trava é contra script de força bruta, não
// contra gente errando a senha.
func mountAuth(r chi.Router, auth controllers.AuthController, registerTimeout time.Duration) {
	// O cadastro tem timeout próprio: ele fala com a DAV de forma síncrona e
	// precisa caber no orçamento de tentativas dela (config.DAVBudget), que é
	// muito maior que o de uma rota normal.
	r.Group(func(r chi.Router) {
		r.Use(rateLimitByIP(20, 1.0/3.0))
		r.Use(middleware.Timeout(registerTimeout))
		r.Post("/auth/register", auth.Register)
	})

	r.Group(func(r chi.Router) {
		r.Use(rateLimitByIP(20, 1.0/3.0))
		r.Use(middleware.Timeout(defaultRouteTimeout))
		r.Post("/auth/login", auth.Login)
	})

	// Rotas autenticadas.
	r.Group(func(r chi.Router) {
		r.Use(middleware.Timeout(defaultRouteTimeout))
		r.Use(controllers.RequireSession(auth.Sessions))
		r.Post("/auth/logout", auth.Logout)
		r.Get("/me", auth.Me)
	})
}
