//go:build integration

package models_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/models"
	"github.com/renovisaude/renovi-care/internal/testsupport"
)

// TestInstrumentStore_Config confere que o seed da migration 0011 carrega: o GRID
// (anel diário) com suas dimensões, os rótulos de emoção e as tags de contexto.
func TestInstrumentStore_Config(t *testing.T) {
	ctx := context.Background()
	dsn := testsupport.StartPostgres(t)
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	store := models.NewInstrumentStore(pool)

	grid, err := store.Config(ctx, "GRID")
	require.NoError(t, err)
	require.Equal(t, "GRID", grid.Codigo)
	require.Equal(t, "diario", grid.Anel)
	require.Len(t, grid.Dimensions, 2)
	require.Len(t, grid.EmotionLabels, 12)
	require.Len(t, grid.ContextTags, 6)

	var valencia *models.InstrumentDimension
	for i := range grid.Dimensions {
		if grid.Dimensions[i].Dimensao == "valencia" {
			valencia = &grid.Dimensions[i]
		}
	}
	require.NotNil(t, valencia, "GRID tem dimensão valencia")
	require.Equal(t, 0.0, valencia.MinScore)
	require.Equal(t, 100.0, valencia.MaxScore)

	who5, err := store.Config(ctx, "WHO5")
	require.NoError(t, err)
	require.Equal(t, "semanal", who5.Anel)

	phq4, err := store.Config(ctx, "PHQ4")
	require.NoError(t, err)
	require.Equal(t, "gatilhado", phq4.Anel)
	require.Len(t, phq4.Dimensions, 2) // depressao + ansiedade

	_, err = store.Config(ctx, "XPTO")
	require.ErrorIs(t, err, models.ErrInstrumentNotFound)
}
