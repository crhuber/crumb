package backend

import (
	"os"
	"path/filepath"

	"crumb/pkg/config"
)

// ResolveBackend returns the appropriate Backend based on profile configuration.
func ResolveBackend(profile *config.ProfileConfig) (Backend, error) {
	if profile.Storage.S3 != nil {
		return &S3Backend{
			Bucket:      profile.Storage.S3.Bucket,
			Key:         profile.Storage.S3.Key,
			EndpointURL: profile.Storage.S3.EndpointURL,
		}, nil
	}

	// File backend: profile local path > default
	var path string
	if profile.Storage.Local != nil {
		path = profile.Storage.Local.Path
	}
	if path == "" {
		path = filepath.Join(os.Getenv("HOME"), ".config", "crumb", "secrets")
	}
	path = config.ExpandTilde(path)

	return &FileBackend{Path: path}, nil
}
