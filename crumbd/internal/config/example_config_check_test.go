package config_test

import (
	"testing"

	"crumbd/internal/config"
)

// TestExampleConfigLoads is a guard against deploy/config.example.yaml
// bit-rotting relative to the actual Config struct/validation rules.
func TestExampleConfigLoads(t *testing.T) {
	cfg, err := config.Load("../../deploy/config.example.yaml")
	if err != nil {
		t.Fatalf("deploy/config.example.yaml failed to load: %v", err)
	}
	if cfg.ListenAddr == "" || cfg.DatabasePath == "" {
		t.Fatalf("expected example config to set listen_addr/database_path, got %+v", cfg)
	}
}
