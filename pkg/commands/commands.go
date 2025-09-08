package commands

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"

	"crumb/pkg/config"
	"crumb/pkg/crypto"
	"crumb/pkg/storage"
)

// SetupCommand handles the setup command for initializing profiles
func SetupCommand(_ context.Context, cmd *cli.Command) error {
	profile := getProfile(cmd)

	// Create ~/.config/crumb directory if it doesn't exist
	configDir := filepath.Join(os.Getenv("HOME"), ".config", "crumb")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(configDir, "config.yaml")

	// Prompt for SSH key paths
	var defaultPublicKey, defaultPrivateKey string
	if profile == "default" {
		defaultPublicKey = "~/.ssh/id_ed25519.pub"
		defaultPrivateKey = "~/.ssh/id_ed25519"
	} else {
		defaultPublicKey = fmt.Sprintf("~/.ssh/%s.pub", profile)
		defaultPrivateKey = fmt.Sprintf("~/.ssh/%s", profile)
	}

	publicKeyPath, err := config.PromptForInput(fmt.Sprintf("Enter path to SSH public key (e.g., %s): ", defaultPublicKey))
	if err != nil {
		return err
	}

	privateKeyPath, err := config.PromptForInput(fmt.Sprintf("Enter path to SSH private key (e.g., %s): ", defaultPrivateKey))
	if err != nil {
		return err
	}

	// Expand tilde in paths
	publicKeyPath = config.ExpandTilde(publicKeyPath)
	privateKeyPath = config.ExpandTilde(privateKeyPath)

	// Validate SSH keys
	if err := crypto.ValidateSSHKeys(publicKeyPath, privateKeyPath); err != nil {
		return fmt.Errorf("invalid or missing SSH key pair. Please generate an SSH key pair using `ssh-keygen -t rsa` or `ssh-keygen -t ed25519` first: %w", err)
	}

	// Get storage path (from CLI flag or prompt if not provided)
	storagePath := cmd.String("storage")
	if storagePath == "" {
		if profile == "default" {
			storagePath = filepath.Join(os.Getenv("HOME"), ".config", "crumb", "secrets")
		} else {
			defaultStorage := fmt.Sprintf("~/.config/crumb/secrets-%s", profile)
			storagePath, err = config.PromptForInput(fmt.Sprintf("Enter storage file path (e.g., %s): ", defaultStorage))
			if err != nil {
				return err
			}
			// Use default if empty
			if strings.TrimSpace(storagePath) == "" {
				storagePath = defaultStorage
			}
		}
	}
	storagePath = config.ExpandTilde(storagePath)

	// Create storage directory if it doesn't exist
	storageDir := filepath.Dir(storagePath)
	if err := os.MkdirAll(storageDir, 0700); err != nil {
		return fmt.Errorf("failed to create storage directory: %w", err)
	}

	// Load existing config or create new one
	var cfg config.Config
	if _, err := os.Stat(configPath); err == nil {
		// Try to load existing config to preserve other profiles
		configData, err := os.ReadFile(configPath)
		if err == nil {
			// Parse existing full config to preserve other profiles
			// This loads the complete config with all profiles
			yaml.Unmarshal(configData, &cfg)
		}
	}

	// Initialize profiles map if it doesn't exist
	if cfg.Profiles == nil {
		cfg.Profiles = make(map[string]config.ProfileConfig)
	}

	// Create profile configuration
	profileConfig := config.ProfileConfig{
		PublicKeyPath:  publicKeyPath,
		PrivateKeyPath: privateKeyPath,
		Storage:        storagePath,
	}

	cfg.Profiles[profile] = profileConfig

	// Save config
	if err := config.SaveConfig(&cfg); err != nil {
		return err
	}

	// Create empty encrypted secrets file
	if err := storage.CreateEmptySecretsFile(storagePath, publicKeyPath); err != nil {
		return fmt.Errorf("failed to create secrets file: %w", err)
	}

	fmt.Printf("Setup completed successfully for profile '%s'!\n", profile)
	fmt.Printf("Config file: %s\n", configPath)
	fmt.Printf("Storage file: %s\n", storagePath)

	return nil
}

