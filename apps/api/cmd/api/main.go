// Command api sobe o servidor HTTP do renovi-care.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	// Embute a base de fusos no binário. O adapter da agenda precisa carregar
	// America/Sao_Paulo para interpretar os DATETIME ingênuos do legado, e uma
	// imagem scratch/distroless não tem tzdata — sem isto, a API sobe em dev e
	// morre no boot em produção, no lugar mais caro possível.
	_ "time/tzdata"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/renovisaude/renovi-care/internal/adapters/agenda"
	"github.com/renovisaude/renovi-care/internal/adapters/dav"
	"github.com/renovisaude/renovi-care/internal/config"
	"github.com/renovisaude/renovi-care/internal/controllers"
	"github.com/renovisaude/renovi-care/internal/db"
	apihttp "github.com/renovisaude/renovi-care/internal/http"
	"github.com/renovisaude/renovi-care/internal/models"
	"github.com/renovisaude/renovi-care/internal/models/scheduling"
)

// version é sobrescrita no build via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("encerrando com erro", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := newLogger(cfg)
	slog.SetDefault(logger)
	// cfg tem LogValue: os segredos (API key da DAV, senha do banco) são
	// redigidos aqui por construção.
	logger.Info("iniciando renovi-care", "version", version, "config", cfg)

	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.CareDatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	auth, err := newAuthController(cfg, pool, logger)
	if err != nil {
		return err
	}

	// O cliente do legado é COMPARTILHADO: o agendamento o usa para ler a agenda, e
	// o admin o usa para validar as especialidades na publicação. Um pool só, criado
	// aqui — o main é onde as camadas se encontram.
	agendaClient, closeAgenda, err := newAgendaClient(cfg, logger)
	if err != nil {
		return err
	}
	if closeAgenda != nil {
		defer closeAgenda()
	}

	scheduling, bookingStore, schedulingReady, err := newSchedulingController(cfg, pool, agendaClient, logger)
	if err != nil {
		return err
	}

	careAdmin := newCareAdminController(cfg, pool, agendaClient, logger)

	// A jornada agenda PELO booking: reusa o MESMO BookingStore do scheduling.
	// Sem booking montado, não há jornada (nem endpoint interno de teste).
	journey, internal := newJourneyControllers(cfg, pool, bookingStore, logger)

	router := apihttp.NewRouter(apihttp.Deps{
		Logger:  logger,
		Version: version,
		Ready: func(ctx context.Context) error {
			if err := db.Ping(ctx, pool); err != nil {
				return err
			}
			// Quando o agendamento está ligado, o MySQL legado é dependência dura:
			// se ele cair, o /readyz precisa reprovar, senão o orquestrador mantém
			// a instância em rotação enquanto todo agendamento falha.
			if schedulingReady != nil {
				return schedulingReady(ctx)
			}
			return nil
		},
		Auth:       auth,
		Scheduling: scheduling,
		CareAdmin:  careAdmin,
		Journey:    journey,
		Internal:   internal,
		AdminToken: cfg.AdminToken,
		// O cadastro precisa de um teto maior que o de uma rota normal: ele
		// espera a DAV de forma síncrona. Derivar do orçamento evita que os dois
		// números divirjam e a última tentativa seja sempre cortada.
		RegisterTimeout: cfg.DAVBudget() + 10*time.Second,
		// O agendamento faz UMA tentativa (escrita na DAV nunca repete), então o
		// orçamento dele é um timeout + folga, e não o DAVBudget inteiro. Prometer
		// 62s ao paciente numa rota que faz uma tentativa só seria mentira.
		BookTimeout: cfg.DAVTimeout + 10*time.Second,
	})

	srv := &http.Server{
		Addr:        cfg.HTTPAddr,
		Handler:     router,
		ReadTimeout: cfg.ReadTimeout,
		// WriteTimeout vale para todas as rotas, mas o cadastro é síncrono contra
		// a DAV e pode passar de 15s. O handler de /auth/register estende o
		// próprio prazo com SetWriteDeadline — ver internal/controllers.
		WriteTimeout: cfg.WriteTimeout,
	}

	// Sobe o servidor e aguarda sinal de término para shutdown gracioso.
	serverErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return err
	case sig := <-stop:
		logger.Info("sinal recebido, encerrando", "signal", sig.String())
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// newAuthController monta a cadeia do cadastro: DAV -> models -> controller.
//
// Sem credencial da DAV, devolve nil e as rotas de /auth não sobem. Isso só
// acontece em `local` — a config exige a credencial em staging/produção, então
// um deploy sem ela falha antes daqui, na subida.
func newAuthController(cfg config.Config, pool *pgxpool.Pool, logger *slog.Logger) (*controllers.AuthController, error) {
	if cfg.DAVBaseURL == "" || cfg.DAVAPIKey == "" {
		logger.Warn("rotas de autenticação DESLIGADAS: faltam RENOVI_DAV_BASE_URL/RENOVI_DAV_API_KEY")
		return nil, nil
	}

	davClient, err := dav.New(dav.Config{
		BaseURL:     cfg.DAVBaseURL,
		APIKey:      cfg.DAVAPIKey,
		Timeout:     cfg.DAVTimeout,
		MaxAttempts: cfg.DAVMaxAttempts,
		Logger:      logger,
	})
	if err != nil {
		return nil, err
	}

	return &controllers.AuthController{
		Accounts:     models.NewAccountStore(pool, davClient),
		Sessions:     models.NewSessionStore(pool, cfg.SessionTTL),
		CookieSecure: cfg.SessionCookieSecure,
		SessionTTL:   cfg.SessionTTL,
		// Precisa cobrir o orçamento da DAV com folga, senão o servidor corta a
		// resposta de um cadastro que deu certo.
		RegisterDeadline: cfg.DAVBudget() + 30*time.Second,
	}, nil
}

