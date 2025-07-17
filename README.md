# Crumb - Secure API Key Management Tool

`crumb` is a command line tool designed to securely store, manage, and export API keys and secrets for developers. It uses an encrypted plain text file as the backend, leveraging the `age` encryption library with SSH public/private key pairs for encryption and decryption.

## Installation

1. Download and add binary to $PATH from https://github.com/crhuber/crumb/releases


## Usage

### Setup Command

The `setup` command initializes the secure storage backend.

```bash
./crumb setup
```

This command:
- Creates a directory at `~/.config/crumb/` if it doesn't exist
- Prompts for your SSH public key path (e.g., `~/.ssh/id_ed25519.pub`)
- Prompts for your SSH private key path (e.g., `~/.ssh/id_ed25519`)
- Validates that the provided keys are of type `ssh-rsa` or `ssh-ed25519`
- Creates `~/.config/crumb/config.yaml` with the key paths
- Creates an empty encrypted secrets file at `~/.config/crumb/secrets`
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
$ ./crumb setup
Enter path to SSH public key (e.g., ~/.ssh/id_ed25519.pub): ~/.ssh/id_ed25519.pub
Enter path to SSH private key (e.g., ~/.ssh/id_ed25519): ~/.ssh/id_ed25519
Setup completed successfully!
Config file: /Users/username/.config/crumb/config.yaml
Secrets file: /Users/username/.config/crumb/secrets
```

### List Command

The `ls` command lists all stored secret keys, optionally filtered by path.

```bash
./crumb ls [path]
```

This command:
- Decrypts the secrets file using the private key
- Displays all secret keys (not values) in sorted order
- Supports optional path filtering with partial matching
- Treats trailing slashes as equivalent (e.g., `/any/` vs `/any`)

#### Example Usage

```bash
# List all secrets
$ ./crumb ls
/any/other/mykey
/any/path/mykey
/prod/api_key
/prod/auth-svc/secret
/prod/billing-svc/api_key
/test/mykey

# Filter by path prefix
$ ./crumb ls /prod
/prod/api_key
/prod/auth-svc/secret
/prod/billing-svc/api_key

# Filter with partial matching
$ ./crumb ls /any
/any/other/mykey
/any/path/mykey

# No secrets found
$ ./crumb ls /nonexistent
No secrets found matching path: /nonexistent

# Empty secrets file
$ ./crumb ls
No secrets found
```

### Set Command

The `set` command adds or updates a secret key-value pair.

```bash
./crumb set <key-path> <value>
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
$ ./crumb set /prod/api_key secret123
Successfully set key: /prod/api_key

# Update an existing secret (with confirmation)
$ ./crumb set /prod/api_key newsecret456
Key '/prod/api_key' already exists with value: secret123
key already exists. Overwrite? (y/n): y
Successfully set key: /prod/api_key

# Invalid key path examples
$ ./crumb set invalid_key value
Error: key path must start with '/'

$ ./crumb set "/test/key with spaces" value
Error: key path cannot contain spaces
```

### Get Command

The `get` command retrieves a secret by its key path.

```bash
./crumb get <key-path> [--show]
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
$ ./crumb get /prod/api_key
/prod/api_key=****

# Get a secret with actual value
$ ./crumb get --show /prod/api_key
/prod/api_key=secret123

# Key not found
$ ./crumb get /nonexistent/key
Key not found.

# Invalid key path
$ ./crumb get invalid_key
Error: key path must start with '/'

$ ./crumb get "/test/key with spaces"
Error: key path cannot contain spaces
```

### Init Command

The `init` command creates a YAML configuration file in the current directory.

```bash
./crumb init
```

This command:
- Creates a `.crumb.yaml` file in the current directory
- Uses a default structure with empty configuration
- Prompts for confirmation if the file already exists
- Validates the YAML structure before writing

#### Example Usage

```bash
# Create a new .crumb.yaml file
$ ./crumb init
Successfully created .crumb.yaml

# File already exists (with confirmation)
$ ./crumb init
Config file .crumb.yaml already exists. Overwrite? (y/n): y
Successfully created .crumb.yaml

# Reject overwrite
$ ./crumb init
Config file .crumb.yaml already exists. Overwrite? (y/n): n
Operation cancelled.
```

#### Default Configuration Structure

The created `.crumb.yaml` file contains:

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

#### Remapping Keys

You can change what finally gets exported to shell by using `path_sync.remap` using the following format:

```
  remap: {
        "FROM": "TO"
    }
