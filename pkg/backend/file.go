package backend

import (
	"os"

	"crumb/pkg/crypto"
)

// FileBackend stores encrypted data on the local filesystem.
type FileBackend struct {
	Path string
}

func (f *FileBackend) Read() ([]byte, error) {
	return crypto.ReadFileWithLock(f.Path)
}

func (f *FileBackend) Write(data []byte) error {
	return crypto.WriteFileWithLock(f.Path, data, 0600)
}

func (f *FileBackend) Exists() (bool, error) {
	_, err := os.Stat(f.Path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
