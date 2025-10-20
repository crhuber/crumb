package config

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/term"
	"gopkg.in/yaml.v3"
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
	Version      string                       `yaml:"version"`
	Environments map[string]EnvironmentConfig `yaml:"environments"`
}

type EnvironmentConfig struct {
	Path  string            `yaml:"path"`
	Remap map[string]string `yaml:"remap"`
	Env   map[string]string `yaml:"env"`
}

// LoadConfig loads the profile configuration from ~/.config/crumb/config.yaml
func LoadConfig(profile string) (*ProfileConfig, error) {
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

// SaveConfig saves the configuration to ~/.config/crumb/config.yaml
func SaveConfig(config *Config) error {
	configDir := filepath.Join(os.Getenv("HOME"), ".config", "crumb")
	configPath := filepath.Join(configDir, "config.yaml")

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	err := encoder.Encode(config)
	encoder.Close()
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// LoadCrumbConfig loads the per-project configuration from .crumb.yaml
func LoadCrumbConfig(configFileName string) (*CrumbConfig, error) {
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

	// Initialize environments map if it's nil
	if config.Environments == nil {
		config.Environments = make(map[string]EnvironmentConfig)
	}

	// Initialize maps within each environment
	for envName, envConfig := range config.Environments {
		if envConfig.Remap == nil {
			envConfig.Remap = make(map[string]string)
		}
		if envConfig.Env == nil {
			envConfig.Env = make(map[string]string)
		}
		config.Environments[envName] = envConfig
	}

	return &config, nil
}

// CreateDefaultCrumbConfig creates a default .crumb.yaml configuration
func CreateDefaultCrumbConfig() *CrumbConfig {
	defaultEnv := EnvironmentConfig{
		Path:  "",
		Remap: make(map[string]string),
		Env:   make(map[string]string),
	}

	environments := make(map[string]EnvironmentConfig)
	environments["default"] = defaultEnv

	return &CrumbConfig{
		Version:      "1.0",
		Environments: environments,
	}
}

// SaveCrumbConfig saves a CrumbConfig to the specified file
func SaveCrumbConfig(config *CrumbConfig, fileName string) error {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(config); err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	encoder.Close()

	if err := os.WriteFile(fileName, buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// ValidateKeyPath validates that a key path follows the required format
func ValidateKeyPath(keyPath string) error {
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

// ExpandTilde expands the tilde in file paths to the home directory
func ExpandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(os.Getenv("HOME"), path[2:])
	}
	return path
}

// GetStoragePath determines the storage path based on CLI flags, profile settings, and defaults
func GetStoragePath(storageFlag string, profile *ProfileConfig) string {
	// Priority: CLI flag > profile storage > default
	if storageFlag != "" {
		return ExpandTilde(storageFlag)
	}

	if profile.Storage != "" {
		return ExpandTilde(profile.Storage)
	}

	// Default storage path
	return filepath.Join(os.Getenv("HOME"), ".config", "crumb", "secrets")
}

// PromptForInput prompts the user for input and returns the trimmed response
func PromptForInput(prompt string) (string, error) {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}
	return strings.TrimSpace(input), nil
}

// PromptForSecret prompts the user for secret input without echoing to terminal
func PromptForSecret(prompt string) (string, error) {
	fmt.Print(prompt)

	// Check if stdin is a terminal
	if !term.IsTerminal(int(syscall.Stdin)) {
		// If not a TTY, read from stdin normally (for testing/scripting)
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("failed to read input: %w", err)
		}
		return strings.TrimSpace(input), nil
	}

	// Read password without echoing
	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}

	// Print newline after input
	fmt.Println()

	return string(bytePassword), nil
}
