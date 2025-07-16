package main

import (
	"bufio"
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

// Config represents the configuration stored in ~/.config/crum/config.yaml
type Config struct {
	PublicKeyPath  string `yaml:"public_key_path"`
	PrivateKeyPath string `yaml:"private_key_path"`
}

// CrumConfig represents the per-project configuration in .crum.yaml
type CrumConfig struct {
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
	app := &cli.App{
		Name:  "crum",
		Usage: "Securely store, manage, and export API keys and secrets",
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
						Usage:   "Configuration file to use (default: .crum.yaml)",
						Value:   ".crum.yaml",
					},
				},
				Action: exportCommand,
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func setupCommand(c *cli.Context) error {
	// Create ~/.config/crum directory if it doesn't exist
	configDir := filepath.Join(os.Getenv("HOME"), ".config", "crum")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(configDir, "config.yaml")
	secretsPath := filepath.Join(configDir, "secrets")

	// Check if files already exist and prompt for confirmation
	if _, err := os.Stat(configPath); err == nil {
		if !confirmOverwrite(fmt.Sprintf("Config file %s", configPath)) {
			fmt.Println("Setup cancelled.")
			return nil
		}
	}

	if _, err := os.Stat(secretsPath); err == nil {
		if !confirmOverwrite(fmt.Sprintf("Secrets file %s", secretsPath)) {
			fmt.Println("Setup cancelled.")
			return nil
		}
	}

	// Prompt for SSH key paths
	publicKeyPath, err := promptForKeyPath("Enter path to SSH public key (e.g., ~/.ssh/id_ed25519.pub): ")
	if err != nil {
		return err
	}

	privateKeyPath, err := promptForKeyPath("Enter path to SSH private key (e.g., ~/.ssh/id_ed25519): ")
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

	// Create config.yaml
	config := Config{
		PublicKeyPath:  publicKeyPath,
		PrivateKeyPath: privateKeyPath,
	}

	configData, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, configData, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	// Create empty encrypted secrets file
	if err := createEmptySecretsFile(secretsPath, publicKeyPath); err != nil {
		return fmt.Errorf("failed to create secrets file: %w", err)
	}

	fmt.Printf("Setup completed successfully!\n")
	fmt.Printf("Config file: %s\n", configPath)
	fmt.Printf("Secrets file: %s\n", secretsPath)

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

	if strings.TrimSpace(keyPath) == "" {
		return fmt.Errorf("key path cannot be empty")
	}

	return nil
}

func loadConfig() (*Config, error) {
	configPath := filepath.Join(os.Getenv("HOME"), ".config", "crum", "config.yaml")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("configuration not found. Run 'crum setup' first")
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}

func loadSecrets(privateKeyPath string) (map[string]string, error) {
	secretsPath := filepath.Join(os.Getenv("HOME"), ".config", "crum", "secrets")

	if _, err := os.Stat(secretsPath); os.IsNotExist(err) {
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
	encryptedData, err := readFileWithLock(secretsPath)
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

func saveSecrets(secrets map[string]string, publicKeyPath string) error {
	secretsPath := filepath.Join(os.Getenv("HOME"), ".config", "crum", "secrets")

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
	return writeFileWithLock(secretsPath, encryptedData, 0600)
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

// Placeholder functions for other commands
func listCommand(c *cli.Context) error {
	// Get optional path filter argument
	pathFilter := ""
	if c.Args().Len() > 0 {
		pathFilter = c.Args().Get(0)
	}

	// Load configuration
	config, err := loadConfig()
	if err != nil {
		return err
	}

	// Load and decrypt secrets
	secrets, err := loadSecrets(config.PrivateKeyPath)
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
		return fmt.Errorf("usage: crum set <key-path> <value>")
	}

	keyPath := c.Args().Get(0)
	value := c.Args().Get(1)

	// Validate key path
	if err := validateKeyPath(keyPath); err != nil {
		return err
	}

	// Load configuration
	config, err := loadConfig()
	if err != nil {
		return err
	}

	// Load and decrypt secrets
	secrets, err := loadSecrets(config.PrivateKeyPath)
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
	if err := saveSecrets(secrets, config.PublicKeyPath); err != nil {
		return err
	}

	fmt.Printf("Successfully set key: %s\n", keyPath)
	return nil
}

func initCommand(c *cli.Context) error {
	configFileName := ".crum.yaml"

	// Check if .crum.yaml already exists
	if _, err := os.Stat(configFileName); err == nil {
		if !confirmOverwrite(fmt.Sprintf("Config file %s", configFileName)) {
			fmt.Println("Operation cancelled.")
			return nil
		}
	}

	// Create default config structure
	defaultConfig := CrumConfig{
		Version: "1.0",
		PathSync: PathSync{
			Path:  "",
			Remap: make(map[string]string),
		},
		Env: make(map[string]EnvConfig),
	}

	// Marshal to YAML
	yamlData, err := yaml.Marshal(defaultConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal default config: %w", err)
	}

	// Write to file
	if err := os.WriteFile(configFileName, yamlData, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("Successfully created %s\n", configFileName)
	return nil
}

func deleteCommand(c *cli.Context) error {
	// Check arguments
	if c.Args().Len() != 1 {
		return fmt.Errorf("usage: crum delete <key-path>")
	}

	keyPath := c.Args().Get(0)

	// Validate key path
	if err := validateKeyPath(keyPath); err != nil {
		return err
	}

	// Load configuration
	config, err := loadConfig()
	if err != nil {
		return err
	}

	// Load and decrypt secrets
	secrets, err := loadSecrets(config.PrivateKeyPath)
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
	if err := saveSecrets(secrets, config.PublicKeyPath); err != nil {
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

	// Load .crum.yaml configuration
	crumConfig, err := loadCrumConfig(configFile)
	if err != nil {
		return err
	}

	// Load application configuration
	config, err := loadConfig()
	if err != nil {
		return err
	}

	// Load and decrypt secrets
	secrets, err := loadSecrets(config.PrivateKeyPath)
	if err != nil {
		return err
	}

	// Collect environment variables to export
	envVars := make(map[string]string)

	// Process path_sync section
	if crumConfig.PathSync.Path != "" {
		// Add comment for clarity
		comment := fmt.Sprintf("# Exported from %s", crumConfig.PathSync.Path)
		if shell == "bash" {
			fmt.Println(comment)
		} else if shell == "fish" {
			fmt.Println(comment)
		}

		// Find all secrets that match the path prefix
		pathPrefix := strings.TrimSuffix(crumConfig.PathSync.Path, "/")
		for secretPath, secretValue := range secrets {
			if strings.HasPrefix(secretPath, pathPrefix) {
				// Extract the key name from the path
				keyName := strings.TrimPrefix(secretPath, pathPrefix)
				keyName = strings.TrimPrefix(keyName, "/")
				keyName = strings.ToUpper(strings.ReplaceAll(keyName, "/", "_"))

				if keyName != "" {
					envVars[keyName] = secretValue
				}
			}
		}
	}

	// Process env section
	for envVarName, envConfig := range crumConfig.Env {
		if secretValue, exists := secrets[envConfig.Path]; exists {
			envVars[envVarName] = secretValue
		}
	}

	// Apply remap mappings
	for originalKey, newKey := range crumConfig.PathSync.Remap {
		if value, exists := envVars[originalKey]; exists {
			envVars[newKey] = value
			delete(envVars, originalKey)
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
		if shell == "bash" {
			fmt.Printf("export %s=%s\n", key, value)
		} else if shell == "fish" {
			fmt.Printf("set -x %s %s\n", key, value)
		}
	}

	return nil
}

func getCommand(c *cli.Context) error {
	// Check arguments
	if c.Args().Len() != 1 {
		return fmt.Errorf("usage: crum get <key-path>")
	}

	keyPath := c.Args().Get(0)
	showValue := c.Bool("show")

	// Validate key path
	if err := validateKeyPath(keyPath); err != nil {
		return err
	}

	// Load configuration
	config, err := loadConfig()
	if err != nil {
		return err
	}

	// Load and decrypt secrets
	secrets, err := loadSecrets(config.PrivateKeyPath)
	if err != nil {
		return err
	}

	// Check if key exists
	value, exists := secrets[keyPath]
	if !exists {
		fmt.Println("Key not found.")
		return nil
	}

	// Display the key and value
	if showValue {
		fmt.Printf("%s=%s\n", keyPath, value)
	} else {
		fmt.Printf("%s=****\n", keyPath)
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

func loadCrumConfig(configFileName string) (*CrumConfig, error) {
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
	var config CrumConfig
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
