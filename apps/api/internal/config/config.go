// Package config carrega a configuração da aplicação a partir de variáveis de
// ambiente (12-factor). Não há segredos hardcoded: os valores default servem
// apenas para o ambiente de desenvolvimento local (docker-compose).
package config

import (
	"fmt"
	"os"
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
	return nil
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
