// Command backfill-cpf-hmac preenche patient_identity.cpf_hmac nas linhas antigas.
//
// A coluna cpf_hmac nasce NULL (migration 0016) e o cadastro só passa a gravá-la
// depois que a integração da Gestão entra. Este comando cobre os pacientes que já
// existiam: lê o CPF em claro, calcula o HMAC com RENOVI_CPF_PEPPER e grava. É
// idempotente (só toca linhas com cpf_hmac ainda NULL) e pode rodar quantas vezes
// precisar. Roda FORA do fluxo normal (one-shot de operação), como o cmd/migrate.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/renovisaude/renovi-care/internal/config"
	"github.com/renovisaude/renovi-care/internal/models/cpf"
)

func main() {
	if err := run(); err != nil {
		slog.Error("backfill-cpf-hmac falhou", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pepper := []byte(cfg.CPFPepper)
	if len(pepper) == 0 {
		return fmt.Errorf("RENOVI_CPF_PEPPER é obrigatório para o backfill (sem pepper não há hmac)")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.CareDatabaseURL)
	if err != nil {
		return fmt.Errorf("conectar ao banco: %w", err)
	}
	defer pool.Close()

	// Coleta tudo ANTES de atualizar: a conexão fica ocupada enquanto as rows do
	// Query estão abertas, e um Exec no meio quebraria.
	type pendente struct {
		accountID uuid.UUID
		cpfRaw    string
	}
	var pendentes []pendente
	rows, err := pool.Query(ctx, `SELECT account_id, cpf FROM patient_identity WHERE cpf_hmac IS NULL`)
	if err != nil {
		return fmt.Errorf("listar identidades sem cpf_hmac: %w", err)
	}
	for rows.Next() {
		var p pendente
		if err := rows.Scan(&p.accountID, &p.cpfRaw); err != nil {
			rows.Close()
			return fmt.Errorf("ler linha: %w", err)
		}
		pendentes = append(pendentes, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("varrer identidades: %w", err)
	}

	var atualizadas, ignoradas int
	for _, p := range pendentes {
		c, err := cpf.Parse(p.cpfRaw)
		if err != nil {
			// CPF inválido no banco não deveria existir (o cadastro valida). Pular e
			// seguir é melhor que abortar o lote inteiro; o account_id fica no log.
			slog.Warn("cpf inválido no banco, pulando", "account_id", p.accountID)
			ignoradas++
			continue
		}
		h, err := c.HMAC(pepper)
		if err != nil {
			return err
		}
		// Idempotente até na corrida: só grava se ainda estiver NULL.
		if _, err := pool.Exec(ctx,
			`UPDATE patient_identity SET cpf_hmac = $1 WHERE account_id = $2 AND cpf_hmac IS NULL`,
			h, p.accountID); err != nil {
			return fmt.Errorf("gravar cpf_hmac de %s: %w", p.accountID, err)
		}
		atualizadas++
	}

	fmt.Printf("backfill concluído: %d atualizada(s), %d ignorada(s) por cpf inválido\n", atualizadas, ignoradas)
	return nil
}
