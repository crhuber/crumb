package sync

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"crumb/pkg/config"
)

// Client is what the sync algorithm needs from a crumbd server. Implemented
// by HTTPClient below; an interface so the algorithm in sync.go can be
// tested without a real server.
type Client interface {
	// Login performs the SSH-signature challenge/response handshake if no
	// still-valid cached session is available.
	Login(ctx context.Context) error
	// GetBlob fetches the vault's current version and encrypted blob.
	GetBlob(ctx context.Context) (version int, blob []byte, err error)
	// PutBlob attempts a compare-and-swap update. On a version conflict it
	// returns a *ConflictError carrying the server's actual current
	// version/blob, rather than a bare error, so the caller can merge and
	// retry without a second round trip.
	PutBlob(ctx context.Context, expectedVersion int, blob []byte) (newVersion int, err error)
}

// ConflictError is returned by PutBlob when the vault's version moved since
// the caller last fetched it.
type ConflictError struct {
	Version int
	Blob    []byte
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("sync server is at version %d, which is newer than expected", e.Version)
}

// HTTPClient implements Client against a real crumbd server, authenticating
// via the profile's existing SSH keypair.
type HTTPClient struct {
	baseURL     string
	vaultID     string
	fingerprint string
	signer      ssh.Signer
	httpClient  *http.Client

	sessionToken     string
	sessionExpiresAt time.Time

	// OnSession, if set, is called after a successful Login so the caller
	// can persist the new session token/expiry into the profile config.
	OnSession func(token string, expiresAt time.Time)
}

// NewHTTPClient builds an HTTPClient for the given profile config. It loads
// the profile's SSH private key as a signer (prompting for a passphrase if
// the key is encrypted) and seeds any still-valid cached session.
func NewHTTPClient(cfg *config.ProfileConfig) (*HTTPClient, error) {
	if cfg.Sync == nil {
		return nil, fmt.Errorf("sync is not configured for this profile; run 'crumb sync init' first")
	}

	signer, err := loadSigner(cfg.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load SSH private key for sync: %w", err)
	}

	c := &HTTPClient{
		baseURL:     strings.TrimSuffix(cfg.Sync.ServerURL, "/"),
		vaultID:     cfg.Sync.VaultID,
		fingerprint: ssh.FingerprintSHA256(signer.PublicKey()),
		signer:      signer,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
	}

	if cfg.Sync.SessionToken != "" && cfg.Sync.SessionExpiresAt != "" {
		if t, err := time.Parse(time.RFC3339, cfg.Sync.SessionExpiresAt); err == nil && time.Now().Before(t) {
			c.sessionToken = cfg.Sync.SessionToken
			c.sessionExpiresAt = t
		}
	}

	return c, nil
}

func loadSigner(privateKeyPath string) (ssh.Signer, error) {
	keyData, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}

	signer, err := ssh.ParsePrivateKey(keyData)
	if err == nil {
		return signer, nil
	}

	var passphraseErr *ssh.PassphraseMissingError
	if !errors.As(err, &passphraseErr) {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	passphrase, err := config.PromptForSecret(fmt.Sprintf("Enter passphrase for %s: ", privateKeyPath))
	if err != nil {
		return nil, err
	}
	signer, err = ssh.ParsePrivateKeyWithPassphrase(keyData, []byte(passphrase))
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key with passphrase: %w", err)
	}
	return signer, nil
}

// signPayload mirrors crumbd's internal/auth.signPayload exactly: the
// server reconstructs and verifies against this same domain-separated
// string, so it must match byte-for-byte.
func signPayload(vaultID, nonce string) []byte {
	return []byte("crumbd-auth-v1|" + vaultID + "|" + nonce)
}

// Login runs the SSH-signature challenge/response handshake, unless a
// still-valid cached session already exists.
func (c *HTTPClient) Login(ctx context.Context) error {
	if c.sessionToken != "" && time.Now().Before(c.sessionExpiresAt.Add(-30*time.Second)) {
		return nil
	}

	var challengeResp struct {
		ChallengeID string `json:"challenge_id"`
		Nonce       string `json:"nonce"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/v0/auth/challenge", false, map[string]string{
		"vault_id":    c.vaultID,
		"fingerprint": c.fingerprint,
	}, &challengeResp); err != nil {
		return fmt.Errorf("failed to request auth challenge: %w", err)
	}

	sig, err := c.signer.Sign(rand.Reader, signPayload(c.vaultID, challengeResp.Nonce))
	if err != nil {
		return fmt.Errorf("failed to sign auth challenge: %w", err)
	}
	sigB64 := base64.StdEncoding.EncodeToString(ssh.Marshal(sig))

	var verifyResp struct {
		SessionToken string `json:"session_token"`
		ExpiresAt    string `json:"expires_at"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/v0/auth/verify", false, map[string]string{
		"challenge_id": challengeResp.ChallengeID,
		"signature":    sigB64,
	}, &verifyResp); err != nil {
		return fmt.Errorf("failed to verify auth challenge: %w", err)
	}

	expiresAt, err := time.Parse(time.RFC3339, verifyResp.ExpiresAt)
	if err != nil {
		expiresAt = time.Now().Add(time.Hour)
	}
	c.sessionToken = verifyResp.SessionToken
	c.sessionExpiresAt = expiresAt

	if c.OnSession != nil {
		c.OnSession(c.sessionToken, c.sessionExpiresAt)
	}
	return nil
}

