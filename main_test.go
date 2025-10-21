package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"crumb/pkg/config"
	"crumb/pkg/storage"
)

// Test helper functions
func createTempDir(t *testing.T) string {
	tempDir, err := os.MkdirTemp("", "crumb_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	return tempDir
}

func createTestSSHKeys(t *testing.T, tempDir string) (string, string) {
	// Create a test SSH key pair (simplified for testing)
	pubKeyPath := filepath.Join(tempDir, "test_key.pub")
	privKeyPath := filepath.Join(tempDir, "test_key")

	// Create mock SSH public key content
	pubKeyContent := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGbzWC4LRQ8L4fz8Q4qP5lqzNbBcQp7qPKW1K2tLPRzA test@example.com"

	err := os.WriteFile(pubKeyPath, []byte(pubKeyContent), 0600)
	if err != nil {
		t.Fatalf("Failed to create test public key: %v", err)
	}

	// Create mock SSH private key content
	privKeyContent := `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACBm81gvC0UPC+H8/EOKj+Zaszm1QEKe6jyltStrSz0cwAAAALBcHhqwXB4a
sAAAAAtzc2gtZWQyNTUxOQAAACBm81gvC0UPC+H8/EOKj+Zaszm1QEKe6jyltStrSz0cwA
AAAECq5PbJe6xbPNKqjqWQEDXJvL8aM4JKwz5eJMN4mL7hQbMtCzPRxGHUzpQWPUJVxhd
mVGlGvjdSQWZQzCl3hWBPfqfVYsYsE8pGhQqCNMzxiUYgKHNYfxJkjM2YPZGnOXJq4J5
o6aJmMrJMxJrMNlWqkMwkKpgPzOzMqJaZMTdGjKQCJYLEJkNMhLIkJc6vhJxvBqEzS0Z
fDLiTUGOyT8uOJaGYOHKWZGKHdBvHCwXCJMYQfVYPJBMWKjFYhzGGqQjJjJjJpJgJU5v
-----END OPENSSH PRIVATE KEY-----`

	err = os.WriteFile(privKeyPath, []byte(privKeyContent), 0600)
	if err != nil {
		t.Fatalf("Failed to create test private key: %v", err)
	}

	return pubKeyPath, privKeyPath
}

// Test key path validation
func TestValidateKeyPath(t *testing.T) {
	tests := []struct {
		name    string
		keyPath string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid key path",
			keyPath: "/prod/billing-svc/vars/mg",
			wantErr: false,
		},
		{
			name:    "valid root path",
			keyPath: "/root",
			wantErr: false,
		},
		{
			name:    "empty path",
			keyPath: "",
			wantErr: true,
			errMsg:  "key path cannot be empty",
		},
		{
			name:    "path not starting with slash",
			keyPath: "prod/billing-svc/vars/mg",
			wantErr: true,
			errMsg:  "key path must start with '/'",
		},
		{
			name:    "path with spaces",
			keyPath: "/prod/billing svc/vars/mg",
			wantErr: true,
			errMsg:  "key path cannot contain spaces",
		},
		{
			name:    "path with equals",
			keyPath: "/prod/billing=svc/vars/mg",
			wantErr: true,
			errMsg:  "key path cannot contain '='",
		},
		{
			name:    "path with newline",
			keyPath: "/prod/billing\nsvc/vars/mg",
			wantErr: true,
			errMsg:  "key path cannot contain newlines",
		},
		{
			name:    "path with tab",
			keyPath: "/prod/billing\tsvc/vars/mg",
			wantErr: true,
			errMsg:  "key path cannot contain tabs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := config.ValidateKeyPath(tt.keyPath)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateKeyPath() expected error but got none")
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("validateKeyPath() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateKeyPath() unexpected error = %v", err)
				}
			}
		})
	}
}

