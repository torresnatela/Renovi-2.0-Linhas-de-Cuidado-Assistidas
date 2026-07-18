//go:build integration

// Package testsupport oferece utilitários para os testes de INTEGRAÇÃO (tag
// `integration`), que sobem dependências reais via testcontainers-go.
//
// Rode com: `make test-integration` (exige Docker rodando).
package testsupport

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/renovisaude/renovi-care/internal/db"
)

// StartPostgres sobe um Postgres efêmero, aplica as migrations do renovi_care e
// devolve a URL de conexão (superusuário renovi). O container é derrubado
// automaticamente ao fim do teste.
func StartPostgres(t *testing.T) string {
	t.Helper()
	superDSN, _ := StartPostgresDSNs(t)
	return superDSN
}

// StartPostgresDSNs sobe o mesmo container do StartPostgres e devolve DUAS URLs:
//
//   - superDSN: o superusuário `renovi`, que roda as migrations (é o "owner").
//   - appDSN:   o role restrito `renovi_app`, como a aplicação conecta em runtime.
//
// O role renovi_app é criado pela migration 0008 (com a mesma senha do usuário),
// então appDSN só é utilizável DEPOIS de `db.MigrateUp`. É por esse role que o
// append-only de journey_event vale no banco — ver approle_integration_test.go.
func StartPostgresDSNs(t *testing.T) (superDSN, appDSN string) {
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

	superDSN, err = container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err, "obter connection string")

	require.NoError(t, db.MigrateUp(superDSN), "aplicar migrations")

	// A migration 0008 criou renovi_app com senha 'renovi_app'. O appDSN é o
	// superDSN com o par usuário:senha trocado — só a credencial (userinfo) muda,
	// nunca o dbname 'renovi_care' (que não contém 'renovi:renovi@').
	appDSN = strings.Replace(superDSN, "renovi:renovi@", "renovi_app:renovi_app@", 1)
	return superDSN, appDSN
}
