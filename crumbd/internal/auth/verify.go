// Package auth implements crumbd's SSH-signature challenge/response
// authentication: a device proves it holds a registered SSH private key by
// signing a server-issued nonce, and is issued a short-lived bearer session
// in return.
package auth

import (
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/ssh"
)

// signPayload builds the exact byte string a device must sign for a given
// challenge. The "crumbd-auth-v1|" prefix domain-separates this signature
// from any other use of the same SSH key (e.g. actual SSH login, git commit
// signing), the same rationale as OpenSSH's PROTOCOL.sshsig "namespace"
// field. The server always reconstructs this itself from data it already
// has (vaultID, nonce) — it never trusts a client-supplied payload.
func signPayload(vaultID, nonce string) []byte {
	return []byte("crumbd-auth-v1|" + vaultID + "|" + nonce)
}

// Fingerprint returns the SHA256 fingerprint (as ssh.FingerprintSHA256 would
// print it) of an authorized_keys-format public key line.
func Fingerprint(publicKeyLine string) (string, error) {
	pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(publicKeyLine))
	if err != nil {
		return "", fmt.Errorf("failed to parse public key: %w", err)
	}
	return ssh.FingerprintSHA256(pubKey), nil
}

// VerifyChallengeResponse verifies that signatureB64 (a base64-encoded,
// ssh.Marshal-ed ssh.Signature) is a valid signature over the challenge
// payload for vaultID/nonce, made by the holder of publicKeyLine's private
// key.
func VerifyChallengeResponse(publicKeyLine, vaultID, nonce, signatureB64 string) error {
	pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(publicKeyLine))
	if err != nil {
		return fmt.Errorf("failed to parse public key: %w", err)
	}

	sigBytes, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}

	var sig ssh.Signature
	if err := ssh.Unmarshal(sigBytes, &sig); err != nil {
		return fmt.Errorf("failed to unmarshal signature: %w", err)
	}

	if err := pubKey.Verify(signPayload(vaultID, nonce), &sig); err != nil {
		return fmt.Errorf("signature verification failed: %w", err)
	}
	return nil
}