```


ie:
```
    remap: {
        "SOME-SECRET-KEY": "MY-KEY"
    }
```
will result in SOME-SECRET-KEY being exported as MY-KEY

```
export MY-KEY=******
```

### Delete Command

The `delete` command deletes a secret key-value pair from the encrypted file.

```bash
./crumb delete <key-path>
```

This command:
- Validates the key path (must start with `/`, no spaces or special characters)
- Decrypts the secrets file using the private key
- Prompts the user to confirm deletion by typing the exact key path
- Removes the key-value pair if it exists
- Re-encrypts and saves the secrets file
- Fails gracefully if the key does not exist

#### Example Usage

```bash
# Delete a secret (with confirmation)
$ ./crumb delete /prod/billing-svc/vars/mg
Type the key path to confirm deletion: /prod/billing-svc/vars/mg
Successfully deleted key: /prod/billing-svc/vars/mg

# Wrong confirmation (deletion cancelled)
$ ./crumb delete /prod/api_key
Type the key path to confirm deletion: /wrong/path
Confirmation failed. Deletion cancelled.

# Key not found
$ ./crumb delete /nonexistent/key
Key not found.

# Invalid key path
$ ./crumb delete invalid_key
Error: key path must start with '/'

$ ./crumb delete "/test/key with spaces"
Error: key path cannot contain spaces
```

### Export Command

The `export` command exports secrets as shell-compatible environment variable assignments based on the `.crumb.yaml` config file.

```bash
./crumb export [--shell=bash|fish] [-f config-file]
```

This command:
- Reads the `.crumb.yaml` configuration file from the current directory (or a custom path with `-f`)
- Validates the YAML structure and paths
- Decrypts the secrets file using the private key
- Processes the `path_sync` section to export secrets matching a path prefix
- Processes the `env` section to export individual secrets
- Applies remapping from the `remap` section
- Outputs in Bash (`export VAR=value`) or Fish (`set -x VAR value`) format
- Includes comments for clarity

#### Example Usage

First, create a `.crumb.yaml` configuration file:

```yaml
version: 1
path_sync:
  path: "/prod/billing-svc"
  remap:
    VARS_MG: "MG_KEY"
    VARS_STRIPE: "STRIPE_KEY"

env:
  DATABASE_URL:
    path: "/prod/billing-svc/db/url"
  API_SECRET:
    path: "/prod/billing-svc/api/secret"
```

Then export the secrets:

```bash
# Export for bash (default)
$ ./crumb export
# Exported from /prod/billing-svc
export API_SECRET=secret123
export DATABASE_URL=postgres://user:pass@localhost/db
export MG_KEY=mgsecret
export STRIPE_KEY=stripesecret

# Export for fish shell
$ ./crumb export --shell=fish
# Exported from /prod/billing-svc
set -x API_SECRET secret123
set -x DATABASE_URL postgres://user:pass@localhost/db
set -x MG_KEY mgsecret
set -x STRIPE_KEY stripesecret

# Use a custom config file
$ ./crumb export -f my-project.yaml
# Exported from /prod/my-project
export MY_SECRET=value123

# Use custom config file with long flag
$ ./crumb export --file my-project.yaml --shell=fish
# Exported from /prod/my-project
set -x MY_SECRET value123

# Source the output directly
$ source <(./crumb export)
$ echo $MG_KEY
mgsecret
```

### Other Commands

All major commands are now implemented. See the command documentation above for full usage details.

## Security Features

- Secrets are encrypted at rest using the `age` encryption library
- Uses SSH public/private key pairs for encryption/decryption
- Private keys are never stored in the secrets file
- File locking prevents concurrent access issues
- Decrypted data is never written to disk or exposed in logs

## Configuration

The tool creates two configuration files:

1. `~/.config/crumb/config.yaml` - Stores SSH key paths
2. `~/.config/crumb/secrets` - Encrypted key-value store


## Development

This project uses [Task](https://taskfile.dev/) for build automation. Common tasks:

- `task test` - Run all tests
- `task test-coverage` - Run tests with coverage report
- `task bench` - Run benchmark tests
- `task build` - Build the application
- `task ci` - Run CI pipeline
- `task clean` - Clean build artifacts
- `task --list` - Show all available tasks
