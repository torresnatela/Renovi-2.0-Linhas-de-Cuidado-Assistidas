package models

import (
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/db/gen"
)

// TestHashInviteToken: o hash guardado é o SHA-256 do token cru. Vetor conferido por
// fora (`printf '%s' 'convite-abc' | shasum -a 256`), não recomputado do mesmo jeito.
func TestHashInviteToken(t *testing.T) {
	const want = "9b77fb741d1aae2c6c83a62dd08515af6802dc0608cc14ab457338689e67f53c"
	got := hashInviteToken("convite-abc")

	require.Len(t, got, 32, "hash deve ter 32 bytes")
	require.Equal(t, want, hex.EncodeToString(got))
	require.Equal(t, got, hashInviteToken("convite-abc"), "determinístico")
	require.NotEqual(t, got, hashInviteToken("convite-abd"), "muda com a entrada")
}

// TestClassifyToken: prioridade usado > revogado > expirado; um convite vivo passa.
func TestClassifyToken(t *testing.T) {
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	future := now.Add(24 * time.Hour)
	past := now.Add(-1 * time.Hour)
	ts := func(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

	cases := []struct {
		name string
		tok  gen.OnboardingToken
		want error
	}{
		{"vivo", gen.OnboardingToken{ExpiresAt: future}, nil},
		{"usado", gen.OnboardingToken{ExpiresAt: future, UsedAt: ts(past)}, ErrTokenUsed},
		{"revogado", gen.OnboardingToken{ExpiresAt: future, RevokedAt: ts(past)}, ErrTokenRevoked},
		{"expirado", gen.OnboardingToken{ExpiresAt: past}, ErrTokenExpired},
		{"expira exatamente agora conta como expirado", gen.OnboardingToken{ExpiresAt: now}, ErrTokenExpired},
		// Usado E expirado: 'usado' é a informação mais útil (foi concluído).
		{"usado vence expirado", gen.OnboardingToken{ExpiresAt: past, UsedAt: ts(past)}, ErrTokenUsed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := classifyToken(tc.tok, now)
			if tc.want == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.want)
		})
	}
}

// TestEnsurePendente: só 'pendente' segue; os demais status viram o erro coerente.
func TestEnsurePendente(t *testing.T) {
	cases := []struct {
		status string
		want   error
	}{
		{"pendente", nil},
		{"vinculado", ErrOnboardingAlreadyDone},
		{"recusado", ErrOnboardingDeclined},
		{"cancelado", ErrTokenRevoked},
	}
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			err := ensurePendente(tc.status)
			if tc.want == nil {
				require.NoError(t, err)
				return
			}
			require.True(t, errors.Is(err, tc.want))
		})
	}
}