func newLogger(cfg config.Config) *slog.Logger {
	var level slog.Level
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}
	if cfg.IsProduction() {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}

// newAgendaClient abre o pool do MySQL legado, COMPARTILHADO pelo agendamento e
// pelo admin. Sem DSN do legado devolve nil (e nil no fechador): as rotas que
// dependem dele simplesmente não sobem. Só acontece em `local`.
func newAgendaClient(cfg config.Config, logger *slog.Logger) (*agenda.Client, func(), error) {
	if cfg.LegacyDatabaseURL == "" {
		return nil, nil, nil
	}
	client, err := agenda.New(agenda.Config{
		DSN:    cfg.LegacyDatabaseURL,
		Logger: logger,
	})
	if err != nil {
		return nil, nil, err
	}
	return client, func() { _ = client.Close() }, nil
}

// newSchedulingController monta a cadeia do agendamento: legado + DAV -> models
// -> controller. Recebe o cliente do legado já pronto (compartilhado) e devolve
// TAMBÉM o BookingStore (a jornada agenda por ele — um store só, uma verdade só)
// e um ping de readiness (nil se desligado).
//
// Sem cliente do legado ou sem credencial da DAV, devolve nil e o agendamento não
// sobe — mesma política do cadastro sem credencial da DAV. Só acontece em `local`.
func newSchedulingController(cfg config.Config, pool *pgxpool.Pool, agendaClient *agenda.Client, logger *slog.Logger) (*controllers.SchedulingController, *models.BookingStore, func(context.Context) error, error) {
	if agendaClient == nil || cfg.DAVBaseURL == "" || cfg.DAVAPIKey == "" {
		logger.Warn("rotas de agendamento DESLIGADAS: faltam RENOVI_LEGACY_DATABASE_URL ou credenciais da DAV")
		return nil, nil, nil, nil
	}

	davClient, err := dav.New(dav.Config{
		BaseURL:     cfg.DAVBaseURL,
		APIKey:      cfg.DAVAPIKey,
		Timeout:     cfg.DAVTimeout,
		MaxAttempts: cfg.DAVMaxAttempts,
		Logger:      logger,
	})
	if err != nil {
		return nil, nil, nil, err
	}

	// A política da janela é montada AQUI, e não na config, para que o pacote de
	// config não precise conhecer tipo de model — a dependência correria ao
	// contrário. O main é justamente o lugar onde as camadas se encontram.
	policy := scheduling.Policy{OpensBefore: cfg.JoinOpensBefore, ClosesAfter: cfg.JoinClosesAfter}

	store := models.NewBookingStore(pool, agendaClient, davClient, policy, logger)
	ctrl := &controllers.SchedulingController{
		Bookings: store,
		// Deriva do timeout da DAV com folga, como o RegisterDeadline do cadastro:
		// o deadline de escrita precisa ser > que o timeout da rota (DAVTimeout+10s,
		// ver router), senão o servidor corta a resposta de um agendamento que deu
		// certo. Manter os dois derivados da mesma fonte evita que divirjam.
		BookDeadline: cfg.DAVTimeout + 30*time.Second,
	}
	return ctrl, store, agendaClient.Ping, nil
}

