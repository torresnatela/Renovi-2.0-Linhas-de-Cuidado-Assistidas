//go:build integration

package models_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/models"
	"github.com/renovisaude/renovi-care/internal/testsupport"
)

func newConsentStore(t *testing.T) (*models.ConsentStore, *pgxpool.Pool) {
	t.Helper()
	dsn := testsupport.StartPostgres(t)
	pool, err := pgxpool.New(context.Background(), dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return models.NewConsentStore(pool), pool
}

// TestConsentStore_GrantRevokeReconcede exercita o ciclo de vida do consentimento
// contra Postgres: concessão, idempotência por termo, reconcessão versionada
// (com o índice parcial garantindo um só ativo), Active e revogação.
func TestConsentStore_GrantRevokeReconcede(t *testing.T) {
	ctx := context.Background()
	store, pool := newConsentStore(t)
	patient := insertPatient(t, pool)
	now := time.Now().UTC().Truncate(time.Microsecond)

	// Sem consentimento: Active devolve ErrNoActiveConsent (pré-condição negada).
	_, err := store.Active(ctx, patient, models.ConsentCheckinHumor)
	require.ErrorIs(t, err, models.ErrNoActiveConsent)

	// Finalidade fora da allowlist é rejeitada.
	_, err = store.Grant(ctx, patient, "xpto", "v1", nil, now)
	require.ErrorIs(t, err, models.ErrConsentInvalid)

	// Concede v1.
	c1, err := store.Grant(ctx, patient, models.ConsentCheckinHumor, "v1", nil, now)
	require.NoError(t, err)
	require.Equal(t, "ativo", c1.Status)

	active, err := store.Active(ctx, patient, models.ConsentCheckinHumor)
	require.NoError(t, err)
	require.Equal(t, c1.ID, active.ID)
	require.Equal(t, "v1", active.VersaoTermo)

	// Reconceder o MESMO termo é idempotente: mesmo id, não recria.
	c1b, err := store.Grant(ctx, patient, models.ConsentCheckinHumor, "v1", nil, now.Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, c1.ID, c1b.ID, "mesmo termo não recria o consentimento")

	// Reconceder OUTRA versão revoga o anterior e cria um novo ativo.
	c2, err := store.Grant(ctx, patient, models.ConsentCheckinHumor, "v2", nil, now.Add(2*time.Hour))
	require.NoError(t, err)
	require.NotEqual(t, c1.ID, c2.ID)

	active, err = store.Active(ctx, patient, models.ConsentCheckinHumor)
	require.NoError(t, err)
	require.Equal(t, "v2", active.VersaoTermo)

	// Invariante do índice parcial: no máximo um ativo por (paciente, finalidade).
	var ativos int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM consent WHERE patient_id=$1 AND finalidade=$2 AND status='ativo'`,
		patient, models.ConsentCheckinHumor).Scan(&ativos))
	require.Equal(t, 1, ativos)

	// Revoga: Active volta a ErrNoActiveConsent.
	require.NoError(t, store.Revoke(ctx, patient, models.ConsentCheckinHumor, now.Add(3*time.Hour)))
	_, err = store.Active(ctx, patient, models.ConsentCheckinHumor)
	require.ErrorIs(t, err, models.ErrNoActiveConsent)

	// Revoke é idempotente: sem ativo, é no-op.
	require.NoError(t, store.Revoke(ctx, patient, models.ConsentCheckinHumor, now.Add(4*time.Hour)))
}
