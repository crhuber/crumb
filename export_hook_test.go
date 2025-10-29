package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"

	"crumb/pkg/commands"
	"crumb/pkg/config"
	"crumb/pkg/storage"
)

func createRealSSHKeys(t *testing.T, tempDir string) (string, string) {
	pubKeyPath := filepath.Join(tempDir, "test_key.pub")
	privKeyPath := filepath.Join(tempDir, "test_key")

	// Generate real SSH keys using ssh-keygen
	cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-f", privKeyPath, "-N", "", "-C", "test@example.com")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to generate SSH keys: %v", err)
	}

	return pubKeyPath, privKeyPath
}

func TestExportWithEmptySecretsNoError(t *testing.T) {
	// This test reproduces the issue where hook command fails silently
	// when .crumb.yaml exists but no secrets match the configured path
	tempDir, err := os.MkdirTemp("", "crumb_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set up test environment
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	pubKeyPath, privKeyPath := createRealSSHKeys(t, tempDir)

	// Create storage file with no secrets
	storagePath := filepath.Join(tempDir, "secrets")
	if err := storage.CreateEmptySecretsFile(storagePath, pubKeyPath); err != nil {
		t.Fatalf("Failed to create secrets file: %v", err)
	}

	// Create config
	cfg := &config.Config{
		Profiles: map[string]config.ProfileConfig{
			"default": {
				PublicKeyPath:  pubKeyPath,
				PrivateKeyPath: privKeyPath,
				Storage:        storagePath,
			},
		},
	}

	configDir := filepath.Join(tempDir, ".config", "crumb")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}

	if err := config.SaveConfig(cfg); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Create .crumb.yaml with path configuration
	crumbConfig := &config.CrumbConfig{
		Version: "1.0",
		Environments: map[string]config.EnvironmentConfig{
			"default": {
				Path:  "/platform/dev/",
				Remap: map[string]string{},
				Env:   map[string]string{},
			},
		},
	}

	crumbYamlPath := filepath.Join(tempDir, ".crumb.yaml")
	if err := config.SaveCrumbConfig(crumbConfig, crumbYamlPath); err != nil {
		t.Fatalf("Failed to save .crumb.yaml: %v", err)
	}

	// Change to test directory where .crumb.yaml exists
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Test export command - should not return an error even with no secrets
	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := &cli.Command{
		Name: "export",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "profile", Value: "default"},
			&cli.StringFlag{Name: "storage", Value: ""},
			&cli.StringFlag{Name: "shell", Value: "bash"},
			&cli.StringFlag{Name: "file", Value: ".crumb.yaml"},
			&cli.StringFlag{Name: "env", Value: "default"},
		},
		Action: commands.ExportCommand,
	}

	cmdErr := cmd.Run(context.Background(), []string{"export"})

	w.Close()
	os.Stdout = oldStdout
	buf.ReadFrom(r)
	output := buf.String()

	// The key issue: export should not error when no secrets are found
	// This allows hook scripts to work without error messages
	if cmdErr != nil {
		t.Logf("Output: %s", output)
		t.Errorf("Export should not return error when no secrets found, got: %v", cmdErr)
	}
}

func TestExportWithMatchingSecrets(t *testing.T) {
	// This test verifies that secrets ARE exported when they match the path
	tempDir, err := os.MkdirTemp("", "crumb_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set up test environment
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	pubKeyPath, privKeyPath := createRealSSHKeys(t, tempDir)

	// Create storage file and add secrets
	storagePath := filepath.Join(tempDir, "secrets")
	if err := storage.CreateEmptySecretsFile(storagePath, pubKeyPath); err != nil {
		t.Fatalf("Failed to create secrets file: %v", err)
	}

	// Load secrets and add test data
	secrets, err := storage.LoadSecrets(privKeyPath, storagePath)
	if err != nil {
		t.Fatalf("Failed to load secrets: %v", err)
	}

	storage.SetSecret(secrets, "/platform/dev/API_KEY", "test-api-key-123")
	storage.SetSecret(secrets, "/platform/dev/DB_PASSWORD", "test-db-pass-456")
	storage.SetSecret(secrets, "/other/path/SECRET", "should-not-appear")

	if err := storage.SaveSecrets(secrets, pubKeyPath, storagePath); err != nil {
		t.Fatalf("Failed to save secrets: %v", err)
	}

	// Create config
	cfg := &config.Config{
		Profiles: map[string]config.ProfileConfig{
			"default": {
				PublicKeyPath:  pubKeyPath,
				PrivateKeyPath: privKeyPath,
				Storage:        storagePath,
			},
		},
	}

	configDir := filepath.Join(tempDir, ".config", "crumb")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}

	if err := config.SaveConfig(cfg); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Create .crumb.yaml
	crumbConfig := &config.CrumbConfig{
		Version: "1.0",
		Environments: map[string]config.EnvironmentConfig{
			"default": {
				Path:  "/platform/dev/",
				Remap: map[string]string{},
				Env:   map[string]string{},
			},
		},
	}

	crumbYamlPath := filepath.Join(tempDir, ".crumb.yaml")
	if err := config.SaveCrumbConfig(crumbConfig, crumbYamlPath); err != nil {
		t.Fatalf("Failed to save .crumb.yaml: %v", err)
	}

	// Change to test directory
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Test export command
	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := &cli.Command{
		Name: "export",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "profile", Value: "default"},
			&cli.StringFlag{Name: "storage", Value: ""},
			&cli.StringFlag{Name: "shell", Value: "bash"},
			&cli.StringFlag{Name: "file", Value: ".crumb.yaml"},
			&cli.StringFlag{Name: "env", Value: "default"},
		},
		Action: commands.ExportCommand,
	}

	cmdErr := cmd.Run(context.Background(), []string{"export"})

	w.Close()
	os.Stdout = oldStdout
	buf.ReadFrom(r)
	output := buf.String()

	if cmdErr != nil {
		t.Fatalf("Export failed: %v", cmdErr)
	}

	// Verify output contains expected exports
	if !strings.Contains(output, "API_KEY") {
		t.Errorf("Output should contain API_KEY, got: %s", output)
	}

	if !strings.Contains(output, "DB_PASSWORD") {
		t.Errorf("Output should contain DB_PASSWORD, got: %s", output)
	}

	if strings.Contains(output, "SECRET") {
		t.Errorf("Output should not contain SECRET from /other/path, got: %s", output)
	}

	if !strings.Contains(output, "export") {
		t.Errorf("Output should contain export statements, got: %s", output)
	}
}
