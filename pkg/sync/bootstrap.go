package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// postJSON is a small standalone helper for the two bootstrap calls
// (CreateVault, JoinVault) that happen before any session/vault exists, so
// they can't go through HTTPClient's authenticated do/doOnce. bearerToken is
// sent as an Authorization header when non-empty (used by CreateVault to
// satisfy a crumbd server running with registration_mode: token).
func postJSON(ctx context.Context, serverURL, path, bearerToken string, body, out any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(serverURL, "/")+path, bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("request to sync server failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errBody struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(respBody, &errBody) == nil && errBody.Error.Message != "" {
			return fmt.Errorf("sync server: %s", errBody.Error.Message)
		}
		return fmt.Errorf("sync server returned status %d", resp.StatusCode)
	}

	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}
	return nil
}

// CreateVault creates a new vault on serverURL and registers publicKeyLine
// as its first (owner) device. registrationToken is only required when the
// server is running with registration_mode: token; pass "" otherwise.
func CreateVault(ctx context.Context, serverURL, publicKeyLine, name, label, registrationToken string) (vaultID, deviceID string, err error) {
	var resp struct {
		VaultID  string `json:"vault_id"`
		DeviceID string `json:"device_id"`
	}
	if err := postJSON(ctx, serverURL, "/api/v0/vaults", registrationToken, map[string]string{
		"name":       name,
		"public_key": publicKeyLine,
		"label":      label,
	}, &resp); err != nil {
		return "", "", err
	}
	return resp.VaultID, resp.DeviceID, nil
}

// JoinVault registers publicKeyLine against an existing vault, consuming
// inviteCode so the device is approved immediately.
func JoinVault(ctx context.Context, serverURL, vaultID, publicKeyLine, label, inviteCode string) (deviceID, status string, err error) {
	var resp struct {
		DeviceID string `json:"device_id"`
		Status   string `json:"status"`
	}
	if err := postJSON(ctx, serverURL, "/api/v0/vaults/"+vaultID+"/devices", "", map[string]string{
		"public_key":  publicKeyLine,
		"label":       label,
		"invite_code": inviteCode,
	}, &resp); err != nil {
		return "", "", err
	}
	return resp.DeviceID, resp.Status, nil
}

// EncodeInvite bundles a vault id and one-time invite code into a single
// token the user can copy to another machine, so `sync init --invite`
// doesn't also need a separate --vault flag.
func EncodeInvite(vaultID, code string) string {
	return vaultID + "." + code
}

// DecodeInvite splits a token produced by EncodeInvite back into its vault
// id and code.
func DecodeInvite(token string) (vaultID, code string, err error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid invite token")
	}
	return parts[0], parts[1], nil
}
