package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Challenge is a single-use auth nonce issued to one device.
type Challenge struct {
	ID       string
	VaultID  string
	DeviceID string
	Nonce    string
}

// CreateChallenge issues a new challenge for a device and returns its id.
func (s *Store) CreateChallenge(vaultID, deviceID, nonce string, expiresAt time.Time) (string, error) {
	id := uuid.NewString()
	_, err := s.db.Exec(
		`INSERT INTO auth_challenges (id, vault_id, device_id, nonce, expires_at) VALUES (?, ?, ?, ?, ?)`,
		id, vaultID, deviceID, nonce, rfc3339(expiresAt),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create challenge: %w", err)
	}
	return id, nil
}

// ConsumeChallenge looks up an unused, unexpired challenge by id and marks it
// used (single-use, regardless of whether the caller's signature ultimately
// verifies) in one atomic step.
func (s *Store) ConsumeChallenge(challengeID string) (*Challenge, error) {
	var c Challenge
	row := s.db.QueryRow(
		`SELECT id, vault_id, device_id, nonce FROM auth_challenges
		 WHERE id = ? AND used_at IS NULL AND expires_at > strftime('%Y-%m-%dT%H:%M:%fZ','now')`,
		challengeID,
	)
	if err := row.Scan(&c.ID, &c.VaultID, &c.DeviceID, &c.Nonce); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidChallenge
		}
		return nil, fmt.Errorf("failed to read challenge: %w", err)
	}

	res, err := s.db.Exec(
		`UPDATE auth_challenges SET used_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = ? AND used_at IS NULL`,
		challengeID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to mark challenge used: %w", err)
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		// Someone else consumed it in the tiny window between our SELECT and UPDATE.
		return nil, ErrInvalidChallenge
	}

	return &c, nil
}