// ListCommand handles the list command
func ListCommand(_ context.Context, cmd *cli.Command) error {
	// Get optional path filter argument
	pathFilter := ""
	if cmd.Args().Len() > 0 {
		pathFilter = cmd.Args().Get(0)
	}

	// Load configuration
	profile := getProfile(cmd)
	cfg, err := config.LoadConfig(profile)
	if err != nil {
		return err
	}

	// Get storage path
	storagePath := config.GetStoragePath(cmd.String("storage"), cfg)

	// Load and decrypt secrets
	secrets, err := storage.LoadSecrets(cfg.PrivateKeyPath, storagePath)
	if err != nil {
		return err
	}

	// Check if secrets is empty
	if len(secrets) == 0 {
		fmt.Println("No secrets found")
		return nil
	}

	// Get filtered and sorted keys
	keys := storage.GetFilteredKeys(secrets, pathFilter)

	// Display keys
	if len(keys) == 0 {
		if pathFilter != "" {
			fmt.Printf("No secrets found matching path: %s\n", pathFilter)
		} else {
			fmt.Println("No secrets found")
		}
		return nil
	}

	for _, key := range keys {
		fmt.Println(key)
	}

	return nil
}

// SetCommand handles the set command
func SetCommand(_ context.Context, cmd *cli.Command) error {
	// Check arguments
	if cmd.Args().Len() != 2 {
		return fmt.Errorf("usage: crumb set <key-path> <value>")
	}

	keyPath := cmd.Args().Get(0)
	value := cmd.Args().Get(1)

	// Validate key path
	if err := config.ValidateKeyPath(keyPath); err != nil {
		return err
	}

	// Load configuration
	profile := getProfile(cmd)
	cfg, err := config.LoadConfig(profile)
	if err != nil {
		return err
	}

	// Get storage path
	storagePath := config.GetStoragePath(cmd.String("storage"), cfg)

	// Load and decrypt secrets
	secrets, err := storage.LoadSecrets(cfg.PrivateKeyPath, storagePath)
	if err != nil {
		return err
	}

	// Check if key exists and prompt for overwrite
	if existingValue, exists := storage.SecretExists(secrets, keyPath); exists {
		fmt.Printf("Key '%s' already exists with value: %s\n", keyPath, existingValue)
		if !crypto.ConfirmOverwrite("key") {
			fmt.Println("Operation cancelled.")
			return nil
		}
	}

	// Update the key-value pair
	storage.SetSecret(secrets, keyPath, value)

	// Save encrypted secrets
	if err := storage.SaveSecrets(secrets, cfg.PublicKeyPath, storagePath); err != nil {
		return err
	}

	fmt.Printf("Successfully set key: %s\n", keyPath)
	return nil
}

// GetCommand handles the get command
func GetCommand(_ context.Context, cmd *cli.Command) error {
	// Check arguments
	if cmd.Args().Len() != 1 {
		return fmt.Errorf("usage: crumb get <key-path>")
	}

	keyPath := cmd.Args().Get(0)
	showValue := cmd.Bool("show")
	exportFormat := cmd.Bool("export")
	shell := cmd.String("shell")

	// Validate key path
	if err := config.ValidateKeyPath(keyPath); err != nil {
		return err
	}

	// Load configuration
	profile := getProfile(cmd)
	cfg, err := config.LoadConfig(profile)
	if err != nil {
		return err
	}

	// Get storage path
	storagePath := config.GetStoragePath(cmd.String("storage"), cfg)

	// Load and decrypt secrets
	secrets, err := storage.LoadSecrets(cfg.PrivateKeyPath, storagePath)
	if err != nil {
		return err
	}

	// Check if key exists
	value, exists := storage.SecretExists(secrets, keyPath)
	if !exists {
		fmt.Println("Key not found.")
		return nil
	}

	// Handle export format
	if exportFormat {
		// Extract variable name from key path
		varName := storage.ExtractVarName(keyPath)
		switch shell {
		case "bash":
			fmt.Printf("export %s=%s\n", varName, value)
		case "fish":
			fmt.Printf("set -x %s %s\n", varName, value)
		default:
			return fmt.Errorf("unsupported shell format: %s (supported: bash, fish)", shell)
		}
		return nil
	}

	// Display the value (existing behavior)
	if showValue {
		fmt.Printf("%s\n", value)
	} else {
		fmt.Println("****")
	}

	return nil
}

