# Crum - Secure API Key Management Tool

`crum` is a command line tool designed to securely store, manage, and export API keys and secrets for developers. It uses an encrypted plain text file as the backend, leveraging the `age` encryption library with SSH public/private key pairs for encryption and decryption.

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

### List Command

The `ls` command lists all stored secret keys, optionally filtered by path.

```bash
./crum ls [path]
```

This command:
- Decrypts the secrets file using the private key
- Displays all secret keys (not values) in sorted order
- Supports optional path filtering with partial matching
- Treats trailing slashes as equivalent (e.g., `/any/` vs `/any`)

#### Example Usage

```bash
# List all secrets
$ ./crum ls
/any/other/mykey
/any/path/mykey
/prod/api_key
/prod/auth-svc/secret
/prod/billing-svc/api_key
/test/mykey

# Filter by path prefix
$ ./crum ls /prod
/prod/api_key
/prod/auth-svc/secret
/prod/billing-svc/api_key

# Filter with partial matching
$ ./crum ls /any
/any/other/mykey
/any/path/mykey

# No secrets found
$ ./crum ls /nonexistent
No secrets found matching path: /nonexistent

# Empty secrets file
$ ./crum ls
No secrets found
```

### Set Command

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

### Get Command

The `get` command retrieves a secret by its key path.

```bash
./crum get <key-path> [--show]
```

This command:
- Validates the key path (must start with `/`, no spaces or special characters)
- Decrypts the secrets file using the private key
- By default, displays the key path and masks the value with `****`
- Supports `--show` flag to display the actual secret value
- Requires a full key path (partial paths are not supported)

#### Example Usage

```bash
# Get a secret (masked value)
$ ./crum get /prod/api_key
/prod/api_key=****

# Get a secret with actual value
$ ./crum get --show /prod/api_key
/prod/api_key=secret123

# Key not found
$ ./crum get /nonexistent/key
Key not found.

# Invalid key path
$ ./crum get invalid_key
Error: key path must start with '/'

$ ./crum get "/test/key with spaces"
Error: key path cannot contain spaces
```

### Init Command

The `init` command creates a YAML configuration file in the current directory.

```bash
./crum init
```

This command:
- Creates a `.crum.yaml` file in the current directory
- Uses a default structure with empty configuration
- Prompts for confirmation if the file already exists
- Validates the YAML structure before writing

#### Example Usage

```bash
# Create a new .crum.yaml file
$ ./crum init
Successfully created .crum.yaml

# File already exists (with confirmation)
$ ./crum init
Config file .crum.yaml already exists. Overwrite? (y/n): y
Successfully created .crum.yaml

# Reject overwrite
$ ./crum init
Config file .crum.yaml already exists. Overwrite? (y/n): n
Operation cancelled.
```

#### Default Configuration Structure

The created `.crum.yaml` file contains:

```yaml
version: "1.0"
path_sync:
    path: ""
    remap: {}
env: {}
```

This structure allows you to configure:
- `path_sync.path`: A path to sync secrets from (e.g., `/prod/billing-svc`)
- `path_sync.remap`: Key remapping for environment variables
- `env`: Individual environment variable configurations

### Other Commands

The following commands are available but not yet implemented:

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
- ✅ List command - Complete
- ✅ Set command - Complete
- ✅ Get command - Complete
- ✅ Init command - Complete
- ⏳ Delete command - Not implemented
- ⏳ Export command - Not implemented
