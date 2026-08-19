package db

import (
	"database/sql"
	"embed"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate applies any migration files not yet recorded in schema_migrations,
// in ascending numeric filename order, each inside its own transaction.
func Migrate(sqlDB *sql.DB) error {
	if _, err := sqlDB.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
	)`); err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("failed to read embedded migrations: %w", err)
	}

	type migration struct {
		version  int
		filename string
	}
	var migrations []migration
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		prefix := strings.SplitN(e.Name(), "_", 2)[0]
		version, err := strconv.Atoi(prefix)
		if err != nil {
			return fmt.Errorf("migration filename %q does not start with a numeric version", e.Name())
		}
		migrations = append(migrations, migration{version: version, filename: e.Name()})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].version < migrations[j].version })

	for _, m := range migrations {
		var already bool
		row := sqlDB.QueryRow(`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = ?)`, m.version)
		if err := row.Scan(&already); err != nil {
			return fmt.Errorf("failed to check migration %d: %w", m.version, err)
		}
		if already {
			continue
		}

		sqlBytes, err := migrationsFS.ReadFile(path.Join("migrations", m.filename))
		if err != nil {
			return fmt.Errorf("failed to read migration %s: %w", m.filename, err)
		}

		tx, err := sqlDB.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction for migration %s: %w", m.filename, err)
		}
		if _, err := tx.Exec(string(sqlBytes)); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to apply migration %s: %w", m.filename, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, m.version); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to record migration %s: %w", m.filename, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %s: %w", m.filename, err)
		}
	}

	return nil
}
