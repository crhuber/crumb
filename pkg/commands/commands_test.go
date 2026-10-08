package commands

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"crumb/pkg/config"
	"crumb/pkg/storage"
)

func TestComputeEnvDiff(t *testing.T) {
	tests := []struct {
		name         string
		newVars      map[string]string
		setupEnv     map[string]string
		expectedDiff string
	}{
		{
			name: "all new variables",
			newVars: map[string]string{
				"BASE_URL":   "http://example.com",
				"K8S_CONFIG": "/path/to/config",
				"API_KEY":    "secret123",
			},
			setupEnv:     map[string]string{},
			expectedDiff: "+API_KEY +BASE_URL +K8S_CONFIG",
		},
		{
			name: "mix of new and modified variables",
			newVars: map[string]string{
				"BASE_URL":   "http://example.com",
				"K8S_CONFIG": "/path/to/config",
				"API_KEY":    "newsecret456",
			},
			setupEnv: map[string]string{
				"API_KEY": "oldsecret123",
				"PATH":    "/usr/bin",
			},
			expectedDiff: "+BASE_URL +K8S_CONFIG ~API_KEY",
		},
		{
			name: "no changes",
			newVars: map[string]string{
				"API_KEY": "secret123",
			},
			setupEnv: map[string]string{
				"API_KEY": "secret123",
			},
			expectedDiff: "",
		},
		{
			name: "modified only",
			newVars: map[string]string{
				"API_KEY": "newsecret456",
			},
			setupEnv: map[string]string{
				"API_KEY": "oldsecret123",
			},
			expectedDiff: "~API_KEY",
		},
		{
			name:         "empty newVars",
			newVars:      map[string]string{},
			setupEnv:     map[string]string{},
			expectedDiff: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup environment variables
			for key, value := range tt.setupEnv {
				os.Setenv(key, value)
			}
			defer func() {
				// Cleanup
				for key := range tt.setupEnv {
					os.Unsetenv(key)
				}
			}()

			// Compute diff
			result := computeEnvDiff(tt.newVars)

			// Compare result
			if result != tt.expectedDiff {
				t.Errorf("computeEnvDiff() = %q, want %q", result, tt.expectedDiff)
			}

			// Additional validation: check that variables are sorted within their category
			if result != "" {
				parts := strings.Split(result, " ")

				// Group by prefix
				addedVars := []string{}
				modifiedVars := []string{}

				for _, part := range parts {
					if len(part) > 0 {
						switch part[0] {
						case '+':
							addedVars = append(addedVars, part[1:])
						case '~':
							modifiedVars = append(modifiedVars, part[1:])
						}
					}
				}

				// Check that added vars are sorted
				for i := 1; i < len(addedVars); i++ {
					if addedVars[i-1] > addedVars[i] {
						t.Errorf("Added variables are not sorted: %v", addedVars)
						break
					}
				}

				// Check that modified vars are sorted
				for i := 1; i < len(modifiedVars); i++ {
					if modifiedVars[i-1] > modifiedVars[i] {
						t.Errorf("Modified variables are not sorted: %v", modifiedVars)
						break
					}
				}
			}
		})
	}
}

func TestBuildLoadEnvVars(t *testing.T) {
	secrets := storage.SecretStore{
		"/xapi/live/db/url":    {Value: "postgres://live"},
		"/xapi/live/api-key":   {Value: "path-api-key"},
		"/xapi/live-old/stale": {Value: "should-not-load"},
		"/shared/API_KEY":      {Value: "shared-api-key"},
		"/shared/token":        {Value: "shared-token"},
	}

	tests := []struct {
		name    string
		ec      config.EnvironmentConfig
		want    map[string]string
		wantErr bool
	}{
		{
			name: "path only, nested keys, no sibling prefix leak",
			ec:   config.EnvironmentConfig{Path: "/xapi/live/"},
			want: map[string]string{"DB_URL": "postgres://live", "API_KEY": "path-api-key"},
		},
		{
			name: "keys only",
			ec:   config.EnvironmentConfig{Keys: map[string]string{"TOKEN": "/shared/token"}},
			want: map[string]string{"TOKEN": "shared-token"},
		},
		{
			name: "keys override path",
			ec: config.EnvironmentConfig{
				Path: "/xapi/live/",
				Keys: map[string]string{"API_KEY": "/shared/API_KEY"},
			},
			want: map[string]string{"DB_URL": "postgres://live", "API_KEY": "shared-api-key"},
		},
		{
			name: "remap applies to path vars only",
			ec: config.EnvironmentConfig{
				Path:  "/xapi/live/",
				Remap: map[string]string{"DB_URL": "DATABASE_URL", "TOKEN": "RENAMED"},
				Keys:  map[string]string{"TOKEN": "/shared/token"},
			},
			want: map[string]string{"DATABASE_URL": "postgres://live", "API_KEY": "path-api-key", "TOKEN": "shared-token"},
		},
		{
			name: "env values are literal",
			ec:   config.EnvironmentConfig{Env: map[string]string{"LOG_LEVEL": "info", "NOT_SECRET": "/shared/API_KEY"}},
			want: map[string]string{"LOG_LEVEL": "info", "NOT_SECRET": "/shared/API_KEY"},
		},
		{
			name:    "missing keys secret fails",
			ec:      config.EnvironmentConfig{Keys: map[string]string{"NOPE": "/shared/missing"}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildLoadEnvVars(secrets, tt.ec)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
