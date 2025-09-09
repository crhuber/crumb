package storage

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseEnvContent(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected map[string]string
	}{
		{
			name: "basic key-value pairs",
			content: `KEY1=value1
KEY2=value2`,
			expected: map[string]string{
				"KEY1": "value1",
				"KEY2": "value2",
			},
		},
		{
			name: "quoted values",
			content: `KEY1="quoted value"
KEY2='single quoted'
KEY3=unquoted`,
			expected: map[string]string{
				"KEY1": "quoted value",
				"KEY2": "single quoted",
				"KEY3": "unquoted",
			},
		},
		{
			name: "empty lines and comments",
			content: `# This is a comment
KEY1=value1

# Another comment
KEY2=value2

`,
			expected: map[string]string{
				"KEY1": "value1",
				"KEY2": "value2",
			},
		},
		{
			name: "values with equals signs",
			content: `DATABASE_URL=postgresql://user:pass@host:5432/db?param=value
JWT_SECRET=abc=def=ghi`,
			expected: map[string]string{
				"DATABASE_URL": "postgresql://user:pass@host:5432/db?param=value",
				"JWT_SECRET":   "abc=def=ghi",
			},
		},
		{
			name: "empty values",
			content: `EMPTY_KEY=
ANOTHER_KEY=""`,
			expected: map[string]string{
				"EMPTY_KEY":   "",
				"ANOTHER_KEY": "",
			},
		},
		{
			name: "whitespace handling",
			content: `  KEY1  =  value1  
	KEY2	=	"  value2  "	`,
			expected: map[string]string{
				"KEY1": "value1",
				"KEY2": "  value2  ",
			},
		},
		{
			name:     "empty content",
			content:  "",
			expected: map[string]string{},
		},
		{
			name: "malformed lines ignored",
			content: `KEY1=value1
this_line_has_no_equals_sign
KEY2=value2
=value_without_key
KEY3=value3`,
			expected: map[string]string{
				"KEY1": "value1",
				"KEY2": "value2",
				"KEY3": "value3",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseEnvContent(tt.content)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("parseEnvContent() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestParseEnvFile(t *testing.T) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "crumb_env_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Test file exists
	t.Run("valid .env file", func(t *testing.T) {
		envFile := filepath.Join(tempDir, ".env")
		content := `API_KEY=secret123
DATABASE_URL="postgresql://localhost/test"
DEBUG=true`

		err := os.WriteFile(envFile, []byte(content), 0644)
		if err != nil {
			t.Fatalf("Failed to write test .env file: %v", err)
		}

		result, err := ParseEnvFile(envFile)
		if err != nil {
			t.Fatalf("ParseEnvFile() error = %v", err)
		}

		expected := map[string]string{
			"API_KEY":      "secret123",
			"DATABASE_URL": "postgresql://localhost/test",
			"DEBUG":        "true",
		}

		if !reflect.DeepEqual(result, expected) {
			t.Errorf("ParseEnvFile() = %v, want %v", result, expected)
		}
	})

	// Test file doesn't exist
	t.Run("non-existent file", func(t *testing.T) {
		nonExistentFile := filepath.Join(tempDir, "nonexistent.env")
		_, err := ParseEnvFile(nonExistentFile)
		if err == nil {
			t.Error("ParseEnvFile() should return error for non-existent file")
		}
	})
}

func TestShellQuoteValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple value no quotes needed",
			input:    "simple_value",
			expected: "simple_value",
		},
		{
			name:     "value with spaces needs quotes",
			input:    "server=host port=5432",
			expected: "\"server=host port=5432\"",
		},
		{
			name:     "empty value needs quotes",
			input:    "",
			expected: "\"\"",
		},
		{
			name:     "value with equals and spaces",
			input:    "value with = equals",
			expected: "\"value with = equals\"",
		},
		{
			name:     "value with existing quotes",
			input:    "value with \"embedded quotes\"",
			expected: "\"value with \\\"embedded quotes\\\"\"",
		},
		{
			name:     "value with special chars",
			input:    "value|with&special;chars",
			expected: "\"value|with&special;chars\"",
		},
		{
			name:     "value with tab",
			input:    "value\twith\ttab",
			expected: "\"value\twith\ttab\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShellQuoteValue(tt.input)
			if result != tt.expected {
				t.Errorf("ShellQuoteValue(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
