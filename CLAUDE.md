# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is **Crumb** - a Go-based CLI tool for secure secret management. It encrypts secrets using the `age` encryption library with SSH key pairs for authentication. The tool supports multi-profile configurations, allowing developers to maintain separate encrypted secret stores for different contexts (work, personal, projects).

## Core Architecture

### Main Components

- **Modular package architecture**: Code organized into focused packages for maintainability
- **CLI framework**: Uses `github.com/urfave/cli/v3` for command-line interface
- **Encryption**: Uses `filippo.io/age` and `filippo.io/age/agessh` for SSH key-based encryption
- **File locking**: Uses `golang.org/x/sys/unix` for safe concurrent access to encrypted files
- **Configuration**: YAML-based configuration using `gopkg.in/yaml.v3`

### Package Structure

- **`pkg/config`**: Configuration management, validation, and file operations
- **`pkg/crypto`**: SSH key validation, encryption/decryption, and file locking
- **`pkg/storage`**: Secret storage, parsing, filtering, and data operations
- **`pkg/commands`**: CLI command implementations and business logic
- **`main.go`**: CLI setup and command routing (~150 lines)

### Key Data Structures

- `Config`: Main configuration with profile support (`~/.config/crumb/config.yaml`)
- `ProfileConfig`: Individual profile settings (SSH keys, storage paths)
- `CrumbConfig`: Per-project configuration (`.crumb.yaml`) for export functionality

### Command Structure

All commands support global flags:
- `--profile`: Specify profile (default: "default", env: `CRUMB_PROFILE`)
- `--storage`: Override storage file path (env: `CRUMB_STORAGE`)

Commands:
- `setup`: Initialize encrypted storage for a profile
- `list/ls`: List stored secret keys with optional path filtering
- `set`: Add/update secrets with overwrite confirmation
- `get`: Retrieve secrets (with `--show`, `--export`, `--shell` flags)
- `delete/rm`: Delete secrets with confirmation prompt
- `move/mv`: Rename secret paths while preserving values
- `init`: Create `.crumb.yaml` project configuration
- `export`: Export secrets as shell environment variables
- `storage`: Manage storage file paths per profile

## Development Commands

### Building and Testing

Use Task (Taskfile.yml) for all development tasks:

```bash
# Build the application
task build

# Run all tests
task test

# Run tests with coverage
task test-coverage

# Run linting (requires golangci-lint)
task lint

# Run CI pipeline
task ci

# Setup development environment
task setup-dev
```

### Test Categories

- **Unit tests**: Run with `task test-unit` or `go test -v ./... -run "^Test"`
- **Integration tests**: Run with `task test-integration` or `go test -v ./... -run "Integration"`
- **Benchmarks**: Run with `task bench`

### Key Testing Files

- `main_test.go`: Unit tests for core functionality
- `integration_test.go`: Integration tests for CLI commands
- `TESTS.md`: Comprehensive test documentation

## Development Guidelines

### Code Organization

The codebase uses a modular package approach with clear separation of concerns:
- **Main CLI setup**: `main.go` - Command definitions and routing
- **Configuration logic**: `pkg/config/` - Profile management, validation, file operations
- **Cryptographic operations**: `pkg/crypto/` - SSH key handling, encryption, file locking
- **Storage operations**: `pkg/storage/` - Secret management, parsing, filtering
- **Command implementations**: `pkg/commands/` - Business logic for all CLI commands

### Key Patterns

1. **Configuration Loading**: Use `config.LoadConfig(profile)` to get profile configuration
2. **Storage Path Resolution**: Use `config.GetStoragePath(storageFlag, profile)` for path precedence
3. **File Locking**: All file operations use `crypto.ReadFileWithLock`/`crypto.WriteFileWithLock`
4. **Secret Format**: Secrets stored as `key=value` pairs, one per line
5. **Path Validation**: Use `config.ValidateKeyPath()` for consistent key path rules
6. **Package Dependencies**: Commands → Storage/Config → Crypto (no circular dependencies)

### Error Handling

- Return descriptive errors with context
- Use confirmation prompts for destructive operations
- Validate inputs before processing
- Handle missing files gracefully

### Security Considerations

- Never log or expose decrypted secrets
- Use file locking to prevent concurrent access corruption
- Validate SSH key formats during setup
- Store configuration files with restricted permissions (0600)

## Testing Strategy

When making changes:

1. Run unit tests first: `task test-unit`
2. Run integration tests: `task test-integration`
3. Check linting: `task lint`
4. Verify full CI pipeline: `task ci`

For specific functionality testing:
```bash
# Test key validation
go test -v -run TestValidateKeyPath

# Test secret filtering
go test -v -run TestGetFilteredKeys

# Test export functionality
go test -v -run TestExportCommandIntegration
```