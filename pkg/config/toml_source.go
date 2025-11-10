package config

import (
	"github.com/urfave/cli/v3"
)

// TomlValueSource implements the ValueSource interface for reading from TOML config
type TomlValueSource struct {
	key string
}

// NewTomlValueSource creates a new TOML value source for the given key
func NewTomlValueSource(key string) cli.ValueSource {
	return &TomlValueSource{key: key}
}

// Lookup implements the ValueSource interface
func (t *TomlValueSource) Lookup() (string, bool) {
	config, err := LoadTomlConfig()
	if err != nil {
		return "", false
	}

	// For now, we only support the "shell" key
	if t.key == "shell" && config.Shell != "" {
		return config.Shell, true
	}

	return "", false
}

// String implements the Stringer interface
func (t *TomlValueSource) String() string {
	return "TomlConfig"
}

// GoString implements the GoStringer interface
func (t *TomlValueSource) GoString() string {
	return "TomlConfig"
}
