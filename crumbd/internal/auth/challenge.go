package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// NewNonce returns a fresh base64-encoded 32-byte random nonce.
func NewNonce() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}
