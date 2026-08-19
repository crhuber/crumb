// Package store implements crumbd's data-access layer over SQLite: vaults
// (one versioned encrypted blob each), devices (SSH public keys authorized
// against a vault), invites, auth challenges, and sessions.
package store

import (
	"database/sql"
	"errors"
	"time"
)

// Store wraps the SQLite connection with crumbd's data-access methods.
type Store struct {
	db *sql.DB
}

// New returns a Store backed by db.
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

var (
	ErrNotFound         = errors.New("not found")
	ErrVersionConflict  = errors.New("version conflict")
	ErrInvalidInvite    = errors.New("invalid or expired invite")
	ErrInvalidSession   = errors.New("invalid or expired session")
	ErrInvalidChallenge = errors.New("invalid or expired challenge")
)

func rfc3339(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.999999999Z")
}
