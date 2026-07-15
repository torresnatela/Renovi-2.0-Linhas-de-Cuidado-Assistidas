package db

import (
	"embed"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // registra o driver pgx5://
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// migrationsFS embute os arquivos .sql para que o binário seja autossuficiente
// (não precisa carregar a pasta migrations no runtime).
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// newMigrator monta um *migrate.Migrate a partir das migrations embutidas e da
// URL do banco. golang-migrate exige o schema "pgx5://" para o driver pgx v5,
// então convertemos "postgres://" / "postgresql://" automaticamente.
func newMigrator(databaseURL string) (*migrate.Migrate, error) {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("db: abrir migrations embutidas: %w", err)
	}

	url := databaseURL
	for _, prefix := range []string{"postgresql://", "postgres://"} {
		if strings.HasPrefix(url, prefix) {
			url = "pgx5://" + strings.TrimPrefix(url, prefix)
			break
		}
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, url)
	if err != nil {
		return nil, fmt.Errorf("db: inicializar migrator: %w", err)
	}
	return m, nil
}

// MigrateUp aplica todas as migrations pendentes. É idempotente: se já estiver
// atualizado, retorna nil.
func MigrateUp(databaseURL string) error {
	m, err := newMigrator(databaseURL)
	if err != nil {
		return err
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("db: migrate up: %w", err)
	}
	return nil
}

// MigrateDown reverte a última migration (passo a passo). Use com cautela.
func MigrateDown(databaseURL string, steps int) error {
	m, err := newMigrator(databaseURL)
	if err != nil {
		return err
	}
	defer m.Close()

	if err := m.Steps(-steps); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("db: migrate down: %w", err)
	}
	return nil
}

// MigrateVersion retorna a versão atual aplicada e se o estado está "dirty".
func MigrateVersion(databaseURL string) (version uint, dirty bool, err error) {
	m, err := newMigrator(databaseURL)
	if err != nil {
		return 0, false, err
	}
	defer m.Close()

	version, dirty, err = m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return 0, false, fmt.Errorf("db: migrate version: %w", err)
	}
	return version, dirty, nil
}
