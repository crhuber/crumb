package commands

import (
	"os"
	"strings"
	"testing"
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

			// Additional validation: check that result parts are sorted
			if result != "" {
				parts := strings.Split(result, " ")
				for i := 1; i < len(parts); i++ {
					// Strip the +/~ prefix for comparison
					prev := parts[i-1][1:]
					curr := parts[i][1:]
					if prev > curr && parts[i-1][0] == parts[i][0] {
						t.Errorf("Result is not sorted: %q", result)
						break
					}
				}
			}
		})
	}
}
