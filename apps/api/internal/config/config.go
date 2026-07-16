// Package config carrega a configuração da aplicação a partir de variáveis de
// ambiente (12-factor). Não há segredos hardcoded: os valores default servem
// apenas para o ambiente de desenvolvimento local (docker-compose).
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Ambientes suportados.
const (
	EnvLocal      = "local"
	EnvStaging    = "staging"
	EnvProduction = "production"
)

// defaultCareDatabaseURL só é aplicado FORA de produção. Em produção, a URL é
// obrigatória (ver Load) para não mascarar um deploy mal configurado.
const defaultCareDatabaseURL = "postgres://renovi:renovi@localhost:5432/renovi_care?sslmode=disable"

// Config concentra todos os parâmetros de execução do serviço.
type Config struct {
	// Env identifica o ambiente: "local", "staging" ou "production".
	Env string
	// HTTPAddr é o endereço de escuta do servidor HTTP (ex.: ":8090").
	HTTPAddr string
	// ReadTimeout / WriteTimeout / ShutdownTimeout controlam o ciclo do servidor.
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
	// LogLevel: "debug", "info", "warn", "error".
	LogLevel string

	// --- Bancos (ver docs/ARQUITETURA.md, seção "Papel de cada banco") ---

	// CareDatabaseURL: Postgres renovi_care (nosso banco, escrita/leitura).
	CareDatabaseURL string
	// LegacyDatabaseURL: MySQL legado (escala/slots). Leitura + escrita restrita
	// à tabela de agendamento, sempre via Adapter Agenda. Opcional no MVP inicial.
	LegacyDatabaseURL string
	// GestaoDatabaseURL: Postgres Gestão 2.0 (empresas/contratos/colaboradores),
	// SOMENTE LEITURA. Opcional no MVP inicial.
	GestaoDatabaseURL string

	// --- Doutor ao Vivo (comportamento real medido em docs/DAV-API-NOTAS.md) ---

	// DAVBaseURL e DAVAPIKey: obrigatórios em staging/produção. A chave vai no
	// header x-api-key e NUNCA deve ser logada (ver LogValue).
	DAVBaseURL string
	DAVAPIKey  string
	// DAVTimeout é o teto de UMA tentativa.
	//
	// O default (30s) é deliberadamente MAIOR que o teto de 29s do AWS API
	// Gateway na frente da DAV. Parece contraintuitivo, mas é o certo: um valor
	// menor faz o NOSSO cliente desistir primeiro e perdermos a resposta deles.
	// Deixando o gateway falar, recebemos o 504 — que prova que a requisição
	// chegou lá e que a criação pode ter sido aplicada.
	//
	// Medido em produção de HML: o GET varia de 0,5s a 6,3s e o POST já bateu no
	// teto dos 29s. Um timeout de 10s reprova cadastros que dariam certo.
	DAVTimeout time.Duration
	// DAVMaxAttempts inclui a primeira tentativa. 2 e não 3: com 30s por
	// tentativa, uma terceira levaria o pior caso a 90s de espera do usuário.
	DAVMaxAttempts int

	// --- Sessão (ADR-010: token opaco em cookie httpOnly) ---

	SessionTTL time.Duration
	// SessionCookieSecure desliga o atributo Secure do cookie. Só faz sentido em
	// desenvolvimento sem TLS; o default é true para que esquecer de configurar
	// erre para o lado seguro.
	SessionCookieSecure bool
}

// LogValue controla como o Config aparece no slog, redigindo os segredos.
//
// Existe porque `slog.Info("subindo", "config", cfg)` é a coisa mais natural do
// mundo de se escrever — e, sem isto, despejaria a API key da DAV e a senha do
// banco no stdout de produção. O CLAUDE.md proíbe; este método faz a proibição
// valer por construção, em vez de por disciplina.
func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("env", c.Env),
		slog.String("http_addr", c.HTTPAddr),
		slog.String("log_level", c.LogLevel),
		slog.Bool("care_database_url_set", c.CareDatabaseURL != ""),
		slog.Bool("legacy_database_url_set", c.LegacyDatabaseURL != ""),
		slog.Bool("gestao_database_url_set", c.GestaoDatabaseURL != ""),
		// A URL da DAV não é segredo e dizer qual ambiente foi alvejado já evitou
		// bug: é o que denuncia um deploy apontado para produção sem querer.
		slog.String("dav_base_url", c.DAVBaseURL),
		slog.Bool("dav_api_key_set", c.DAVAPIKey != ""),
		slog.Duration("dav_timeout", c.DAVTimeout),
		slog.Int("dav_max_attempts", c.DAVMaxAttempts),
		slog.Duration("session_ttl", c.SessionTTL),
		slog.Bool("session_cookie_secure", c.SessionCookieSecure),
	)
}

