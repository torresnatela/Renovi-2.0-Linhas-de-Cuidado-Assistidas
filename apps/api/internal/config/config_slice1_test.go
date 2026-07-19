package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_AdminToken(t *testing.T) {
	t.Run("default vazio (rotas admin desligadas)", func(t *testing.T) {
		cfg, err := Load()
		require.NoError(t, err)
		assert.Empty(t, cfg.AdminToken)
	})

	t.Run("lê da env", func(t *testing.T) {
		t.Setenv("RENOVI_ADMIN_TOKEN", "token-de-operacao")
		cfg, err := Load()
		require.NoError(t, err)
		assert.Equal(t, "token-de-operacao", cfg.AdminToken)
	})

	t.Run("nunca aparece no LogValue", func(t *testing.T) {
		t.Setenv("RENOVI_ADMIN_TOKEN", "segredo-que-nao-pode-vazar")
		cfg, err := Load()
		require.NoError(t, err)
		// O grupo do LogValue traz admin_token_set=true, mas nunca o valor.
		var setSeen, valueSeen bool
		for _, attr := range cfg.LogValue().Group() {
			if attr.Key == "admin_token_set" {
				setSeen = true
				assert.Equal(t, "true", attr.Value.String())
			}
			if attr.Value.String() == "segredo-que-nao-pode-vazar" {
				valueSeen = true
			}
		}
		assert.True(t, setSeen, "LogValue precisa reportar admin_token_set")
		assert.False(t, valueSeen, "o valor do token NUNCA pode ir para o log")
	})
}

func TestLoad_TestEndpoints(t *testing.T) {
	t.Run("default false", func(t *testing.T) {
		cfg, err := Load()
		require.NoError(t, err)
		assert.False(t, cfg.TestEndpoints)
	})

	t.Run("proibido em produção", func(t *testing.T) {
		t.Setenv("RENOVI_ENV", "production")
		t.Setenv("RENOVI_CARE_DATABASE_URL", "postgres://u:p@db:5432/care")
		t.Setenv("RENOVI_DAV_BASE_URL", "https://api.example")
		t.Setenv("RENOVI_DAV_API_KEY", "chave")
		t.Setenv("RENOVI_TEST_ENDPOINTS", "true")

		_, err := Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "RENOVI_TEST_ENDPOINTS")
	})

	t.Run("permitido fora de produção", func(t *testing.T) {
		t.Setenv("RENOVI_ENV", "staging")
		t.Setenv("RENOVI_CARE_DATABASE_URL", "postgres://u:p@db:5432/care")
		t.Setenv("RENOVI_DAV_BASE_URL", "https://api.example")
		t.Setenv("RENOVI_DAV_API_KEY", "chave")
		t.Setenv("RENOVI_TEST_ENDPOINTS", "true")

		cfg, err := Load()
		require.NoError(t, err)
		assert.True(t, cfg.TestEndpoints)
	})
}

func TestLoad_CancelCountThreshold(t *testing.T) {
	t.Run("default 24h", func(t *testing.T) {
		cfg, err := Load()
		require.NoError(t, err)
		assert.Equal(t, 24*time.Hour, cfg.CancelCountThreshold)
	})

	t.Run("lê da env", func(t *testing.T) {
		t.Setenv("RENOVI_CANCEL_COUNT_THRESHOLD", "12h")
		cfg, err := Load()
		require.NoError(t, err)
		assert.Equal(t, 12*time.Hour, cfg.CancelCountThreshold)
	})

	t.Run("negativo é erro", func(t *testing.T) {
		t.Setenv("RENOVI_CANCEL_COUNT_THRESHOLD", "-1h")
		_, err := Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "RENOVI_CANCEL_COUNT_THRESHOLD")
	})
}
