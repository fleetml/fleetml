package db

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // registers the pgx5:// driver
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/fleetml/fleetml/internal/db/migrations"
)

// Migrate applies all pending up migrations from the embedded migration files.
// It is safe to run repeatedly and concurrently with other migrators (golang-migrate
// takes an advisory lock).
func Migrate(databaseURL string) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("loading embedded migrations: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, pgx5URL(databaseURL))
	if err != nil {
		return fmt.Errorf("initialising migrator: %w", err)
	}
	defer m.Close() //nolint:errcheck // best-effort close of source and db handles

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("applying migrations: %w", err)
	}

	version, dirty, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return fmt.Errorf("reading migration version: %w", err)
	}
	slog.Info("database migrations applied", "version", version, "dirty", dirty)
	return nil
}

// pgx5URL rewrites a postgres:// DSN to the pgx5:// scheme golang-migrate's pgx driver expects.
func pgx5URL(databaseURL string) string {
	for _, prefix := range []string{"postgres://", "postgresql://"} {
		if strings.HasPrefix(databaseURL, prefix) {
			return "pgx5://" + strings.TrimPrefix(databaseURL, prefix)
		}
	}
	return databaseURL
}
