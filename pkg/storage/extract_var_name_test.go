package storage

import (
	"testing"
)

func TestExtractVarName(t *testing.T) {
	tests := []struct {
		name     string
		keyPath  string
		expected string
	}{
		{
			name:     "simple key name",
			keyPath:  "/foo/live/DB_DSN",
			expected: "DB_DSN",
		},
		{
			name:     "key with dashes",
			keyPath:  "/prod/billing-svc/api-key",
			expected: "API_KEY",
		},
		{
			name:     "nested path with final key",
			keyPath:  "/prod/billing-svc/vars/mg",
			expected: "MG",
		},
		{
			name:     "single level key",
			keyPath:  "/API_KEY",
			expected: "API_KEY",
		},
		{
			name:     "key with underscores and dashes",
			keyPath:  "/dev/test/secret-key_name",
			expected: "SECRET_KEY_NAME",
		},
		{
			name:     "lowercase key",
			keyPath:  "/env/database_url",
			expected: "DATABASE_URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractVarName(tt.keyPath)
			if result != tt.expected {
				t.Errorf("ExtractVarName(%q) = %q, want %q", tt.keyPath, result, tt.expected)
			}
		})
	}
}