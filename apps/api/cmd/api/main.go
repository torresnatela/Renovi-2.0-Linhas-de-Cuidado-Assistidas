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

	"github.com/renovisaude/renovi-care/internal/config"
	"github.com/renovisaude/renovi-care/internal/db"
	apihttp "github.com/renovisaude/renovi-care/internal/http"
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
	logger.Info("iniciando renovi-care", "version", version, "env", cfg.Env, "addr", cfg.HTTPAddr)

	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.CareDatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	router := apihttp.NewRouter(apihttp.Deps{
		Logger:  logger,
		Version: version,
		Ready: func(ctx context.Context) error {
			return db.Ping(ctx, pool)
		},
	})

	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      router,
		ReadTimeout:  cfg.ReadTimeout,
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
