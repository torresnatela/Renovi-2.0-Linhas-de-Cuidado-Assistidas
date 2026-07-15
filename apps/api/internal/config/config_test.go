package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Defaults(t *testing.T) {
	// Sem variáveis setadas, Load deve aplicar os defaults de dev e validar OK.
	t.Setenv("RENOVI_ENV", "")
	t.Setenv("RENOVI_HTTP_ADDR", "")
	t.Setenv("RENOVI_CARE_DATABASE_URL", "")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, EnvLocal, cfg.Env)
	assert.Equal(t, ":8090", cfg.HTTPAddr)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.NotEmpty(t, cfg.CareDatabaseURL)
	assert.False(t, cfg.IsProduction())
}

func TestLoad_OverrideFromEnv(t *testing.T) {
	t.Setenv("RENOVI_ENV", "production")
	t.Setenv("RENOVI_HTTP_ADDR", ":9090")
	t.Setenv("RENOVI_HTTP_READ_TIMEOUT", "30s")
	t.Setenv("RENOVI_CARE_DATABASE_URL", "postgres://u:p@db:5432/care")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, ":9090", cfg.HTTPAddr)
	assert.Equal(t, 30*time.Second, cfg.ReadTimeout)
	assert.True(t, cfg.IsProduction())
}

func TestLoad_InvalidLogLevel(t *testing.T) {
	t.Setenv("RENOVI_LOG_LEVEL", "verbose")

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RENOVI_LOG_LEVEL")
}

func TestLoad_InvalidEnv(t *testing.T) {
	// "prod" não é um ambiente válido — deve falhar em vez de silenciosamente
	// desligar o comportamento de produção.
	t.Setenv("RENOVI_ENV", "prod")

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RENOVI_ENV")
}

func TestLoad_ProductionRequiresDatabaseURL(t *testing.T) {
	t.Setenv("RENOVI_ENV", "production")
	t.Setenv("RENOVI_CARE_DATABASE_URL", "")

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RENOVI_CARE_DATABASE_URL")
}

func TestLoad_InvalidDuration(t *testing.T) {
	// "60" sem unidade é malformado; deve falhar em vez de virar o default 15s.
	t.Setenv("RENOVI_HTTP_READ_TIMEOUT", "60")

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RENOVI_HTTP_READ_TIMEOUT")
}
