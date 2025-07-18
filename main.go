package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"filippo.io/age"
	"filippo.io/age/agessh"
	"github.com/urfave/cli/v2"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

// Version information (injected by GoReleaser)
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// Config represents the configuration stored in ~/.config/crumb/config.yaml
type Config struct {
	Profiles map[string]ProfileConfig `yaml:"profiles"`
}

// ProfileConfig represents a single profile configuration
type ProfileConfig struct {
	PublicKeyPath  string `yaml:"public_key_path"`
	PrivateKeyPath string `yaml:"private_key_path"`
	Storage        string `yaml:"storage,omitempty"`
}

// CrumbConfig represents the per-project configuration in .crumb.yaml
type CrumbConfig struct {
	Version  string               `yaml:"version"`
	PathSync PathSync             `yaml:"path_sync"`
	Env      map[string]EnvConfig `yaml:"env"`
}

type PathSync struct {
	Path  string            `yaml:"path"`
	Remap map[string]string `yaml:"remap"`
}

type EnvConfig struct {
	Path string `yaml:"path"`
}

func main() {
	cli.VersionPrinter = func(cCtx *cli.Context) {
		fmt.Printf("version=%s commit=%s date=%s\n", cCtx.App.Version, commit, date)
	}
	app := &cli.App{
		Name:    "crumb",
		Usage:   "Securely store, manage, and export API keys and secrets",
		Version: version,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "profile",
				Usage:   "Profile to use for configuration",
				Value:   "default",
				EnvVars: []string{"CRUMB_PROFILE"},
			},
			&cli.StringFlag{
				Name:    "storage",
				Usage:   "Storage file path",
				EnvVars: []string{"CRUMB_STORAGE"},
			},
		},
		Commands: []*cli.Command{
			{
				Name:   "setup",
				Usage:  "Initialize the secure storage backend",
				Action: setupCommand,
			},
			{
				Name:      "ls",
				Usage:     "List stored secret keys",
				Action:    listCommand,
				ArgsUsage: "[path]",
			},
			{
				Name:      "set",
				Usage:     "Add or update a secret key-value pair",
				Action:    setCommand,
				ArgsUsage: "<key-path> <value>",
			},
			{
				Name:      "get",
				Usage:     "Retrieve a secret by its key path",
				Action:    getCommand,
				ArgsUsage: "<key-path>",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "show",
						Usage: "Display the actual secret value instead of masking it",
					},
				},
			},
			{
				Name:   "init",
				Usage:  "Create a YAML configuration file in current directory",
				Action: initCommand,
			},
			{
				Name:      "delete",
				Usage:     "Delete a secret key-value pair",
				Action:    deleteCommand,
				ArgsUsage: "<key-path>",
			},
			{
				Name:  "export",
				Usage: "Export secrets as shell-compatible environment variables",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "shell",
						Usage: "Shell format (bash or fish)",
						Value: "bash",
					},
					&cli.StringFlag{
						Name:    "file",
						Aliases: []string{"f"},
						Usage:   "Configuration file to use (default: .crumb.yaml)",
						Value:   ".crumb.yaml",
					},
				},
				Action: exportCommand,
			},
			{
				Name:  "storage",
				Usage: "Manage storage file configuration",
				Subcommands: []*cli.Command{
					{
						Name:      "set",
						Usage:     "Set storage file path for current profile",
						ArgsUsage: "<path>",
						Action:    storageSetCommand,
					},
					{
						Name:   "get",
						Usage:  "Show current storage file path for current profile",
						Action: storageGetCommand,
					},
					{
						Name:   "clear",
						Usage:  "Clear storage file path for current profile (use default)",
						Action: storageClearCommand,
					},
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func setupCommand(c *cli.Context) error {
	profile := getProfile(c)

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

	publicKeyPath, err := promptForKeyPath(fmt.Sprintf("Enter path to SSH public key (e.g., %s): ", defaultPublicKey))
	if err != nil {
		return err
	}

	privateKeyPath, err := promptForKeyPath(fmt.Sprintf("Enter path to SSH private key (e.g., %s): ", defaultPrivateKey))
	if err != nil {
		return err
	}

	// Expand tilde in paths
	publicKeyPath = expandTilde(publicKeyPath)
	privateKeyPath = expandTilde(privateKeyPath)

	// Validate SSH keys
	if err := validateSSHKeys(publicKeyPath, privateKeyPath); err != nil {
		return fmt.Errorf("invalid or missing SSH key pair. Please generate an SSH key pair using `ssh-keygen -t rsa` or `ssh-keygen -t ed25519` first: %w", err)
	}

	// Get storage path (from CLI flag or prompt if not provided)
	storagePath := c.String("storage")
	if storagePath == "" {
		if profile == "default" {
			storagePath = filepath.Join(os.Getenv("HOME"), ".config", "crumb", "secrets")
		} else {
			defaultStorage := fmt.Sprintf("~/.config/crumb/secrets-%s", profile)
			storagePath, err = promptForKeyPath(fmt.Sprintf("Enter storage file path (e.g., %s): ", defaultStorage))
			if err != nil {
				return err
			}
			// Use default if empty
			if strings.TrimSpace(storagePath) == "" {
				storagePath = defaultStorage
			}
		}
	}
	storagePath = expandTilde(storagePath)

	// Create storage directory if it doesn't exist
	storageDir := filepath.Dir(storagePath)
	if err := os.MkdirAll(storageDir, 0700); err != nil {
		return fmt.Errorf("failed to create storage directory: %w", err)
	}

	// Load existing config or create new one
	var config Config
	if _, err := os.Stat(configPath); err == nil {
		configData, err := os.ReadFile(configPath)
		if err == nil {
			yaml.Unmarshal(configData, &config)
		}
	}

	// Initialize profiles map if it doesn't exist
	if config.Profiles == nil {
		config.Profiles = make(map[string]ProfileConfig)
	}

	// Create profile configuration
	profileConfig := ProfileConfig{
		PublicKeyPath:  publicKeyPath,
		PrivateKeyPath: privateKeyPath,
		Storage:        storagePath,
	}

	config.Profiles[profile] = profileConfig

	// Save config
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	err = encoder.Encode(&config)
	encoder.Close()
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	// Create empty encrypted secrets file
	if err := createEmptySecretsFile(storagePath, publicKeyPath); err != nil {
		return fmt.Errorf("failed to create secrets file: %w", err)
	}

	fmt.Printf("Setup completed successfully for profile '%s'!\n", profile)
	fmt.Printf("Config file: %s\n", configPath)
	fmt.Printf("Storage file: %s\n", storagePath)

	return nil
}

func promptForKeyPath(prompt string) (string, error) {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}
	return strings.TrimSpace(input), nil
}

func expandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(os.Getenv("HOME"), path[2:])
	}
	return path
}