// Test filtered keys functionality
func TestGetFilteredKeys(t *testing.T) {
	secrets := map[string]string{
		"/prod/billing-svc/vars/mg":     "secret1",
		"/prod/billing-svc/vars/stripe": "secret2",
		"/prod/billing-svc/configs/app": "secret3",
		"/prod/auth-svc/api_key":        "secret4",
		"/dev/billing-svc/vars/mg":      "secret5",
		"/staging/test":                 "secret6",
	}

	tests := []struct {
		name       string
		pathFilter string
		expected   []string
	}{
		{
			name:       "no filter",
			pathFilter: "",
			expected: []string{
				"/dev/billing-svc/vars/mg",
				"/prod/auth-svc/api_key",
				"/prod/billing-svc/configs/app",
				"/prod/billing-svc/vars/mg",
				"/prod/billing-svc/vars/stripe",
				"/staging/test",
			},
		},
		{
			name:       "filter by /prod",
			pathFilter: "/prod",
			expected: []string{
				"/prod/auth-svc/api_key",
				"/prod/billing-svc/configs/app",
				"/prod/billing-svc/vars/mg",
				"/prod/billing-svc/vars/stripe",
			},
		},
		{
			name:       "filter by /prod/billing-svc",
			pathFilter: "/prod/billing-svc",
			expected: []string{
				"/prod/billing-svc/configs/app",
				"/prod/billing-svc/vars/mg",
				"/prod/billing-svc/vars/stripe",
			},
		},
		{
			name:       "filter with trailing slash",
			pathFilter: "/prod/billing-svc/",
			expected: []string{
				"/prod/billing-svc/configs/app",
				"/prod/billing-svc/vars/mg",
				"/prod/billing-svc/vars/stripe",
			},
		},
		{
			name:       "filter with no matches",
			pathFilter: "/nonexistent",
			expected:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := storage.GetFilteredKeys(secrets, tt.pathFilter)

			if len(result) != len(tt.expected) {
				t.Errorf("getFilteredKeys() got %d keys, want %d", len(result), len(tt.expected))
			}

			for i, key := range result {
				if i < len(tt.expected) && key != tt.expected[i] {
					t.Errorf("getFilteredKeys() got key %q at index %d, want %q", key, i, tt.expected[i])
				}
			}
		})
	}
}

// Test YAML configuration parsing
func TestLoadCrumbConfig(t *testing.T) {
	tempDir := createTempDir(t)
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name        string
		configFile  string
		content     string
		wantErr     bool
		errContains string
		validate    func(*testing.T, *config.CrumbConfig)
	}{
		{
			name:       "valid config",
			configFile: "valid.yaml",
			content: `version: "1.0"
environments:
  default:
    path: "/prod/billing-svc"
    remap:
      VARS_MG: "MG_KEY"
    env:
      DATABASE_URL: "/prod/billing-svc/db/url"`,
			wantErr: false,
			validate: func(t *testing.T, cfg *config.CrumbConfig) {
				if cfg.Version != "1.0" {
					t.Errorf("Expected version '1.0', got '%s'", cfg.Version)
				}

				defaultEnv, exists := cfg.Environments["default"]
				if !exists {
					t.Errorf("Expected 'default' environment to exist")
					return
				}

				if defaultEnv.Path != "/prod/billing-svc" {
					t.Errorf("Expected path '/prod/billing-svc', got '%s'", defaultEnv.Path)
				}
				if defaultEnv.Remap["VARS_MG"] != "MG_KEY" {
					t.Errorf("Expected remap VARS_MG -> MG_KEY, got '%s'", defaultEnv.Remap["VARS_MG"])
				}
				if defaultEnv.Env["DATABASE_URL"] != "/prod/billing-svc/db/url" {
					t.Errorf("Expected env path '/prod/billing-svc/db/url', got '%s'", defaultEnv.Env["DATABASE_URL"])
				}
			},
		},
		{
			name:       "missing version",
			configFile: "no-version.yaml",
			content: `path_sync:
  path: "/prod/billing-svc"
env: {}`,
			wantErr:     true,
			errContains: "missing version",
		},
		{
			name:        "invalid yaml",
			configFile:  "invalid.yaml",
			content:     `invalid: yaml: content: [`,
			wantErr:     true,
			errContains: "failed to parse",
		},
		{
			name:        "nonexistent file",
			configFile:  "nonexistent.yaml",
			content:     "",
			wantErr:     true,
			errContains: "nonexistent.yaml found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(tempDir, tt.configFile)

			if tt.content != "" {
				err := os.WriteFile(configPath, []byte(tt.content), 0600)
				if err != nil {
					t.Fatalf("Failed to write test config: %v", err)
				}
			}

			cfg, err := config.LoadCrumbConfig(configPath)

			if tt.wantErr {
				if err == nil {
					t.Errorf("loadCrumbConfig() expected error but got none")
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("loadCrumbConfig() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("loadCrumbConfig() unexpected error = %v", err)
				}
				if cfg != nil && tt.validate != nil {
					tt.validate(t, cfg)
				}
			}
		})
	}
}

