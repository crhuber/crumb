package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTomlConfig(t *testing.T) {
	// Create a temporary directory for test config
	tempDir, err := os.MkdirTemp("", "crumb_toml_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set HOME to temp directory
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	tests := []struct {
		name          string
		tomlContent   string
		setupFile     bool
		expectedShell string
		expectError   bool
	}{
		{
			name:          "no config file",
			setupFile:     false,
			expectedShell: "",
			expectError:   false,
		},
		{
			name:          "config with bash",
			tomlContent:   `shell = "bash"`,
			setupFile:     true,
			expectedShell: "bash",
			expectError:   false,
		},
		{
			name:          "config with fish",
			tomlContent:   `shell = "fish"`,
			setupFile:     true,
			expectedShell: "fish",
			expectError:   false,
		},
		{
			name:          "config with zsh",
			tomlContent:   `shell = "zsh"`,
			setupFile:     true,
			expectedShell: "zsh",
			expectError:   false,
		},
		{
			name:          "empty config",
			tomlContent:   ``,
			setupFile:     true,
			expectedShell: "",
			expectError:   false,
		},
		{
			name:        "invalid toml",
			tomlContent: `shell = `,
			setupFile:   true,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up any existing config
			configDir := filepath.Join(tempDir, ".config", "crumb")
			os.RemoveAll(configDir)

			if tt.setupFile {
				// Create config directory
				if err := os.MkdirAll(configDir, 0700); err != nil {
					t.Fatalf("Failed to create config dir: %v", err)
				}

				// Create TOML config file
				configPath := filepath.Join(configDir, "crumb.toml")
				if err := os.WriteFile(configPath, []byte(tt.tomlContent), 0600); err != nil {
					t.Fatalf("Failed to write config file: %v", err)
				}
			}

			config, err := LoadTomlConfig()

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if config.Shell != tt.expectedShell {
				t.Errorf("Expected shell %q, got %q", tt.expectedShell, config.Shell)
			}
		})
	}
}

func TestGetShellFromConfig(t *testing.T) {
	// Create a temporary directory for test config
	tempDir, err := os.MkdirTemp("", "crumb_toml_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set HOME to temp directory
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	tests := []struct {
		name          string
		tomlContent   string
		setupFile     bool
		expectedShell string
	}{
		{
			name:          "no config file",
			setupFile:     false,
			expectedShell: "",
		},
		{
			name:          "config with fish",
			tomlContent:   `shell = "fish"`,
			setupFile:     true,
			expectedShell: "fish",
		},
		{
			name:          "invalid toml returns empty",
			tomlContent:   `shell = `,
			setupFile:     true,
			expectedShell: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up any existing config
			configDir := filepath.Join(tempDir, ".config", "crumb")
			os.RemoveAll(configDir)

			if tt.setupFile {
				// Create config directory
				if err := os.MkdirAll(configDir, 0700); err != nil {
					t.Fatalf("Failed to create config dir: %v", err)
				}

				// Create TOML config file
				configPath := filepath.Join(configDir, "crumb.toml")
				if err := os.WriteFile(configPath, []byte(tt.tomlContent), 0600); err != nil {
					t.Fatalf("Failed to write config file: %v", err)
				}
			}

			shell := GetShellFromConfig()

			if shell != tt.expectedShell {
				t.Errorf("Expected shell %q, got %q", tt.expectedShell, shell)
			}
		})
	}
}

func TestTomlValueSource(t *testing.T) {
	// Create a temporary directory for test config
	tempDir, err := os.MkdirTemp("", "crumb_toml_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set HOME to temp directory
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// Create config directory and file
	configDir := filepath.Join(tempDir, ".config", "crumb")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	configPath := filepath.Join(configDir, "crumb.toml")
	tomlContent := `shell = "fish"`
	if err := os.WriteFile(configPath, []byte(tomlContent), 0600); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Test the value source
	vs := NewTomlValueSource("shell")

	value, found := vs.Lookup()
	if !found {
		t.Error("Expected to find shell value")
	}
	if value != "fish" {
		t.Errorf("Expected shell value %q, got %q", "fish", value)
	}

	// Test String method
	if vs.String() != "TomlConfig" {
		t.Errorf("Expected String() to return 'TomlConfig', got %q", vs.String())
	}

	// Test GoString method
	if vs.GoString() != "TomlConfig" {
		t.Errorf("Expected GoString() to return 'TomlConfig', got %q", vs.GoString())
	}
}

func TestTomlValueSourceNoConfig(t *testing.T) {
	// Create a temporary directory for test config
	tempDir, err := os.MkdirTemp("", "crumb_toml_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set HOME to temp directory (no config file exists)
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// Test the value source with no config file
	vs := NewTomlValueSource("shell")

	value, found := vs.Lookup()
	if found {
		t.Error("Expected not to find shell value when no config exists")
	}
	if value != "" {
		t.Errorf("Expected empty value, got %q", value)
	}
}

func TestTomlValueSourceShowValues(t *testing.T) {
	// Create a temporary directory for test config
	tempDir, err := os.MkdirTemp("", "crumb_toml_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set HOME to temp directory
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	// Create config directory
	configDir := filepath.Join(tempDir, ".config", "crumb")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	tests := []struct {
		name          string
		tomlContent   string
		key           string
		expectedValue string
		expectedFound bool
	}{
		{
			name:          "show_values true",
			tomlContent:   `show_values = true`,
			key:           "show",
			expectedValue: "true",
			expectedFound: true,
		},
		{
			name:          "show_values false",
			tomlContent:   `show_values = false`,
			key:           "show",
			expectedValue: "",
			expectedFound: false,
		},
		{
			name:          "show_values not set",
			tomlContent:   ``,
			key:           "show",
			expectedValue: "",
			expectedFound: false,
		},
		{
			name:          "both shell and show_values",
			tomlContent:   "shell = \"fish\"\nshow_values = true",
			key:           "show",
			expectedValue: "true",
			expectedFound: true,
		},
		{
			name:          "both shell and show_values - lookup shell",
			tomlContent:   "shell = \"fish\"\nshow_values = true",
			key:           "shell",
			expectedValue: "fish",
			expectedFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write config file
			configPath := filepath.Join(configDir, "crumb.toml")
			if err := os.WriteFile(configPath, []byte(tt.tomlContent), 0600); err != nil {
				t.Fatalf("Failed to write config file: %v", err)
			}

			// Test the value source
			vs := NewTomlValueSource(tt.key)

			value, found := vs.Lookup()
			if found != tt.expectedFound {
				t.Errorf("Expected found=%v, got found=%v", tt.expectedFound, found)
			}
			if value != tt.expectedValue {
				t.Errorf("Expected value %q, got %q", tt.expectedValue, value)
			}
		})
	}
}