// InitCommand handles the init command
func InitCommand(_ context.Context, _ *cli.Command) error {
	configFileName := ".crumb.yaml"

	// Check if .crumb.yaml already exists
	if _, err := os.Stat(configFileName); err == nil {
		if !crypto.ConfirmOverwrite(fmt.Sprintf("Config file %s", configFileName)) {
			fmt.Println("Operation cancelled.")
			return nil
		}
	}

	// Create default config structure
	defaultConfig := config.CreateDefaultCrumbConfig()

	// Save to file
	if err := config.SaveCrumbConfig(defaultConfig, configFileName); err != nil {
		return err
	}

	fmt.Printf("Successfully created %s\n", configFileName)
	return nil
}

// DeleteCommand handles the delete command
func DeleteCommand(_ context.Context, cmd *cli.Command) error {
	// Check arguments
	if cmd.Args().Len() != 1 {
		return fmt.Errorf("usage: crumb delete <key-path>")
	}

	keyPath := cmd.Args().Get(0)

	// Validate key path
	if err := config.ValidateKeyPath(keyPath); err != nil {
		return err
	}

	// Load configuration
	profile := getProfile(cmd)
	cfg, err := config.LoadConfig(profile)
	if err != nil {
		return err
	}

	// Get storage path
	storagePath := config.GetStoragePath(cmd.String("storage"), cfg)

	// Load and decrypt secrets
	secrets, err := storage.LoadSecrets(cfg.PrivateKeyPath, storagePath)
	if err != nil {
		return err
	}

	// Check if key exists
	if _, exists := storage.SecretExists(secrets, keyPath); !exists {
		fmt.Println("Key not found.")
		return nil
	}

	// Prompt user to confirm by typing exact key path
	fmt.Printf("Type the key path to confirm deletion: ")
	reader := bufio.NewReader(os.Stdin)
	confirmation, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read confirmation: %w", err)
	}

	confirmation = strings.TrimSpace(confirmation)
	if confirmation != keyPath {
		fmt.Println("Confirmation failed. Deletion cancelled.")
		return nil
	}

	// Remove the key-value pair
	if !storage.DeleteSecret(secrets, keyPath) {
		fmt.Println("Key not found.")
		return nil
	}

	// Save encrypted secrets
	if err := storage.SaveSecrets(secrets, cfg.PublicKeyPath, storagePath); err != nil {
		return err
	}

	fmt.Printf("Successfully deleted key: %s\n", keyPath)
	return nil
}

