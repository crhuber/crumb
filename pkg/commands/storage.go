package commands

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"

	"crumb/pkg/config"
	"crumb/pkg/storage"
)

// StorageSetCommand handles the storage set command
func StorageSetCommand(_ context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() != 1 {
		return fmt.Errorf("usage: crumb storage set <path>")
	}

	storagePath := cmd.Args().Get(0)
	profile := getProfile(cmd)

	// Load or create config
	configDir := filepath.Clean(filepath.Join(os.Getenv("HOME"), ".config", "crumb"))
	configPath := filepath.Clean(filepath.Join(configDir, "config.yaml"))

	var cfg config.Config
	if _, err := os.Stat(configPath); err == nil {
		configData, err := os.ReadFile(configPath)
		if err == nil {
			yaml.Unmarshal(configData, &cfg)
		}
	}

	if cfg.Profiles == nil {
		cfg.Profiles = make(map[string]config.ProfileConfig)
	}

	profileConfig := cfg.Profiles[profile]
	if profileConfig.PublicKeyPath == "" {
		return fmt.Errorf("profile '%s' not found. Run 'crumb setup --profile %s' first", profile, profile)
	}

	// Update local storage path
	expandedPath := config.ExpandTilde(storagePath)
	profileConfig.Storage.Local = &config.LocalStorageConfig{Path: expandedPath}
	profileConfig.Storage.S3 = nil // Clear S3 if switching to local
	cfg.Profiles[profile] = profileConfig

	if err := config.SaveConfig(&cfg); err != nil {
		return err
	}

	fmt.Printf("Storage path set to: %s (profile: %s)\n", expandedPath, profile)
	return nil
}

// StorageGetCommand handles the storage get command
func StorageGetCommand(_ context.Context, cmd *cli.Command) error {
	profile := getProfile(cmd)
	cfg, err := config.LoadConfig(profile)
	if err != nil {
		return err
	}

	if cfg.Storage.S3 != nil {
		s3 := cfg.Storage.S3
		fmt.Printf("Storage: s3://%s/%s (profile: %s)\n", s3.Bucket, s3.Key, profile)
		if s3.EndpointURL != "" {
			fmt.Printf("Endpoint: %s\n", s3.EndpointURL)
		}
	} else {
		path := config.GetLocalStoragePath(cfg)
		if path == "" {
			path = filepath.Join(os.Getenv("HOME"), ".config", "crumb", "secrets")
		}
		fmt.Printf("Storage: %s (profile: %s)\n", path, profile)
	}
	return nil
}

// StorageClearCommand handles the storage clear command
func StorageClearCommand(_ context.Context, cmd *cli.Command) error {
	profile := getProfile(cmd)

	configDir := filepath.Clean(filepath.Join(os.Getenv("HOME"), ".config", "crumb"))
	configPath := filepath.Clean(filepath.Join(configDir, "config.yaml"))

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

	if cfg.Profiles != nil && cfg.Profiles[profile].PublicKeyPath != "" {
		profileConfig := cfg.Profiles[profile]
		profileConfig.Storage = config.StorageConfig{}
		cfg.Profiles[profile] = profileConfig
	}

	if err := config.SaveConfig(&cfg); err != nil {
		return err
	}

	fmt.Printf("Storage path cleared for profile: %s (using default)\n", profile)
	return nil
}

// StorageShowCommand decrypts and displays all secrets in plain text
func StorageShowCommand(_ context.Context, cmd *cli.Command) error {
	cfg, b, err := resolveBackend(cmd)
	if err != nil {
		return err
	}

	secrets, err := storage.LoadSecrets(cfg.PrivateKeyPath, b)
	if err != nil {
		return err
	}

	if len(secrets) == 0 {
		fmt.Println("No secrets found")
		return nil
	}

	fmt.Print("This will display all secrets in plain text. Continue? (y/n): ")
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}
	response = strings.TrimSpace(strings.ToLower(response))
	if response != "y" && response != "yes" {
		fmt.Println("Operation cancelled.")
		return nil
	}

	var keys []string
	for key := range secrets {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		fmt.Printf("%s=%s\n", key, secrets[key])
	}

	return nil
}

// StorageEditCommand decrypts secrets to a temp file, opens $EDITOR, and re-encrypts on save
func StorageEditCommand(_ context.Context, cmd *cli.Command) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		return fmt.Errorf("$EDITOR is not set. Set it with: export EDITOR=vim")
	}

	cfg, b, err := resolveBackend(cmd)
	if err != nil {
		return err
	}

	secrets, err := storage.LoadSecrets(cfg.PrivateKeyPath, b)
	if err != nil {
		return err
	}

	// Build sorted key=value content
	var lines []string
	for key, value := range secrets {
		lines = append(lines, fmt.Sprintf("%s=%s", key, value))
	}
	sort.Strings(lines)
	content := strings.Join(lines, "\n") + "\n"

	// Write to temp file
	tmpFile, err := os.CreateTemp("", "crumb-edit-*.txt")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if err := os.Chmod(tmpPath, 0600); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to set temp file permissions: %w", err)
	}

	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	tmpFile.Close()

	// Open editor — editor is sourced from the user's own $EDITOR/$VISUAL env var
	editorCmd := exec.Command(editor, tmpPath) // #nosec G702 -- intentionally executing user-configured editor
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr

	if err := editorCmd.Run(); err != nil {
		return fmt.Errorf("editor exited with error: %w", err)
	}

	// Read edited content
	editedData, err := os.ReadFile(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to read edited file: %w", err)
	}

	// Parse edited secrets
	newSecrets := storage.ParseSecrets(string(editedData))

	// Save re-encrypted secrets
	if err := storage.SaveSecrets(newSecrets, cfg.PublicKeyPath, b); err != nil {
		return err
	}

	fmt.Printf("Successfully updated secrets (%d keys)\n", len(newSecrets))
	return nil
}