// GetBlob fetches the vault's current version and blob.
func (c *HTTPClient) GetBlob(ctx context.Context) (int, []byte, error) {
	var resp struct {
		Version int    `json:"version"`
		Blob    string `json:"blob"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v0/vaults/"+c.vaultID+"/blob", true, nil, &resp); err != nil {
		return 0, nil, err
	}
	blob, err := base64.StdEncoding.DecodeString(resp.Blob)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to decode blob: %w", err)
	}
	return resp.Version, blob, nil
}

// PutBlob attempts a compare-and-swap push, returning a *ConflictError if
// the vault's version has moved since expectedVersion.
func (c *HTTPClient) PutBlob(ctx context.Context, expectedVersion int, blob []byte) (int, error) {
	reqBody := map[string]any{
		"expected_version": expectedVersion,
		"blob":             base64.StdEncoding.EncodeToString(blob),
	}

	var resp struct {
		Version int `json:"version"`
	}
	err := c.do(ctx, http.MethodPut, "/api/v0/vaults/"+c.vaultID+"/blob", true, reqBody, &resp)
	if err == nil {
		return resp.Version, nil
	}

	var httpErr *httpStatusError
	if errors.As(err, &httpErr) && httpErr.status == http.StatusConflict {
		var conflictBody struct {
			Version int    `json:"version"`
			Blob    string `json:"blob"`
		}
		if jsonErr := json.Unmarshal(httpErr.body, &conflictBody); jsonErr == nil {
			conflictBlob, decodeErr := base64.StdEncoding.DecodeString(conflictBody.Blob)
			if decodeErr == nil {
				return 0, &ConflictError{Version: conflictBody.Version, Blob: conflictBlob}
			}
		}
	}
	return 0, err
}

// CreateInvite mints a one-time invite code for adding a new device to this
// client's vault. The plaintext code is returned exactly once.
func (c *HTTPClient) CreateInvite(ctx context.Context, maxUses int, ttl time.Duration) (code string, expiresAt time.Time, err error) {
	var resp struct {
		Code      string `json:"code"`
		ExpiresAt string `json:"expires_at"`
	}
	body := map[string]any{
		"max_uses":    maxUses,
		"ttl_seconds": int(ttl.Seconds()),
	}
	if err := c.do(ctx, http.MethodPost, "/api/v0/vaults/"+c.vaultID+"/invites", true, body, &resp); err != nil {
		return "", time.Time{}, err
	}
	expires, err := time.Parse(time.RFC3339, resp.ExpiresAt)
	if err != nil {
		expires = time.Now().Add(ttl)
	}
	return resp.Code, expires, nil
}

// httpStatusError carries a non-2xx HTTP response for callers that need to
// inspect the status/body (e.g. PutBlob's 409 handling).
type httpStatusError struct {
	status int
	body   []byte
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("sync server returned status %d", e.status)
}

// do issues a JSON request and decodes a JSON response, retrying once after
// a fresh Login if the server responds 401 (a cached session expired
// server-side sooner than our locally cached expiry).
func (c *HTTPClient) do(ctx context.Context, method, path string, authenticated bool, body, out any) error {
	err := c.doOnce(ctx, method, path, authenticated, body, out)
	if err == nil || !authenticated {
		return err
	}

	var httpErr *httpStatusError
	if errors.As(err, &httpErr) && httpErr.status == http.StatusUnauthorized {
		c.sessionToken = ""
		if loginErr := c.Login(ctx); loginErr != nil {
			return loginErr
		}
		return c.doOnce(ctx, method, path, authenticated, body, out)
	}
	return err
}

func (c *HTTPClient) doOnce(ctx context.Context, method, path string, authenticated bool, body, out any) error {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to encode request: %w", err)
		}
		reader = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if authenticated {
		req.Header.Set("Authorization", "Bearer "+c.sessionToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request to sync server failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &httpStatusError{status: resp.StatusCode, body: respBody}
	}

	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}
	return nil
}
