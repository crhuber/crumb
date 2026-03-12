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

but, designed for the non-enterprise developer without access to a cloud Secrets Manager, raft storage, etc. Crumb was born out of the need to be able to switch secrets between different projects, without leaving project secrets unencrypted on disk.

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
```

## Features

- **SSH Key Encryption/Decryption**: Securely encrypt your password storage file using SSH Keys
- **S3 Storage Backend**: Store encrypted secrets in AWS S3 or S3-compatible services (MinIO, LocalStack)
- **Bulk Export**: Exports multiple secrets from an entire path like `/myapp/dev/`
- **Multi-Profile Support**: Manage separate secret stores for work, personal, or different projects
- **.env Import**: Import multiple secrets from `.env` files
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
crumb set <key-path>
```


#### Example Usage

```bash
# Add a new secret
$ crumb set /myapp/api_key
Enter secret value: [secret not shown]
Successfully set key: /myapp/api_key

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
crumb ls [path]
```


#### Example Usage

```bash
# List all secrets
$ crumb ls
/any/other/mykey
/any/path/mykey

# Filter by path prefix
$ crumb ls /myapp
/myapp/api_key
/myapp/auth-svc/secret
```


### Get Command

The `get` command retrieves a secret by its key path.

```bash
crumb get <key-path> [--show] [--export] [--shell=bash|fish]
```


#### Example Usage

```bash
# Get a secret
$ crumb get /myapp/api_key
secret123

# Get a secret masked
$ crumb get /myapp/api_key --mask
***

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
echo "Key: $KEY"

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
    path: "/myapp/dev"
    remap: {}
    env:
      FOO: "bar"
  production:
    path: "/myapp/prod"
    remap: {}
    env:
      FOO: "baz"
```

#### Remapping Keys

You can change what finally gets exported to shell by using the `remap` section within an environment:

```yaml
version: "1.0"
environments:
  default:
    path: "/myapp/dev"
    remap:
      "FROM": "TO"
    env: {}
```

For example:
```yaml
version: "1.0"
environments:
  default:
    path: "/myapp/dev"
    remap:
      "SOME_SECRET_KEY": "MY_KEY"
    env: {}
```
will result in SOME_SECRET_KEY being exported as MY_KEY

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
# Create a .env file
$ cat > .env << EOF
API_KEY=secret123
DATABASE_URL="postgresql://localhost:5432/mydb"
REDIS_URL=redis://localhost:6379
DEBUG=true
EOF

# Import to /myapp/dev path
$ crumb import --file .env --path /myapp/dev
Found 4 environment variables in .env
New keys to import: 4
Successfully imported 4 secrets from .env to /myapp/dev

# Verify the imported secrets
$ crumb ls /myapp/dev
/myapp/dev/API_KEY
/myapp/dev/DATABASE_URL
/myapp/dev/DEBUG
/myapp/dev/REDIS_URL
```

**Using with different profiles:**
```bash
# Import to work profile
$ crumb --profile work import --file work.env --path /work/secrets
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
Storage path set to: /Users/username/.config/crumb/work-secrets (profile: work)

# Set storage path for default profile
$ crumb storage set ~/personal-secrets
Storage path set to: /Users/username/personal-secrets (profile: default)
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
    path: "/myapp/dev"
    remap:
      DEFAULT_SECRET: "SECRET"
    env:
      MESSAGE: "Hello default"
  staging:
    path: "/myapp/staging"
    remap:
      STAGING_SECRET: "SECRET"
    env:
      MESSAGE: "Hello staging"
```

Then export the secrets:

```bash
# Export default environment for bash (default)
$ crumb export
# Exported from /myapp/dev (environment: default)
export API_SECRET=secret123
export MESSAGE=Hello default
export SECRET=somesecret

# Export staging environment
$ crumb export --env staging

# Export for fish shell
$ crumb export --shell fish
# Exported from /myapp/api-key (environment: default)
set -x API_SECRET secret123
set -x DATABASE_URL postgres://user:pass@localhost/db
set -x MG_KEY mgsecret
set -x STRIPE_KEY stripesecret

