package storage

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"filippo.io/age"
	"github.com/BurntSushi/toml"

	"crumb/pkg/backend"
	"crumb/pkg/crypto"
)

// SecretEntry holds a secret value and its metadata.
type SecretEntry struct {
	Value   string `toml:"value"`
	Updated string `toml:"updated"`
	Expires string `toml:"expires"`
}

// SecretStore is the top-level structure: map of key-path to entry.
type SecretStore map[string]SecretEntry

// LoadSecrets loads and decrypts secrets from the given backend.
func LoadSecrets(privateKeyPath string, b backend.Backend) (SecretStore, error) {
	exists, err := b.Exists()
	if err != nil {
		return nil, fmt.Errorf("failed to check storage: %w", err)
	}
	if !exists {
		return make(SecretStore), nil
	}

	identity, err := crypto.ParseSSHPrivateKey(privateKeyPath)
	if err != nil {
		return nil, err
	}

	encryptedData, err := b.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read secrets: %w", err)
	}

	if len(encryptedData) == 0 {
		return make(SecretStore), nil
	}

	decryptedData, err := crypto.DecryptData(encryptedData, identity)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt secrets: %w", err)
	}

	content := strings.TrimSpace(decryptedData)
	if content == "" {
		return make(SecretStore), nil
	}

	format := detectFormat(content)
	if format == "toml" {
		return parseSecretsToml(content)
	}
	return parseLegacySecrets(content), nil
}

// SaveSecrets encrypts and saves secrets to the given backend.
func SaveSecrets(secrets SecretStore, publicKeyPath string, b backend.Backend) error {
	recipient, err := crypto.ParseSSHPublicKey(publicKeyPath)
	if err != nil {
		return err
	}

	content, err := serializeSecrets(secrets)
	if err != nil {
		return fmt.Errorf("failed to serialize secrets: %w", err)
	}

	encryptedData, err := crypto.EncryptData(content, []age.Recipient{recipient})
	if err != nil {
		return fmt.Errorf("failed to encrypt secrets: %w", err)
	}

	return b.Write(encryptedData)
}

// CreateEmptyStorage creates an empty encrypted storage via the given backend.
func CreateEmptyStorage(publicKeyPath string, b backend.Backend) error {
	recipient, err := crypto.ParseSSHPublicKey(publicKeyPath)
	if err != nil {
		return err
	}

	encryptedData, err := crypto.EncryptData("", []age.Recipient{recipient})
	if err != nil {
		return fmt.Errorf("failed to encrypt empty secrets: %w", err)
	}

	return b.Write(encryptedData)
}

// GetFilteredKeys returns a sorted list of keys that match the given path filter.
func GetFilteredKeys(secrets SecretStore, pathFilter string) []string {
	var keys []string

	if pathFilter != "" && pathFilter != "/" {
		pathFilter = strings.TrimSuffix(pathFilter, "/")
	}

	for key := range secrets {
		keys = append(keys, key)
	}

	if pathFilter != "" {
		var filteredKeys []string
		for _, key := range keys {
			if matchesPathFilter(key, pathFilter) {
				filteredKeys = append(filteredKeys, key)
			}
		}
		keys = filteredKeys
	}

	sort.Strings(keys)
	return keys
}

// ExtractVarName converts a key path to a valid environment variable name.
func ExtractVarName(keyPath string) string {
	trimmed := strings.TrimPrefix(keyPath, "/")
	pathSegments := strings.Split(trimmed, "/")
	if len(pathSegments) > 0 {
		varName := pathSegments[len(pathSegments)-1]
		varName = strings.ReplaceAll(varName, "-", "_")
		varName = strings.ToUpper(varName)
		return varName
	}
	return ""
}

