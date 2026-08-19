package store

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CreateInvite stores a new invite, keyed by the SHA-256 hash of its code
// (the plaintext code is never persisted). Returns the invite id.
func (s *Store) CreateInvite(vaultID, codeHash string, maxUses int, expiresAt time.Time) (string, error) {
	id := uuid.NewString()
	_, err := s.db.Exec(
		`INSERT INTO invites (id, vault_id, code_hash, max_uses, expires_at) VALUES (?, ?, ?, ?, ?)`,
		id, vaultID, codeHash, maxUses, rfc3339(expiresAt),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create invite: %w", err)
	}
	return id, nil
}

// ConsumeInvite atomically increments an invite's use count if it exists for
// the vault, isn't revoked, isn't expired, and hasn't hit max_uses. Returns
// ErrInvalidInvite for any failure mode, deliberately without distinguishing
// "wrong code" from "expired" to avoid an oracle.
func (s *Store) ConsumeInvite(vaultID, codeHash string) error {
	res, err := s.db.Exec(
		`UPDATE invites
		    SET use_count = use_count + 1
		  WHERE vault_id = ? AND code_hash = ? AND revoked_at IS NULL
		    AND use_count < max_uses AND expires_at > strftime('%Y-%m-%dT%H:%M:%fZ','now')`,
		vaultID, codeHash,
	)
	if err != nil {
		return fmt.Errorf("failed to consume invite: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check invite consumption: %w", err)
	}
	if affected == 0 {
		return ErrInvalidInvite
	}
	return nil
}
