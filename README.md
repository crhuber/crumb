# Crumb - Secret Management Tool

`crumb` is a command line tool designed to securely store, manage, and export API keys and secrets for developers. It uses an encrypted plain text file as the backend, leveraging the `age` encryption library with SSH public/private key pairs for encryption and decryption.

`crumb` is inspired by tools such as:

- [Hashicorp Vault](https://www.hashicorp.com/en/products/vault)
- [Teller](https://github.com/tellerops/teller)
- [Gopass](https://github.com/gopasspw/gopass)
- [Sops](https://github.com/getsops/sops)
- [Direnv](https://direnv.net/)
- [SecretSpec](https://devenv.sh/blog/2025/07/21/announcing-secretspec-declarative-secrets-management/)

but, designed for the non-enterprise developer without access to a cloud Secrets Manager, raft storage, etc. Crumb was born out of the need to be able to switch secrets between different projects, without leaving project secrets unencrypted on disk.

## Installation

1. Download and add binary to $PATH from https://github.com/crhuber/crumb/releases

OR

2. Use [kelp](https://github.com/crhuber/kelp)

```bash
kelp add crhuber/crumb --install
```

## Features

- **Multi-Profile Support**: Manage separate secret stores for work, personal, or different projects
- **Custom Storage Paths**: Store secrets in different locations per profile
- **Environment Variables**: Use `CRUMB_PROFILE` and `CRUMB_STORAGE` for easy integration
- **Global Flags**: Override settings with `--profile` and `--storage` flags
- **Storage Management**: Built-in commands to configure storage paths per profile
- **Bulk Import**: Import multiple secrets from `.env` files with conflict detection and overwrite protection

## Global Flags

All commands support these global flags:

- `--profile <name>`: Specify which profile to use (default: "default")
- `--storage <path>`: Override storage file path for the current command

Environment variables:
- `CRUMB_PROFILE`: Set the default profile
- `CRUMB_STORAGE`: Set the default storage path

## Usage

### Setup Command

The `setup` command initializes the secure storage backend for a specific profile.

```bash
crumb setup [--profile <profile-name>]
```

This command:
- Creates a directory at `~/.config/crumb/` if it doesn't exist
- Prompts for your SSH public key path (e.g., `~/.ssh/id_ed25519.pub`)
- Prompts for your SSH private key path (e.g., `~/.ssh/id_ed25519`)
- For non-default profiles, prompts for a custom storage file path
- Validates that the provided keys are of type `ssh-rsa` or `ssh-ed25519`
- Creates or updates `~/.config/crumb/config.yaml` with the profile configuration
- Creates an empty encrypted secrets file at the specified storage location
- Prompts for confirmation if files already exist

#### Prerequisites

Before running setup, you need to have SSH keys generated. If you don't have them, create them with:

```bash
# For Ed25519 keys (recommended)
ssh-keygen -t ed25519 -C "your_email@example.com"

# For RSA keys
ssh-keygen -t rsa -b 4096 -C "your_email@example.com"
```

#### Example Setup Sessions

**Default profile setup:**
```bash
$ crumb setup
Enter path to SSH public key (e.g., ~/.ssh/id_ed25519.pub): ~/.ssh/id_ed25519.pub
Enter path to SSH private key (e.g., ~/.ssh/id_ed25519): ~/.ssh/id_ed25519
Setup completed successfully for profile 'default'!
Config file: /Users/username/.config/crumb/config.yaml
Storage file: /Users/username/.config/crumb/secrets
```

**Work profile setup:**
```bash
$ crumb --profile work setup
Enter path to SSH public key (e.g., ~/.ssh/work.pub): ~/.ssh/work.pub
Enter path to SSH private key (e.g., ~/.ssh/work): ~/.ssh/work
Enter storage file path (e.g., ~/.config/crumb/secrets-work): ~/.config/crumb/work-secrets
Setup completed successfully for profile 'work'!
Config file: /Users/username/.config/crumb/config.yaml
Storage file: /Users/username/.config/crumb/work-secrets
```

### List Command

The `ls` command lists all stored secret keys, optionally filtered by path.

```bash
crumb ls [path]
```

This command:
- Decrypts the secrets file using the private key
- Displays all secret keys (not values) in sorted order
- Supports optional path filtering with partial matching
- Treats trailing slashes as equivalent (e.g., `/any/` vs `/any`)

#### Example Usage

```bash
# List all secrets
$ crumb ls
/any/other/mykey
/any/path/mykey
/prod/api_key
/prod/auth-svc/secret
/prod/billing-svc/api_key
/test/mykey

# Filter by path prefix
$ crumb ls /prod
/prod/api_key
/prod/auth-svc/secret
/prod/billing-svc/api_key

# Filter with partial matching
$ crumb ls /any
/any/other/mykey
/any/path/mykey

# No secrets found
$ crumb ls /nonexistent
No secrets found matching path: /nonexistent

# Empty secrets file
$ crumb ls
No secrets found
```

### Set Command

The `set` command adds or updates a secret key-value pair. The value is entered securely on a new line and is not echoed to the terminal or stored in shell history.

```bash
crumb set <key-path>
```

This command:
- Validates the key path (must start with `/`, no spaces or special characters)
- Decrypts the secrets file using the private key
- Checks if the key already exists and prompts for confirmation if it does
- Prompts you to enter the secret value securely (not echoed to terminal)
- Adds or updates the key-value pair
- Re-encrypts and saves the secrets file

#### Example Usage

```bash
# Add a new secret
$ crumb set /prod/api_key
Enter secret value: [secret not shown]
Successfully set key: /prod/api_key

# Update an existing secret (with confirmation)
$ crumb set /prod/api_key
Key '/prod/api_key' already exists.
key already exists. Overwrite? (y/n): y
Enter secret value: [secret not shown]
Successfully set key: /prod/api_key

# Invalid key path examples
$ crumb set invalid_key
Error: key path must start with '/'

$ crumb set "/test/key with spaces"
Error: key path cannot contain spaces
```

### Get Command

The `get` command retrieves a secret by its key path.

```bash
crumb get <key-path> [--show] [--export] [--shell=bash|fish]
```

This command:
- Validates the key path (must start with `/`, no spaces or special characters)
- Decrypts the secrets file using the private key
- By default, displays `****` to mask the value
- Supports `--show` flag to display the actual secret value
- Supports `--export` flag to output in shell-compatible format for sourcing
- Supports `--shell` flag to select output format when using `--export` (bash or fish, defaulting to bash)
- When `--export` is used, `--show` is ignored and the secret value is always displayed
- Requires a full key path (partial paths are not supported)

#### Example Usage

```bash
# Get a secret (masked value)
$ crumb get /prod/api_key
****

# Get a secret with actual value
$ crumb get /prod/api_key --show
secret123

# Export a secret for bash sourcing
$ crumb get /prod/api_key --export
export PROD_API_KEY=secret123

# Export a secret for fish shell
$ crumb get /prod/api_key --export --shell fish
set -x PROD_API_KEY secret123


# Export with complex key path
$ crumb get /api/my-service/auth-token --export
export API_MY_SERVICE_AUTH_TOKEN=token123

# Source the export directly in bash
$ eval "$(crumb get /prod/api_key --export)"
$ echo $PROD_API_KEY
secret123

# Key not found
$ crumb get /nonexistent/key
Key not found.

# Invalid key path
$ crumb get invalid_key
Error: key path must start with '/'

$ crumb get "/test/key with spaces"
Error: key path cannot contain spaces


#### Variable Name Conversion

When using the `--export` flag, the key path is automatically converted to a valid environment variable name:

- Leading slash (`/`) is removed
- Remaining slashes (`/`) are converted to underscores (`_`)
- Hyphens (`-`) are converted to underscores (`_`)
- The result is converted to uppercase

Examples:
- `/api/key` → `API_KEY`
- `/prod/my-service/auth-token` → `PROD_MY_SERVICE_AUTH_TOKEN`
- `/billing-svc/vars/mg` → `BILLING_SVC_VARS_MG`

#### Shell Integration

The `--export` flag makes it easy to integrate secrets into shell scripts and workflows:

**Bash:**
```bash
# Source a single secret
eval "$(crumb get /api/key --export)"
echo "API Key: $API_KEY"

# Source multiple secrets in a script
#!/bin/bash
eval "$(crumb get /prod/database-url --export)"
eval "$(crumb get /prod/api-key --export)"
eval "$(crumb get /prod/stripe-secret --export)"

# Now use the environment variables
psql "$PROD_DATABASE_URL" -c "SELECT COUNT(*) FROM users;"
curl -H "Authorization: Bearer $PROD_API_KEY" https://api.example.com/
```

**Fish:**
```fish
# Source a single secret
eval (crumb get /api/key --export --shell fish)
echo "API Key: $API_KEY"

# Source multiple secrets
eval (crumb get /prod/database-url --export --shell fish)
eval (crumb get /prod/api-key --export --shell fish)
```

### Init Command

The `init` command creates a YAML configuration file in the current directory.

```bash
crumb init
```

This command:
- Creates a `.crumb.yaml` file in the current directory
- Uses a default structure with empty configuration
- Prompts for confirmation if the file already exists
- Validates the YAML structure before writing

#### Example Usage

```bash
# Create a new .crumb.yaml file
$ crumb init
Successfully created .crumb.yaml

# File already exists (with confirmation)
$ crumb init
Config file .crumb.yaml already exists. Overwrite? (y/n): y
Successfully created .crumb.yaml

# Reject overwrite
$ crumb init
Config file .crumb.yaml already exists. Overwrite? (y/n): n
Operation cancelled.
```

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
- `path`: A path to sync secrets from (e.g., `/prod/billing-svc`)
- `remap`: Key remapping for environment variables
- `env`: Individual environment variable configurations

You can add additional environments for different deployment contexts:

```yaml
version: "1.0"
environments:
  default:
    path: "/dev/myapp"
    remap: {}
    env:
      DATABASE_URL: "/dev/myapp/db_url"
  staging:
    path: "/staging/myapp"
    remap:
      API_KEY: "STAGING_API_KEY"
    env:
      DATABASE_URL: "/staging/myapp/db_url"
  production:
    path: "/prod/myapp"
    remap: {}
    env:
      DATABASE_URL: "/prod/myapp/db_url"
      API_SECRET: "/prod/myapp/secret"
```

#### Remapping Keys

You can change what finally gets exported to shell by using the `remap` section within an environment:

```yaml
version: "1.0"
environments:
  default:
    path: "/some/path"
    remap:
      "FROM": "TO"
    env: {}
```

For example:
```yaml
version: "1.0"
environments:
  default:
    path: "/some/path"
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
$ crumb delete /prod/billing-svc/vars/mg
Type the key path to confirm deletion: /prod/billing-svc/vars/mg
Successfully deleted key: /prod/billing-svc/vars/mg

# Wrong confirmation (deletion cancelled)
$ crumb delete /prod/api_key
Type the key path to confirm deletion: /wrong/path
Confirmation failed. Deletion cancelled.

# Key not found
$ crumb delete /nonexistent/key
Key not found.

# Invalid key path
$ crumb delete invalid_key
Error: key path must start with '/'

$ crumb delete "/test/key with spaces"
Error: key path cannot contain spaces
```

### Move Command

The `move` (or `mv`) command renames a secret key to a new path, preserving its value. This is useful for reorganizing or refactoring your secret key structure without losing data.

```bash
crumb move <old-key-path> <new-key-path>
# or using the alias
crumb mv <old-key-path> <new-key-path>
```

This command:
- Validates both the old and new key paths (must start with `/`, no spaces or special characters)
- Decrypts the secrets file using the private key
- Checks that the old key exists
- Checks if the new key already exists and prompts for confirmation before overwriting
- Moves the value from the old key to the new key
- Removes the old key
- Re-encrypts and saves the secrets file
- Fails gracefully if the old key does not exist

#### Example Usage

```bash
# Move a secret key to a new path
$ crumb move /foo/bar /baz/bar
Successfully moved key from /foo/bar to /baz/bar

# Attempt to move a non-existent key
$ crumb move /does/not/exist /new/path
old key not found: /does/not/exist

# Overwrite an existing key (with confirmation)
$ crumb move /prod/api_key /test/api_key
New key path '/test/api_key' already exists with value: testsecret
key already exists. Overwrite? (y/n): y
Successfully moved key from /prod/api_key to /test/api_key

# Invalid key path
$ crumb move invalid_key /new/path
invalid old key path: key path must start with '/'
```

### Import Command

The `import` command allows you to import multiple secrets from a `.env` file into your encrypted storage. This is particularly useful when migrating from `.env` files to Crumb or when setting up a new project with existing environment variables.

```bash
crumb import --file <path-to-env-file> --path <destination-path>
```

This command:
- Parses a `.env` file to extract environment variables and their values
- Validates the destination path (must start with `/`, no spaces or special characters)
- Preserves the original case of environment variable names in storage paths
- Detects conflicts with existing secrets and prompts for confirmation
- Creates the destination path structure if it doesn't exist
- Imports all variables as individual secrets under the specified path
- Re-encrypts and saves the secrets file

#### .env File Format Support

The import command supports standard `.env` file formats:

```bash
# Comments are ignored
API_KEY=secret123
DATABASE_URL="postgresql://localhost:5432/mydb"
DEBUG=true
EMPTY_VAR=
QUOTED_VALUE='single-quoted-value'
COMPLEX_URL=https://api.example.com?token=abc123&refresh=def456
```

Supported features:
- Comments (lines starting with `#`)
- Empty lines (ignored)
- Quoted values (both single and double quotes)
- Values containing special characters and equals signs
- Empty values

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

# Import to /dev/myapp path
$ crumb import --file .env --path /dev/myapp
Found 4 environment variables in .env
New keys to import: 4
Successfully imported 4 secrets from .env to /dev/myapp

# Verify the imported secrets
$ crumb ls /dev/myapp
/dev/myapp/API_KEY
/dev/myapp/DATABASE_URL
/dev/myapp/DEBUG
/dev/myapp/REDIS_URL
```

**Using with different profiles:**
```bash
# Import to work profile
$ crumb --profile work import --file work.env --path /work/secrets
Found 6 environment variables in work.env
New keys to import: 6
Successfully imported 6 secrets from work.env to /work/secrets

# Import to custom storage location
$ crumb --storage ~/project-secrets import --file project.env --path /project/config
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

# Verify it's using default path
$ crumb --profile work storage get
Storage: /Users/username/.config/crumb/secrets (profile: work)
```

## Profile Management

### Multiple Profiles

You can maintain separate secret stores for different contexts (work, personal, projects):

```bash
# Set up different profiles
crumb --profile work setup
crumb --profile personal setup
crumb --profile project-x setup

# Add secrets to different profiles
crumb --profile work set /api/key
# Enter "work-secret" when prompted
crumb --profile personal set /github/token
# Enter "personal-token" when prompted
crumb --profile project-x set /db/password
# Enter "project-password" when prompted

# List secrets by profile
crumb --profile work ls
crumb --profile personal ls
crumb --profile project-x ls
```

### Using Environment Variables

```bash
# Set default profile via environment variable
export CRUMB_PROFILE=work
crumb ls  # Lists work profile secrets

# Override storage path temporarily
export CRUMB_STORAGE=~/temp-secrets
crumb set /temp/key "temporary value"
```

### Command-line Overrides

```bash
# Use a specific profile for one command
crumb --profile work get /api/key

# Override storage path for one command
crumb --storage ~/backup-secrets ls

# Combine profile and storage overrides
crumb --profile work --storage ~/work-backup get /api/key
```

## Practical Examples

### Migrating from .env Files

```bash
# Existing project with .env file
$ cat .env
DATABASE_URL=postgresql://localhost/myapp
API_KEY=abc123
REDIS_URL=redis://localhost:6379
SECRET_KEY=super-secret-key

# Set up Crumb for the project
crumb setup

# Import all environment variables at once
crumb import --file .env --path /myapp/config
Found 4 environment variables in .env
New keys to import: 4
Successfully imported 4 secrets from .env to /myapp/config

# Verify the import
crumb ls /myapp/config
/myapp/config/API_KEY
/myapp/config/DATABASE_URL
/myapp/config/REDIS_URL
/myapp/config/SECRET_KEY

# Create project configuration for easy exporting
crumb init
cat > .crumb.yaml << EOF
version: "1.0"
environments:
  default:
    path: "/myapp/config"
    remap:
      SECRET_KEY: "APP_SECRET_KEY"
    env: {}
EOF

# Test the export (should match original .env vars)
crumb export
# Output:
# export API_KEY=abc123
# export DATABASE_URL=postgresql://localhost/myapp
# export REDIS_URL=redis://localhost:6379
# export APP_SECRET_KEY=super-secret-key

# Now you can safely remove the .env file
# rm .env

# Use in development
eval "$(crumb export)"
echo $DATABASE_URL  # postgresql://localhost/myapp
```

### Work and Personal Separation

```bash
# Set up work profile with company SSH keys
crumb --profile work setup
# Enter work SSH key paths when prompted

# Set up personal profile with personal SSH keys
crumb --profile personal setup
# Enter personal SSH key paths when prompted

# Add work secrets
crumb --profile work set /company/api-key
# Enter work-secret-123 when prompted
crumb --profile work set /company/db-password
# Enter work-db-pass when prompted

# Add personal secrets
crumb --profile personal set /github/token
# Enter ghp_personal123 when prompted
crumb --profile personal set /aws/access-key
# Enter personal-aws-key when prompted

# List work secrets only
crumb --profile work ls

# List personal secrets only
crumb --profile personal ls

# Use environment variable for default profile
export CRUMB_PROFILE=work
crumb get /company/api-key  # Uses work profile

# Export individual secrets for sourcing
eval "$(crumb --profile work get --export /company/api-key)"
eval "$(crumb --profile work get --export /company/db-password)"
echo $COMPANY_API_KEY
echo $COMPANY_DB_PASSWORD
```

### Project-Specific Storage

```bash
# Create a project-specific storage location
crumb --profile project-alpha setup
crumb --profile project-alpha storage set ~/projects/alpha/secrets

# Add project secrets
crumb --profile project-alpha set /alpha/api-key
# Enter "alpha-secret" when prompted
crumb --profile project-alpha set /alpha/db-url
# Enter "postgres://alpha-db" when prompted

# Create project config for easy exporting
cd ~/projects/alpha
crumb init

# Edit .crumb.yaml to reference project secrets
cat > .crumb.yaml << EOF
version: "1.0"
environments:
  default:
    path: "/alpha"
    remap:
      API_KEY: "ALPHA_API_KEY"
      DB_URL: "ALPHA_DATABASE_URL"
    env: {}
EOF

# Export project environment variables
CRUMB_PROFILE=project-alpha crumb export
```

### Backup and Migration

```bash
# Create a backup of work secrets to a different location
crumb --profile work --storage ~/backups/work-secrets-backup ls
# This will show empty since backup doesn't exist yet

# Copy secrets by exporting and re-importing (manual process)
crumb --profile work ls  # Note the keys you want to backup

# Temporarily use backup storage to set up new secrets
crumb --profile work --storage ~/backups/work-secrets-backup set /api/key "$(crumb --profile work get --show /api/key)"

# Switch work profile to use backup storage permanently
crumb --profile work storage set ~/backups/work-secrets-backup
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

**Config-based mode** (when no `--path` flag is provided):
- Uses the specified profile (or default) for accessing secrets
- Reads the `.crumb.yaml` configuration file from the current directory (or a custom path with `-f`)
- Uses the specified environment (or "default" with `--env` flag)
- Validates the YAML structure and paths
- Decrypts the secrets file using the profile's private key and storage location
- Processes the `path` section to export secrets matching a path prefix
- Processes the `env` section to export individual secrets
- Applies remapping from the `remap` section
- Outputs in Bash (`export VAR=value`) or Fish (`set -x VAR value`) format
- Includes comments for clarity

**Direct path mode** (when `--path` flag is provided):
- Bypasses the need for a `.crumb.yaml` configuration file
- Exports all secrets that start with the specified path prefix
- Converts secret paths to environment variable names by removing the prefix and transforming to uppercase with underscores
- Perfect for quick exports without setting up configuration files

#### Example Usage

First, create a `.crumb.yaml` configuration file:

```yaml
version: "1.0"
environments:
  default:
    path: "/prod/billing-svc"
    remap:
      VARS_MG: "MG_KEY"
      VARS_STRIPE: "STRIPE_KEY"
    env:
      DATABASE_URL: "/prod/billing-svc/db/url"
      API_SECRET: "/prod/billing-svc/api/secret"
  staging:
    path: "/staging/billing-svc"
    remap:
      VARS_MG: "STAGING_MG_KEY"
    env:
      DATABASE_URL: "/staging/billing-svc/db/url"
```

Then export the secrets:

```bash
# Export default environment for bash (default)
$ crumb export
# Exported from /prod/billing-svc (environment: default)
export API_SECRET=secret123
export DATABASE_URL=postgres://user:pass@localhost/db
export MG_KEY=mgsecret
export STRIPE_KEY=stripesecret

# Export staging environment
$ crumb export --env staging
# Exported from /staging/billing-svc (environment: staging)
export DATABASE_URL=postgres://staging-user:pass@staging-db/db
export STAGING_MG_KEY=staging-mgsecret

# Export for fish shell
$ crumb export --shell fish
# Exported from /prod/billing-svc (environment: default)
set -x API_SECRET secret123
set -x DATABASE_URL postgres://user:pass@localhost/db
set -x MG_KEY mgsecret
set -x STRIPE_KEY stripesecret

# Use a custom config file
$ crumb export -f my-project.yaml
# Exported from /prod/my-project
export MY_SECRET=value123

# Use custom config file
$ crumb export --file my-project.yaml --shell fish
# Exported from /prod/my-project
set -x MY_SECRET value123

# Export from work profile
$ crumb export --profile work
# Exported from /prod/billing-svc
export WORK_API_KEY=work-secret
export WORK_DB_URL=postgres://work-db

# Use environment variable for profile
$ CRUMB_PROFILE=work crumb export --shell=fish
# Exported from /prod/billing-svc
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
$ crumb export --path /prod/api --profile production
# Exported from /prod/api
export API_KEY=secret123
export SERVICE_TOKEN=token456

# Source directly into shell
$ eval "$(crumb export --path /api)"
```

**Path to Variable Name Conversion**:
- Only the final segment (actual secret name) is used, intermediate path segments are ignored
- Hyphens in the secret name are converted to underscores, and the result is uppercase
mgsecret

# Source work profile secrets
$ source <(crumb --profile work export)
$ echo $WORK_API_KEY
work-secret
```

#### Local shell population

```bash
# Default profile
eval "$(crumb export)"

# Work profile
eval "$(CRUMB_PROFILE=work crumb export)"

# Specific profile and config
eval "$(crumb --profile project-x export -f project-config.yaml)"
```

## Security Features

- Secrets are encrypted at rest using the `age` encryption library
- Uses SSH public/private key pairs for encryption/decryption
- Private keys are never stored in the secrets file
- File locking prevents concurrent access issues
- Decrypted data is never written to disk or exposed in logs

## Configuration

The tool creates configuration files to manage multiple profiles and their storage locations:

### Main Configuration File

`~/.config/crumb/config.yaml` - Stores profile configurations with SSH key paths and storage locations.

**Multi-profile structure:**
```yaml
profiles:
  default:
    public_key_path: ~/.ssh/id_ed25519.pub
    private_key_path: ~/.ssh/id_ed25519
    storage: ~/.config/crumb/secrets
  work:
    public_key_path: ~/.ssh/work.pub
    private_key_path: ~/.ssh/work
    storage: ~/.config/crumb/work-secrets
  personal:
    public_key_path: ~/.ssh/personal.pub
    private_key_path: ~/.ssh/personal
    storage: ~/personal-secrets
```

### Storage Files

Each profile has its own encrypted storage file:
- Default profile: `~/.config/crumb/secrets` (unless customized)
- Named profiles: Configurable per profile (e.g., `~/.config/crumb/work-secrets`)

### Project Configuration

`.crumb.yaml` - Per-project configuration for exporting secrets to environment variables.

```yaml
version: "1.0"
environments:
  default:
    path: "/prod/my-service"
    remap:
      API_KEY: "SERVICE_API_KEY"
    env:
      DATABASE_URL: "/prod/my-service/db/url"
  staging:
    path: "/staging/my-service"
    remap:
      API_KEY: "STAGING_SERVICE_API_KEY"
    env:
      DATABASE_URL: "/staging/my-service/db/url"
```

## Migration from Legacy Configuration

If you have an existing configuration file with the old format:

```yaml
public_key_path: ~/.ssh/id_ed25519.pub
private_key_path: ~/.ssh/id_ed25519
```

You need to migrate it to the new profile-based format. Run `crumb setup` to create a new profile-based configuration, or manually update your `~/.config/crumb/config.yaml` to:

```yaml
profiles:
  default:
    public_key_path: ~/.ssh/id_ed25519.pub
    private_key_path: ~/.ssh/id_ed25519
    storage: ~/.config/crumb/secrets
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
