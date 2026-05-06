package storage

import (
	"strings"
	"testing"
	"time"
)

func TestParseSecretsToml(t *testing.T) {
	content := `["app1/database/primary"]
value = "pg_password"
updated = "2026-05-01T10:30:00Z"
expires = ""

["app1/database/replica"]
value = "pg_readonly"
updated = "2026-05-01T10:30:00Z"
expires = "2026-12-31T00:00:00Z"
`

	store, err := parseSecretsToml(content)
	if err != nil {
		t.Fatalf("parseSecretsToml() error: %v", err)
	}

	if len(store) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(store))
	}

	entry := store["/app1/database/primary"]
	if entry.Value != "pg_password" {
		t.Errorf("Expected value 'pg_password', got %q", entry.Value)
	}
	if entry.Updated != "2026-05-01T10:30:00Z" {
		t.Errorf("Expected updated '2026-05-01T10:30:00Z', got %q", entry.Updated)
	}
	if entry.Expires != "" {
		t.Errorf("Expected empty expires, got %q", entry.Expires)
	}

	replica := store["/app1/database/replica"]
	if replica.Expires != "2026-12-31T00:00:00Z" {
		t.Errorf("Expected expires '2026-12-31T00:00:00Z', got %q", replica.Expires)
	}
}

func TestSerializeSecretsRoundTrip(t *testing.T) {
	store := SecretStore{
		"/app1/database/primary": {
			Value:   "pg_password",
			Updated: "2026-05-01T10:30:00Z",
			Expires: "",
		},
		"/app1/database/replica": {
			Value:   "pg_readonly",
			Updated: "2026-05-01T10:30:00Z",
			Expires: "2026-12-31T00:00:00Z",
		},
	}

	content, err := serializeSecrets(store)
	if err != nil {
		t.Fatalf("serializeSecrets() error: %v", err)
	}

	parsed, err := parseSecretsToml(content)
	if err != nil {
		t.Fatalf("parseSecretsToml() error on round-trip: %v", err)
	}

	if len(parsed) != len(store) {
		t.Fatalf("Round-trip: expected %d entries, got %d", len(store), len(parsed))
	}

	for key, expected := range store {
		actual, exists := parsed[key]
		if !exists {
			t.Errorf("Key %q not found after round-trip", key)
			continue
		}
		if actual.Value != expected.Value {
			t.Errorf("Key %q: value mismatch: %q vs %q", key, actual.Value, expected.Value)
		}
		if actual.Updated != expected.Updated {
			t.Errorf("Key %q: updated mismatch: %q vs %q", key, actual.Updated, expected.Updated)
		}
		if actual.Expires != expected.Expires {
			t.Errorf("Key %q: expires mismatch: %q vs %q", key, actual.Expires, expected.Expires)
		}
	}
}