// MoveCommand handles the move command
func MoveCommand(_ context.Context, cmd *cli.Command) error {
	// Check arguments
	if cmd.Args().Len() != 2 {
		return fmt.Errorf("usage: crumb move <old-key-path> <new-key-path>")
	}

	oldKeyPath := cmd.Args().Get(0)
	newKeyPath := cmd.Args().Get(1)

	// Validate key paths
	if err := config.ValidateKeyPath(oldKeyPath); err != nil {
		return fmt.Errorf("invalid old key path: %w", err)
	}
	if err := config.ValidateKeyPath(newKeyPath); err != nil {
		return fmt.Errorf("invalid new key path: %w", err)
	}

	// Load configuration
	profile := getProfile(cmd)
	cfg, err := config.LoadConfig(profile)
	if err != nil {
		return err
	}

	// Get storage path
	storagePath := config.GetStoragePath(cmd.String("storage"), cfg)

	// Load and decrypt secrets
	secrets, err := storage.LoadSecrets(cfg.PrivateKeyPath, storagePath)
	if err != nil {
		return err
	}

	// Use storage package's MoveSecret function
	if err := storage.MoveSecret(secrets, oldKeyPath, newKeyPath); err != nil {
		return err
	}

	// Save encrypted secrets
	if err := storage.SaveSecrets(secrets, cfg.PublicKeyPath, storagePath); err != nil {
		return err
	}

	fmt.Printf("Successfully moved key from %s to %s\n", oldKeyPath, newKeyPath)
	return nil
}

