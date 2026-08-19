package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"

	"crumb/pkg/backend"
	"crumb/pkg/config"
	"crumb/pkg/sync"
)

// saveProfileConfig persists an updated ProfileConfig back into
// ~/.config/crumb/config.yaml, alongside every other profile.
func saveProfileConfig(profile string, pc *config.ProfileConfig) error {
	configDir := filepath.Clean(filepath.Join(os.Getenv("HOME"), ".config", "crumb"))
	configPath := filepath.Clean(filepath.Join(configDir, "config.yaml"))

	var cfg config.Config
	if data, err := os.ReadFile(configPath); err == nil {
		_ = yaml.Unmarshal(data, &cfg)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = make(map[string]config.ProfileConfig)
	}
	cfg.Profiles[profile] = *pc
	return config.SaveConfig(&cfg)
}

func readTrimmedFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

func deviceLabel() string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "crumb-device"
}

// SyncInitCommand configures a profile for sync: either creating a new
// vault (no --invite) or joining an existing one (--invite <token>).
func SyncInitCommand(ctx context.Context, cmd *cli.Command) error {
	profile := getProfile(cmd)

	serverURL := strings.TrimSpace(cmd.String("server"))
	if serverURL == "" {
		return fmt.Errorf("--server is required (e.g. --server https://sync.example.com)")
	}
	inviteToken := strings.TrimSpace(cmd.String("invite"))

	pc, err := config.LoadConfig(profile)
	if err != nil {
		return err
	}
	if pc.Sync != nil && pc.Sync.VaultID != "" {
		return fmt.Errorf(
			"profile '%s' is already configured for sync (vault %s on %s); remove the 'sync' section from config.yaml first to re-init",
			profile, pc.Sync.VaultID, pc.Sync.ServerURL,
		)
	}

	pubKeyLine, err := readTrimmedFile(pc.PublicKeyPath)
	if err != nil {
		return err
	}
	label := deviceLabel()

	var vaultID, deviceID string
	if inviteToken == "" {
		vaultID, deviceID, err = sync.CreateVault(ctx, serverURL, pubKeyLine, fmt.Sprintf("crumb-%s", profile), label)
		if err != nil {
			return fmt.Errorf("failed to create vault: %w", err)
		}
		fmt.Printf("Created new vault %s and registered this device as owner.\n", vaultID)
	} else {
		var code string
		vaultID, code, err = sync.DecodeInvite(inviteToken)
		if err != nil {
			return err
		}
		var status string
		deviceID, status, err = sync.JoinVault(ctx, serverURL, vaultID, pubKeyLine, label, code)
		if err != nil {
			return fmt.Errorf("failed to join vault: %w", err)
		}
		if status != "approved" {
			return fmt.Errorf("device registered but is %q, not approved; ask an existing device owner to approve device %s", status, deviceID)
		}
		fmt.Printf("Joined vault %s.\n", vaultID)
	}

	pc.Sync = &config.SyncConfig{
		ServerURL: serverURL,
		VaultID:   vaultID,
		DeviceID:  deviceID,
	}
	if err := saveProfileConfig(profile, pc); err != nil {
		return err
	}
	if err := sync.ResetLocalState(profile); err != nil {
		return err
	}

	// Seed the server on a fresh vault: if this profile already has local
	// secrets and nobody has pushed yet, one sync makes them the starting
	// point rather than leaving the vault empty (or, if it isn't empty
	// because we just joined, reconciles the two).
	b, err := backend.ResolveBackend(pc)
	if err != nil {
		return err
	}
	client, err := sync.NewHTTPClient(pc)
	if err != nil {
		return err
	}
	client.OnSession = onSessionSaver(profile, pc)

	result, err := sync.Run(ctx, profile, pc, b, client)
	if err != nil {
		return fmt.Errorf("initial sync failed: %w", err)
	}
	printSyncResult(result)

	return nil
}

// onSessionSaver returns a callback that persists a freshly minted session
// token/expiry into the profile's on-disk config.
func onSessionSaver(profile string, pc *config.ProfileConfig) func(token string, expiresAt time.Time) {
	return func(token string, expiresAt time.Time) {
		pc.Sync.SessionToken = token
		pc.Sync.SessionExpiresAt = expiresAt.UTC().Format(time.RFC3339)
		_ = saveProfileConfig(profile, pc) // best-effort; a failed save just costs one extra login next time
	}
}

