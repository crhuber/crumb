# Crumb - Secret Management Tool

`crumb` is a command line tool designed to securely store, manage, and export API keys and secrets for developers. It uses `age` encryption with SSH public/private key pairs, and supports both local file storage and S3-compatible backends (AWS S3, MinIO, LocalStack).

`crumb` is inspired by tools such as:

- [Hashicorp Vault](https://www.hashicorp.com/en/products/vault)
- [Teller](https://github.com/tellerops/teller)
- [Gopass](https://github.com/gopasspw/gopass)
- [Sops](https://github.com/getsops/sops)
- [Direnv](https://direnv.net/)
- [SecretSpec](https://devenv.sh/blog/2025/07/21/announcing-secretspec-declarative-secrets-management/)
- [Fnox](https://github.com/jdx/fnox)
- [Scrt](https://scrt.run/)
- [Chezmoi](https://www.chezmoi.io/user-guide/password-managers/)

but, designed for the non-enterprise developer without access to a cloud Secrets Manager, raft storage, etc. Crumb was born out of the need to be able to load secrets without leaving ecrets unencrypted on disk.

## How it works
crumb keeps all your secrets inside a single file encryped with an ssh key.

When getting on your secrets, crumb loads the file into memory, decrypts the payload, creates, retrieves or updates secrets, and, if necessary, encrypts the changes back to the store. Each secret is referenced by a path.

## Quick Start

```bash
# 1. Install crumb
kelp add crhuber/crumb --install

# 2. Initialize your secret store (uses your existing SSH keys)
crumb setup

# 3. Store a secret
crumb set /myapp/api_key
# Enter secret value: [hidden]

# 4. Retrieve it
crumb get /myapp/api_key

# 5. Export secrets as environment variables
eval "$(crumb export --path /myapp)"

# 6. List keys
crumb ls
```

## Features

- **SSH Key Encryption/Decryption**: Securely encrypt your password storage file using SSH Keys
- **S3 Storage Backend**: Store encrypted secrets in AWS S3 or S3-compatible services (MinIO, LocalStack)
- **Bulk Export**: Exports multiple secrets from an entire path like `/myapp/dev/`
- **Multi-Profile Support**: Manage separate secret stores for work, personal, or different projects
- **.env Import**: Import multiple secrets from `.env` files
- **Interactive Selection**: Fuzzy finder for picking secrets with `-i` flag on `get` and `info`
- **Shell Integration**: Automatic secret loading with shell hooks (bash, zsh, fish)

## Installation

1. Download and add binary to $PATH from https://github.com/crhuber/crumb/releases

OR

2. Use [kelp](https://github.com/crhuber/kelp)

```bash
kelp add crhuber/crumb --install
```

## Usage

### Setup Command

The `setup` command initializes the secure storage backend for a specific profile.

```bash
crumb setup [--profile <profile-name>]
```

#### Prerequisites

Before running setup, you need to have SSH keys generated. If you don't have them, create them with:

```bash
# For Ed25519 keys (recommended)
ssh-keygen -t ed25519 -C "your_email@example.com"
```

#### Example Setup Sessions

**Default setup:**
```bash
$ crumb setup
```

**Setup with S3 storage:**
```bash
$ crumb setup --storage s3 --s3-bucket my-secrets-bucket --s3-key /crumb/secrets
```

**Setup with non default profile**
```bash
$ crumb --profile work setup
```


### Set Command

The `set` command adds or updates a secret key-value pair. The value is entered securely on a new line and is not echoed to the terminal or stored in shell history.

```bash
crumb set <key-path> [value] [--expires <RFC3339>]
```


#### Example Usage

```bash
# Add a new secret
$ crumb set /myapp/api_key
Enter secret value: [secret not shown]
Successfully set key: /myapp/api_key

# Set with an expiry date
$ crumb set /myapp/api_key sk_live_abc123 --expires 2026-12-31

# Update an existing secret (with confirmation)
$ crumb set /myapp/api_key
Key '/myapp/api_key' already exists.
key already exists. Overwrite? (y/n): y
Enter secret value: [secret not shown]
Successfully set key: /myapp/api_key
```


### List Command

The `ls` command lists all stored secret keys, optionally filtered by path.

```bash
crumb ls [path] [--long]
```


#### Example Usage

```bash
# List all secrets
$ crumb ls
/myapp/dev/api_key

# Filter by path prefix
$ crumb ls /myapp
/myapp/api_key
/myapp/secret

# Show metadata (updated, expires) in table format
$ crumb ls -l
KEY                    UPDATED               EXPIRES
/myapp/api_key         2026-05-01T10:30:00Z  (none)
/myapp/secret          2026-05-01T10:30:00Z  2026-12-31T00:00:00Z
```


### Get Command

The `get` command retrieves a secret by its key path.

```bash
crumb get <key-path> [--mask] [--export] [--shell=bash|fish] [-i]
```


#### Example Usage

```bash
# Interactively pick a secret with fuzzy finder
$ crumb get -i

# Get a secret
$ crumb get /myapp/api_key
secret123

# Get a secret masked
$ crumb get /myapp/api_key --mask
****

# Export a secret for bash sourcing
$ crumb get /myapp/api_key --export
export API_KEY=secret123

# Export a secret for fish shell
$ crumb get /myapp/api_key --export --shell fish
set -x API_KEY secret123

# Source the export directly in bash
$ eval "$(crumb get /myapp/api_key --export)"
$ echo $API_KEY
secret123
```

#### Variable Name Conversion

When using the `--export` flag, the key path is automatically converted to a valid environment variable name:

- Leading slash (`/`) is removed
- Remaining slashes (`/`) are converted to underscores (`_`)
- Hyphens (`-`) are converted to underscores (`_`)
- The result is converted to uppercase

Examples:
- `/myapp/my-service/auth-token` → `AUTH_TOKEN`


#### Shell Integration

The `--export` flag makes it easy to integrate secrets into shell scripts and workflows:

**Bash:**
```bash
# Source a single secret
eval "$(crumb get /myapp/key --export)"
echo "Key: $KEY"

# Source multiple secrets in a script
#!/bin/bash
eval "$(crumb get /myapp/  --export)"
```

**Fish:**
```fish
# Source a single secret
eval (crumb get /myapp/key --export --shell fish)

# Source multiple secrets
eval (crumb get /myapp/ --export --shell fish)
```

### Init Command

The `init` command creates a YAML configuration file in the current project directory.

```bash
crumb init
```

#### Example Usage

```bash
# Create a new .crumb.yaml file
$ crumb init
Successfully created .crumb.yaml

#### Default Configuration Structure

The created `.crumb.yaml` file contains:

```yaml
version: "1.0"
environments:
  default:
    path: ""
    remap: {}
    env: {}
```

This structure allows you to configure multiple environments, each with:
- `path`: A path to sync secrets from (e.g., `/myapp/api-key`)
- `remap`: Key remapping for environment variables
- `env`: Individual environment variable configurations

You can add additional environments for different deployment contexts:

```yaml
version: "1.0"
environments:
  default:
    path: "/myapp/dev/"
    remap: {}
    env: {}
  production:
    path: "/myapp/prod/"
    remap: {}
    env: {}
```

```bash
export MY_KEY=******
```


### Delete Command

The `delete` command deletes a secret key-value pair from the encrypted file.

```bash
crumb delete <key-path>
```

#### Example Usage

```bash
# Delete a secret (with confirmation)
$ crumb delete /myapp/dev/api_key
Type the key path to confirm deletion: /myapp/dev/api_key
Successfully deleted key: /myapp/dev/api_key
```

### Move Command

The `move` (or `mv`) command renames a secret key to a new path, preserving its value. This is useful for reorganizing or refactoring your secret key structure without losing data.

```bash
crumb move <old-key-path> <new-key-path>
# or using the alias
crumb mv <old-key-path> <new-key-path>
```


### Info Command

The `info` command shows metadata for a secret without revealing its value.

```bash
crumb info <key-path> [-i]
```

#### Example Usage

```bash
# Interactively pick a secret to inspect
$ crumb info -i

$ crumb info /myapp/api_key
Key:     /myapp/api_key
Updated: 2026-05-01T10:30:00Z
Expires: (none)
```

### Migrate Command

The `migrate` command converts secrets from the legacy `key=value` format to the new TOML-based storage format. A backup of the encrypted file is created before migration.

```bash
crumb migrate [--profile <profile-name>]
```

#### Example Usage

```bash
$ crumb migrate
Backed up to /Users/username/.config/crumb/secrets.bak
Migrated 12 secrets to TOML format.
```

### Import Command

The `import` command allows you to import multiple secrets from a `.env` file into your encrypted storage. This is particularly useful when migrating from `.env` files to Crumb or when setting up a new project with existing environment variables.

```bash
crumb import --file <path-to-env-file> --path <destination-path>
```

#### .env File Format Support

The import command supports standard `.env` file formats:

```bash
# Comments are ignored
API_KEY=secret123
DATABASE_URL="postgresql://localhost:5432/mydb"
DEBUG=true
```

#### Example Usage

**Basic import:**
```bash
# Import to /myapp/dev path
$ crumb import --file .env --path /myapp/dev/

# Verify the imported secrets
$ crumb ls /myapp/dev/
```

**Using with different profiles:**
```bash
# Import to work profile
$ crumb --profile work import --file work.env --path /work/secrets/
```

### Storage Management Commands

The `storage` command provides subcommands to manage storage file paths for profiles.

#### Storage Set

Set the storage file path for the current profile:

```bash
crumb storage set <path> [--profile <profile-name>]
```

Example:
```bash
# Set storage path for work profile
$ crumb --profile work storage set ~/.config/crumb/work-secrets

# Set storage path for default profile
$ crumb storage set ~/personal-secrets
```

#### Storage Get

Show the current storage file path for the current profile:

```bash
crumb storage get [--profile <profile-name>]
```

Example:
```bash
# Check storage path for work profile
$ crumb --profile work storage get
Storage: /Users/username/.config/crumb/work-secrets (profile: work)

# Check storage path for default profile
$ crumb storage get
Storage: /Users/username/.config/crumb/secrets (profile: default)

```

#### Storage Clear

Clear the storage file path for the current profile (reverts to default):

```bash
crumb storage clear [--profile <profile-name>]
```

Example:
```bash
# Clear custom storage path for work profile
$ crumb --profile work storage clear
Storage path cleared for profile: work (using default)
```

## Profile Management

### Multiple Profiles

You can maintain separate secret stores for different contexts (work, personal, projects):

```bash
# Set up different profiles
crumb --profile work setup

# Add secrets to different profiles
crumb --profile work set /myapp/key
# Enter "work-secret" when prompted

# List secrets by profile
crumb --profile work ls

# Set default profile via environment variable
export CRUMB_PROFILE=work
crumb ls  # Lists work profile secrets
```

### Export Command

The `export` command exports secrets as shell-compatible environment variable assignments. It supports two modes:

1. **Config-based export**: Uses a `.crumb.yaml` configuration file (traditional mode)
2. **Direct path export**: Exports all secrets from a specific path without requiring a config file (new!)

```bash
# Config-based export
crumb export [-f config-file] [--env environment] [--shell=bash|fish] [--profile <profile-name>]

# Direct path export
crumb export --path <secret-path> [--shell=bash|fish] [--profile <profile-name>]
```

#### Example Usage

First, create a `.crumb.yaml` configuration file:

```yaml
version: "1.0"
environments:
  default:
    path: "/myapp/dev/"
    remap: {}
    env: {}
```

Then export the secrets:

```bash
# Export default environment
$ crumb export

# Export staging environment
$ crumb export --env staging

# Export for fish shell
$ crumb export --shell fish

# Use a custom config file
$ crumb export -f my-project.yaml

# Use custom config file
$ crumb export --file my-project.yaml --shell fish

# Export from work profile
$ crumb export --profile work

# Use environment variable for profile
$ CRUMB_PROFILE=work crumb export --shell=fish

# Source the output directly
$ eval "$(crumb export)"

#### Direct Path Export Examples

The `--path` flag allows you to export secrets directly without a `.crumb.yaml` file:

```bash
# Export all secrets from /api path
$ crumb export --path /myapp/dev/

# Export with fish shell format
$ crumb export --path /myapp/dev/ --shell fish

# Use with different profile
$ crumb export --path /myapp/dev/ --profile work

# Source directly into shell
$ eval "$(crumb export --path /myapp/)"
```

**Path to Variable Name Conversion**:
- Only the final segment (actual secret name) is used, intermediate path segments are ignored
- Hyphens in the secret name are converted to underscores, and the result is uppercase
mgsecret


#### Remapping Keys

You can change what finally gets exported to shell by using the `remap` section within an environment:

```yaml
version: "1.0"
environments:
  default:
    path: "/myapp/dev/"
    remap:
      "FROM": "TO"
```

For example:
```yaml
version: "1.0"
environments:
  default:
    path: "/myapp/dev/"
    remap:
      "SOME_SECRET_KEY": "MY_KEY"
```
will result in SOME_SECRET_KEY being exported as MY_KEY

#### Manually Setting Environment Varables

Say you want to also export a variable that isnt in your secrets file you can do so by adding it in the `env` key.

```yaml
environments:
  default:
    ...
    env:
      MESSAGE: "Hello staging"
```
If you want to load a single secret from a key you can do it like this:

```yaml
environments:
  default:
    ...
    env:
      API_KEY: "/myapp/staging/api_key"
```


### Hook Command

The `hook` command generates shell integration scripts that automatically load secrets when you enter a directory containing a `.crumb.yaml` file. This provides seamless, automatic environment variable management similar to direnv.

```bash
crumb hook <shell>
```

Supported shells:
- `bash`
- `zsh`
- `fish`

#### Setup Instructions

Add the following to your shell's configuration file:

**Bash** (`~/.bashrc` or `~/.bash_profile`):
```bash
eval "$(crumb hook --shell bash)"
```

**Zsh** (`~/.zshrc`):
```bash
eval "$(crumb hook --shell zsh)"
```

**Fish** (`~/.config/fish/config.fish`):
```fish
crumb hook --shell fish | source
```

#### How It Works

Once the hook is installed:

1. When you enter a directory containing a `.crumb.yaml` file, the hook automatically runs `crumb export`
2. The secrets defined in `.crumb.yaml` are loaded as environment variables
3. When you leave the directory, the environment variables remain (they are not automatically unloaded)

#### Example Workflow

```bash
# 1. Create a project with secrets
$ mkdir myapp && cd myapp
$ crumb init

# 2. Edit .crumb.yaml to configure your secrets
$ cat > .crumb.yaml << EOF
version: "1.0"
environments:
  default:
    path: "/myapp/dev"
    env: {}
    remap: {}
EOF

# 3. Add some secrets to crumb
$ crumb set /myapp/dev/API_KEY
Enter secret value: ********

$ crumb set /myapp/dev/DATABASE_URL
Enter secret value: ********

# 4. With the hook installed, cd into the directory
$ cd myapp
# Secrets are automatically loaded!

$ echo $API_KEY
your-api-key-value

$ echo $DATABASE_URL
your-database-url
```

#### Notes

- The hook checks for `.crumb.yaml` in the current directory only (not parent directories)
- Errors from `crumb export` are silently suppressed (redirected to `/dev/null`)
- The hook preserves the exit status of the previous command (important for bash prompt functions)
- For bash/zsh, the hook runs on each prompt display and directory change
- For fish, the hook runs on PWD changes and prompt events


## Configuration


`~/.config/crumb/config.yaml` - Stores profile configurations with SSH key paths and storage locations.

**Multi-profile structure:**
```yaml
profiles:
  default:
    public_key_path: ~/.ssh/id_ed25519.pub
    private_key_path: ~/.ssh/id_ed25519
    storage:
      local:
        path: ~/.config/crumb/secrets
  work:
    public_key_path: ~/.ssh/work.pub
    private_key_path: ~/.ssh/work
    storage:
      local:
        path: ~/.config/crumb/work-secrets
```

### Storage Files

Each profile has its own encrypted storage file:
- Default profile: `~/.config/crumb/secrets` (unless customized)
- Named profiles: Configurable per profile (e.g., `~/.config/crumb/work-secrets`)

### User Preferences

`~/.config/crumb/crumb.toml` - Optional TOML configuration file for user preferences.

**Shell configuration:**
```toml
shell = "bash". # Supported values: "bash", "fish", "zsh". Default: "bash"
mask_values = true
```

**Priority order for shell configuration:**
1. Command-line flag (e.g., `crumb hook --shell fish`)
2. TOML config file (`~/.config/crumb/crumb.toml`)
3. Default value (`bash`)

## Development

This project uses [Task](https://taskfile.dev/) for build automation. Common tasks:

- `task test` - Run all tests
- `task test-coverage` - Run tests with coverage report
- `task bench` - Run benchmark tests
- `task build` - Build the application
- `task ci` - Run CI pipeline
- `task clean` - Clean build artifacts
- `task --list` - Show all available tasks


## Contributing
If you find bugs, please open an issue first.
