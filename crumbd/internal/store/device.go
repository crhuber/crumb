package store

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// Device is an SSH public key registered against a vault.
type Device struct {
	ID          string
	VaultID     string
	PublicKey   string
	Fingerprint string
	Label       string
	Status      string // pending | approved | revoked
	Role        string // owner | member
	CreatedAt   string
	ApprovedAt  sql.NullString
	RevokedAt   sql.NullString
}

// CreateDevice registers a new device against a vault with the given status/role.
func (s *Store) CreateDevice(vaultID, publicKey, fingerprint, label, status, role string) (string, error) {
	id := uuid.NewString()
	_, err := s.db.Exec(
		`INSERT INTO devices (id, vault_id, public_key, fingerprint, label, status, role,
		                      approved_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, CASE WHEN ? = 'approved' THEN strftime('%Y-%m-%dT%H:%M:%fZ','now') END)`,
		id, vaultID, publicKey, fingerprint, label, status, role, status,
	)
	if err != nil {
		return "", fmt.Errorf("failed to create device: %w", err)
	}
	return id, nil
}

func scanDevice(row *sql.Row) (*Device, error) {
	var d Device
	err := row.Scan(&d.ID, &d.VaultID, &d.PublicKey, &d.Fingerprint, &d.Label, &d.Status, &d.Role,
		&d.CreatedAt, &d.ApprovedAt, &d.RevokedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to read device: %w", err)
	}
	return &d, nil
}

const deviceColumns = `id, vault_id, public_key, fingerprint, label, status, role, created_at, approved_at, revoked_at`

// GetDevice looks up a device by id.
func (s *Store) GetDevice(deviceID string) (*Device, error) {
	row := s.db.QueryRow(`SELECT `+deviceColumns+` FROM devices WHERE id = ?`, deviceID)
	return scanDevice(row)
}

// GetDeviceByFingerprint looks up a device within a vault by SSH key fingerprint.
func (s *Store) GetDeviceByFingerprint(vaultID, fingerprint string) (*Device, error) {
	row := s.db.QueryRow(`SELECT `+deviceColumns+` FROM devices WHERE vault_id = ? AND fingerprint = ?`, vaultID, fingerprint)
	return scanDevice(row)
}

// ListDevices returns every device registered against a vault.
func (s *Store) ListDevices(vaultID string) ([]Device, error) {
	rows, err := s.db.Query(`SELECT `+deviceColumns+` FROM devices WHERE vault_id = ? ORDER BY created_at`, vaultID)
	if err != nil {
		return nil, fmt.Errorf("failed to list devices: %w", err)
	}
	defer rows.Close()

	var devices []Device
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.ID, &d.VaultID, &d.PublicKey, &d.Fingerprint, &d.Label, &d.Status, &d.Role,
			&d.CreatedAt, &d.ApprovedAt, &d.RevokedAt); err != nil {
			return nil, fmt.Errorf("failed to scan device: %w", err)
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

// ApproveDevice marks a pending device as approved.
func (s *Store) ApproveDevice(vaultID, deviceID string) error {
	res, err := s.db.Exec(
		`UPDATE devices SET status = 'approved', approved_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		 WHERE id = ? AND vault_id = ? AND status = 'pending'`,
		deviceID, vaultID,
	)
	if err != nil {
		return fmt.Errorf("failed to approve device: %w", err)
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

// RevokeDevice marks a device revoked and immediately invalidates its
// sessions and any unused auth challenges.
func (s *Store) RevokeDevice(vaultID, deviceID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`UPDATE devices SET status = 'revoked', revoked_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		 WHERE id = ? AND vault_id = ?`,
		deviceID, vaultID,
	)
	if err != nil {
		return fmt.Errorf("failed to revoke device: %w", err)
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return ErrNotFound
	}

	if _, err := tx.Exec(`DELETE FROM sessions WHERE device_id = ?`, deviceID); err != nil {
		return fmt.Errorf("failed to delete sessions: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM auth_challenges WHERE device_id = ? AND used_at IS NULL`, deviceID); err != nil {
		return fmt.Errorf("failed to delete challenges: %w", err)
	}

	return tx.Commit()
}