// ExportCommand handles the export command
func ExportCommand(_ context.Context, cmd *cli.Command) error {
	// Get shell format (default to bash)
	shell := cmd.String("shell")
	if shell == "" {
		shell = "bash"
	}

	// Check if --path flag is provided
	pathFlag := cmd.String("path")

	// Load application configuration
	profile := getProfile(cmd)
	cfg, err := config.LoadConfig(profile)
	if err != nil {
		return err
	}

	// Get storage path
	storagePath := config.GetStoragePath(cmd.String("storage"), cfg)

	// Load and decrypt secrets
	secrets, err := storage.LoadSecrets(cfg.PrivateKeyPath, storagePath)
	if err != nil {
		return err
	}

	// Collect environment variables to export
	envVars := make(map[string]string)

	if pathFlag != "" {
		// Direct path export mode - export all secrets matching the path
		pathPrefix := strings.TrimSuffix(pathFlag, "/")

		// Add comment for clarity
		comment := fmt.Sprintf("# Exported from %s", pathPrefix)
		switch shell {
		case "bash":
			fmt.Println(comment)
		case "fish":
			fmt.Println(comment)
		}

		// Find all secrets that match the path prefix
		pathSecrets := storage.GetSecretsForPath(secrets, pathPrefix)
		for secretPath, secretValue := range pathSecrets {
			keyName := storage.ConvertPathToEnvVar(secretPath, pathPrefix)
			if keyName != "" {
				envVars[keyName] = secretValue
			}
		}
	} else {
		// .crumb.yaml mode - existing logic
		configFile := cmd.String("file")

		// Load .crumb.yaml configuration
		crumbConfig, err := config.LoadCrumbConfig(configFile)
		if err != nil {
			return err
		}

		// Process path_sync section
		if crumbConfig.PathSync.Path != "" {
			// Add comment for clarity
			comment := fmt.Sprintf("# Exported from %s", crumbConfig.PathSync.Path)
			switch shell {
			case "bash":
				fmt.Println(comment)
			case "fish":
				fmt.Println(comment)
			}

			// Find all secrets that match the path prefix
			pathPrefix := strings.TrimSuffix(crumbConfig.PathSync.Path, "/")
			pathSecrets := storage.GetSecretsForPath(secrets, pathPrefix)
			for secretPath, secretValue := range pathSecrets {
				// Extract the key name from the path
				keyName := strings.TrimPrefix(secretPath, pathPrefix)
				keyName = strings.TrimPrefix(keyName, "/")
				keyName = strings.ToUpper(strings.ReplaceAll(keyName, "/", "_"))
				keyName = strings.ReplaceAll(keyName, "-", "_")

				if keyName != "" {
					envVars[keyName] = secretValue
				}
			}
		}

		// Process env section
		for envVarName, envConfig := range crumbConfig.Env {
			if secretValue, exists := storage.SecretExists(secrets, envConfig.Path); exists {
				// Sanitize environment variable name
				sanitizedEnvVarName := strings.ToUpper(strings.ReplaceAll(envVarName, "-", "_"))
				envVars[sanitizedEnvVarName] = secretValue
			}
		}

		// Apply remap mappings
		for originalKey, newKey := range crumbConfig.PathSync.Remap {
			// Sanitize both original and new key names
			sanitizedOriginalKey := strings.ToUpper(strings.ReplaceAll(originalKey, "-", "_"))
			sanitizedNewKey := strings.ToUpper(strings.ReplaceAll(newKey, "-", "_"))

			if value, exists := envVars[sanitizedOriginalKey]; exists {
				envVars[sanitizedNewKey] = value
				delete(envVars, sanitizedOriginalKey)
			}
		}
	}

	// Generate shell output
	if len(envVars) == 0 {
		return fmt.Errorf("no secrets found to export")
	}

	// Sort keys for consistent output
	var keys []string
	for key := range envVars {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Output environment variables
	for _, key := range keys {
		value := envVars[key]
		switch shell {
		case "bash":
			fmt.Printf("export %s=%s\n", key, value)
		case "fish":
			fmt.Printf("set -x %s %s\n", key, value)
		}
	}

	return nil
}

// ImportCommand handles importing secrets from a .env file
func ImportCommand(_ context.Context, cmd *cli.Command) error {
	// Check required flags
	filePath := cmd.String("file")
	basePath := cmd.String("path")

	if filePath == "" {
		return fmt.Errorf("--file flag is required")
	}
	if basePath == "" {
		return fmt.Errorf("--path flag is required")
	}

	// Validate base path
	if err := config.ValidateKeyPath(basePath); err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Parse .env file
	envVars, err := storage.ParseEnvFile(filePath)
	if err != nil {
		return err
	}

	if len(envVars) == 0 {
		fmt.Println("No environment variables found in the .env file")
		return nil
	}

	// Load configuration
	profile := getProfile(cmd)
	cfg, err := config.LoadConfig(profile)
	if err != nil {
		return err
	}

	// Get storage path
	storagePath := config.GetStoragePath(cmd.String("storage"), cfg)

	// Load and decrypt existing secrets
	secrets, err := storage.LoadSecrets(cfg.PrivateKeyPath, storagePath)
	if err != nil {
		return err
	}

	// Ensure base path ends without trailing slash for consistency
	basePath = strings.TrimSuffix(basePath, "/")

	// Track conflicts and new keys
	var conflicts []string
	var newKeys []string

	// Process each environment variable
	for envKey := range envVars {
		// Create full key path (preserve original case)
		fullKeyPath := basePath + "/" + envKey

		// Check if key already exists
		if _, exists := storage.SecretExists(secrets, fullKeyPath); exists {
			conflicts = append(conflicts, fullKeyPath)
		} else {
			newKeys = append(newKeys, fullKeyPath)
		}
	}

	// Display summary
	fmt.Printf("Found %d environment variables in %s\n", len(envVars), filePath)
	if len(newKeys) > 0 {
		fmt.Printf("New keys to import: %d\n", len(newKeys))
	}
	if len(conflicts) > 0 {
		fmt.Printf("Existing keys that will be updated: %d\n", len(conflicts))
		for _, key := range conflicts {
			fmt.Printf("  - %s\n", key)
		}
	}

	// Confirm import if there are conflicts
	if len(conflicts) > 0 {
		fmt.Print("Continue with import? This will overwrite existing keys. (y/n): ")
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Import cancelled.")
			return nil
		}
	}

	// Import the secrets
	importedCount := 0
	for envKey, envValue := range envVars {
		fullKeyPath := basePath + "/" + envKey
		storage.SetSecret(secrets, fullKeyPath, envValue)
		importedCount++
	}

	// Save encrypted secrets
	if err := storage.SaveSecrets(secrets, cfg.PublicKeyPath, storagePath); err != nil {
		return err
	}

	fmt.Printf("Successfully imported %d secrets from %s to %s\n", importedCount, filePath, basePath)
	return nil
}

// Helper functions

func getProfile(cmd *cli.Command) string {
	return cmd.String("profile")
}
