// Package db concentra o acesso ao Postgres renovi_care: pool de conexões (pgx)
// e execução de migrations (golang-migrate). O código SQL tipado é gerado pelo
// sqlc no subpacote gen/ (rode `make generate`).
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect cria um pool de conexões para o Postgres renovi_care.
//
// pgxpool.New é preguiçoso: não abre conexão imediatamente, então o serviço sobe
// mesmo com o banco temporariamente indisponível. Use Ping para checar prontidão
// (ver /readyz).
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("db: criar pool: %w", err)
	}
	return pool, nil
}

// Ping verifica se o banco está acessível.
func Ping(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return fmt.Errorf("db: pool não inicializado")
	}
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("db: ping falhou: %w", err)
	}
	return nil
}
