// Command worker hospeda os jobs de cron do renovi-care.
//
// STATUS: STUB (fundação). No MVP entram aqui (ver SPEC §5.2/§5.3):
//   - reconciliação de agendamento (PENDING_LEGACY, retry DAV) — cron 5 min;
//   - auto-conclusão de consultas (CONFIRMED vencidas -> COMPLETED) — cron 15 min;
//   - lembretes por e-mail (P1).
package main

import (
	"log/slog"
	"os"

	"github.com/renovisaude/renovi-care/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("worker: config", "error", err)
		os.Exit(1)
	}
	slog.Info("worker stub — nenhum job registrado ainda", "env", cfg.Env)
	// TODO(mvp): registrar e agendar os jobs de reconciliação e auto-conclusão.
}