// ParseSecrets parses decrypted content into a SecretStore.
// Supports both TOML and legacy key=value formats.
func ParseSecrets(content string) SecretStore {
	content = strings.TrimSpace(content)
	if content == "" {
		return make(SecretStore)
	}
	format := detectFormat(content)
	if format == "toml" {
		store, err := parseSecretsToml(content)
		if err != nil {
			return make(SecretStore)
		}
		return store
	}
	return parseLegacySecrets(content)
}

// DetectFormat returns "toml" or "legacy" based on content inspection.
func DetectFormat(content string) string {
	return detectFormat(content)
}

func detectFormat(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return "toml"
	}
	var store SecretStore
	if _, err := toml.Decode(content, &store); err == nil && len(store) > 0 {
		return "toml"
	}
	return "legacy"
}

// parseSecretsToml parses TOML-formatted secrets content.
// TOML keys are stored without leading slash; this restores them.
func parseSecretsToml(content string) (SecretStore, error) {
	var raw SecretStore
	if _, err := toml.Decode(content, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse TOML secrets: %w", err)
	}
	store := make(SecretStore, len(raw))
	for key, entry := range raw {
		if !strings.HasPrefix(key, "/") {
			key = "/" + key
		}
		store[key] = entry
	}
	return store, nil
}

// ParseLegacySecrets parses the old key=value format into a SecretStore.
func ParseLegacySecrets(content string) SecretStore {
	return parseLegacySecrets(content)
}

func parseLegacySecrets(content string) SecretStore {
	secrets := make(SecretStore)
	lines := strings.Split(strings.TrimSpace(content), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		secrets[key] = SecretEntry{Value: value}
	}

	return secrets
}