// Load lê a configuração do ambiente, aplicando defaults de desenvolvimento e
// validando os campos obrigatórios. Falha (erro) em vez de mascarar
// configuração inválida: duração malformada, ambiente desconhecido ou URL do
// banco ausente em produção.
func Load() (Config, error) {
	cfg := Config{
		Env:               env("RENOVI_ENV", EnvLocal),
		HTTPAddr:          env("RENOVI_HTTP_ADDR", ":8090"),
		LogLevel:          env("RENOVI_LOG_LEVEL", "info"),
		LegacyDatabaseURL: env("RENOVI_LEGACY_DATABASE_URL", ""),
		GestaoDatabaseURL: env("RENOVI_GESTAO_DATABASE_URL", ""),
	}

	var err error
	if cfg.ReadTimeout, err = envDuration("RENOVI_HTTP_READ_TIMEOUT", 15*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.WriteTimeout, err = envDuration("RENOVI_HTTP_WRITE_TIMEOUT", 15*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.ShutdownTimeout, err = envDuration("RENOVI_HTTP_SHUTDOWN_TIMEOUT", 10*time.Second); err != nil {
		return Config{}, err
	}

	if cfg.DAVTimeout, err = envDuration("RENOVI_DAV_TIMEOUT", 30*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.SessionTTL, err = envDuration("RENOVI_SESSION_TTL", 12*time.Hour); err != nil {
		return Config{}, err
	}
	if cfg.DAVMaxAttempts, err = envInt("RENOVI_DAV_MAX_ATTEMPTS", 2); err != nil {
		return Config{}, err
	}
	if cfg.SessionCookieSecure, err = envBool("RENOVI_SESSION_COOKIE_SECURE", true); err != nil {
		return Config{}, err
	}

	cfg.DAVBaseURL = env("RENOVI_DAV_BASE_URL", "")
	cfg.DAVAPIKey = env("RENOVI_DAV_API_KEY", "")

	// A URL do banco é obrigatória em produção; fora dela, cai no default local.
	cfg.CareDatabaseURL = env("RENOVI_CARE_DATABASE_URL", "")
	if cfg.CareDatabaseURL == "" {
		if cfg.Env == EnvProduction {
			return Config{}, fmt.Errorf("config: RENOVI_CARE_DATABASE_URL é obrigatório em produção")
		}
		cfg.CareDatabaseURL = defaultCareDatabaseURL
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// IsProduction indica se estamos rodando em produção (afeta logs e detalhes de erro).
func (c Config) IsProduction() bool { return c.Env == EnvProduction }

// DAVBudget é o pior caso de tempo de uma operação no adapter da DAV: todas as
// tentativas estourando, mais o backoff entre elas.
//
// Existe para que o timeout da rota de cadastro DERIVE do orçamento em vez de
// ser um número solto. Foi exatamente essa divergência que reprovou o primeiro
// cadastro real: 3 tentativas de 10s davam 30s, e o timeout da requisição também
// era 30s — a última tentativa era cortada no meio, sempre.
func (c Config) DAVBudget() time.Duration {
	const backoffSlack = 2 * time.Second
	return time.Duration(c.DAVMaxAttempts)*c.DAVTimeout + backoffSlack
}

func (c Config) validate() error {
	switch c.Env {
	case EnvLocal, EnvStaging, EnvProduction:
	default:
		return fmt.Errorf("config: RENOVI_ENV inválido: %q (use local|staging|production)", c.Env)
	}
	if c.HTTPAddr == "" {
		return fmt.Errorf("config: RENOVI_HTTP_ADDR não pode ser vazio")
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("config: RENOVI_LOG_LEVEL inválido: %q", c.LogLevel)
	}

	// Fora de local a DAV é obrigatória. Sem isto, um deploy sem credencial só
	// quebraria no primeiro cadastro de um paciente real, como um 401 sem
	// explicação — falhar na subida é muito mais barato.
	// As mensagens citam o NOME da variável, nunca o valor.
	if c.Env != EnvLocal {
		if c.DAVBaseURL == "" {
			return fmt.Errorf("config: RENOVI_DAV_BASE_URL é obrigatório em %s", c.Env)
		}
		if c.DAVAPIKey == "" {
			return fmt.Errorf("config: RENOVI_DAV_API_KEY é obrigatório em %s", c.Env)
		}
	}
	// Um cookie de sessão sem Secure fora do dev viaja em claro no primeiro
	// acesso HTTP e entrega a sessão a quem estiver na rede. Falhar na subida é
	// melhor que descobrir isso em produção.
	if c.Env != EnvLocal && !c.SessionCookieSecure {
		return fmt.Errorf("config: RENOVI_SESSION_COOKIE_SECURE=false é proibido em %s", c.Env)
	}
	if c.DAVMaxAttempts < 1 {
		return fmt.Errorf("config: RENOVI_DAV_MAX_ATTEMPTS deve ser >= 1")
	}
	if c.DAVTimeout <= 0 || c.SessionTTL <= 0 {
		return fmt.Errorf("config: RENOVI_DAV_TIMEOUT e RENOVI_SESSION_TTL devem ser > 0")
	}
	return nil
}

// envInt lê um inteiro; presente porém malformado é ERRO, na mesma regra do
// envDuration — um "três" não pode virar 3 silenciosamente.
func envInt(key string, fallback int) (int, error) {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0, fmt.Errorf("config: %s inválido: %w", key, err)
	}
	return n, nil
}

func envBool(key string, fallback bool) (bool, error) {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return fallback, nil
	}
	b, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		return false, fmt.Errorf("config: %s inválido: %w", key, err)
	}
	return b, nil
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}

// envDuration lê uma duração; um valor presente porém malformado é ERRO (não é
// silenciosamente trocado pelo default) — assim um "60" sem unidade não vira 15s
// escondido do operador.
func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("config: %s inválido: %w", key, err)
	}
	return d, nil
}
