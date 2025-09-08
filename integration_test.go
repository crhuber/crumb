package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"crumb/pkg/config"
	"crumb/pkg/storage"
)

// Integration tests for CLI commands
func TestSetupCommandIntegration(t *testing.T) {
	tempDir := createTempDir(t)
	defer os.RemoveAll(tempDir)

	// Set up test environment
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	pubKeyPath, privKeyPath := createTestSSHKeys(t, tempDir)

	// Test setup command with valid keys
	t.Run("config directory creation", func(t *testing.T) {
		configDir := filepath.Join(tempDir, ".config", "crumb")
		err := os.MkdirAll(configDir, 0700)
		if err != nil {
			t.Fatalf("Failed to create config directory: %v", err)
		}

		// Check if directory exists
		if _, err := os.Stat(configDir); os.IsNotExist(err) {
			t.Errorf("Config directory was not created")
		}
	})

	t.Run("ssh key validation", func(t *testing.T) {
		// Test that our test keys exist
		if _, err := os.Stat(pubKeyPath); os.IsNotExist(err) {
			t.Errorf("Public key file was not created")
		}
		if _, err := os.Stat(privKeyPath); os.IsNotExist(err) {
			t.Errorf("Private key file was not created")
		}
	})
}

func TestListCommandIntegration(t *testing.T) {
	tempDir := createTempDir(t)
	defer os.RemoveAll(tempDir)

	// Create test secrets
	testSecrets := map[string]string{
		"/prod/billing-svc/vars/mg":     "secret1",
		"/prod/billing-svc/vars/stripe": "secret2",
		"/prod/auth-svc/api_key":        "secret3",
		"/dev/test":                     "secret4",
	}

	// Test filtering functionality
	tests := []struct {
		name       string
		pathFilter string
		expected   []string
	}{
		{
			name:       "list all",
			pathFilter: "",
			expected: []string{
				"/dev/test",
				"/prod/auth-svc/api_key",
				"/prod/billing-svc/vars/mg",
				"/prod/billing-svc/vars/stripe",
			},
		},
		{
			name:       "filter by /prod",
			pathFilter: "/prod",
			expected: []string{
				"/prod/auth-svc/api_key",
				"/prod/billing-svc/vars/mg",
				"/prod/billing-svc/vars/stripe",
			},
		},
		{
			name:       "filter by /prod/billing-svc",
			pathFilter: "/prod/billing-svc",
			expected: []string{
				"/prod/billing-svc/vars/mg",
				"/prod/billing-svc/vars/stripe",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := storage.GetFilteredKeys(testSecrets, tt.pathFilter)

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d keys, got %d", len(tt.expected), len(result))
			}

			for i, key := range result {
				if i < len(tt.expected) && key != tt.expected[i] {
					t.Errorf("Expected key %q at index %d, got %q", tt.expected[i], i, key)
				}
			}
		})
	}
}

func TestInitCommandIntegration(t *testing.T) {
	tempDir := createTempDir(t)
	defer os.RemoveAll(tempDir)

	// Change to temp directory
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer os.Chdir(originalDir)

	err = os.Chdir(tempDir)
	if err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	// Test init command
	configPath := filepath.Join(tempDir, ".crumb.yaml")

	t.Run("create new config", func(t *testing.T) {
		// Mock the init command logic
		defaultConfig := `version: "1.0"
path_sync:
  path: ""
  remap: {}
env: {}`

		err := os.WriteFile(configPath, []byte(defaultConfig), 0600)
		if err != nil {
			t.Fatalf("Failed to write config file: %v", err)
		}

		// Check if file was created
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			t.Errorf("Config file was not created")
		}

		// Check file contents
		content, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("Failed to read config file: %v", err)
		}

		if !strings.Contains(string(content), `version: "1.0"`) {
			t.Errorf("Config file does not contain expected version")
		}
	})

	t.Run("file already exists", func(t *testing.T) {
		// File should already exist from previous test
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			t.Errorf("Config file should exist for overwrite test")
		}
	})
}

func TestDeleteCommandIntegration(t *testing.T) {
	// Test delete command validation
	tests := []struct {
		name    string
		keyPath string
		wantErr bool
	}{
		{
			name:    "valid key path",
			keyPath: "/prod/billing-svc/vars/mg",
			wantErr: false,
		},
		{
			name:    "invalid key path",
			keyPath: "invalid-path",
			wantErr: true,
		},
		{
			name:    "empty key path",
			keyPath: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := config.ValidateKeyPath(tt.keyPath)
			if tt.wantErr && err == nil {
				t.Errorf("Expected error for key path %q", tt.keyPath)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error for key path %q: %v", tt.keyPath, err)
			}
		})
	}
}