// serializeSecrets converts a SecretStore to a TOML string.
func serializeSecrets(store SecretStore) (string, error) {
	if len(store) == 0 {
		return "", nil
	}

	var keys []string
	for key := range store {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var buf strings.Builder
	for i, key := range keys {
		entry := store[key]
		if i > 0 {
			buf.WriteString("\n")
		}

		tomlKey := strings.TrimPrefix(key, "/")
		fmt.Fprintf(&buf, "[%q]\n", tomlKey)

		if strings.Contains(entry.Value, "\n") {
			fmt.Fprintf(&buf, "value = \"\"\"\n%s\"\"\"\n", entry.Value)
		} else {
			fmt.Fprintf(&buf, "value = %q\n", entry.Value)
		}

		fmt.Fprintf(&buf, "updated = %q\n", entry.Updated)
		fmt.Fprintf(&buf, "expires = %q\n", entry.Expires)
	}

	return buf.String(), nil
}

// SerializeSecretsForDisplay returns a human-readable TOML representation of the store.
func SerializeSecretsForDisplay(store SecretStore) string {
	content, err := serializeSecrets(store)
	if err != nil {
		return ""
	}
	return content
}

func matchesPathFilter(key, pathFilter string) bool {
	if pathFilter == "/" {
		return true
	}
	return strings.HasPrefix(key, pathFilter)
}

// SecretExists checks if a secret with the given key exists.
func SecretExists(secrets SecretStore, key string) (SecretEntry, bool) {
	entry, exists := secrets[key]
	return entry, exists
}

// SetSecret sets a secret in the store with the current timestamp.
func SetSecret(secrets SecretStore, key, value string) {
	secrets[key] = SecretEntry{
		Value:   value,
		Updated: time.Now().UTC().Format(time.RFC3339),
	}
}

// SetSecretWithExpires sets a secret with an explicit expiry timestamp.
func SetSecretWithExpires(secrets SecretStore, key, value, expires string) {
	secrets[key] = SecretEntry{
		Value:   value,
		Updated: time.Now().UTC().Format(time.RFC3339),
		Expires: expires,
	}
}

// ParseExpiryDate parses a human-friendly date string into RFC3339 format.
func ParseExpiryDate(input string) (string, error) {
	formats := []string{
		time.RFC3339,
		"2006-01-02",
		"02.01.2006",
		"02/01/2006",
	}
	for _, layout := range formats {
		if t, err := time.Parse(layout, input); err == nil {
			return t.UTC().Format(time.RFC3339), nil
		}
	}
	return "", fmt.Errorf("invalid date format %q, accepted: YYYY-MM-DD, DD.MM.YYYY, DD/MM/YYYY", input)
}

// SetSecretExpiry updates only the expiry on an existing secret.
func SetSecretExpiry(secrets SecretStore, key, expires string) {
	entry := secrets[key]
	entry.Expires = expires
	secrets[key] = entry
}

// DeleteSecret removes a secret from the store.
func DeleteSecret(secrets SecretStore, key string) bool {
	if _, exists := secrets[key]; exists {
		delete(secrets, key)
		return true
	}
	return false
}

// MoveSecret moves a secret from one key to another, preserving metadata.
func MoveSecret(secrets SecretStore, oldKey, newKey string) error {
	entry, exists := secrets[oldKey]
	if !exists {
		return fmt.Errorf("old key not found: %s", oldKey)
	}

	if _, exists := secrets[newKey]; exists {
		if !crypto.ConfirmOverwrite("key") {
			return fmt.Errorf("operation cancelled")
		}
	}

	entry.Updated = time.Now().UTC().Format(time.RFC3339)
	secrets[newKey] = entry
	delete(secrets, oldKey)

	return nil
}

// GetSecretsForPath returns values for all secrets matching a path prefix.
func GetSecretsForPath(secrets SecretStore, pathPrefix string) map[string]string {
	result := make(map[string]string)
	pathPrefix = strings.TrimSuffix(pathPrefix, "/")

	for secretPath, entry := range secrets {
		if strings.HasPrefix(secretPath, pathPrefix) {
			result[secretPath] = entry.Value
		}
	}

	return result
}

// ConvertPathToEnvVar converts a secret path to an environment variable name for direct export.
func ConvertPathToEnvVar(secretPath, pathPrefix string) string {
	remainingPath := strings.TrimPrefix(secretPath, pathPrefix)
	remainingPath = strings.TrimPrefix(remainingPath, "/")

	pathSegments := strings.Split(remainingPath, "/")
	if len(pathSegments) > 0 {
		keyName := pathSegments[len(pathSegments)-1]
		keyName = strings.ToUpper(strings.ReplaceAll(keyName, "-", "_"))
		return keyName
	}

	return ""
}

// ParseEnvFile parses a .env file and returns a map of key-value pairs.
func ParseEnvFile(filePath string) (map[string]string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read .env file: %w", err)
	}

	return parseEnvContent(string(content)), nil
}

// parseEnvContent parses .env file content into a map.
func parseEnvContent(content string) map[string]string {
	envVars := make(map[string]string)
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		eqIndex := strings.Index(line, "=")
		if eqIndex == -1 {
			continue
		}

		key := strings.TrimSpace(line[:eqIndex])
		value := strings.TrimSpace(line[eqIndex+1:])

		if len(value) >= 2 {
			if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
				(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
				value = value[1 : len(value)-1]
			}
		}

		if key != "" {
			envVars[key] = value
		}
	}

	return envVars
}

// ShellQuoteValue quotes a value for safe shell consumption if needed.
func ShellQuoteValue(value string) string {
	needsQuoting := false

	for _, char := range value {
		if char == ' ' || char == '\t' || char == '|' || char == '&' ||
			char == ';' || char == '(' || char == ')' || char == '<' ||
			char == '>' || char == '`' || char == '$' || char == '"' ||
			char == '\'' || char == '\\' || char == '*' || char == '?' ||
			char == '[' || char == ']' || char == '{' || char == '}' ||
			char == '~' || char == '#' || char == '!' {
			needsQuoting = true
			break
		}
	}

	if value == "" {
		needsQuoting = true
	}

	if needsQuoting {
		escaped := strings.ReplaceAll(value, "\"", "\\\"")
		return "\"" + escaped + "\""
	}

	return value
}