# Use a custom config file
$ crumb export -f my-project.yaml
# Exported from /myapp/my-project
export MY_SECRET=value123

# Use custom config file
$ crumb export --file my-project.yaml --shell fish
# Exported from /myapp/my-project
set -x MY_SECRET value123

# Export from work profile
$ crumb export --profile work
# Exported from /myapp/api-key
export WORK_API_KEY=work-secret
export WORK_DB_URL=postgres://work-db

# Use environment variable for profile
$ CRUMB_PROFILE=work crumb export --shell=fish
# Exported from /myapp/api-key
set -x WORK_API_KEY work-secret
set -x WORK_DB_URL postgres://work-db

# Source the output directly
$ source <(crumb export)
$ echo $MG_KEY
mgsecret

#### Direct Path Export Examples

The `--path` flag allows you to export secrets directly without a `.crumb.yaml` file:

```bash
# Export all secrets from /api path
$ crumb export --path /api
# Exported from /api
export CLIENT=foo
export CLIENT_SECRET=bar

# Export with fish shell format
$ crumb export --path /api --shell fish
# Exported from /api
set -x CLIENT foo
set -x CLIENT_SECRET bar

# Use with different profile
$ crumb export --path /myapp/api --profile myappuction
# Exported from /myapp/api
export API_KEY=secret123
export SERVICE_TOKEN=token456

# Source directly into shell
$ eval "$(crumb export --path /api)"
```

**Path to Variable Name Conversion**:
- Only the final segment (actual secret name) is used, intermediate path segments are ignored
- Hyphens in the secret name are converted to underscores, and the result is uppercase
mgsecret


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
$ mkdir myproject && cd myproject
$ crumb init

# 2. Edit .crumb.yaml to configure your secrets
$ cat > .crumb.yaml << EOF
version: "1.0"
environments:
  default:
    path: "/myproject/dev"
    env: {}
    remap: {}
EOF

# 3. Add some secrets to crumb
$ crumb set /myproject/dev/API_KEY
Enter secret value: ********

$ crumb set /myproject/dev/DATABASE_URL
Enter secret value: ********

# 4. With the hook installed, cd into the directory
$ cd myproject
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

### User Preferences (Optional)

`~/.config/crumb/crumb.toml` - Optional TOML configuration file for user preferences.

**Shell configuration:**
```toml
# Specifies the default shell format for get, export, and hook commands
# Supported values: "bash", "fish", "zsh"
# Default: "bash"
shell = "bash"
```

This allows you to set a default shell format without specifying `--shell` on every command.

**Priority order for shell configuration:**
1. Command-line flag (e.g., `crumb hook --shell fish`)
2. TOML config file (`~/.config/crumb/crumb.toml`)
3. Default value (`bash`)

**Show values configuration:**
```toml
# Display actual secret values by default in get command
# When true, behaves as if --show flag is always set
# Default: false
show_values = true
```

This allows you to configure whether the `get` command shows actual secret values or masks them by default.

**Priority order for show values:**
1. Command-line flag (e.g., `crumb get /key --show`)
2. TOML config file (`~/.config/crumb/crumb.toml`)
3. Default value (masked - `****`)

**Example configuration file:**
```toml
# ~/.config/crumb/crumb.toml
shell = "fish"
show_values = true
```

**Example usage:**

```bash
# Without TOML config - must specify shell flag
$ crumb hook --shell fish

# With TOML config (shell = "fish")
$ crumb hook
# Automatically uses fish shell from config

# CLI flag still overrides TOML config
$ crumb hook --shell bash
# Uses bash despite TOML config

# With show_values = true in TOML config
$ crumb get /myapp/key
my_secret_value
# Shows actual value without needing --show flag

# With show_values = false (or not set) in TOML config
$ crumb get /myapp/key
my_secret_value
# Masks the value

# CLI flag overrides TOML config
$ crumb get /myapp/key --mask
****
# Always shows value when --show flag is used
```


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
