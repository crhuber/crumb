// Package sync implements crumb's multi-machine sync: a versioned encrypted
// blob is pushed to / pulled from a self-hosted crumbd server, with a
// lightweight three-way merge on conflict so two machines editing different
// secrets while both offline don't clobber each other.
package sync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"crumb/pkg/backend"
	"crumb/pkg/config"
	"crumb/pkg/storage"
)

// Result summarizes what a sync run did.
type Result struct {
	Pushed        bool
	Merged        bool
	ConflictKeys  []string
	LocalVersion  int
	RemoteVersion int
}

const maxPushAttempts = 3

// Run reconciles this profile's local secrets with the sync server: it logs
// in if needed, pulls the server's current state, merges it against local
// changes (if anyone has pushed since this machine's last sync), pushes the
// result, and writes the merged secrets back to the local backend so
// get/list/export keep working unchanged.
func Run(ctx context.Context, profile string, cfg *config.ProfileConfig, b backend.Backend, client Client) (Result, error) {
	if cfg.Sync == nil {
		return Result{}, fmt.Errorf("sync is not configured for this profile; run 'crumb sync init' first")
	}

	if err := client.Login(ctx); err != nil {
		return Result{}, fmt.Errorf("failed to authenticate with sync server: %w", err)
	}

	local, err := storage.LoadSecrets(cfg.PrivateKeyPath, b)
	if err != nil {
		return Result{}, fmt.Errorf("failed to load local secrets: %w", err)
	}

	st, err := loadState(profile)
	if err != nil {
		return Result{}, err
	}

	baseBlob, err := loadBaseBlob(profile)
	if err != nil {
		return Result{}, err
	}
	base, err := storage.DecryptSecrets(baseBlob, cfg.PrivateKeyPath)
	if err != nil {
		return Result{}, fmt.Errorf("failed to decrypt local sync base: %w", err)
	}

	remoteVersion, remoteBlob, err := client.GetBlob(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("failed to fetch remote secrets: %w", err)
	}
	remote, err := storage.DecryptSecrets(remoteBlob, cfg.PrivateKeyPath)
	if err != nil {
		return Result{}, fmt.Errorf("failed to decrypt remote secrets (do you have the right key?): %w", err)
	}

	result := Result{RemoteVersion: remoteVersion}

	merged := local
	if remoteVersion != st.Version {
		merged, result.ConflictKeys = Merge(base, local, remote)
		result.Merged = true
	}

	newVersion, merged, err := pushWithRetry(ctx, client, cfg, remoteVersion, base, local, merged, &result)
	if err != nil {
		return Result{}, err
	}
	result.Pushed = true
	result.LocalVersion = newVersion

	if err := storage.SaveSecrets(merged, cfg.PublicKeyPath, b); err != nil {
		return Result{}, fmt.Errorf("failed to write merged secrets locally: %w", err)
	}

	finalBlob, err := storage.EncryptSecrets(merged, cfg.PublicKeyPath)
	if err != nil {
		return Result{}, fmt.Errorf("failed to re-encrypt merged secrets for sync base: %w", err)
	}
	if err := saveBaseBlob(profile, finalBlob); err != nil {
		return Result{}, err
	}
	if err := saveState(profile, localState{
		Version:      newVersion,
		LastSyncAt:   time.Now().UTC().Format(time.RFC3339),
		SeededLegacy: st.SeededLegacy,
	}); err != nil {
		return Result{}, err
	}

	return result, nil
}

// pushWithRetry pushes merged (encrypting it fresh each attempt), and on a
// version conflict (someone else pushed again in the tiny window since
// GetBlob) re-merges against the fresher server state and retries, up to
// maxPushAttempts total.
func pushWithRetry(
	ctx context.Context, client Client, cfg *config.ProfileConfig, expectedVersion int,
	base, local, merged storage.SecretStore, result *Result,
) (newVersion int, finalMerged storage.SecretStore, err error) {
	for attempt := 1; attempt <= maxPushAttempts; attempt++ {
		encrypted, encErr := storage.EncryptSecrets(merged, cfg.PublicKeyPath)
		if encErr != nil {
			return 0, nil, fmt.Errorf("failed to encrypt merged secrets: %w", encErr)
		}

		version, putErr := client.PutBlob(ctx, expectedVersion, encrypted)
		if putErr == nil {
			return version, merged, nil
		}

		var conflict *ConflictError
		if !errors.As(putErr, &conflict) {
			return 0, nil, fmt.Errorf("failed to push secrets to sync server: %w", putErr)
		}
		if attempt == maxPushAttempts {
			return 0, nil, fmt.Errorf("sync server keeps moving out from under us; please re-run 'crumb sync'")
		}

		freshRemote, decErr := storage.DecryptSecrets(conflict.Blob, cfg.PrivateKeyPath)
		if decErr != nil {
			return 0, nil, fmt.Errorf("failed to decrypt remote secrets during conflict retry: %w", decErr)
		}
		merged, result.ConflictKeys = Merge(base, local, freshRemote)
		result.Merged = true
		expectedVersion = conflict.Version
	}

	return 0, nil, fmt.Errorf("sync failed after %d attempts", maxPushAttempts)
}
