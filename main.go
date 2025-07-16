package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"filippo.io/age"
	"filippo.io/age/agessh"
	"github.com/urfave/cli/v2"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

// Config represents the configuration stored in ~/.config/crum/config.yaml
type Config struct {
	PublicKeyPath  string `yaml:"public_key_path"`
	PrivateKeyPath string `yaml:"private_key_path"`
}

// CrumConfig represents the per-project configuration in .crum.yaml
type CrumConfig struct {
	Version  string               `yaml:"version"`
	PathSync PathSync             `yaml:"path_sync"`
	Env      map[string]EnvConfig `yaml:"env"`
}

type PathSync struct {
	Path  string            `yaml:"path"`
	Remap map[string]string `yaml:"remap"`
}

type EnvConfig struct {
	Path string `yaml:"path"`
}

func main() {
	app := &cli.App{
		Name:  "crum",
		Usage: "Securely store, manage, and export API keys and secrets",
		Commands: []*cli.Command{
			{
				Name:   "setup",
				Usage:  "Initialize the secure storage backend",
				Action: setupCommand,
			},
			{
				Name:   "ls",
				Usage:  "List stored secret keys",
				Action: listCommand,
			},
			{
				Name:   "set",
				Usage:  "Add or update a secret key-value pair",
				Action: setCommand,
			},
			{
				Name:   "init",
				Usage:  "Create a YAML configuration file in current directory",
				Action: initCommand,
			},
			{
				Name:   "delete",
				Usage:  "Delete a secret key-value pair",
				Action: deleteCommand,
			},
			{
				Name:  "export",
				Usage: "Export secrets as shell-compatible environment variables",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "shell",
						Usage: "Shell format (bash or fish)",
						Value: "bash",
					},
				},
				Action: exportCommand,
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func setupCommand(c *cli.Context) error {
	// Create ~/.config/crum directory if it doesn't exist
	configDir := filepath.Join(os.Getenv("HOME"), ".config", "crum")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(configDir, "config.yaml")
	secretsPath := filepath.Join(configDir, "secrets")

	// Check if files already exist and prompt for confirmation
	if _, err := os.Stat(configPath); err == nil {
		if !confirmOverwrite(configPath) {
			fmt.Println("Setup cancelled.")
			return nil
		}
	}

	if _, err := os.Stat(secretsPath); err == nil {
		if !confirmOverwrite(secretsPath) {
			fmt.Println("Setup cancelled.")
			return nil
		}
	}

	// Prompt for SSH key paths
	publicKeyPath, err := promptForKeyPath("Enter path to SSH public key (e.g., ~/.ssh/id_ed25519.pub): ")
	if err != nil {
		return err
	}

	privateKeyPath, err := promptForKeyPath("Enter path to SSH private key (e.g., ~/.ssh/id_ed25519): ")
	if err != nil {
		return err
	}

	// Expand tilde in paths
	publicKeyPath = expandTilde(publicKeyPath)
	privateKeyPath = expandTilde(privateKeyPath)

	// Validate SSH keys
	if err := validateSSHKeys(publicKeyPath, privateKeyPath); err != nil {
		return fmt.Errorf("invalid or missing SSH key pair. Please generate an SSH key pair using `ssh-keygen -t rsa` or `ssh-keygen -t ed25519` first: %w", err)
	}

	// Create config.yaml
	config := Config{
		PublicKeyPath:  publicKeyPath,
		PrivateKeyPath: privateKeyPath,
	}

	configData, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, configData, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	// Create empty encrypted secrets file
	if err := createEmptySecretsFile(secretsPath, publicKeyPath); err != nil {
		return fmt.Errorf("failed to create secrets file: %w", err)
	}

	fmt.Printf("Setup completed successfully!\n")
	fmt.Printf("Config file: %s\n", configPath)
	fmt.Printf("Secrets file: %s\n", secretsPath)

	return nil
}

func promptForKeyPath(prompt string) (string, error) {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}
	return strings.TrimSpace(input), nil
}

func expandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(os.Getenv("HOME"), path[2:])
	}
	return path
}

func confirmOverwrite(filePath string) bool {
	fmt.Printf("File %s already exists. Overwrite? (y/n): ", filePath)

	// Use terminal to read a single character
	oldState, err := term.MakeRaw(int(syscall.Stdin))
	if err != nil {
		return false
	}
	defer term.Restore(int(syscall.Stdin), oldState)

	var response [1]byte
	_, err = os.Stdin.Read(response[:])
	if err != nil {
		return false
	}

	fmt.Println() // Print newline after the character

	return response[0] == 'y' || response[0] == 'Y'
}

func validateSSHKeys(publicKeyPath, privateKeyPath string) error {
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

func createEmptySecretsFile(secretsPath, publicKeyPath string) error {
	// Read public key
	publicKeyData, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read public key: %w", err)
	}

	// Parse public key as age recipient using agessh
	recipient, err := agessh.ParseRecipient(strings.TrimSpace(string(publicKeyData)))
	if err != nil {
		return fmt.Errorf("failed to parse public key as recipient: %w", err)
	}

	// Create empty secrets content
	emptyContent := ""

	// Encrypt the empty content
	encryptedData, err := encryptData(emptyContent, []age.Recipient{recipient})
	if err != nil {
		return fmt.Errorf("failed to encrypt empty secrets: %w", err)
	}

	// Write encrypted file with file locking
	return writeFileWithLock(secretsPath, encryptedData, 0600)
}

func encryptData(data string, recipients []age.Recipient) ([]byte, error) {
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

func writeFileWithLock(filePath string, data []byte, perm os.FileMode) error {
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

// Placeholder functions for other commands
func listCommand(c *cli.Context) error {
	fmt.Println("List command not implemented yet")
	return nil
}

func setCommand(c *cli.Context) error {
	fmt.Println("Set command not implemented yet")
	return nil
}

func initCommand(c *cli.Context) error {
	fmt.Println("Init command not implemented yet")
	return nil
}

func deleteCommand(c *cli.Context) error {
	fmt.Println("Delete command not implemented yet")
	return nil
}

func exportCommand(c *cli.Context) error {
	fmt.Println("Export command not implemented yet")
	return nil
}
