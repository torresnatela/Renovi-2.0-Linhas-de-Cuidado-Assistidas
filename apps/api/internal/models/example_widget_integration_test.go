//go:build integration

package models_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/db"
	"github.com/renovisaude/renovi-care/internal/models"
	"github.com/renovisaude/renovi-care/internal/testsupport"
)

// Teste de INTEGRAÇÃO (tag `integration`): sobe Postgres real, aplica migrations
// e exercita o repository sqlc de ponta a ponta. Rode com `make test-integration`.
func TestExampleWidgetStore_CreateAndGet(t *testing.T) {
	ctx := context.Background()
	dsn := testsupport.StartPostgres(t)

	pool, err := db.Connect(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	store := models.NewExampleWidgetStore(pool)

	created, err := store.Create(ctx, "Widget A", "ACTIVE", json.RawMessage(`{"k":"v"}`))
	require.NoError(t, err)
	assert.NotEqual(t, created.ID.String(), "00000000-0000-0000-0000-000000000000")
	assert.Equal(t, "Widget A", created.Name)

	got, err := store.Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
	assert.Equal(t, "ACTIVE", got.Status)
	assert.JSONEq(t, `{"k":"v"}`, string(got.Config))
}
