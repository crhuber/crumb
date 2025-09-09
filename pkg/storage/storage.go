package storage

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"filippo.io/age"

	"crumb/pkg/crypto"
)

// LoadSecrets loads and decrypts secrets from the storage file
func LoadSecrets(privateKeyPath, storagePath string) (map[string]string, error) {
	if _, err := os.Stat(storagePath); os.IsNotExist(err) {
		return make(map[string]string), nil
	}

	// Parse private key identity
	identity, err := crypto.ParseSSHPrivateKey(privateKeyPath)
	if err != nil {
		return nil, err
	}

	// Read and decrypt secrets file
	encryptedData, err := crypto.ReadFileWithLock(storagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read secrets file: %w", err)
	}

	if len(encryptedData) == 0 {
		return make(map[string]string), nil
	}

	decryptedData, err := crypto.DecryptData(encryptedData, identity)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt secrets: %w", err)
	}

	// Parse key-value pairs
	secrets := parseSecrets(decryptedData)
	return secrets, nil
}

// SaveSecrets encrypts and saves secrets to the storage file
func SaveSecrets(secrets map[string]string, publicKeyPath, storagePath string) error {
	// Parse public key as recipient
	recipient, err := crypto.ParseSSHPublicKey(publicKeyPath)
	if err != nil {
		return err
	}

	// Convert secrets map to string format
	var lines []string
	for key, value := range secrets {
		lines = append(lines, fmt.Sprintf("%s=%s", key, value))
	}

	// Sort lines for consistent output
	sort.Strings(lines)
	content := strings.Join(lines, "\n")

	// Encrypt the content
	encryptedData, err := crypto.EncryptData(content, []age.Recipient{recipient})
	if err != nil {
		return fmt.Errorf("failed to encrypt secrets: %w", err)
	}

	// Write encrypted file with locking
	return crypto.WriteFileWithLock(storagePath, encryptedData, 0600)
}

// CreateEmptySecretsFile creates an empty encrypted secrets file
func CreateEmptySecretsFile(secretsPath, publicKeyPath string) error {
	// Parse public key as recipient
	recipient, err := crypto.ParseSSHPublicKey(publicKeyPath)
	if err != nil {
		return err
	}

	// Create empty secrets content
	emptyContent := ""

	// Encrypt the empty content
	encryptedData, err := crypto.EncryptData(emptyContent, []age.Recipient{recipient})
	if err != nil {
		return fmt.Errorf("failed to encrypt empty secrets: %w", err)
	}

	// Write encrypted file with file locking
	return crypto.WriteFileWithLock(secretsPath, encryptedData, 0600)
}

// GetFilteredKeys returns a sorted list of keys that match the given path filter
func GetFilteredKeys(secrets map[string]string, pathFilter string) []string {
	var keys []string

	// Normalize path filter (remove trailing slash if present)
	if pathFilter != "" && pathFilter != "/" {
		pathFilter = strings.TrimSuffix(pathFilter, "/")
	}

	// Extract keys from secrets map
	for key := range secrets {
		keys = append(keys, key)
	}

	// Filter keys if path filter is provided
	if pathFilter != "" {
		var filteredKeys []string
		for _, key := range keys {
			if matchesPathFilter(key, pathFilter) {
				filteredKeys = append(filteredKeys, key)
			}
		}
		keys = filteredKeys
	}

	// Sort keys alphabetically
	sort.Strings(keys)

	return keys
}

// ExtractVarName converts a key path to a valid environment variable name
func ExtractVarName(keyPath string) string {
	// Remove leading slash
	trimmed := strings.TrimPrefix(keyPath, "/")

	// Get the last segment of the path (the actual secret name)
	pathSegments := strings.Split(trimmed, "/")
	if len(pathSegments) > 0 {
		varName := pathSegments[len(pathSegments)-1]
		// Replace hyphens with underscores
		varName = strings.ReplaceAll(varName, "-", "_")
		// Convert to uppercase
		varName = strings.ToUpper(varName)
		return varName
	}

	return ""
}

