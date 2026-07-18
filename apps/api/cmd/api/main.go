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

	scheduling, schedulingReady, closeAgenda, err := newSchedulingController(cfg, pool, logger)
	if err != nil {
		return err
	}
	if closeAgenda != nil {
		defer closeAgenda()
	}

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

// newSchedulingController monta a cadeia do agendamento: legado + DAV -> models
// -> controller. Devolve também um ping de readiness (nil se desligado) e o
// fechador do pool do legado.
//
// Sem DSN do legado, devolve nil e o agendamento não sobe — mesma política do
// cadastro sem credencial da DAV. Só acontece em `local`.
func newSchedulingController(cfg config.Config, pool *pgxpool.Pool, logger *slog.Logger) (*controllers.SchedulingController, func(context.Context) error, func(), error) {
	if cfg.LegacyDatabaseURL == "" || cfg.DAVBaseURL == "" || cfg.DAVAPIKey == "" {
		logger.Warn("rotas de agendamento DESLIGADAS: faltam RENOVI_LEGACY_DATABASE_URL ou credenciais da DAV")
		return nil, nil, nil, nil
	}

	agendaClient, err := agenda.New(agenda.Config{
		DSN:    cfg.LegacyDatabaseURL,
		Logger: logger,
	})
	if err != nil {
		return nil, nil, nil, err
	}

	davClient, err := dav.New(dav.Config{
		BaseURL:     cfg.DAVBaseURL,
		APIKey:      cfg.DAVAPIKey,
		Timeout:     cfg.DAVTimeout,
		MaxAttempts: cfg.DAVMaxAttempts,
		Logger:      logger,
	})
	if err != nil {
		_ = agendaClient.Close()
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
	return ctrl, agendaClient.Ping, func() { _ = agendaClient.Close() }, nil
}
