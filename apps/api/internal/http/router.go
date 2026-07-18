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
	// Consent monta /me/consent (Verificador de Humor, Anexo C). Nil desliga.
	// Depende de Auth: exige sessão do paciente.
	Consent *controllers.ConsentController
	// Mood monta /me/mood/* (Verificador de Humor, Anexo C). Nil desliga.
	// Depende de Auth: exige sessão do paciente.
	Mood *controllers.MoodController
	// CareAdmin monta as rotas /admin/* (catálogo + matrícula). Nil desliga (sem
	// RENOVI_ADMIN_TOKEN ou sem agenda para validar o publish). NÃO depende de Auth:
	// autentica pelo token de admin, nunca pela sessão do paciente.
	CareAdmin *controllers.CareLineAdminController
	// AdminToken é o token estático exigido pelas rotas /admin (header
	// X-Admin-Token). Só é usado quando CareAdmin != nil.
	AdminToken string
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

			// Verificador de Humor (Anexo C): consentimento é a primeira rota do
			// paciente; também exige sessão.
			if d.Consent != nil {
				mountConsent(r, *d.Consent, *d.Auth)
			}
			if d.Mood != nil {
				mountMood(r, *d.Mood, *d.Auth)
			}
		}

		// As rotas /admin NÃO dependem de Auth: autenticam pelo token de admin, não
		// pela sessão do paciente. Só sobem quando o controller foi montado (token
		// presente E agenda disponível — ver cmd/api/main.go).
		if d.CareAdmin != nil {
			mountCareAdmin(r, *d.CareAdmin, d.AdminToken)
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

// mountConsent monta /me/consent (Anexo C), atrás de sessão. Timeout normal: são
// operações rápidas contra o Postgres próprio.
func mountConsent(r chi.Router, c controllers.ConsentController, auth controllers.AuthController) {
	r.Group(func(r chi.Router) {
		r.Use(middleware.Timeout(defaultRouteTimeout))
		r.Use(controllers.RequireSession(auth.Sessions))
		r.Get("/me/consent", c.GetConsent)
		r.Post("/me/consent", c.GrantConsent)
		r.Post("/me/consent/revoke", c.RevokeConsent)
	})
}

// mountMood monta /me/mood/* (Anexo C), atrás de sessão. Timeout normal.
func mountMood(r chi.Router, c controllers.MoodController, auth controllers.AuthController) {
	r.Group(func(r chi.Router) {
		r.Use(middleware.Timeout(defaultRouteTimeout))
		r.Use(controllers.RequireSession(auth.Sessions))
		r.Get("/me/mood/instruments/{codigo}", c.GetInstrument)
		r.Post("/me/mood/checkin", c.RecordCheckin)
		r.Get("/me/mood/today", c.GetToday)
		r.Get("/me/mood/history", c.GetHistory)
	})
}

// mountCareAdmin monta as rotas /admin/*, todas atrás do token de admin.
//
// O timeout é o normal: são operações rápidas contra o Postgres próprio. O publish
// consulta o legado, mas é uma leitura de catálogo, não a saga lenta da DAV — cabe
// no teto de 30s como as demais rotas.
func mountCareAdmin(r chi.Router, c controllers.CareLineAdminController, adminToken string) {
	r.Group(func(r chi.Router) {
		r.Use(controllers.RequireAdminToken(adminToken))
		r.Use(middleware.Timeout(defaultRouteTimeout))

		r.Route("/admin", func(r chi.Router) {
			r.Post("/care-lines", c.CreateCareLine)
			r.Get("/care-lines", c.ListCareLines)
			r.Post("/care-lines/{care_line_id}/items", c.CreateCareLineItem)
			r.Post("/care-lines/{care_line_id}/items/{item_ref}/rules", c.CreateCareLineItemRule)
			r.Post("/care-lines/{care_line_id}/publish", c.PublishCareLine)
			r.Post("/enrollments", c.CreateEnrollment)
			r.Post("/enrollments/{enrollment_id}/renew", c.RenewEnrollment)
			r.Post("/enrollments/{enrollment_id}/end", c.EndEnrollment)
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
