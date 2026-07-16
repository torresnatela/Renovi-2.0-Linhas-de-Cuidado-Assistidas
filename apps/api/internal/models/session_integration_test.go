//go:build integration

package models_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/models"
)

func newSessionStore(t *testing.T) (*models.SessionStore, *models.AccountStore, *pgxpool.Pool) {
	t.Helper()
	accounts, pool := newStore(t, &fakeDAV{})
	return models.NewSessionStore(pool, time.Hour), accounts, pool
}

func registeredAccount(t *testing.T, accounts *models.AccountStore) models.Account {
	t.Helper()
	acc, err := accounts.Register(context.Background(), validInput())
	require.NoError(t, err)
	return acc
}

func TestSession_CriaEValida(t *testing.T) {
	sessions, accounts, _ := newSessionStore(t)
	acc := registeredAccount(t, accounts)

	token, expiresAt, err := sessions.Create(context.Background(), acc.ID)
	require.NoError(t, err)
	require.NotEmpty(t, token)
	require.True(t, expiresAt.After(time.Now()))

	got, err := sessions.Validate(context.Background(), token)
	require.NoError(t, err)
	require.Equal(t, acc.ID, got.ID)
	require.Equal(t, acc.Email, got.Email)
}

// O banco só pode guardar o HASH. Quem ler um dump não pode se passar por ninguém.
func TestSession_BancoNuncaGuardaOTokenEmClaro(t *testing.T) {
	sessions, accounts, pool := newSessionStore(t)
	acc := registeredAccount(t, accounts)

	token, _, err := sessions.Create(context.Background(), acc.ID)
	require.NoError(t, err)

	// Procura o token cru em qualquer forma textual da tabela.
	var achou bool
	err = pool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM session
			WHERE encode(token_hash, 'escape') = $1
			   OR encode(token_hash, 'hex')    = $1
			   OR encode(token_hash, 'base64') = $1
		)`, token).Scan(&achou)
	require.NoError(t, err)
	require.False(t, achou, "o token cru está recuperável a partir da tabela session")

	// E o que está gravado é um SHA-256 (32 bytes).
	var tamanho int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT octet_length(token_hash) FROM session`).Scan(&tamanho))
	require.Equal(t, 32, tamanho)
}

func TestSession_TokensSaoUnicos(t *testing.T) {
	sessions, accounts, _ := newSessionStore(t)
	acc := registeredAccount(t, accounts)

	a, _, err := sessions.Create(context.Background(), acc.ID)
	require.NoError(t, err)
	b, _, err := sessions.Create(context.Background(), acc.ID)
	require.NoError(t, err)

	require.NotEqual(t, a, b, "dois logins produziram o mesmo token")
	// Entropia suficiente para não ser adivinhável (32 bytes -> 43 chars base64url).
	require.GreaterOrEqual(t, len(a), 43)
}

func TestSession_ValidateRecusa(t *testing.T) {
	sessions, accounts, pool := newSessionStore(t)
	acc := registeredAccount(t, accounts)

	t.Run("token inexistente", func(t *testing.T) {
		_, err := sessions.Validate(context.Background(), "token-que-nunca-existiu")
		require.ErrorIs(t, err, models.ErrNoSession)
	})

	t.Run("token vazio", func(t *testing.T) {
		_, err := sessions.Validate(context.Background(), "")
		require.ErrorIs(t, err, models.ErrNoSession)
	})

	t.Run("token revogado", func(t *testing.T) {
		token, _, err := sessions.Create(context.Background(), acc.ID)
		require.NoError(t, err)
		require.NoError(t, sessions.Revoke(context.Background(), token))

		_, err = sessions.Validate(context.Background(), token)
		require.ErrorIs(t, err, models.ErrNoSession, "logout precisa matar a sessão na hora")
	})

	t.Run("token expirado", func(t *testing.T) {
		token, _, err := sessions.Create(context.Background(), acc.ID)
		require.NoError(t, err)

		// Envelhece a sessão no banco em vez de esperar o relógio.
		_, err = pool.Exec(context.Background(),
			`UPDATE session SET expires_at = now() - interval '1 second'`)
		require.NoError(t, err)

		_, err = sessions.Validate(context.Background(), token)
		require.ErrorIs(t, err, models.ErrNoSession)
	})

	// Bloquear a conta precisa derrubar as sessões dela imediatamente — é o que
	// dá sentido a ter escolhido sessão opaca em vez de JWT.
	t.Run("conta bloqueada depois de logar", func(t *testing.T) {
		token, _, err := sessions.Create(context.Background(), acc.ID)
		require.NoError(t, err)

		_, err = pool.Exec(context.Background(),
			`UPDATE patient_account SET status = 'BLOCKED' WHERE id = $1`, acc.ID)
		require.NoError(t, err)

		_, err = sessions.Validate(context.Background(), token)
		require.ErrorIs(t, err, models.ErrNoSession, "conta bloqueada não pode ter sessão viva")
	})
}

func TestSession_RevokeDeTokenInexistenteNaoErra(t *testing.T) {
	sessions, _, _ := newSessionStore(t)
	// Logout com cookie velho é rotina, não erro.
	require.NoError(t, sessions.Revoke(context.Background(), "token-qualquer"))
}
