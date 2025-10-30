package crypto

import (
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"

	"filippo.io/age"
	"filippo.io/age/agessh"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// ValidateSSHKeys validates that the provided SSH key pair is valid and compatible
func ValidateSSHKeys(publicKeyPath, privateKeyPath string) error {
	// Check if files exist
	if _, err := os.Stat(publicKeyPath); os.IsNotExist(err) {
		return fmt.Errorf("public key file not found: %s", publicKeyPath)
	}

	if _, err := os.Stat(privateKeyPath); os.IsNotExist(err) {
		return fmt.Errorf("private key file not found: %s", privateKeyPath)
	}

	// Read public key
	publicKeyData, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read public key: %w", err)
	}

	// Validate public key format (ssh-rsa or ssh-ed25519)
	publicKeyStr := strings.TrimSpace(string(publicKeyData))
	if !strings.HasPrefix(publicKeyStr, "ssh-rsa ") && !strings.HasPrefix(publicKeyStr, "ssh-ed25519 ") {
		return fmt.Errorf("public key must be of type ssh-rsa or ssh-ed25519")
	}

	// Try to parse the public key with agessh
	_, err = agessh.ParseRecipient(publicKeyStr)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %w", err)
	}

	// Read private key
	privateKeyData, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read private key: %w", err)
	}

	// Try to parse the private key with agessh
	_, err = agessh.ParseIdentity(privateKeyData)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	return nil
}

// EncryptData encrypts the given data using age encryption with the provided recipients
func EncryptData(data string, recipients []age.Recipient) ([]byte, error) {
	var buf strings.Builder
	w, err := age.Encrypt(&buf, recipients...)
	if err != nil {
		return nil, fmt.Errorf("failed to create encryptor: %w", err)
	}

	if _, err := io.WriteString(w, data); err != nil {
		return nil, fmt.Errorf("failed to write data to encryptor: %w", err)
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("failed to close encryptor: %w", err)
	}

	return []byte(buf.String()), nil
}

// DecryptData decrypts the given encrypted data using the provided identity
func DecryptData(encryptedData []byte, identity age.Identity) (string, error) {
	r, err := age.Decrypt(strings.NewReader(string(encryptedData)), identity)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt data: %w", err)
	}

	decryptedData, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("failed to read decrypted data: %w", err)
	}

	return string(decryptedData), nil
}

// ParseSSHPublicKey reads and parses an SSH public key file, returning an age recipient
func ParseSSHPublicKey(publicKeyPath string) (age.Recipient, error) {
	// Read public key
	publicKeyData, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read public key: %w", err)
	}

	// Parse public key as age recipient using agessh
	recipient, err := agessh.ParseRecipient(strings.TrimSpace(string(publicKeyData)))
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key as recipient: %w", err)
	}

	return recipient, nil
}

// ParseSSHPrivateKey reads and parses an SSH private key file, returning an age identity
func ParseSSHPrivateKey(privateKeyPath string) (age.Identity, error) {
	// Read private key
	privateKeyData, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}

	// Parse private key identity
	identity, err := agessh.ParseIdentity(privateKeyData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	return identity, nil
}

// WriteFileWithLock writes data to a file with exclusive locking to prevent concurrent access
func WriteFileWithLock(filePath string, data []byte, perm os.FileMode) error {
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Apply file lock
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX); err != nil {
		return fmt.Errorf("failed to lock file: %w", err)
	}
	defer unix.Flock(int(file.Fd()), unix.LOCK_UN)

	_, err = file.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}

	return nil
}

// ReadFileWithLock reads data from a file with shared locking
func ReadFileWithLock(filePath string) ([]byte, error) {
	file, err := os.OpenFile(filePath, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Apply file lock
	if err := unix.Flock(int(file.Fd()), unix.LOCK_SH); err != nil {
		return nil, fmt.Errorf("failed to lock file: %w", err)
	}
	defer unix.Flock(int(file.Fd()), unix.LOCK_UN)

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return data, nil
}

// ConfirmOverwrite prompts the user for confirmation before overwriting something
func ConfirmOverwrite(item string) bool {
	fmt.Printf("%s already exists. Overwrite? (y/n): ", item)

	// Use terminal to read a single character
	oldState, err := term.MakeRaw(int(syscall.Stdin))
	if err != nil {
		return false
	}
	defer term.Restore(int(syscall.Stdin), oldState)

	var response [1]byte
	n, err := os.Stdin.Read(response[:])
	if err != nil || n == 0 {
		return false
	}

	fmt.Println() // Print newline after the character

	return response[0] == 'y' || response[0] == 'Y'
}
