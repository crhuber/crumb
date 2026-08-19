package store

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// CreateVault inserts a new, empty vault (version 0, no blob) and returns its id.
func (s *Store) CreateVault(name string) (string, error) {
	id := uuid.NewString()
	if _, err := s.db.Exec(
		`INSERT INTO vaults (id, name) VALUES (?, ?)`,
		id, name,
	); err != nil {
		return "", fmt.Errorf("failed to create vault: %w", err)
	}
	return id, nil
}

// VaultExists reports whether a vault with the given id exists.
func (s *Store) VaultExists(vaultID string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM vaults WHERE id = ?)`, vaultID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check vault existence: %w", err)
	}
	return exists, nil
}

// GetBlob returns the vault's current version and blob (blob may be nil if
// nothing has ever been pushed).
func (s *Store) GetBlob(vaultID string) (version int, blob []byte, updatedAt string, err error) {
	row := s.db.QueryRow(`SELECT current_version, blob, updated_at FROM vaults WHERE id = ?`, vaultID)
	if scanErr := row.Scan(&version, &blob, &updatedAt); scanErr != nil {
		if errors.Is(scanErr, sql.ErrNoRows) {
			return 0, nil, "", ErrNotFound
		}
		return 0, nil, "", fmt.Errorf("failed to read vault blob: %w", scanErr)
	}
	return version, blob, updatedAt, nil
}

// CompareAndSwapBlob atomically replaces the vault's blob and increments its
// version, but only if the vault's current version still equals
// expectedVersion. On a version mismatch it returns ErrVersionConflict along
// with the vault's actual current version and blob, so the caller can merge
// and retry without a second round trip.
func (s *Store) CompareAndSwapBlob(vaultID string, expectedVersion int, blob []byte) (newVersion int, err error) {
	res, err := s.db.Exec(
		`UPDATE vaults
		   SET blob = ?, current_version = current_version + 1, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		 WHERE id = ? AND current_version = ?`,
		blob, vaultID, expectedVersion,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to update vault blob: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to check update result: %w", err)
	}
	if affected == 0 {
		exists, existsErr := s.VaultExists(vaultID)
		if existsErr != nil {
			return 0, existsErr
		}
		if !exists {
			return 0, ErrNotFound
		}
		return 0, ErrVersionConflict
	}

	version, _, _, err := s.GetBlob(vaultID)
	if err != nil {
		return 0, err
	}
	return version, nil
}
