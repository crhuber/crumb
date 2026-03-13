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

	"crumb/pkg/backend"
	"crumb/pkg/config"
	"crumb/pkg/crypto"
	"crumb/pkg/storage"
)

// SetupCommand handles the setup command for initializing profiles
func SetupCommand(_ context.Context, cmd *cli.Command) error {
	profile := getProfile(cmd)

	// Create ~/.config/crumb directory if it doesn't exist
	configDir := filepath.Clean(filepath.Join(os.Getenv("HOME"), ".config", "crumb"))
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Clean(filepath.Join(configDir, "config.yaml"))

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

	// Determine storage backend type
	storageType := cmd.String("storage")
	if storageType == "" {
		storageType = "local"
	}

	var profileConfig config.ProfileConfig
	profileConfig.PublicKeyPath = publicKeyPath
	profileConfig.PrivateKeyPath = privateKeyPath

	var b backend.Backend

	switch storageType {
	case "s3":
		bucket := cmd.String("s3-bucket")
		key := cmd.String("s3-key")
		endpointURL := cmd.String("s3-endpoint-url")

		if bucket == "" {
			return fmt.Errorf("--s3-bucket is required when using S3 storage")
		}
		if key == "" {
			return fmt.Errorf("--s3-key is required when using S3 storage")
		}

		s3Backend := &backend.S3Backend{
			Bucket:      bucket,
			Key:         key,
			EndpointURL: endpointURL,
		}

		// Verify S3 connectivity
		if _, err := s3Backend.Exists(); err != nil {
			return fmt.Errorf("failed to connect to S3: %w", err)
		}

		profileConfig.Storage.S3 = &config.S3StorageConfig{
			Bucket:      bucket,
			Key:         key,
			EndpointURL: endpointURL,
		}
		b = s3Backend

	default: // "local"
		var storagePath string
		if profile == "default" {
			storagePath = filepath.Join(os.Getenv("HOME"), ".config", "crumb", "secrets")
		} else {
			defaultStorage := fmt.Sprintf("~/.config/crumb/secrets-%s", profile)
			storagePath, err = config.PromptForInput(fmt.Sprintf("Enter storage file path (e.g., %s): ", defaultStorage))
			if err != nil {
				return err
			}
			if strings.TrimSpace(storagePath) == "" {
				storagePath = defaultStorage
			}
		}
		storagePath = config.ExpandTilde(storagePath)

		// Create storage directory if it doesn't exist
		storageDir := filepath.Clean(filepath.Dir(storagePath))
		if err := os.MkdirAll(storageDir, 0700); err != nil {
			return fmt.Errorf("failed to create storage directory: %w", err)
		}

		profileConfig.Storage.Local = &config.LocalStorageConfig{Path: storagePath}
		b = &backend.FileBackend{Path: storagePath}
	}

	// Load existing config or create new one
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

	cfg.Profiles[profile] = profileConfig

	// Save config
	if err := config.SaveConfig(&cfg); err != nil {
		return err
	}

	// Create empty encrypted storage
	if err := storage.CreateEmptyStorage(publicKeyPath, b); err != nil {
		return fmt.Errorf("failed to create secrets storage: %w", err)
	}

	fmt.Printf("Setup completed successfully for profile '%s'!\n", profile)
	fmt.Printf("Config file: %s\n", configPath)
	if profileConfig.Storage.S3 != nil {
		fmt.Printf("Storage: s3://%s/%s\n", profileConfig.Storage.S3.Bucket, profileConfig.Storage.S3.Key)
	} else if profileConfig.Storage.Local != nil {
		fmt.Printf("Storage file: %s\n", profileConfig.Storage.Local.Path)
	}

	return nil
}

// resolveBackend is a helper that loads config and resolves the backend for a command.
func resolveBackend(cmd *cli.Command) (*config.ProfileConfig, backend.Backend, error) {
	profile := getProfile(cmd)
	cfg, err := config.LoadConfig(profile)
	if err != nil {
		return nil, nil, err
	}

	b, err := backend.ResolveBackend(cfg)
	if err != nil {
		return nil, nil, err
	}

	return cfg, b, nil
}

