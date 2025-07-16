# Crum CLI Tool Tests

This directory contains comprehensive unit and integration tests for the crum CLI tool.

## Test Files

- **main_test.go**: Core unit tests for key functionality
  - Key path validation
  - Configuration parsing
  - Secret parsing
  - Shell output formatting
  - Environment variable name generation

- **integration_test.go**: Integration tests for CLI commands
  - Setup command integration
  - List command with filtering
  - Init command functionality
  - Delete command validation
  - Export command configuration loading
  - Error handling scenarios

## Running Tests

### All Tests
```bash
task test
```

### Unit Tests Only
```bash
task test-unit
```

### Integration Tests Only
```bash
task test-integration
```

### With Coverage
```bash
task test-coverage
```

### With Coverage (Text Output)
```bash
task test-coverage-text
```

### Benchmarks
```bash
task bench
```

### Specific Test
```bash
go test -v -run TestValidateKeyPath
```

### Using Go Test Directly
```bash
go test -v ./...
```

## Test Categories

### Unit Tests
- **TestValidateKeyPath**: Tests key path validation rules
- **TestGetFilteredKeys**: Tests secret filtering by path
- **TestLoadCrumConfig**: Tests YAML configuration loading
- **TestShellOutputFormatting**: Tests bash and fish output formats
- **TestConfigInitialization**: Tests config structure initialization
- **TestSecretParsing**: Tests secret parsing from encrypted content
- **TestEnvVarNameGeneration**: Tests environment variable name generation

### Integration Tests
- **TestSetupCommandIntegration**: Tests setup command functionality
- **TestListCommandIntegration**: Tests list command with filtering
- **TestInitCommandIntegration**: Tests init command config creation
- **TestDeleteCommandIntegration**: Tests delete command validation
- **TestExportCommandIntegration**: Tests export command config loading
- **TestGetCommandIntegration**: Tests get command validation
- **TestErrorHandling**: Tests error scenarios

### Benchmark Tests
- **BenchmarkGetFilteredKeys**: Performance test for key filtering
- **BenchmarkParseSecrets**: Performance test for secret parsing
- **BenchmarkValidateKeyPath**: Performance test for key path validation

## Test Data

Tests use temporary directories and mock SSH keys for safe testing without affecting real configuration files.