// newJourneyControllers monta a jornada do paciente (/me/*) e, se o ambiente
// permitir, o endpoint interno de teste. Os dois usam o MESMO JourneyStore.
//
//   - Journey só sobe quando o scheduling subiu (bookingStore != nil): a jornada
//     agenda pelo booking, e sem ele /me/journey mentiria "pode agendar".
//   - Internal só sobe com RENOVI_TEST_ENDPOINTS (proibido em produção pela
//     config) — em produção a rota simplesmente não existe.
func newJourneyControllers(cfg config.Config, pool *pgxpool.Pool, bookingStore *models.BookingStore, logger *slog.Logger) (*controllers.JourneyController, *controllers.InternalController) {
	if bookingStore == nil {
		logger.Warn("rotas da jornada DESLIGADAS: agendamento não montado (a jornada agenda pelo booking)")
		return nil, nil
	}

	store := models.NewJourneyStore(models.NewJourneyRepo(pool), bookingStore, cfg.CancelCountThreshold, logger)
	journey := &controllers.JourneyController{
		Journeys: store,
		// Mesma derivação do scheduling: o deadline de escrita precisa cobrir a
		// chamada síncrona à DAV com folga sobre o timeout da rota.
		BookDeadline: cfg.DAVTimeout + 30*time.Second,
	}

	var internal *controllers.InternalController
	if cfg.TestEndpoints {
		logger.Warn("rotas internas de TESTE ligadas (RENOVI_TEST_ENDPOINTS) — nunca use em produção")
		internal = &controllers.InternalController{Journeys: store}
	}
	return journey, internal
}

// newCareAdminController monta as rotas /admin (catálogo + matrícula). Duas
// condições para subir, ambas registradas em log quando falham:
//
//   - RENOVI_ADMIN_TOKEN presente: sem token não há como autenticar a operação, e
//     montar as rotas sem trava seria expor a criação de linhas e matrículas.
//   - agenda disponível: a publicação de uma linha VALIDA as especialidades contra
//     o legado; sem esse cliente o publish não teria como confirmar o template.
func newCareAdminController(cfg config.Config, pool *pgxpool.Pool, agendaClient *agenda.Client, logger *slog.Logger) *controllers.CareLineAdminController {
	if cfg.AdminToken == "" {
		logger.Warn("rotas admin DESLIGADAS: RENOVI_ADMIN_TOKEN vazio")
		return nil
	}
	if agendaClient == nil {
		logger.Warn("rotas admin DESLIGADAS: sem RENOVI_LEGACY_DATABASE_URL (o publish valida especialidades no legado)")
		return nil
	}
	return &controllers.CareLineAdminController{
		Catalog:     models.NewCareLineStore(pool, agendaClient),
		Enrollments: models.NewEnrollmentStore(pool),
	}
}
