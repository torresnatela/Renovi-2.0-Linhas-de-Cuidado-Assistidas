//go:build integration

// Package testsupport oferece utilitários para os testes de INTEGRAÇÃO (tag
// `integration`), que sobem dependências reais via testcontainers-go.
//
// Rode com: `make test-integration` (exige Docker rodando).
package testsupport

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/renovisaude/renovi-care/internal/db"
)

// StartPostgres sobe um Postgres efêmero, aplica as migrations do renovi_care e
// devolve a URL de conexão. O container é derrubado automaticamente ao fim do teste.
func StartPostgres(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("renovi_care"),
		postgres.WithUsername("renovi"),
		postgres.WithPassword("renovi"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err, "subir container postgres")

	t.Cleanup(func() {
		_ = container.Terminate(context.Background())
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err, "obter connection string")

	require.NoError(t, db.MigrateUp(dsn), "aplicar migrations")
	return dsn
}