func confirmOverwrite(item string) bool {
	fmt.Printf("%s already exists. Overwrite? (y/n): ", item)

	// Use terminal to read a single character
	oldState, err := term.MakeRaw(int(syscall.Stdin))
	if err != nil {
		return false
	}
	defer term.Restore(int(syscall.Stdin), oldState)

	var response [1]byte
	_, err = os.Stdin.Read(response[:])
	if err != nil {
		return false
	}

	fmt.Println() // Print newline after the character

	return response[0] == 'y' || response[0] == 'Y'
}

func validateSSHKeys(publicKeyPath, privateKeyPath string) error {
	// Check if files exist
	if _, err := os.Stat(publicKeyPath); os.IsNotExist(err) {
		return fmt.Errorf("public key file not found: %s", publicKeyPath)
	}

	if _, err := os.Stat(privateKeyPath); os.IsNotExist(err) {
		return fmt.Errorf("private key file not found: %s", privateKeyPath)
	}

	// Read public key
	publicKeyData, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read public key: %w", err)
	}

	// Validate public key format (ssh-rsa or ssh-ed25519)
	publicKeyStr := strings.TrimSpace(string(publicKeyData))
	if !strings.HasPrefix(publicKeyStr, "ssh-rsa ") && !strings.HasPrefix(publicKeyStr, "ssh-ed25519 ") {
		return fmt.Errorf("public key must be of type ssh-rsa or ssh-ed25519")
	}

	// Try to parse the public key with agessh
	_, err = agessh.ParseRecipient(publicKeyStr)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %w", err)
	}

	// Read private key
	privateKeyData, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read private key: %w", err)
	}

	// Try to parse the private key with agessh
	_, err = agessh.ParseIdentity(privateKeyData)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	return nil
}

