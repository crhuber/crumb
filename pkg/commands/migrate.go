package commands

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"filippo.io/age"
	"github.com/urfave/cli/v3"

	"crumb/pkg/backend"
	"crumb/pkg/crypto"
	"crumb/pkg/storage"
)

// MigrateCommand migrates secrets from legacy key=value format to TOML format.
func MigrateCommand(_ context.Context, cmd *cli.Command) error {
	cfg, b, err := resolveBackend(cmd)
	if err != nil {
		return err
	}

	exists, err := b.Exists()
	if err != nil {
		return fmt.Errorf("failed to check storage: %w", err)
	}
	if !exists {
		return fmt.Errorf("no storage file found. Run 'crumb setup' first")
	}

	identity, err := crypto.ParseSSHPrivateKey(cfg.PrivateKeyPath)
	if err != nil {
		return err
	}

	encryptedData, err := b.Read()
	if err != nil {
		return fmt.Errorf("failed to read secrets: %w", err)
	}

	if len(encryptedData) == 0 {
		fmt.Println("Storage is empty, nothing to migrate.")
		return nil
	}

	decryptedData, err := crypto.DecryptData(encryptedData, identity)
	if err != nil {
		return fmt.Errorf("failed to decrypt secrets: %w", err)
	}

	content := strings.TrimSpace(decryptedData)
	if content == "" {
		fmt.Println("Storage is empty, nothing to migrate.")
		return nil
	}

	if storage.DetectFormat(content) == "toml" {
		fmt.Println("Storage is already in TOML format.")
		return nil
	}

	// Parse legacy format
	legacySecrets := storage.ParseLegacySecrets(content)
	if len(legacySecrets) == 0 {
		fmt.Println("No secrets found to migrate.")
		return nil
	}

	// Set updated timestamp for all entries
	now := time.Now().UTC().Format(time.RFC3339)
	for key, entry := range legacySecrets {
		entry.Updated = now
		legacySecrets[key] = entry
	}

	// Backup the encrypted file
	backupPath, err := backupStorage(b, encryptedData)
	if err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}
	if backupPath != "" {
		fmt.Printf("Backed up to %s\n", backupPath)
	}

	// Re-encrypt and save in TOML format
	recipient, err := crypto.ParseSSHPublicKey(cfg.PublicKeyPath)
	if err != nil {
		return err
	}

	tomlContent := storage.SerializeSecretsForDisplay(legacySecrets)

	newEncrypted, err := crypto.EncryptData(tomlContent, []age.Recipient{recipient})
	if err != nil {
		return fmt.Errorf("failed to encrypt migrated secrets: %w", err)
	}

	if err := b.Write(newEncrypted); err != nil {
		return fmt.Errorf("failed to write migrated secrets: %w", err)
	}

	fmt.Printf("Migrated %d secrets to TOML format.\n", len(legacySecrets))
	return nil
}

func backupStorage(b backend.Backend, encryptedData []byte) (string, error) {
	switch fb := b.(type) {
	case *backend.FileBackend:
		backupPath := fb.Path + ".bak"
		if err := os.WriteFile(backupPath, encryptedData, 0600); err != nil {
			return "", err
		}
		return backupPath, nil
	case *backend.S3Backend:
		backupBackend := &backend.S3Backend{
			Bucket:      fb.Bucket,
			Key:         fb.Key + ".bak",
			EndpointURL: fb.EndpointURL,
		}
		if err := backupBackend.Write(encryptedData); err != nil {
			return "", err
		}
		return fmt.Sprintf("s3://%s/%s.bak", fb.Bucket, fb.Key), nil
	default:
		return "", nil
	}
}
