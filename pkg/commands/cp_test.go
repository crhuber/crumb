package commands

import (
	"testing"

	"crumb/pkg/storage"
)

func TestCollectSecretsToCopy(t *testing.T) {
	secrets := storage.SecretStore{
		"/myapp/api-key": {Value: "secret1"},
		"/myapp/db-url":  {Value: "secret2"},
		"/other/api-key": {Value: "secret3"},
	}

	t.Run("single key", func(t *testing.T) {
		got, err := collectSecretsToCopy(secrets, "/myapp/api-key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := map[string]string{"API_KEY": "secret1"}
		if len(got) != len(want) || got["API_KEY"] != want["API_KEY"] {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("whole path with trailing slash", func(t *testing.T) {
		got, err := collectSecretsToCopy(secrets, "/myapp/")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := map[string]string{"API_KEY": "secret1", "DB_URL": "secret2"}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("got[%q] = %q, want %q", k, got[k], v)
			}
		}
	})

	t.Run("no match returns error", func(t *testing.T) {
		if _, err := collectSecretsToCopy(secrets, "/does-not-exist"); err == nil {
			t.Error("expected error for non-existent path, got nil")
		}
	})

	t.Run("no match on empty prefix returns error", func(t *testing.T) {
		if _, err := collectSecretsToCopy(secrets, "/does-not-exist/"); err == nil {
			t.Error("expected error for non-existent path prefix, got nil")
		}
	})
}

func TestNewProvider(t *testing.T) {
	t.Run("unsupported provider", func(t *testing.T) {
		if _, err := newProvider("aws", "owner/repo"); err == nil {
			t.Error("expected error for unsupported provider, got nil")
		}
	})

	t.Run("github provider", func(t *testing.T) {
		p, err := newProvider("github", "owner/repo")
		if err != nil {
			t.Skipf("gh CLI not available in test environment: %v", err)
		}
		gp, ok := p.(githubProvider)
		if !ok {
			t.Fatalf("expected githubProvider, got %T", p)
		}
		if gp.repo != "owner/repo" {
			t.Errorf("repo = %q, want %q", gp.repo, "owner/repo")
		}
	})
}