func TestSerializeSecretsMultiline(t *testing.T) {
	cert := "-----BEGIN CERTIFICATE-----\nMIIBxTCCAWugAwIBAgIJAJE\n-----END CERTIFICATE-----"
	store := SecretStore{
		"/test/cert": {
			Value:   cert,
			Updated: "2026-05-01T10:30:00Z",
			Expires: "",
		},
	}

	content, err := serializeSecrets(store)
	if err != nil {
		t.Fatalf("serializeSecrets() error: %v", err)
	}

	if !strings.Contains(content, `"""`+"\n") {
		t.Error("Expected triple-quoted string for multiline value")
	}

	parsed, err := parseSecretsToml(content)
	if err != nil {
		t.Fatalf("parseSecretsToml() error on multiline round-trip: %v", err)
	}

	entry := parsed["/test/cert"]
	if entry.Value != cert {
		t.Errorf("Multiline round-trip failed: got %q, want %q", entry.Value, cert)
	}
}

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "empty content",
			content:  "",
			expected: "toml",
		},
		{
			name: "toml content",
			content: `["app1/db"]
value = "secret"
updated = "2026-05-01T10:30:00Z"
expires = ""`,
			expected: "toml",
		},
		{
			name:     "legacy content",
			content:  "/prod/key=value123\n/dev/key=value456",
			expected: "legacy",
		},
		{
			name:     "single legacy line",
			content:  "/test/key=myvalue",
			expected: "legacy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectFormat(tt.content)
			if result != tt.expected {
				t.Errorf("detectFormat() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestParseLegacySecrets(t *testing.T) {
	content := "/prod/key1=value1\n/prod/key2=value2\n/dev/key3=value=with=equals"

	store := parseLegacySecrets(content)

	if len(store) != 3 {
		t.Fatalf("Expected 3 entries, got %d", len(store))
	}

	if store["/prod/key1"].Value != "value1" {
		t.Errorf("Expected 'value1', got %q", store["/prod/key1"].Value)
	}
	if store["/dev/key3"].Value != "value=with=equals" {
		t.Errorf("Expected 'value=with=equals', got %q", store["/dev/key3"].Value)
	}
	if store["/prod/key1"].Updated != "" {
		t.Errorf("Legacy entries should have empty Updated, got %q", store["/prod/key1"].Updated)
	}
}

func TestSetSecret(t *testing.T) {
	store := make(SecretStore)
	SetSecret(store, "/test/key", "myvalue")

	entry, exists := store["/test/key"]
	if !exists {
		t.Fatal("Key not found after SetSecret")
	}
	if entry.Value != "myvalue" {
		t.Errorf("Expected value 'myvalue', got %q", entry.Value)
	}
	if entry.Updated == "" {
		t.Error("Updated should be set automatically")
	}

	_, err := time.Parse(time.RFC3339, entry.Updated)
	if err != nil {
		t.Errorf("Updated is not valid RFC3339: %v", err)
	}
}

func TestSetSecretWithExpires(t *testing.T) {
	store := make(SecretStore)
	expires := "2026-12-31T00:00:00Z"
	SetSecretWithExpires(store, "/test/key", "myvalue", expires)

	entry := store["/test/key"]
	if entry.Value != "myvalue" {
		t.Errorf("Expected value 'myvalue', got %q", entry.Value)
	}
	if entry.Expires != expires {
		t.Errorf("Expected expires %q, got %q", expires, entry.Expires)
	}
	if entry.Updated == "" {
		t.Error("Updated should be set automatically")
	}
}

func TestMoveSecretPreservesMetadata(t *testing.T) {
	store := SecretStore{
		"/old/key": {
			Value:   "secret",
			Updated: "2026-01-01T00:00:00Z",
			Expires: "2026-12-31T00:00:00Z",
		},
	}

	err := MoveSecret(store, "/old/key", "/new/key")
	if err != nil {
		t.Fatalf("MoveSecret() error: %v", err)
	}

	if _, exists := store["/old/key"]; exists {
		t.Error("Old key should be removed")
	}

	entry, exists := store["/new/key"]
	if !exists {
		t.Fatal("New key not found")
	}
	if entry.Value != "secret" {
		t.Errorf("Value not preserved: got %q", entry.Value)
	}
	if entry.Expires != "2026-12-31T00:00:00Z" {
		t.Errorf("Expires not preserved: got %q", entry.Expires)
	}
	if entry.Updated == "2026-01-01T00:00:00Z" {
		t.Error("Updated should be refreshed on move")
	}
}

func TestParseSecretsAutoDetect(t *testing.T) {
	t.Run("toml input", func(t *testing.T) {
		content := `["test/key"]
value = "secret123"
updated = "2026-05-01T10:30:00Z"
expires = ""`

		store := ParseSecrets(content)
		if len(store) != 1 {
			t.Fatalf("Expected 1 entry, got %d", len(store))
		}
		if store["/test/key"].Value != "secret123" {
			t.Errorf("Expected 'secret123', got %q", store["/test/key"].Value)
		}
	})

	t.Run("legacy input", func(t *testing.T) {
		content := "/test/key=secret123"
		store := ParseSecrets(content)
		if len(store) != 1 {
			t.Fatalf("Expected 1 entry, got %d", len(store))
		}
		if store["/test/key"].Value != "secret123" {
			t.Errorf("Expected 'secret123', got %q", store["/test/key"].Value)
		}
	})

	t.Run("empty input", func(t *testing.T) {
		store := ParseSecrets("")
		if len(store) != 0 {
			t.Errorf("Expected empty store, got %d entries", len(store))
		}
	})
}

func TestSecretExistsReturnsEntry(t *testing.T) {
	store := SecretStore{
		"/test/key": {
			Value:   "myvalue",
			Updated: "2026-05-01T10:30:00Z",
			Expires: "2026-12-31T00:00:00Z",
		},
	}

	entry, exists := SecretExists(store, "/test/key")
	if !exists {
		t.Fatal("Expected key to exist")
	}
	if entry.Value != "myvalue" {
		t.Errorf("Expected value 'myvalue', got %q", entry.Value)
	}
	if entry.Updated != "2026-05-01T10:30:00Z" {
		t.Errorf("Expected updated timestamp, got %q", entry.Updated)
	}

	_, exists = SecretExists(store, "/nonexistent")
	if exists {
		t.Error("Expected key to not exist")
	}
}

func TestGetSecretsForPath(t *testing.T) {
	store := SecretStore{
		"/prod/db/primary": {Value: "pg_pass"},
		"/prod/db/replica": {Value: "pg_read"},
		"/prod/api/key":    {Value: "api_key"},
		"/dev/db/primary":  {Value: "dev_pass"},
	}

	result := GetSecretsForPath(store, "/prod/db")

	if len(result) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(result))
	}
	if result["/prod/db/primary"] != "pg_pass" {
		t.Errorf("Expected 'pg_pass', got %q", result["/prod/db/primary"])
	}
	if result["/prod/db/replica"] != "pg_read" {
		t.Errorf("Expected 'pg_read', got %q", result["/prod/db/replica"])
	}
}

