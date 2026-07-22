package config

import (
	"bytes"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Segredos FICTÍCIOS para os testes da ingestão da Gestão (ADR-043).
const (
	pepperDeTeste = "PEPPER0FICTICIO0DE0TESTE0NAO0USE"
	tokenDeTeste  = "TOKEN0DE0INTEGRACAO0FICTICIO0NAO0USE"
)

func TestLoad_IngestaoDefaults(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	// 7 dias é o TTL do convite (INVITE_TTL) do piloto.
	assert.Equal(t, 168*time.Hour, cfg.InviteTTL)
	// Sem pepper e sem token, a integração da Gestão nasce DESLIGADA.
	assert.Empty(t, cfg.CPFPepper)
	assert.Empty(t, cfg.GestaoIntegrationToken)
	// Em local, a URL do front cai no default de dev (Vite).
	assert.Equal(t, "http://localhost:5173", cfg.WebBaseURL)
}

func TestLoad_IngestaoDoAmbiente(t *testing.T) {
	t.Setenv("RENOVI_CPF_PEPPER", pepperDeTeste)
	t.Setenv("RENOVI_GESTAO_INTEGRATION_TOKEN", tokenDeTeste)
	t.Setenv("RENOVI_INVITE_TTL", "48h")
	t.Setenv("RENOVI_WEB_BASE_URL", "https://app.renovisaude.com.br")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, pepperDeTeste, cfg.CPFPepper)
	assert.Equal(t, tokenDeTeste, cfg.GestaoIntegrationToken)
	assert.Equal(t, 48*time.Hour, cfg.InviteTTL)
	assert.Equal(t, "https://app.renovisaude.com.br", cfg.WebBaseURL)
}

func TestLoad_RejeitaInviteTTLInvalido(t *testing.T) {
	t.Setenv("RENOVI_INVITE_TTL", "sete-dias")
	_, err := Load()
	require.Error(t, err)
}

// O pepper e o token de integração são segredos: nunca podem ir para o log, só o
// fato de estarem setados (mesma trava do DAV_API_KEY / ADMIN_TOKEN).
func TestConfig_LogValueRedigeIngestao(t *testing.T) {
	t.Setenv("RENOVI_CPF_PEPPER", pepperDeTeste)
	t.Setenv("RENOVI_GESTAO_INTEGRATION_TOKEN", tokenDeTeste)
	t.Setenv("RENOVI_WEB_BASE_URL", "https://app.renovisaude.com.br")

	cfg, err := Load()
	require.NoError(t, err)

	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("subindo", "config", cfg)
	logs := buf.String()

	assert.NotContains(t, logs, pepperDeTeste, "o pepper do CPF foi para o log")
	assert.NotContains(t, logs, tokenDeTeste, "o token de integração foi para o log")
	assert.Contains(t, logs, "cpf_pepper_set")
	assert.Contains(t, logs, "gestao_integration_token_set")
	// A URL do front não é segredo e é útil para conferir o ambiente.
	assert.Contains(t, logs, "app.renovisaude.com.br")
}

// Com a integração ligada (token setado), montar o invite_url exige a URL do front.
// Sem ela, um convite apontaria para lugar nenhum — falhar na subida é mais barato.
func TestLoad_WebBaseURLObrigatorioComIntegracao(t *testing.T) {
	base := map[string]string{
		"RENOVI_ENV":               "staging",
		"RENOVI_CARE_DATABASE_URL": "postgres://x/y",
		"RENOVI_DAV_BASE_URL":      "https://api.v2.doutoraovivo.com.br",
		"RENOVI_DAV_API_KEY":       chaveDeTeste,
	}

	t.Run("token setado sem web base url falha", func(t *testing.T) {
		for k, v := range base {
			t.Setenv(k, v)
		}
		t.Setenv("RENOVI_GESTAO_INTEGRATION_TOKEN", tokenDeTeste)
		t.Setenv("RENOVI_WEB_BASE_URL", "")

		_, err := Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "RENOVI_WEB_BASE_URL")
	})

	t.Run("token setado com web base url passa", func(t *testing.T) {
		for k, v := range base {
			t.Setenv(k, v)
		}
		t.Setenv("RENOVI_GESTAO_INTEGRATION_TOKEN", tokenDeTeste)
		t.Setenv("RENOVI_WEB_BASE_URL", "https://app.renovisaude.com.br")

		_, err := Load()
		require.NoError(t, err)
	})

	t.Run("sem token, web base url é dispensável", func(t *testing.T) {
		for k, v := range base {
			t.Setenv(k, v)
		}
		t.Setenv("RENOVI_WEB_BASE_URL", "")

		_, err := Load()
		require.NoError(t, err)
	})
}
