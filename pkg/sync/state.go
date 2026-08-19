package sync

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"crumb/pkg/crypto"
)

// localState is the small bit of bookkeeping crumb keeps per profile to
// know what it last successfully synced.
type localState struct {
	Version      int    `json:"version"`
	LastSyncAt   string `json:"last_sync_at,omitempty"`
	SeededLegacy bool   `json:"seeded_legacy"`
}

func syncDir(profile string) string {
	return filepath.Join(os.Getenv("HOME"), ".config", "crumb", "sync", profile)
}

func statePath(profile string) string {
	return filepath.Join(syncDir(profile), "state.json")
}

func baseBlobPath(profile string) string {
	return filepath.Join(syncDir(profile), "base.age")
}

// LocalState is the exported view of a profile's local sync bookkeeping,
// for status reporting.
type LocalState struct {
	Version    int
	LastSyncAt string
}

// LoadLocalState returns the profile's last-known-synced version and when
// it last synced (zero value if it has never synced).
func LoadLocalState(profile string) (LocalState, error) {
	st, err := loadState(profile)
	if err != nil {
		return LocalState{}, err
	}
	return LocalState{Version: st.Version, LastSyncAt: st.LastSyncAt}, nil
}

func loadState(profile string) (localState, error) {
	data, err := os.ReadFile(statePath(profile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return localState{}, nil
		}
		return localState{}, fmt.Errorf("failed to read sync state: %w", err)
	}
	var st localState
	if err := json.Unmarshal(data, &st); err != nil {
		return localState{}, fmt.Errorf("failed to parse sync state: %w", err)
	}
	return st, nil
}

func saveState(profile string, st localState) error {
	if err := os.MkdirAll(syncDir(profile), 0700); err != nil {
		return fmt.Errorf("failed to create sync state directory: %w", err)
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode sync state: %w", err)
	}
	if err := crypto.WriteFileWithLock(statePath(profile), data, 0600); err != nil {
		return fmt.Errorf("failed to write sync state: %w", err)
	}
	return nil
}

// loadBaseBlob returns the raw ciphertext of the blob as of the last
// successful sync, or nil if none has happened yet.
func loadBaseBlob(profile string) ([]byte, error) {
	data, err := crypto.ReadFileWithLock(baseBlobPath(profile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read sync base blob: %w", err)
	}
	return data, nil
}

// ResetLocalState discards a profile's local sync bookkeeping (last-known
// version and base blob). Called by 'sync init' before its first run
// against a newly created or joined vault, so stale state from any prior
// vault this profile was synced to can't be mistaken for state that
// corresponds to the new one.
func ResetLocalState(profile string) error {
	if err := os.Remove(statePath(profile)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to reset sync state: %w", err)
	}
	if err := os.Remove(baseBlobPath(profile)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to reset sync base blob: %w", err)
	}
	return nil
}

func saveBaseBlob(profile string, blob []byte) error {
	if err := os.MkdirAll(syncDir(profile), 0700); err != nil {
		return fmt.Errorf("failed to create sync state directory: %w", err)
	}
	if err := crypto.WriteFileWithLock(baseBlobPath(profile), blob, 0600); err != nil {
		return fmt.Errorf("failed to write sync base blob: %w", err)
	}
	return nil
}