// ParseSecrets parses the decrypted secrets content into a map (public for testing)
func ParseSecrets(content string) map[string]string {
	return parseSecrets(content)
}

// parseSecrets parses the decrypted secrets content into a map
func parseSecrets(content string) map[string]string {
	secrets := make(map[string]string)
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
		secrets[key] = value
	}

	return secrets
}

// matchesPathFilter checks if a key matches the given path filter
func matchesPathFilter(key, pathFilter string) bool {
	// Handle root path filter
	if pathFilter == "/" {
		return true
	}

	// Check if key starts with the path filter
	// This provides partial matching as specified in the PRD
	// e.g., "/any" matches "/any/path/mykey"
	return strings.HasPrefix(key, pathFilter)
}

// SecretExists checks if a secret with the given key exists
func SecretExists(secrets map[string]string, key string) (string, bool) {
	value, exists := secrets[key]
	return value, exists
}

// SetSecret sets a secret in the secrets map
func SetSecret(secrets map[string]string, key, value string) {
	secrets[key] = value
}

// DeleteSecret removes a secret from the secrets map
func DeleteSecret(secrets map[string]string, key string) bool {
	if _, exists := secrets[key]; exists {
		delete(secrets, key)
		return true
	}
	return false
}

// MoveSecret moves a secret from one key to another
func MoveSecret(secrets map[string]string, oldKey, newKey string) error {
	value, exists := secrets[oldKey]
	if !exists {
		return fmt.Errorf("old key not found: %s", oldKey)
	}

	// Check if new key already exists
	if _, exists := secrets[newKey]; exists {
		if !crypto.ConfirmOverwrite("key") {
			return fmt.Errorf("operation cancelled")
		}
	}

	// Move the key-value pair
	secrets[newKey] = value
	delete(secrets, oldKey)

	return nil
}

// GetSecretsForPath returns all secrets that match a given path prefix
func GetSecretsForPath(secrets map[string]string, pathPrefix string) map[string]string {
	result := make(map[string]string)
	pathPrefix = strings.TrimSuffix(pathPrefix, "/")

	for secretPath, secretValue := range secrets {
		if strings.HasPrefix(secretPath, pathPrefix) {
			result[secretPath] = secretValue
		}
	}

	return result
}

// ConvertPathToEnvVar converts a secret path to an environment variable name for direct export
func ConvertPathToEnvVar(secretPath, pathPrefix string) string {
	// Extract just the final segment (actual secret name) from the path
	remainingPath := strings.TrimPrefix(secretPath, pathPrefix)
	remainingPath = strings.TrimPrefix(remainingPath, "/")

	// Get the last segment of the path (the actual secret name)
	pathSegments := strings.Split(remainingPath, "/")
	if len(pathSegments) > 0 {
		keyName := pathSegments[len(pathSegments)-1]
		keyName = strings.ToUpper(strings.ReplaceAll(keyName, "-", "_"))
		return keyName
	}

	return ""
}

// ParseEnvFile parses a .env file and returns a map of key-value pairs
func ParseEnvFile(filePath string) (map[string]string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read .env file: %w", err)
	}

	return parseEnvContent(string(content)), nil
}

// parseEnvContent parses .env file content into a map
func parseEnvContent(content string) map[string]string {
	envVars := make(map[string]string)
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Find the first = sign
		eqIndex := strings.Index(line, "=")
		if eqIndex == -1 {
			continue
		}

		key := strings.TrimSpace(line[:eqIndex])
		value := strings.TrimSpace(line[eqIndex+1:])

		// Remove quotes from value if present
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

// ShellQuoteValue quotes a value for safe shell consumption if needed
func ShellQuoteValue(value string) string {
	// Check if value needs quoting (contains spaces, special chars, etc.)
	needsQuoting := false
	
	// Characters that require quoting in shell contexts
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
	
	// Also quote if value is empty
	if value == "" {
		needsQuoting = true
	}
	
	if needsQuoting {
		// Use double quotes and escape any existing double quotes
		escaped := strings.ReplaceAll(value, "\"", "\\\"")
		return "\"" + escaped + "\""
	}
	
	return value
}