// ListCommand handles the list command
func ListCommand(_ context.Context, cmd *cli.Command) error {
	pathFilter := ""
	if cmd.Args().Len() > 0 {
		pathFilter = cmd.Args().Get(0)
	}

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

	keys := storage.GetFilteredKeys(secrets, pathFilter)

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
	if cmd.Args().Len() < 1 || cmd.Args().Len() > 2 {
		return fmt.Errorf("usage: crumb set <key-path> [value]")
	}

	keyPath := cmd.Args().Get(0)

	if err := config.ValidateKeyPath(keyPath); err != nil {
		return err
	}

	cfg, b, err := resolveBackend(cmd)
	if err != nil {
		return err
	}

	secrets, err := storage.LoadSecrets(cfg.PrivateKeyPath, b)
	if err != nil {
		return err
	}

	if _, exists := storage.SecretExists(secrets, keyPath); exists {
		fmt.Printf("Key '%s' already exists.\n", keyPath)
		if !crypto.ConfirmOverwrite("key") {
			fmt.Println("Operation cancelled.")
			return nil
		}
		os.Stdout.Sync()
	}

	var value string
	if cmd.Args().Len() == 2 {
		value = cmd.Args().Get(1)
	} else {
		value, err = config.PromptForSecret("Enter secret value: ")
		if err != nil {
			return err
		}
	}

	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("secret value cannot be empty")
	}

	storage.SetSecret(secrets, keyPath, value)

	if err := storage.SaveSecrets(secrets, cfg.PublicKeyPath, b); err != nil {
		return err
	}

	fmt.Printf("Successfully set key: %s\n", keyPath)
	return nil
}

// GetCommand handles the get command
func GetCommand(_ context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() != 1 {
		return fmt.Errorf("usage: crumb get <key-path>")
	}

	keyPath := cmd.Args().Get(0)
	maskValue := cmd.Bool("mask")
	exportFormat := cmd.Bool("export")
	shell := cmd.String("shell")

	if err := config.ValidateKeyPath(keyPath); err != nil {
		return err
	}

	cfg, b, err := resolveBackend(cmd)
	if err != nil {
		return err
	}

	secrets, err := storage.LoadSecrets(cfg.PrivateKeyPath, b)
	if err != nil {
		return err
	}

	value, exists := storage.SecretExists(secrets, keyPath)
	if !exists {
		fmt.Println("Key not found.")
		return nil
	}

	if exportFormat {
		varName := storage.ExtractVarName(keyPath)
		switch shell {
		case "bash":
			quotedValue := storage.ShellQuoteValue(value)
			fmt.Printf("export %s=%s\n", varName, quotedValue)
		case "fish":
			quotedValue := storage.ShellQuoteValue(value)
			fmt.Printf("set -x -g %s %s\n", varName, quotedValue)
		default:
			return fmt.Errorf("unsupported shell format: %s (supported: bash, fish)", shell)
		}
		return nil
	}

	if maskValue {
		fmt.Println("****")
	} else {
		fmt.Printf("%s\n", value)
	}

	return nil
}

// InitCommand handles the init command
func InitCommand(_ context.Context, _ *cli.Command) error {
	configFileName := ".crumb.yaml"

	if _, err := os.Stat(configFileName); err == nil {
		if !crypto.ConfirmOverwrite(fmt.Sprintf("Config file %s", configFileName)) {
			fmt.Println("Operation cancelled.")
			return nil
		}
	}

	defaultConfig := config.CreateDefaultCrumbConfig()

	if err := config.SaveCrumbConfig(defaultConfig, configFileName); err != nil {
		return err
	}

	fmt.Printf("Successfully created %s\n", configFileName)
	return nil
}

// DeleteCommand handles the delete command
func DeleteCommand(_ context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() != 1 {
		return fmt.Errorf("usage: crumb delete <key-path>")
	}

	keyPath := cmd.Args().Get(0)

	if err := config.ValidateKeyPath(keyPath); err != nil {
		return err
	}

	cfg, b, err := resolveBackend(cmd)
	if err != nil {
		return err
	}

	secrets, err := storage.LoadSecrets(cfg.PrivateKeyPath, b)
	if err != nil {
		return err
	}

	if _, exists := storage.SecretExists(secrets, keyPath); !exists {
		fmt.Println("Key not found.")
		return nil
	}

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

	if !storage.DeleteSecret(secrets, keyPath) {
		fmt.Println("Key not found.")
		return nil
	}

	if err := storage.SaveSecrets(secrets, cfg.PublicKeyPath, b); err != nil {
		return err
	}

	fmt.Printf("Successfully deleted key: %s\n", keyPath)
	return nil
}