func createEmptySecretsFile(secretsPath, publicKeyPath string) error {
	// Read public key
	publicKeyData, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read public key: %w", err)
	}

	// Parse public key as age recipient using agessh
	recipient, err := agessh.ParseRecipient(strings.TrimSpace(string(publicKeyData)))
	if err != nil {
		return fmt.Errorf("failed to parse public key as recipient: %w", err)
	}

	// Create empty secrets content
	emptyContent := ""

	// Encrypt the empty content
	encryptedData, err := encryptData(emptyContent, []age.Recipient{recipient})
	if err != nil {
		return fmt.Errorf("failed to encrypt empty secrets: %w", err)
	}

	// Write encrypted file with file locking
	return writeFileWithLock(secretsPath, encryptedData, 0600)
}

func encryptData(data string, recipients []age.Recipient) ([]byte, error) {
	var buf strings.Builder
	w, err := age.Encrypt(&buf, recipients...)
	if err != nil {
		return nil, fmt.Errorf("failed to create encryptor: %w", err)
	}

	if _, err := io.WriteString(w, data); err != nil {
		return nil, fmt.Errorf("failed to write data to encryptor: %w", err)
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("failed to close encryptor: %w", err)
	}

	return []byte(buf.String()), nil
}

func writeFileWithLock(filePath string, data []byte, perm os.FileMode) error {
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Apply file lock
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX); err != nil {
		return fmt.Errorf("failed to lock file: %w", err)
	}
	defer unix.Flock(int(file.Fd()), unix.LOCK_UN)

	_, err = file.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}

	return nil
}

// Helper functions for key validation and secret management

func validateKeyPath(keyPath string) error {
	if keyPath == "" {
		return fmt.Errorf("key path cannot be empty")
	}

	if !strings.HasPrefix(keyPath, "/") {
		return fmt.Errorf("key path must start with '/'")
	}

	if strings.Contains(keyPath, " ") {
		return fmt.Errorf("key path cannot contain spaces")
	}

	if strings.Contains(keyPath, "=") {
		return fmt.Errorf("key path cannot contain '=' character")
	}

	if strings.Contains(keyPath, "\n") {
		return fmt.Errorf("key path cannot contain newlines")
	}

	if strings.Contains(keyPath, "\t") {
		return fmt.Errorf("key path cannot contain tabs")
	}

	return nil
}

func loadConfig(profile string) (*ProfileConfig, error) {
	configPath := filepath.Join(os.Getenv("HOME"), ".config", "crumb", "config.yaml")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("configuration not found. Run 'crumb setup' first")
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Check for profile
	if config.Profiles != nil {
		if profileConfig, exists := config.Profiles[profile]; exists {
			return &profileConfig, nil
		}
	}

	return nil, fmt.Errorf("profile '%s' not found. Run 'crumb setup --profile %s' first", profile, profile)
}

// Helper functions for profile management
func getProfile(c *cli.Context) string {
	return c.String("profile")
}

func getStoragePath(c *cli.Context, profile *ProfileConfig) string {
	// Priority: CLI flag > profile storage > default
	if storageFlag := c.String("storage"); storageFlag != "" {
		return expandTilde(storageFlag)
	}

	if profile.Storage != "" {
		return expandTilde(profile.Storage)
	}

	// Default storage path
	return filepath.Join(os.Getenv("HOME"), ".config", "crumb", "secrets")
}

func loadSecrets(privateKeyPath, storagePath string) (map[string]string, error) {
	if _, err := os.Stat(storagePath); os.IsNotExist(err) {
		return make(map[string]string), nil
	}

	// Read private key
	privateKeyData, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}

	// Parse private key identity
	identity, err := agessh.ParseIdentity(privateKeyData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	// Read and decrypt secrets file
	encryptedData, err := readFileWithLock(storagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read secrets file: %w", err)
	}

	if len(encryptedData) == 0 {
		return make(map[string]string), nil
	}

	decryptedData, err := decryptData(encryptedData, identity)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt secrets: %w", err)
	}

	// Parse key-value pairs
	secrets := make(map[string]string)
	lines := strings.Split(strings.TrimSpace(decryptedData), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		secrets[key] = value
	}

	return secrets, nil
}