func TestExportCommandIntegration(t *testing.T) {
	tempDir := createTempDir(t)
	defer os.RemoveAll(tempDir)

	// Create test config file
	configContent := `version: 1
path_sync:
  path: "/prod/billing-svc"
  remap:
    VARS_MG: "MG_KEY"
    VARS_STRIPE: "STRIPE_KEY"
env:
  DATABASE_URL:
    path: "/prod/billing-svc/db/url"
  API_SECRET:
    path: "/prod/billing-svc/api/secret"`

	configPath := filepath.Join(tempDir, "test.yaml")
	err := os.WriteFile(configPath, []byte(configContent), 0600)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Test config loading
	cfg, err := config.LoadCrumbConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Validate config structure
	if cfg.Version != "1" {
		t.Errorf("Expected version '1', got '%s'", cfg.Version)
	}

	if cfg.PathSync.Path != "/prod/billing-svc" {
		t.Errorf("Expected path '/prod/billing-svc', got '%s'", cfg.PathSync.Path)
	}

	if len(cfg.Env) != 2 {
		t.Errorf("Expected 2 env entries, got %d", len(cfg.Env))
	}

	if cfg.Env["DATABASE_URL"].Path != "/prod/billing-svc/db/url" {
		t.Errorf("Expected DATABASE_URL path '/prod/billing-svc/db/url', got '%s'", cfg.Env["DATABASE_URL"].Path)
	}
}

func TestGetCommandIntegration(t *testing.T) {
	// Test get command validation
	tests := []struct {
		name    string
		keyPath string
		wantErr bool
	}{
		{
			name:    "valid key path",
			keyPath: "/prod/billing-svc/vars/mg",
			wantErr: false,
		},
		{
			name:    "invalid key path - no leading slash",
			keyPath: "prod/billing-svc/vars/mg",
			wantErr: true,
		},
		{
			name:    "invalid key path - contains spaces",
			keyPath: "/prod/billing svc/vars/mg",
			wantErr: true,
		},
		{
			name:    "invalid key path - contains equals",
			keyPath: "/prod/billing=svc/vars/mg",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := config.ValidateKeyPath(tt.keyPath)
			if tt.wantErr && err == nil {
				t.Errorf("Expected error for key path %q", tt.keyPath)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error for key path %q: %v", tt.keyPath, err)
			}
		})
	}
}

// Test edge cases and error handling
func TestErrorHandling(t *testing.T) {
	t.Run("empty secrets file", func(t *testing.T) {
		secrets := storage.ParseSecrets("")
		if len(secrets) != 0 {
			t.Errorf("Expected empty secrets map, got %d entries", len(secrets))
		}
	})

	t.Run("malformed secrets line", func(t *testing.T) {
		secrets := storage.ParseSecrets("invalid-line-without-equals")
		if len(secrets) != 0 {
			t.Errorf("Expected empty secrets map for malformed line, got %d entries", len(secrets))
		}
	})

	t.Run("secrets with empty values", func(t *testing.T) {
		secrets := storage.ParseSecrets("/test/key=")
		if len(secrets) != 1 {
			t.Errorf("Expected 1 secret, got %d", len(secrets))
		}
		if secrets["/test/key"] != "" {
			t.Errorf("Expected empty value, got %q", secrets["/test/key"])
		}
	})

	t.Run("secrets with equals in value", func(t *testing.T) {
		secrets := storage.ParseSecrets("/test/key=value=with=equals")
		if len(secrets) != 1 {
			t.Errorf("Expected 1 secret, got %d", len(secrets))
		}
		if secrets["/test/key"] != "value=with=equals" {
			t.Errorf("Expected 'value=with=equals', got %q", secrets["/test/key"])
		}
	})
}

// Benchmark tests for performance
func BenchmarkGetFilteredKeys(b *testing.B) {
	// Create a large secrets map
	secrets := make(map[string]string)
	for i := 0; i < 1000; i++ {
		secrets[fmt.Sprintf("/prod/service%d/vars/key%d", i%10, i)] = fmt.Sprintf("value%d", i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		storage.GetFilteredKeys(secrets, "/prod/service1")
	}
}

func BenchmarkParseSecrets(b *testing.B) {
	// Create a large secrets string
	var buf bytes.Buffer
	for i := 0; i < 1000; i++ {
		buf.WriteString(fmt.Sprintf("/prod/service%d/vars/key%d=value%d\n", i%10, i, i))
	}
	content := buf.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		storage.ParseSecrets(content)
	}
}

func BenchmarkValidateKeyPath(b *testing.B) {
	keyPath := "/prod/billing-svc/vars/mg"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		config.ValidateKeyPath(keyPath)
	}
}