// SyncInviteCommand mints a one-time invite code+vault token for adding a
// new device to the current profile's vault.
func SyncInviteCommand(ctx context.Context, cmd *cli.Command) error {
	profile := getProfile(cmd)
	pc, err := config.LoadConfig(profile)
	if err != nil {
		return err
	}
	if pc.Sync == nil {
		return fmt.Errorf("profile '%s' is not configured for sync; run 'crumb sync init' first", profile)
	}

	maxUses := int(cmd.Int("max-uses"))
	if maxUses <= 0 {
		maxUses = 1
	}
	ttl := cmd.Duration("ttl")
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}

	client, err := sync.NewHTTPClient(pc)
	if err != nil {
		return err
	}
	client.OnSession = onSessionSaver(profile, pc)
	if err := client.Login(ctx); err != nil {
		return fmt.Errorf("failed to authenticate with sync server: %w", err)
	}

	code, expiresAt, err := client.CreateInvite(ctx, maxUses, ttl)
	if err != nil {
		return fmt.Errorf("failed to create invite: %w", err)
	}

	token := sync.EncodeInvite(pc.Sync.VaultID, code)
	fmt.Printf("Invite (expires %s, max %d use(s)):\n\n  %s\n\n", expiresAt.Format(time.RFC3339), maxUses, token)
	fmt.Println("On the other machine, run:")
	fmt.Printf("  crumb sync init --server %s --invite %s\n", pc.Sync.ServerURL, token)

	return nil
}

// SyncStatusCommand prints this profile's sync configuration and a summary
// of the remote state, without mutating anything.
func SyncStatusCommand(ctx context.Context, cmd *cli.Command) error {
	profile := getProfile(cmd)
	pc, err := config.LoadConfig(profile)
	if err != nil {
		return err
	}
	if pc.Sync == nil {
		fmt.Printf("Sync is not configured for profile '%s'. Run 'crumb sync init --server <url>' to set it up.\n", profile)
		return nil
	}

	fmt.Printf("Profile:    %s\n", profile)
	fmt.Printf("Server:     %s\n", pc.Sync.ServerURL)
	fmt.Printf("Vault ID:   %s\n", pc.Sync.VaultID)
	fmt.Printf("Device ID:  %s\n", pc.Sync.DeviceID)

	st, err := sync.LoadLocalState(profile)
	if err != nil {
		return err
	}
	if st.LastSyncAt != "" {
		fmt.Printf("Last sync:  %s (local version %d)\n", st.LastSyncAt, st.Version)
	} else {
		fmt.Println("Last sync:  never")
	}

	client, err := sync.NewHTTPClient(pc)
	if err != nil {
		return err
	}
	client.OnSession = onSessionSaver(profile, pc)
	if err := client.Login(ctx); err != nil {
		return fmt.Errorf("failed to authenticate with sync server: %w", err)
	}
	remoteVersion, _, err := client.GetBlob(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch remote state: %w", err)
	}
	fmt.Printf("Remote version: %d\n", remoteVersion)
	if remoteVersion == st.Version {
		fmt.Println("Up to date.")
	} else {
		fmt.Println("Out of date — run 'crumb sync' to reconcile.")
	}

	return nil
}

// SyncCommand runs a full sync: login, pull, merge (if needed), push, and
// write the result back to local storage.
func SyncCommand(ctx context.Context, cmd *cli.Command) error {
	profile := getProfile(cmd)
	pc, b, err := resolveBackend(cmd)
	if err != nil {
		return err
	}
	if pc.Sync == nil {
		return fmt.Errorf("profile '%s' is not configured for sync; run 'crumb sync init --server <url>' first", profile)
	}

	client, err := sync.NewHTTPClient(pc)
	if err != nil {
		return err
	}
	client.OnSession = onSessionSaver(profile, pc)

	result, err := sync.Run(ctx, profile, pc, b, client)
	if err != nil {
		return err
	}
	printSyncResult(result)
	return nil
}

func printSyncResult(result sync.Result) {
	switch {
	case result.Merged && len(result.ConflictKeys) > 0:
		fmt.Printf("Synced (merged, %d key(s) changed on both sides — newest write won):\n", len(result.ConflictKeys))
		for _, k := range result.ConflictKeys {
			fmt.Printf("  %s\n", k)
		}
	case result.Merged:
		fmt.Println("Synced (merged remote changes).")
	case !result.Pushed:
		fmt.Println("Already up to date.")
	default:
		fmt.Println("Synced.")
	}
	fmt.Printf("Now at version %d.\n", result.LocalVersion)
}
