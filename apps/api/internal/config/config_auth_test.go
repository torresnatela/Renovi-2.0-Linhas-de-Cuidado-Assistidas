package config

import (
	"bytes"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Valor FICTÍCIO, no formato de 40 chars alfanuméricos que o AWS API Gateway da
// DAV usa. Nunca ponha a chave real aqui: teste é código versionado.
const chaveDeTeste = "CHAVE0DE0TESTE0FICTICIA0NAO0USE0EM0NADA0"

func TestLoad_DAVDefaults(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	// 30s por tentativa: LOGO ACIMA do teto de 29s do AWS API Gateway na frente
	// da DAV, para que o 504 dele chegue até nós em vez de o nosso cliente
	// desistir antes. Um cadastro real com 10s aqui reprovou todas as tentativas
	// (ver docs/DAV-API-NOTAS.md).
	assert.Equal(t, 30*time.Second, cfg.DAVTimeout)
	assert.Equal(t, 2, cfg.DAVMaxAttempts)
	assert.Equal(t, 12*time.Hour, cfg.SessionTTL)
	assert.True(t, cfg.SessionCookieSecure, "cookie inseguro só por opção explícita")
}

// O orçamento da DAV precisa caber no timeout da rota de cadastro. Quando os
// dois números são independentes eles divergem — e foi assim que a última
// tentativa passou a ser cortada no meio, silenciosamente.
func TestConfig_DAVBudgetCobreTodasAsTentativas(t *testing.T) {
	t.Setenv("RENOVI_DAV_TIMEOUT", "30s")
	t.Setenv("RENOVI_DAV_MAX_ATTEMPTS", "2")

	cfg, err := Load()
	require.NoError(t, err)

	assert.GreaterOrEqual(t, cfg.DAVBudget(), 60*time.Second,
		"o orçamento tem que cobrir 2 tentativas de 30s")
	assert.Less(t, cfg.DAVBudget(), 70*time.Second, "e não pode inflar sem motivo")
}

// Fora de produção a DAV é opcional (dá para subir a API sem credencial), mas em
// produção a ausência precisa falhar alto — como já acontece com o banco. Um
// deploy sem chave falharia depois, no primeiro cadastro, como um 401 misterioso.
func TestLoad_DAVObrigatorioEmProducao(t *testing.T) {
	tests := []struct {
		nome    string
		env     map[string]string
		querErr bool
	}{
		{
			nome:    "produção sem base URL nem chave",
			env:     map[string]string{"RENOVI_ENV": "production", "RENOVI_CARE_DATABASE_URL": "postgres://x/y"},
			querErr: true,
		},
		{
			nome: "produção só com a chave",
			env: map[string]string{"RENOVI_ENV": "production", "RENOVI_CARE_DATABASE_URL": "postgres://x/y",
				"RENOVI_DAV_API_KEY": chaveDeTeste},
			querErr: true,
		},
		{
			nome: "produção completa",
			env: map[string]string{"RENOVI_ENV": "production", "RENOVI_CARE_DATABASE_URL": "postgres://x/y",
				"RENOVI_DAV_API_KEY": chaveDeTeste, "RENOVI_DAV_BASE_URL": "https://api.v2.doutoraovivo.com.br"},
			querErr: false,
		},
		{
			nome:    "local sem DAV é permitido",
			env:     map[string]string{"RENOVI_ENV": "local"},
			querErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.nome, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			_, err := Load()
			if tt.querErr {
				require.Error(t, err)
				assert.NotContains(t, err.Error(), chaveDeTeste, "a mensagem de erro vazou a chave")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestLoad_RejeitaDAVMaxAttemptsInvalido(t *testing.T) {
	t.Setenv("RENOVI_DAV_MAX_ATTEMPTS", "zero")
	_, err := Load()
	require.Error(t, err)
}

// O CLAUDE.md proíbe logar segredo. Um slog.Info("config", "cfg", cfg) é a coisa
// mais natural do mundo de se escrever — e sem LogValue ele despeja a API key no
// stdout de produção. Este teste é a trava.
func TestConfig_LogValueRedigeASenhaEAChave(t *testing.T) {
	t.Setenv("RENOVI_DAV_API_KEY", chaveDeTeste)
	t.Setenv("RENOVI_DAV_BASE_URL", "https://api.v2.hom.doutoraovivo.com.br")
	t.Setenv("RENOVI_CARE_DATABASE_URL", "postgres://renovi:senha-secreta@localhost:5432/renovi_care")

	cfg, err := Load()
	require.NoError(t, err)

	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("subindo", "config", cfg)
	logs := buf.String()

	assert.NotContains(t, logs, chaveDeTeste, "a API key da DAV foi para o log")
	assert.NotContains(t, logs, "senha-secreta", "a senha do banco foi para o log")
	// E o que é útil precisa continuar aparecendo, senão o log perde a serventia.
	assert.Contains(t, logs, "local")
	assert.Contains(t, logs, "api.v2.hom.doutoraovivo.com.br")
}

// Um cookie sem Secure fora do dev viaja em claro no primeiro acesso HTTP e
// entrega a sessão a quem estiver na rede. É erro de configuração, não opção.
func TestLoad_CookieInseguroProibidoForaDeLocal(t *testing.T) {
	base := map[string]string{
		"RENOVI_CARE_DATABASE_URL": "postgres://x/y",
		"RENOVI_DAV_BASE_URL":      "https://api.v2.doutoraovivo.com.br",
		"RENOVI_DAV_API_KEY":       chaveDeTeste,
	}
	for _, env := range []string{"production", "staging"} {
		t.Run(env, func(t *testing.T) {
			for k, v := range base {
				t.Setenv(k, v)
			}
			t.Setenv("RENOVI_ENV", env)
			t.Setenv("RENOVI_SESSION_COOKIE_SECURE", "false")

			_, err := Load()
			require.Error(t, err, "%s aceitou cookie sem Secure", env)
			assert.Contains(t, err.Error(), "RENOVI_SESSION_COOKIE_SECURE")
		})
	}

	t.Run("local pode, porque não tem TLS", func(t *testing.T) {
		t.Setenv("RENOVI_ENV", "local")
		t.Setenv("RENOVI_SESSION_COOKIE_SECURE", "false")
		_, err := Load()
		require.NoError(t, err)
	})
}