func saveSecrets(secrets map[string]string, publicKeyPath, storagePath string) error {
	// Read public key
	publicKeyData, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read public key: %w", err)
	}

	// Parse public key as recipient
	recipient, err := agessh.ParseRecipient(strings.TrimSpace(string(publicKeyData)))
	if err != nil {
		return fmt.Errorf("failed to parse public key: %w", err)
	}

	// Convert secrets map to string format
	var lines []string
	for key, value := range secrets {
		lines = append(lines, fmt.Sprintf("%s=%s", key, value))
	}

	// Sort lines for consistent output
	sort.Strings(lines)
	content := strings.Join(lines, "\n")

	// Encrypt the content
	encryptedData, err := encryptData(content, []age.Recipient{recipient})
	if err != nil {
		return fmt.Errorf("failed to encrypt secrets: %w", err)
	}

	// Write encrypted file with locking
	return writeFileWithLock(storagePath, encryptedData, 0600)
}

func readFileWithLock(filePath string) ([]byte, error) {
	file, err := os.OpenFile(filePath, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Apply file lock
	if err := unix.Flock(int(file.Fd()), unix.LOCK_SH); err != nil {
		return nil, fmt.Errorf("failed to lock file: %w", err)
	}
	defer unix.Flock(int(file.Fd()), unix.LOCK_UN)

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return data, nil
}

func decryptData(encryptedData []byte, identity age.Identity) (string, error) {
	r, err := age.Decrypt(strings.NewReader(string(encryptedData)), identity)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt data: %w", err)
	}

	decryptedData, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("failed to read decrypted data: %w", err)
	}

	return string(decryptedData), nil
}

// parseSecrets parses the decrypted secrets content into a map
func parseSecrets(content string) map[string]string {
	secrets := make(map[string]string)
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			secrets[parts[0]] = parts[1]
		}
	}

	return secrets
}

