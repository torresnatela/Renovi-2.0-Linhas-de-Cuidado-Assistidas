// Command seed aplica os templates de linha de cuidado (arquivos JSON versionados
// em apps/api/seeds/care_lines/) no banco renovi_care.
//
// STATUS: STUB (fundação). No MVP este comando lê os seeds, valida a aciclicidade
// das dependências (DAG) e grava care_line_template + care_line_item (+ deps).
// Ver SPEC §3.2 e seeds/care_lines/README.md.
package main

import (
	"log/slog"
	"os"

	"github.com/renovisaude/renovi-care/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("seed: config", "error", err)
		os.Exit(1)
	}
	slog.Info("seed stub — nenhum template aplicado ainda", "env", cfg.Env)
	// TODO(mvp): carregar seeds/care_lines/*.json, validar DAG e gravar templates.
}
