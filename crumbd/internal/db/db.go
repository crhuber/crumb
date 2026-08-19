// Package db manages crumbd's SQLite connection and schema migrations.
package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // pure-Go sqlite driver, registers as "sqlite"
)

// Open opens (creating if necessary) the SQLite database at path with the
// pragmas crumbd needs, and applies any pending migrations.
func Open(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)", path)

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	sqlDB.SetMaxOpenConns(1) // modernc.org/sqlite: keep writes serialized, WAL still allows concurrent reads

	if err := Migrate(sqlDB); err != nil {
		sqlDB.Close()
		return nil, err
	}

	return sqlDB, nil
}