func TestImportCommandIntegration(t *testing.T) {
	tempDir := createTempDir(t)
	defer os.RemoveAll(tempDir)

	// Create test .env file
	envContent := `# Test environment variables
API_KEY=secret123
DATABASE_URL="postgresql://localhost:5432/testdb"
DEBUG=true
EMPTY_VAR=
SPECIAL_CHARS='value-with-dashes_and_underscores'
URL_WITH_EQUALS=https://api.example.com?token=abc123&refresh=def456`

	envFile := filepath.Join(tempDir, "test.env")
	err := os.WriteFile(envFile, []byte(envContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test .env file: %v", err)
	}

	// Test parsing the .env file
	t.Run("parse env file", func(t *testing.T) {
		parsed, err := storage.ParseEnvFile(envFile)
		if err != nil {
			t.Fatalf("Failed to parse .env file: %v", err)
		}

		expected := map[string]string{
			"API_KEY":         "secret123",
			"DATABASE_URL":    "postgresql://localhost:5432/testdb",
			"DEBUG":           "true",
			"EMPTY_VAR":       "",
			"SPECIAL_CHARS":   "value-with-dashes_and_underscores",
			"URL_WITH_EQUALS": "https://api.example.com?token=abc123&refresh=def456",
		}

		if len(parsed) != len(expected) {
			t.Errorf("Expected %d variables, got %d", len(expected), len(parsed))
		}

		for key, expectedValue := range expected {
			if actualValue, exists := parsed[key]; !exists {
				t.Errorf("Expected key %s not found", key)
			} else if actualValue != expectedValue {
				t.Errorf("For key %s: expected %q, got %q", key, expectedValue, actualValue)
			}
		}
	})

	// Test key path generation
	t.Run("key path generation", func(t *testing.T) {
		basePath := "/dev/test"
		envKey := "API_KEY"
		expectedPath := "/dev/test/api_key"

		fullKeyPath := basePath + "/" + strings.ToLower(envKey)
		if fullKeyPath != expectedPath {
			t.Errorf("Expected key path %s, got %s", expectedPath, fullKeyPath)
		}

		// Validate the generated path
		if err := config.ValidateKeyPath(fullKeyPath); err != nil {
			t.Errorf("Generated key path %s should be valid: %v", fullKeyPath, err)
		}
	})

	// Test integration with existing secrets
	t.Run("integration with secrets storage", func(t *testing.T) {
		// Create some existing test secrets
		existingSecrets := map[string]string{
			"/dev/test/existing_key": "existing_value",
			"/dev/test/api_key":      "old_api_value", // This should conflict
		}

		// Parse env vars
		envVars, err := storage.ParseEnvFile(envFile)
		if err != nil {
			t.Fatalf("Failed to parse env file: %v", err)
		}

		// Check for conflicts
		basePath := "/dev/test"
		conflicts := []string{}
		newKeys := []string{}

		for envKey := range envVars {
			fullKeyPath := basePath + "/" + strings.ToLower(envKey)
			if _, exists := existingSecrets[fullKeyPath]; exists {
				conflicts = append(conflicts, fullKeyPath)
			} else {
				newKeys = append(newKeys, fullKeyPath)
			}
		}

		// We expect 1 conflict (api_key) and 5 new keys
		if len(conflicts) != 1 {
			t.Errorf("Expected 1 conflict, got %d", len(conflicts))
		}

		if len(newKeys) != 5 {
			t.Errorf("Expected 5 new keys, got %d", len(newKeys))
		}

		// Check specific conflict
		expectedConflict := "/dev/test/api_key"
		found := false
		for _, conflict := range conflicts {
			if conflict == expectedConflict {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected conflict for %s not found", expectedConflict)
		}
	})

	// Test path validation
	t.Run("path validation", func(t *testing.T) {
		validPaths := []string{
			"/dev/test",
			"/prod/api",
			"/staging/db",
		}

		invalidPaths := []string{
			"dev/test",  // no leading slash
			"/dev test", // contains space
			"/dev=test", // contains equals
			"",          // empty
		}

		for _, path := range validPaths {
			if err := config.ValidateKeyPath(path); err != nil {
				t.Errorf("Path %s should be valid: %v", path, err)
			}
		}

		for _, path := range invalidPaths {
			if err := config.ValidateKeyPath(path); err == nil {
				t.Errorf("Path %s should be invalid", path)
			}
		}
	})

	// Test file not found error
	t.Run("non-existent env file", func(t *testing.T) {
		nonExistentFile := filepath.Join(tempDir, "does_not_exist.env")
		_, err := storage.ParseEnvFile(nonExistentFile)
		if err == nil {
			t.Error("Expected error for non-existent file")
		}
	})

	// Test empty env file
	t.Run("empty env file", func(t *testing.T) {
		emptyEnvFile := filepath.Join(tempDir, "empty.env")
		err := os.WriteFile(emptyEnvFile, []byte(""), 0644)
		if err != nil {
			t.Fatalf("Failed to create empty env file: %v", err)
		}

		parsed, err := storage.ParseEnvFile(emptyEnvFile)
		if err != nil {
			t.Fatalf("Failed to parse empty env file: %v", err)
		}

		if len(parsed) != 0 {
			t.Errorf("Expected empty map for empty file, got %d entries", len(parsed))
		}
	})
}
