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

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/renovisaude/renovi-care/internal/adapters/dav"
	"github.com/renovisaude/renovi-care/internal/config"
	"github.com/renovisaude/renovi-care/internal/controllers"
	"github.com/renovisaude/renovi-care/internal/db"
	apihttp "github.com/renovisaude/renovi-care/internal/http"
	"github.com/renovisaude/renovi-care/internal/models"
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

	router := apihttp.NewRouter(apihttp.Deps{
		Logger:  logger,
		Version: version,
		Ready: func(ctx context.Context) error {
			return db.Ping(ctx, pool)
		},
		Auth: auth,
		// O cadastro precisa de um teto maior que o de uma rota normal: ele
		// espera a DAV de forma síncrona. Derivar do orçamento evita que os dois
		// números divirjam e a última tentativa seja sempre cortada.
		RegisterTimeout: cfg.DAVBudget() + 10*time.Second,
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
