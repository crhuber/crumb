package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"

	"crumb/pkg/config"
)

// StorageSetCommand handles the storage set command
func StorageSetCommand(_ context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() != 1 {
		return fmt.Errorf("usage: crumb storage set <path>")
	}

	storagePath := cmd.Args().Get(0)
	profile := getProfile(cmd)

	// Load or create config
	configDir := filepath.Join(os.Getenv("HOME"), ".config", "crumb")
	configPath := filepath.Join(configDir, "config.yaml")

	var cfg config.Config
	if _, err := os.Stat(configPath); err == nil {
		configData, err := os.ReadFile(configPath)
		if err == nil {
			yaml.Unmarshal(configData, &cfg)
		}
	}

	// Initialize profiles map if needed
	if cfg.Profiles == nil {
		cfg.Profiles = make(map[string]config.ProfileConfig)
	}

	// Get existing profile config or create new one
	profileConfig := cfg.Profiles[profile]
	if profileConfig.PublicKeyPath == "" {
		return fmt.Errorf("profile '%s' not found. Run 'crumb setup --profile %s' first", profile, profile)
	}

	// Update storage path
	profileConfig.Storage = config.ExpandTilde(storagePath)
	cfg.Profiles[profile] = profileConfig

	// Save config
	if err := config.SaveConfig(&cfg); err != nil {
		return err
	}

	fmt.Printf("Storage path set to: %s (profile: %s)\n", profileConfig.Storage, profile)
	return nil
}

// StorageGetCommand handles the storage get command
func StorageGetCommand(_ context.Context, cmd *cli.Command) error {
	profile := getProfile(cmd)
	cfg, err := config.LoadConfig(profile)
	if err != nil {
		return err
	}

	storagePath := config.GetStoragePath(cmd.String("storage"), cfg)
	fmt.Printf("Storage: %s (profile: %s)\n", storagePath, profile)
	return nil
}

// StorageClearCommand handles the storage clear command
func StorageClearCommand(_ context.Context, cmd *cli.Command) error {
	profile := getProfile(cmd)

	// Load config
	configDir := filepath.Join(os.Getenv("HOME"), ".config", "crumb")
	configPath := filepath.Join(configDir, "config.yaml")

	var cfg config.Config
	if _, err := os.Stat(configPath); err != nil {
		return fmt.Errorf("config file not found")
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	if err := yaml.Unmarshal(configData, &cfg); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Clear storage for the profile
	if cfg.Profiles != nil && cfg.Profiles[profile].PublicKeyPath != "" {
		profileConfig := cfg.Profiles[profile]
		profileConfig.Storage = ""
		cfg.Profiles[profile] = profileConfig
	}

	// Save config
	if err := config.SaveConfig(&cfg); err != nil {
		return err
	}

	fmt.Printf("Storage path cleared for profile: %s (using default)\n", profile)
	return nil
}