// Placeholder functions for other commands
func listCommand(c *cli.Context) error {
	// Get optional path filter argument
	pathFilter := ""
	if c.Args().Len() > 0 {
		pathFilter = c.Args().Get(0)
	}

	// Load configuration
	profile := getProfile(c)
	config, err := loadConfig(profile)
	if err != nil {
		return err
	}

	// Get storage path
	storagePath := getStoragePath(c, config)

	// Load and decrypt secrets
	secrets, err := loadSecrets(config.PrivateKeyPath, storagePath)
	if err != nil {
		return err
	}

	// Check if secrets is empty
	if len(secrets) == 0 {
		fmt.Println("No secrets found")
		return nil
	}

	// Get filtered and sorted keys
	keys := getFilteredKeys(secrets, pathFilter)

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

func setCommand(c *cli.Context) error {
	// Check arguments
	if c.Args().Len() != 2 {
		return fmt.Errorf("usage: crumb set <key-path> <value>")
	}

	keyPath := c.Args().Get(0)
	value := c.Args().Get(1)

	// Validate key path
	if err := validateKeyPath(keyPath); err != nil {
		return err
	}

	// Load configuration
	profile := getProfile(c)
	config, err := loadConfig(profile)
	if err != nil {
		return err
	}

	// Get storage path
	storagePath := getStoragePath(c, config)

	// Load and decrypt secrets
	secrets, err := loadSecrets(config.PrivateKeyPath, storagePath)
	if err != nil {
		return err
	}

	// Check if key exists and prompt for overwrite
	if existingValue, exists := secrets[keyPath]; exists {
		fmt.Printf("Key '%s' already exists with value: %s\n", keyPath, existingValue)
		if !confirmOverwrite("key") {
			fmt.Println("Operation cancelled.")
			return nil
		}
	}

	// Update the key-value pair
	secrets[keyPath] = value

	// Save encrypted secrets
	if err := saveSecrets(secrets, config.PublicKeyPath, storagePath); err != nil {
		return err
	}

	fmt.Printf("Successfully set key: %s\n", keyPath)
	return nil
}

func initCommand(_ *cli.Context) error {
	configFileName := ".crumb.yaml"

	// Check if .crumb.yaml already exists
	if _, err := os.Stat(configFileName); err == nil {
		if !confirmOverwrite(fmt.Sprintf("Config file %s", configFileName)) {
			fmt.Println("Operation cancelled.")
			return nil
		}
	}

	// Create default config structure
	defaultConfig := CrumbConfig{
		Version: "1.0",
		PathSync: PathSync{
			Path:  "",
			Remap: make(map[string]string),
		},
		Env: make(map[string]EnvConfig),
	}

	// Marshal to YAML with 2-space indentation
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(&defaultConfig); err != nil {
		return fmt.Errorf("failed to marshal default config: %w", err)
	}
	encoder.Close()

	// Write to file
	if err := os.WriteFile(configFileName, buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("Successfully created %s\n", configFileName)
	return nil
}

func deleteCommand(c *cli.Context) error {
	// Check arguments
	if c.Args().Len() != 1 {
		return fmt.Errorf("usage: crumb delete <key-path>")
	}

	keyPath := c.Args().Get(0)

	// Validate key path
	if err := validateKeyPath(keyPath); err != nil {
		return err
	}

	// Load configuration
	profile := getProfile(c)
	config, err := loadConfig(profile)
	if err != nil {
		return err
	}

	// Get storage path
	storagePath := getStoragePath(c, config)

	// Load and decrypt secrets
	secrets, err := loadSecrets(config.PrivateKeyPath, storagePath)
	if err != nil {
		return err
	}

	// Check if key exists
	if _, exists := secrets[keyPath]; !exists {
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
	delete(secrets, keyPath)

	// Save encrypted secrets
	if err := saveSecrets(secrets, config.PublicKeyPath, storagePath); err != nil {
		return err
	}

	fmt.Printf("Successfully deleted key: %s\n", keyPath)
	return nil
}

func exportCommand(c *cli.Context) error {
	// Get shell format (default to bash)
	shell := c.String("shell")
	if shell == "" {
		shell = "bash"
	}

	// Get config file path from flag
	configFile := c.String("file")

	// Load .crumb.yaml configuration
	crumbConfig, err := loadCrumbConfig(configFile)
	if err != nil {
		return err
	}

	// Load application configuration
	profile := getProfile(c)
	config, err := loadConfig(profile)
	if err != nil {
		return err
	}

	// Get storage path
	storagePath := getStoragePath(c, config)

	// Load and decrypt secrets
	secrets, err := loadSecrets(config.PrivateKeyPath, storagePath)
	if err != nil {
		return err
	}

	// Collect environment variables to export
	envVars := make(map[string]string)

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
		for secretPath, secretValue := range secrets {
			if strings.HasPrefix(secretPath, pathPrefix) {
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
	}

	// Process env section
	for envVarName, envConfig := range crumbConfig.Env {
		if secretValue, exists := secrets[envConfig.Path]; exists {
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

func getCommand(c *cli.Context) error {
	// Check arguments
	if c.Args().Len() != 1 {
		return fmt.Errorf("usage: crumb get <key-path>")
	}

	keyPath := c.Args().Get(0)
	showValue := c.Bool("show")

	// Validate key path
	if err := validateKeyPath(keyPath); err != nil {
		return err
	}

	// Load configuration
	profile := getProfile(c)
	config, err := loadConfig(profile)
	if err != nil {
		return err
	}

	// Get storage path
	storagePath := getStoragePath(c, config)

	// Load and decrypt secrets
	secrets, err := loadSecrets(config.PrivateKeyPath, storagePath)
	if err != nil {
		return err
	}

	// Check if key exists
	value, exists := secrets[keyPath]
	if !exists {
		fmt.Println("Key not found.")
		return nil
	}

	// Display the value
	if showValue {
		fmt.Printf("%s\n", value)
	} else {
		fmt.Println("****")
	}

	return nil
}

func getFilteredKeys(secrets map[string]string, pathFilter string) []string {
	var keys []string

	// Normalize path filter (remove trailing slash if present)
	if pathFilter != "" && pathFilter != "/" {
		pathFilter = strings.TrimSuffix(pathFilter, "/")
	}

	// Extract keys from secrets map
	for key := range secrets {
		keys = append(keys, key)
	}

	// Filter keys if path filter is provided
	if pathFilter != "" {
		var filteredKeys []string
		for _, key := range keys {
			if matchesPathFilter(key, pathFilter) {
				filteredKeys = append(filteredKeys, key)
			}
		}
		keys = filteredKeys
	}

	// Sort keys alphabetically
	sort.Strings(keys)

	return keys
}

func matchesPathFilter(key, pathFilter string) bool {
	// Handle root path filter
	if pathFilter == "/" {
		return true
	}

	// Check if key starts with the path filter
	// This provides partial matching as specified in the PRD
	// e.g., "/any" matches "/any/path/mykey"
	return strings.HasPrefix(key, pathFilter)
}

func loadCrumbConfig(configFileName string) (*CrumbConfig, error) {
	// Check if config file exists
	if _, err := os.Stat(configFileName); os.IsNotExist(err) {
		return nil, fmt.Errorf("no %s found", configFileName)
	}

	// Read the config file
	data, err := os.ReadFile(configFileName)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %v", configFileName, err)
	}

	// Parse YAML
	var config CrumbConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %v", configFileName, err)
	}

	// Validate version
	if config.Version == "" {
		return nil, fmt.Errorf("invalid %s: missing version", configFileName)
	}

	// Initialize maps if they're nil
	if config.Env == nil {
		config.Env = make(map[string]EnvConfig)
	}
	if config.PathSync.Remap == nil {
		config.PathSync.Remap = make(map[string]string)
	}

	return &config, nil
}

// Storage management commands
func storageSetCommand(c *cli.Context) error {
	if c.Args().Len() != 1 {
		return fmt.Errorf("usage: crumb storage set <path>")
	}

	storagePath := c.Args().Get(0)
	profile := getProfile(c)

	// Load or create config
	configDir := filepath.Join(os.Getenv("HOME"), ".config", "crumb")
	configPath := filepath.Join(configDir, "config.yaml")

	var config Config
	if _, err := os.Stat(configPath); err == nil {
		configData, err := os.ReadFile(configPath)
		if err == nil {
			yaml.Unmarshal(configData, &config)
		}
	}

	// Initialize profiles map if needed
	if config.Profiles == nil {
		config.Profiles = make(map[string]ProfileConfig)
	}

	// Get existing profile config or create new one
	profileConfig := config.Profiles[profile]
	if profileConfig.PublicKeyPath == "" {
		return fmt.Errorf("profile '%s' not found. Run 'crumb setup --profile %s' first", profile, profile)
	}

	// Update storage path
	profileConfig.Storage = expandTilde(storagePath)
	config.Profiles[profile] = profileConfig

	// Save config
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(&config); err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	encoder.Close()

	// Create config directory if needed
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.WriteFile(configPath, buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("Storage path set to: %s (profile: %s)\n", profileConfig.Storage, profile)
	return nil
}

func storageGetCommand(c *cli.Context) error {
	profile := getProfile(c)
	config, err := loadConfig(profile)
	if err != nil {
		return err
	}

	storagePath := getStoragePath(c, config)
	fmt.Printf("Storage: %s (profile: %s)\n", storagePath, profile)
	return nil
}

func storageClearCommand(c *cli.Context) error {
	profile := getProfile(c)

	// Load config
	configDir := filepath.Join(os.Getenv("HOME"), ".config", "crumb")
	configPath := filepath.Join(configDir, "config.yaml")

	var config Config
	if _, err := os.Stat(configPath); err != nil {
		return fmt.Errorf("config file not found")
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	if err := yaml.Unmarshal(configData, &config); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Clear storage for the profile
	if config.Profiles != nil && config.Profiles[profile].PublicKeyPath != "" {
		profileConfig := config.Profiles[profile]
		profileConfig.Storage = ""
		config.Profiles[profile] = profileConfig
	}

	// Save config
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(&config); err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	encoder.Close()

	if err := os.WriteFile(configPath, buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("Storage path cleared for profile: %s (using default)\n", profile)
	return nil
}