// Test shell output formatting
func TestShellOutputFormatting(t *testing.T) {
	tests := []struct {
		name     string
		shell    string
		envVars  map[string]string
		expected []string
	}{
		{
			name:  "bash format",
			shell: "bash",
			envVars: map[string]string{
				"API_KEY":      "secret123",
				"DATABASE_URL": "postgres://localhost/db",
			},
			expected: []string{
				"export API_KEY=secret123",
				"export DATABASE_URL=postgres://localhost/db",
			},
		},
		{
			name:  "fish format",
			shell: "fish",
			envVars: map[string]string{
				"API_KEY":      "secret123",
				"DATABASE_URL": "postgres://localhost/db",
			},
			expected: []string{
				"set -x API_KEY secret123",
				"set -x DATABASE_URL postgres://localhost/db",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result []string

			// Sort keys for consistent output
			keys := make([]string, 0, len(tt.envVars))
			for key := range tt.envVars {
				keys = append(keys, key)
			}
			// Simple sort for testing
			for i := 0; i < len(keys); i++ {
				for j := i + 1; j < len(keys); j++ {
					if keys[i] > keys[j] {
						keys[i], keys[j] = keys[j], keys[i]
					}
				}
			}

			for _, key := range keys {
				value := tt.envVars[key]
				var line string
				switch tt.shell {
				case "bash":
					line = fmt.Sprintf("export %s=%s", key, value)
				case "fish":
					line = fmt.Sprintf("set -x %s %s", key, value)
				}
				result = append(result, line)
			}

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d lines, got %d", len(tt.expected), len(result))
			}

			for i, line := range result {
				if i < len(tt.expected) && line != tt.expected[i] {
					t.Errorf("Line %d: expected %q, got %q", i, tt.expected[i], line)
				}
			}
		})
	}
}

// Test config structure initialization
func TestConfigInitialization(t *testing.T) {
	tempDir := createTempDir(t)
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "test.yaml")

	// Test with minimal config
	minimalConfig := `version: "1.0"
environments:
  default:
    path: ""
    remap: {}
    env: {}`
	err := os.WriteFile(configPath, []byte(minimalConfig), 0600)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := config.LoadCrumbConfig(configPath)
	if err != nil {
		t.Fatalf("loadCrumbConfig() failed: %v", err)
	}

	// Test that environments are initialized
	if cfg.Environments == nil {
		t.Error("Environments map should be initialized")
	}

	defaultEnv, exists := cfg.Environments["default"]
	if !exists {
		t.Error("Default environment should exist")
		return
	}

	if defaultEnv.Remap == nil {
		t.Error("Default environment Remap map should be initialized")
	}

	if defaultEnv.Env == nil {
		t.Error("Default environment Env map should be initialized")
	}

	// Test they are empty but not nil
	if len(defaultEnv.Env) != 0 {
		t.Error("Default environment Env map should be empty")
	}

	if len(defaultEnv.Remap) != 0 {
		t.Error("Default environment Remap map should be empty")
	}
}

// Test secret parsing and formatting
func TestSecretParsing(t *testing.T) {
	secretsContent := `/prod/billing-svc/vars/mg=secret123
/prod/billing-svc/vars/stripe=sk_test_123
/prod/auth-svc/api_key=api_key_456
/dev/test=value`

	expected := map[string]string{
		"/prod/billing-svc/vars/mg":     "secret123",
		"/prod/billing-svc/vars/stripe": "sk_test_123",
		"/prod/auth-svc/api_key":        "api_key_456",
		"/dev/test":                     "value",
	}

	result := storage.ParseSecrets(secretsContent)

	if len(result) != len(expected) {
		t.Errorf("Expected %d secrets, got %d", len(expected), len(result))
	}

	for key, expectedValue := range expected {
		if actualValue, exists := result[key]; !exists {
			t.Errorf("Expected key %q not found", key)
		} else if actualValue != expectedValue {
			t.Errorf("Key %q: expected value %q, got %q", key, expectedValue, actualValue)
		}
	}
}

// Test environment variable name generation from paths
func TestEnvVarNameGeneration(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		prefix   string
		expected string
	}{
		{
			name:     "simple path",
			path:     "/prod/billing-svc/vars/mg",
			prefix:   "/prod/billing-svc",
			expected: "VARS_MG",
		},
		{
			name:     "nested path",
			path:     "/prod/billing-svc/configs/app/database",
			prefix:   "/prod/billing-svc",
			expected: "CONFIGS_APP_DATABASE",
		},
		{
			name:     "single level",
			path:     "/prod/billing-svc/api_key",
			prefix:   "/prod/billing-svc",
			expected: "API_KEY",
		},
		{
			name:     "with dots",
			path:     "/prod/billing-svc/configs/app.yml",
			prefix:   "/prod/billing-svc",
			expected: "CONFIGS_APP.YML",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Extract the key name from the path
			keyName := strings.TrimPrefix(tt.path, tt.prefix)
			keyName = strings.TrimPrefix(keyName, "/")
			keyName = strings.ToUpper(strings.ReplaceAll(keyName, "/", "_"))

			if keyName != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, keyName)
			}
		})
	}
}