// MoveCommand handles the move command
func MoveCommand(_ context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() != 2 {
		return fmt.Errorf("usage: crumb move <old-key-path> <new-key-path>")
	}

	oldKeyPath := cmd.Args().Get(0)
	newKeyPath := cmd.Args().Get(1)

	if err := config.ValidateKeyPath(oldKeyPath); err != nil {
		return fmt.Errorf("invalid old key path: %w", err)
	}
	if err := config.ValidateKeyPath(newKeyPath); err != nil {
		return fmt.Errorf("invalid new key path: %w", err)
	}

	cfg, b, err := resolveBackend(cmd)
	if err != nil {
		return err
	}

	secrets, err := storage.LoadSecrets(cfg.PrivateKeyPath, b)
	if err != nil {
		return err
	}

	if err := storage.MoveSecret(secrets, oldKeyPath, newKeyPath); err != nil {
		return err
	}

	if err := storage.SaveSecrets(secrets, cfg.PublicKeyPath, b); err != nil {
		return err
	}

	fmt.Printf("Successfully moved key from %s to %s\n", oldKeyPath, newKeyPath)
	return nil
}

// computeEnvDiff compares current environment with new variables and returns a formatted diff string
func computeEnvDiff(newVars map[string]string) string {
	var added []string
	var modified []string

	currentEnv := make(map[string]string)
	for _, envVar := range os.Environ() {
		parts := strings.SplitN(envVar, "=", 2)
		if len(parts) == 2 {
			currentEnv[parts[0]] = parts[1]
		}
	}

	for key, newValue := range newVars {
		if currentValue, exists := currentEnv[key]; exists {
			if currentValue != newValue {
				modified = append(modified, key)
			}
		} else {
			added = append(added, key)
		}
	}

	sort.Strings(added)
	sort.Strings(modified)

	var parts []string
	for _, key := range added {
		parts = append(parts, "+"+key)
	}
	for _, key := range modified {
		parts = append(parts, "~"+key)
	}

	return strings.Join(parts, " ")
}

