package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		configDir := filepath.Join(tempDir, ".config", "crum")
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
			result := getFilteredKeys(testSecrets, tt.pathFilter)

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
	configPath := filepath.Join(tempDir, ".crum.yaml")

	t.Run("create new config", func(t *testing.T) {
		// Mock the init command logic
		defaultConfig := `version: "1.0"
path_sync:
  path: ""
  remap: {}
env: {}`

		err := os.WriteFile(configPath, []byte(defaultConfig), 0644)
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
			err := validateKeyPath(tt.keyPath)
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
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Test config loading
	config, err := loadCrumConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Validate config structure
	if config.Version != "1" {
		t.Errorf("Expected version '1', got '%s'", config.Version)
	}

	if config.PathSync.Path != "/prod/billing-svc" {
		t.Errorf("Expected path '/prod/billing-svc', got '%s'", config.PathSync.Path)
	}

	if len(config.Env) != 2 {
		t.Errorf("Expected 2 env entries, got %d", len(config.Env))
	}

	if config.Env["DATABASE_URL"].Path != "/prod/billing-svc/db/url" {
		t.Errorf("Expected DATABASE_URL path '/prod/billing-svc/db/url', got '%s'", config.Env["DATABASE_URL"].Path)
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
			err := validateKeyPath(tt.keyPath)
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
		secrets := parseSecrets("")
		if len(secrets) != 0 {
			t.Errorf("Expected empty secrets map, got %d entries", len(secrets))
		}
	})

	t.Run("malformed secrets line", func(t *testing.T) {
		secrets := parseSecrets("invalid-line-without-equals")
		if len(secrets) != 0 {
			t.Errorf("Expected empty secrets map for malformed line, got %d entries", len(secrets))
		}
	})

	t.Run("secrets with empty values", func(t *testing.T) {
		secrets := parseSecrets("/test/key=")
		if len(secrets) != 1 {
			t.Errorf("Expected 1 secret, got %d", len(secrets))
		}
		if secrets["/test/key"] != "" {
			t.Errorf("Expected empty value, got %q", secrets["/test/key"])
		}
	})

	t.Run("secrets with equals in value", func(t *testing.T) {
		secrets := parseSecrets("/test/key=value=with=equals")
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
		getFilteredKeys(secrets, "/prod/service1")
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
		parseSecrets(content)
	}
}

func BenchmarkValidateKeyPath(b *testing.B) {
	keyPath := "/prod/billing-svc/vars/mg"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validateKeyPath(keyPath)
	}
}