// Test environment variable name sanitization
func TestEnvVarSanitization(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "with dashes",
			input:    "client-id",
			expected: "CLIENT_ID",
		},
		{
			name:     "with mixed case and dashes",
			input:    "Api-Key-Value",
			expected: "API_KEY_VALUE",
		},
		{
			name:     "already uppercase with underscores",
			input:    "DATABASE_URL",
			expected: "DATABASE_URL",
		},
		{
			name:     "mixed dashes and underscores",
			input:    "some-var_name",
			expected: "SOME_VAR_NAME",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the sanitization logic used in the export command
			result := strings.ToUpper(strings.ReplaceAll(tt.input, "-", "_"))
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// Test export command environment variable sanitization
func TestExportCommandSanitization(t *testing.T) {
	tempDir := createTempDir(t)
	defer os.RemoveAll(tempDir)

	// Create test config with dashes
	configPath := filepath.Join(tempDir, ".crumb.yaml")
	configContent := `version: "1.0"
environments:
  default:
    path: "/test"
    remap:
      CLIENT_ID: "API_CLIENT_ID"
    env:
      database-url: "/test/db-url"
      api-key: "/test/api-key"`

	err := os.WriteFile(configPath, []byte(configContent), 0600)
	if err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	// Test secrets map
	secrets := map[string]string{
		"/test/client-id": "test-client-123",
		"/test/api-key":   "secret-api-key",
		"/test/db-url":    "postgresql://localhost:5432/test",
	}

	// Simulate the export logic for environment variable sanitization
	crumbConfig, err := config.LoadCrumbConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	envVars := make(map[string]string)

	// Get default environment
	defaultEnv, exists := crumbConfig.Environments["default"]
	if !exists {
		t.Fatalf("Default environment not found")
	}

	// Process path section (simplified)
	if defaultEnv.Path != "" {
		pathPrefix := strings.TrimSuffix(defaultEnv.Path, "/")
		for secretPath, secretValue := range secrets {
			if strings.HasPrefix(secretPath, pathPrefix) {
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
	for envVarName, envVarPath := range defaultEnv.Env {
		if secretValue, exists := secrets[envVarPath]; exists {
			sanitizedEnvVarName := strings.ToUpper(strings.ReplaceAll(envVarName, "-", "_"))
			envVars[sanitizedEnvVarName] = secretValue
		}
	}

	// Verify that dashes were converted to underscores
	expectedVars := map[string]string{
		"CLIENT_ID":    "test-client-123",
		"API_KEY":      "secret-api-key",
		"DATABASE_URL": "postgresql://localhost:5432/test",
		"DB_URL":       "postgresql://localhost:5432/test",
	}

	for expectedKey, expectedValue := range expectedVars {
		if actualValue, exists := envVars[expectedKey]; !exists {
			t.Errorf("Expected environment variable %q not found", expectedKey)
		} else if actualValue != expectedValue {
			t.Errorf("Environment variable %q: expected value %q, got %q", expectedKey, expectedValue, actualValue)
		}
	}

	// Verify no variables with dashes exist
	for envVarName := range envVars {
		if strings.Contains(envVarName, "-") {
			t.Errorf("Environment variable %q contains dashes, which is invalid", envVarName)
		}
	}
}

func TestExportCommandLiteralEnvValues(t *testing.T) {
	tempDir := createTempDir(t)
	defer os.RemoveAll(tempDir)

	// Create test config with both literal values and path-based values
	configPath := filepath.Join(tempDir, ".crumb.yaml")
	configContent := `version: "1.0"
environments:
  default:
    path: "/contacts/live"
    remap:
      JWT_KEY: "CONTACTS_JWT_KEY"
      JWT_KEY_STATIC: "CONTACTS_JWT_KEY_STATIC"
      DB_DSN: "CONTACTS_DB_DSN"
    env:
      DB_TYPE: "postgres"
      DB_HOST: "localhost"
      DB_PORT: "5432"
  production:
    path: "/contacts/prod"
    remap: {}
    env:
      DB_TYPE: "mysql"
      SECRET_PATH: "/contacts/prod/secret"`

	err := os.WriteFile(configPath, []byte(configContent), 0600)
	if err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	// Test secrets map - only contains path-based secrets
	secrets := map[string]string{
		"/contacts/live/jwt-key":        "live-jwt-secret",
		"/contacts/live/jwt-key-static": "live-jwt-static-secret",
		"/contacts/live/db-dsn":         "postgres://live-db",
		"/contacts/prod/secret":         "prod-secret-value",
	}

	// Load config
	crumbConfig, err := config.LoadCrumbConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Test default environment with literal values
	t.Run("default environment with literal values", func(t *testing.T) {
		envVars := make(map[string]string)

		defaultEnv, exists := crumbConfig.Environments["default"]
		if !exists {
			t.Fatalf("Default environment not found")
		}

		// Process path section
		if defaultEnv.Path != "" {
			pathPrefix := strings.TrimSuffix(defaultEnv.Path, "/")
			for secretPath, secretValue := range secrets {
				if strings.HasPrefix(secretPath, pathPrefix) {
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

		// Process env section with literal value support
		for envVarName, envVarValue := range defaultEnv.Env {
			sanitizedEnvVarName := strings.ToUpper(strings.ReplaceAll(envVarName, "-", "_"))

			// If value starts with '/', treat it as a path to a secret
			// Otherwise, use it as a literal value
			if strings.HasPrefix(envVarValue, "/") {
				if secretValue, exists := secrets[envVarValue]; exists {
					envVars[sanitizedEnvVarName] = secretValue
				}
			} else {
				// Use literal value directly
				envVars[sanitizedEnvVarName] = envVarValue
			}
		}

		// Apply remap mappings
		for originalKey, newKey := range defaultEnv.Remap {
			sanitizedOriginalKey := strings.ToUpper(strings.ReplaceAll(originalKey, "-", "_"))
			sanitizedNewKey := strings.ToUpper(strings.ReplaceAll(newKey, "-", "_"))

			if value, exists := envVars[sanitizedOriginalKey]; exists {
				envVars[sanitizedNewKey] = value
				delete(envVars, sanitizedOriginalKey)
			}
		}

		// Verify expected variables
		expectedVars := map[string]string{
			"DB_TYPE":                 "postgres",  // Literal value
			"DB_HOST":                 "localhost", // Literal value
			"DB_PORT":                 "5432",      // Literal value
			"CONTACTS_JWT_KEY":        "live-jwt-secret",
			"CONTACTS_JWT_KEY_STATIC": "live-jwt-static-secret",
			"CONTACTS_DB_DSN":         "postgres://live-db",
		}

		for expectedKey, expectedValue := range expectedVars {
			if actualValue, exists := envVars[expectedKey]; !exists {
				t.Errorf("Expected environment variable %q not found", expectedKey)
			} else if actualValue != expectedValue {
				t.Errorf("Environment variable %q: expected value %q, got %q", expectedKey, expectedValue, actualValue)
			}
		}

		// Verify we have the right number of vars (no extras)
		if len(envVars) != len(expectedVars) {
			t.Errorf("Expected %d environment variables, got %d", len(expectedVars), len(envVars))
			t.Logf("Actual vars: %+v", envVars)
		}
	})

	// Test production environment with mixed literal and path values
	t.Run("production environment with mixed values", func(t *testing.T) {
		envVars := make(map[string]string)

		prodEnv, exists := crumbConfig.Environments["production"]
		if !exists {
			t.Fatalf("Production environment not found")
		}

		// Process env section
		for envVarName, envVarValue := range prodEnv.Env {
			sanitizedEnvVarName := strings.ToUpper(strings.ReplaceAll(envVarName, "-", "_"))

			if strings.HasPrefix(envVarValue, "/") {
				if secretValue, exists := secrets[envVarValue]; exists {
					envVars[sanitizedEnvVarName] = secretValue
				}
			} else {
				envVars[sanitizedEnvVarName] = envVarValue
			}
		}

		// Verify expected variables
		expectedVars := map[string]string{
			"DB_TYPE":     "mysql",             // Literal value
			"SECRET_PATH": "prod-secret-value", // Path-based value
		}

		for expectedKey, expectedValue := range expectedVars {
			if actualValue, exists := envVars[expectedKey]; !exists {
				t.Errorf("Expected environment variable %q not found", expectedKey)
			} else if actualValue != expectedValue {
				t.Errorf("Environment variable %q: expected value %q, got %q", expectedKey, expectedValue, actualValue)
			}
		}
	})
}
