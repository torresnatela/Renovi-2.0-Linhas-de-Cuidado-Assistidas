// Command migrate aplica/reverte as migrations do banco renovi_care.
//
// Uso:
//
//	migrate up                 # aplica todas as pendentes
//	migrate down [n]           # reverte n passos (default 1)
//	migrate version            # mostra a versão atual
//
// A URL do banco vem de RENOVI_CARE_MIGRATE_DATABASE_URL (ver internal/config):
// as migrations rodam como OWNER, pois criam tabelas e o role restrito renovi_app.
// Quando essa variável não é definida, cai em RENOVI_CARE_DATABASE_URL.
package main

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/renovisaude/renovi-care/internal/config"
	"github.com/renovisaude/renovi-care/internal/db"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("migrate falhou", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("uso: migrate <up|down|version> [n]")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	url := cfg.CareMigrateDatabaseURL

	switch args[0] {
	case "up":
		if err := db.MigrateUp(url); err != nil {
			return err
		}
		fmt.Println("migrations aplicadas")
		return nil
	case "down":
		steps := 1
		if len(args) > 1 {
			if steps, err = strconv.Atoi(args[1]); err != nil {
				return fmt.Errorf("n inválido: %w", err)
			}
		}
		// n deve ser positivo: um valor negativo inverteria a direção
		// (m.Steps(-(-1)) = m.Steps(1)) e aplicaria migrations em vez de reverter.
		if steps <= 0 {
			return fmt.Errorf("n deve ser positivo (recebido %d)", steps)
		}
		if err := db.MigrateDown(url, steps); err != nil {
			return err
		}
		fmt.Printf("revertidos %d passo(s)\n", steps)
		return nil
	case "version":
		v, dirty, err := db.MigrateVersion(url)
		if err != nil {
			return err
		}
		fmt.Printf("versão=%d dirty=%t\n", v, dirty)
		return nil
	default:
		return fmt.Errorf("comando desconhecido: %q (use up|down|version)", args[0])
	}
}
