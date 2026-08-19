// Package config loads crumbd's server configuration from a YAML file with
// environment variable overrides.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// RegistrationMode controls who may create a new vault.
type RegistrationMode string

const (
	RegistrationOpen   RegistrationMode = "open"
	RegistrationToken  RegistrationMode = "token"
	RegistrationClosed RegistrationMode = "closed"
)

// Config holds crumbd's runtime configuration.
type Config struct {
	ListenAddr        string        `yaml:"listen_addr"`
	DatabasePath      string        `yaml:"database_path"`
	RegistrationMode  string        `yaml:"registration_mode"`
	RegistrationToken string        `yaml:"registration_token"`
	MaxBlobSize       int64         `yaml:"max_blob_size"`
	SessionTTL        time.Duration `yaml:"session_ttl"`
	ChallengeTTL      time.Duration `yaml:"challenge_ttl"`
	InviteTTL         time.Duration `yaml:"invite_ttl"`
}

// Default returns a Config with sensible defaults for local development.
func Default() Config {
	return Config{
		ListenAddr:       "127.0.0.1:8420",
		DatabasePath:     "crumbd.db",
		RegistrationMode: string(RegistrationOpen),
		MaxBlobSize:      8 * 1024 * 1024, // 8 MiB
		SessionTTL:       time.Hour,
		ChallengeTTL:     120 * time.Second,
		InviteTTL:        15 * time.Minute,
	}
}

// Load reads a YAML config file (if it exists) over the defaults, then
// applies CRUMBD_* environment variable overrides.
func Load(path string) (Config, error) {
	cfg := Default()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			if !os.IsNotExist(err) {
				return cfg, fmt.Errorf("failed to read config file: %w", err)
			}
		} else if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("failed to parse config file: %w", err)
		}
	}

	if v := os.Getenv("CRUMBD_LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}
	if v := os.Getenv("CRUMBD_DATABASE_PATH"); v != "" {
		cfg.DatabasePath = v
	}
	if v := os.Getenv("CRUMBD_REGISTRATION_MODE"); v != "" {
		cfg.RegistrationMode = v
	}
	if v := os.Getenv("CRUMBD_REGISTRATION_TOKEN"); v != "" {
		cfg.RegistrationToken = v
	}

	switch RegistrationMode(cfg.RegistrationMode) {
	case RegistrationOpen, RegistrationToken, RegistrationClosed:
	default:
		return cfg, fmt.Errorf("invalid registration_mode %q (want open, token, or closed)", cfg.RegistrationMode)
	}
	if RegistrationMode(cfg.RegistrationMode) == RegistrationToken && cfg.RegistrationToken == "" {
		return cfg, fmt.Errorf("registration_mode is 'token' but no registration_token is configured")
	}

	return cfg, nil
}
