package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Session is a resolved bearer session: which vault/device it authenticates.
type Session struct {
	VaultID  string
	DeviceID string
}

// CreateSession stores a new session keyed by the SHA-256 hash of its bearer
// token (the plaintext token is never persisted).
func (s *Store) CreateSession(vaultID, deviceID, tokenHash string, expiresAt time.Time) error {
	_, err := s.db.Exec(
		`INSERT INTO sessions (token_hash, vault_id, device_id, expires_at) VALUES (?, ?, ?, ?)`,
		tokenHash, vaultID, deviceID, rfc3339(expiresAt),
	)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	return nil
}

// LookupSession resolves a hashed bearer token to its (vault, device), if it
// exists and hasn't expired.
func (s *Store) LookupSession(tokenHash string) (*Session, error) {
	var sess Session
	row := s.db.QueryRow(
		`SELECT vault_id, device_id FROM sessions WHERE token_hash = ? AND expires_at > strftime('%Y-%m-%dT%H:%M:%fZ','now')`,
		tokenHash,
	)
	if err := row.Scan(&sess.VaultID, &sess.DeviceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidSession
		}
		return nil, fmt.Errorf("failed to look up session: %w", err)
	}
	return &sess, nil
}