func TestParseExpiryDate(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		wantErr  bool
	}{
		{"2026-12-31", "2026-12-31T00:00:00Z", false},
		{"31.12.2026", "2026-12-31T00:00:00Z", false},
		{"31/12/2026", "2026-12-31T00:00:00Z", false},
		{"2026-12-31T00:00:00Z", "2026-12-31T00:00:00Z", false},
		{"2026-06-15T14:30:00+02:00", "2026-06-15T12:30:00Z", false},
		{"not-a-date", "", true},
		{"12-31-2026", "", true},
		{"2026/12/31", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseExpiryDate(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error for input %q, got %q", tt.input, result)
				}
				return
			}
			if err != nil {
				t.Fatalf("Unexpected error for input %q: %v", tt.input, err)
			}
			if result != tt.expected {
				t.Errorf("ParseExpiryDate(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSetSecretExpiry(t *testing.T) {
	store := SecretStore{
		"/test/key": {
			Value:   "myvalue",
			Updated: "2026-05-01T10:30:00Z",
			Expires: "",
		},
	}

	SetSecretExpiry(store, "/test/key", "2026-12-31T00:00:00Z")

	entry := store["/test/key"]
	if entry.Value != "myvalue" {
		t.Errorf("Value should be preserved, got %q", entry.Value)
	}
	if entry.Updated != "2026-05-01T10:30:00Z" {
		t.Errorf("Updated should be preserved, got %q", entry.Updated)
	}
	if entry.Expires != "2026-12-31T00:00:00Z" {
		t.Errorf("Expected expires '2026-12-31T00:00:00Z', got %q", entry.Expires)
	}
}

func TestSerializeSecretsEmpty(t *testing.T) {
	store := make(SecretStore)
	content, err := serializeSecrets(store)
	if err != nil {
		t.Fatalf("serializeSecrets() error: %v", err)
	}
	if content != "" {
		t.Errorf("Expected empty string for empty store, got %q", content)
	}
}
