# Crum - Secure API Key Management Tool

`crum` is a comman### Set Command

The `set` command adds or updates a secret key-value pair.

```bash
./crum set <key-path> <value>
```

This command:
- Validates the key path (must start with `/`, no spaces or special characters)
- Decrypts the secrets file using the private key
- Checks if the key already exists and prompts for confirmation if it does
- Adds or updates the key-value pair
- Re-encrypts and saves the secrets file

#### Example Usage

```bash
# Add a new secret
$ ./crum set /prod/api_key secret123
Successfully set key: /prod/api_key

# Update an existing secret (with confirmation)
$ ./crum set /prod/api_key newsecret456
Key '/prod/api_key' already exists with value: secret123
key already exists. Overwrite? (y/n): y
Successfully set key: /prod/api_key

# Invalid key path examples
$ ./crum set invalid_key value
Error: key path must start with '/'

$ ./crum set "/test/key with spaces" value
Error: key path cannot contain spaces
```

### Other Commands

The following commands are available but not yet implemented:

- `crum ls [path]` - List stored secret keys
- `crum init` - Create a YAML configuration file in current directory
- `crum delete <key-path>` - Delete a secret key-value pair
- `crum export [path]` - Export secrets as shell-compatible environment variablesol designed to securely store, manage, and export API keys and secrets for developers. It uses an encrypted plain text file as the backend, leveraging the `age` encryption library with SSH public/private key pairs for encryption and decryption.

## Installation

1. Clone the repository:
   ```bash
   git clone <repository-url>
   cd crum
   ```

2. Build the project:
   ```bash
   go build
   ```

## Usage

### Setup Command

The `setup` command initializes the secure storage backend.

```bash
./crum setup
```

This command:
- Creates a directory at `~/.config/crum/` if it doesn't exist
- Prompts for your SSH public key path (e.g., `~/.ssh/id_ed25519.pub`)
- Prompts for your SSH private key path (e.g., `~/.ssh/id_ed25519`)
- Validates that the provided keys are of type `ssh-rsa` or `ssh-ed25519`
- Creates `~/.config/crum/config.yaml` with the key paths
- Creates an empty encrypted secrets file at `~/.config/crum/secrets`
- Prompts for confirmation if files already exist

#### Prerequisites

Before running setup, you need to have SSH keys generated. If you don't have them, create them with:

```bash
# For Ed25519 keys (recommended)
ssh-keygen -t ed25519 -C "your_email@example.com"

# For RSA keys
ssh-keygen -t rsa -b 4096 -C "your_email@example.com"
```

#### Example Setup Session

```bash
$ ./crum setup
Enter path to SSH public key (e.g., ~/.ssh/id_ed25519.pub): ~/.ssh/id_ed25519.pub
Enter path to SSH private key (e.g., ~/.ssh/id_ed25519): ~/.ssh/id_ed25519
Setup completed successfully!
Config file: /Users/username/.config/crum/config.yaml
Secrets file: /Users/username/.config/crum/secrets
```

### Other Commands

The following commands are available but not yet implemented:

- `crum ls [path]` - List stored secret keys
- `crum set <key-path> <value>` - Add or update a secret key-value pair
- `crum init` - Create a YAML configuration file in current directory
- `crum delete <key-path>` - Delete a secret key-value pair
- `crum export [path]` - Export secrets as shell-compatible environment variables

## Security Features

- Secrets are encrypted at rest using the `age` encryption library
- Uses SSH public/private key pairs for encryption/decryption
- Private keys are never stored in the secrets file
- File locking prevents concurrent access issues
- Decrypted data is never written to disk or exposed in logs

## Configuration

The tool creates two configuration files:

1. `~/.config/crum/config.yaml` - Stores SSH key paths
2. `~/.config/crum/secrets` - Encrypted key-value store

## Dependencies

- Go 1.20 or later
- `github.com/urfave/cli/v2` - CLI framework
- `filippo.io/age` - Encryption library
- `gopkg.in/yaml.v3` - YAML parsing
- `golang.org/x/term` - Terminal handling
- `golang.org/x/sys/unix` - File locking

## Error Handling

The tool provides clear error messages for common issues:

- Missing or invalid SSH key files
- Decryption failures due to mismatched keys
- Invalid key paths or values
- File permission issues
- Concurrent access attempts

## Development Status

- ✅ Setup command - Complete
- ⏳ List command - Not implemented
- ✅ Set command - Complete
- ⏳ Init command - Not implemented
- ⏳ Delete command - Not implemented
- ⏳ Export command - Not implemented