// ExportCommand handles the export command
func ExportCommand(_ context.Context, cmd *cli.Command) error {
	shell := cmd.String("shell")
	if shell == "" {
		shell = "bash"
	}

	pathFlag := cmd.String("path")

	cfg, b, err := resolveBackend(cmd)
	if err != nil {
		return err
	}

	secrets, err := storage.LoadSecrets(cfg.PrivateKeyPath, b)
	if err != nil {
		return err
	}

	envVars := make(map[string]string)

	if pathFlag != "" {
		isPathPrefix := strings.HasSuffix(pathFlag, "/")

		if isPathPrefix {
			pathPrefix := strings.TrimSuffix(pathFlag, "/")

			comment := fmt.Sprintf("# Exported from %s", pathPrefix)
			switch shell {
			case "bash":
				fmt.Println(comment)
			case "fish":
				fmt.Println(comment)
			}

			pathSecrets := storage.GetSecretsForPath(secrets, pathPrefix)
			for secretPath, secretValue := range pathSecrets {
				keyName := storage.ConvertPathToEnvVar(secretPath, pathPrefix)
				if keyName != "" {
					envVars[keyName] = secretValue
				}
			}
		} else {
			if secretValue, exists := storage.SecretExists(secrets, pathFlag); exists {
				comment := fmt.Sprintf("# Exported from %s", pathFlag)
				switch shell {
				case "bash":
					fmt.Println(comment)
				case "fish":
					fmt.Println(comment)
				}

				keyName := storage.ExtractVarName(pathFlag)
				if keyName != "" {
					envVars[keyName] = secretValue
				}
			}
		}
	} else {
		configFile := cmd.String("file")
		environmentName := cmd.String("env")

		crumbConfig, err := config.LoadCrumbConfig(configFile)
		if err != nil {
			return err
		}

		envConfig, exists := crumbConfig.Environments[environmentName]
		if !exists {
			return fmt.Errorf("environment '%s' not found in %s", environmentName, configFile)
		}

		if envConfig.Path != "" {
			comment := fmt.Sprintf("# Exported from %s (environment: %s)", envConfig.Path, environmentName)
			switch shell {
			case "bash":
				fmt.Println(comment)
			case "fish":
				fmt.Println(comment)
			}

			pathPrefix := strings.TrimSuffix(envConfig.Path, "/")
			pathSecrets := storage.GetSecretsForPath(secrets, pathPrefix)
			for secretPath, secretValue := range pathSecrets {
				keyName := strings.TrimPrefix(secretPath, pathPrefix)
				keyName = strings.TrimPrefix(keyName, "/")
				keyName = strings.ToUpper(strings.ReplaceAll(keyName, "/", "_"))
				keyName = strings.ReplaceAll(keyName, "-", "_")

				if keyName != "" {
					envVars[keyName] = secretValue
				}
			}
		}

		for envVarName, envVarValue := range envConfig.Env {
			sanitizedEnvVarName := strings.ToUpper(strings.ReplaceAll(envVarName, "-", "_"))

			if strings.HasPrefix(envVarValue, "/") {
				if secretValue, exists := storage.SecretExists(secrets, envVarValue); exists {
					envVars[sanitizedEnvVarName] = secretValue
				}
			} else {
				envVars[sanitizedEnvVarName] = envVarValue
			}
		}

		for originalKey, newKey := range envConfig.Remap {
			sanitizedOriginalKey := strings.ToUpper(strings.ReplaceAll(originalKey, "-", "_"))
			sanitizedNewKey := strings.ToUpper(strings.ReplaceAll(newKey, "-", "_"))

			if value, exists := envVars[sanitizedOriginalKey]; exists {
				envVars[sanitizedNewKey] = value
				delete(envVars, sanitizedOriginalKey)
			}
		}
	}

	if len(envVars) == 0 {
		return fmt.Errorf("no secrets found to export")
	}

	diffStatus := computeEnvDiff(envVars)
	if diffStatus != "" {
		fmt.Fprintf(os.Stderr, "crumb: export %s\n", diffStatus)
	}

	var keys []string
	for key := range envVars {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		value := envVars[key]
		switch shell {
		case "bash":
			quotedValue := storage.ShellQuoteValue(value)
			fmt.Printf("export %s=%s\n", key, quotedValue)
		case "fish":
			quotedValue := storage.ShellQuoteValue(value)
			fmt.Printf("set -x -g %s %s\n", key, quotedValue)
		}
	}

	return nil
}

// ImportCommand handles importing secrets from a .env file
func ImportCommand(_ context.Context, cmd *cli.Command) error {
	filePath := cmd.String("file")
	basePath := cmd.String("path")

	if filePath == "" {
		return fmt.Errorf("--file flag is required")
	}
	if basePath == "" {
		return fmt.Errorf("--path flag is required")
	}

	if err := config.ValidateKeyPath(basePath); err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	envVars, err := storage.ParseEnvFile(filePath)
	if err != nil {
		return err
	}

	if len(envVars) == 0 {
		fmt.Println("No environment variables found in the .env file")
		return nil
	}

	cfg, b, err := resolveBackend(cmd)
	if err != nil {
		return err
	}

	secrets, err := storage.LoadSecrets(cfg.PrivateKeyPath, b)
	if err != nil {
		return err
	}

	basePath = strings.TrimSuffix(basePath, "/")

	var conflicts []string
	var newKeys []string

	for envKey := range envVars {
		fullKeyPath := basePath + "/" + envKey

		if _, exists := storage.SecretExists(secrets, fullKeyPath); exists {
			conflicts = append(conflicts, fullKeyPath)
		} else {
			newKeys = append(newKeys, fullKeyPath)
		}
	}

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

	importedCount := 0
	for envKey, envValue := range envVars {
		fullKeyPath := basePath + "/" + envKey
		storage.SetSecret(secrets, fullKeyPath, envValue)
		importedCount++
	}

	if err := storage.SaveSecrets(secrets, cfg.PublicKeyPath, b); err != nil {
		return err
	}

	fmt.Printf("Successfully imported %d secrets from %s to %s\n", importedCount, filePath, basePath)
	return nil
}

// Helper functions

func getProfile(cmd *cli.Command) string {
	return cmd.String("profile")
}
