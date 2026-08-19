package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// NewSessionToken returns a fresh, high-entropy bearer token (base64url,
// unpadded) suitable for returning to a client, plus its SHA-256 hash (hex)
// for storage — the plaintext token itself is never persisted server-side.
func NewSessionToken() (token string, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("failed to generate session token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(buf)
	return token, HashToken(token), nil
}

// HashToken returns the SHA-256 hash (hex) of a bearer token or invite code,
// as stored server-side.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
