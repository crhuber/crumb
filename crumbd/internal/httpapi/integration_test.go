package httpapi_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"crumbd/internal/config"
	"crumbd/internal/db"
	"crumbd/internal/httpapi"
	"crumbd/internal/store"
)

// testDevice bundles an SSH keypair with the helpers a real crumb client
// would use to authenticate against crumbd.
type testDevice struct {
	signer        ssh.Signer
	publicKeyLine string
	fingerprint   string
}

func newTestDevice(t *testing.T) testDevice {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("failed to build signer: %v", err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("failed to build public key: %v", err)
	}
	return testDevice{
		signer:        signer,
		publicKeyLine: string(ssh.MarshalAuthorizedKey(sshPub)),
		fingerprint:   ssh.FingerprintSHA256(sshPub),
	}
}

// sign replicates exactly what a real crumb client must do to answer a
// challenge: sign the domain-separated payload and base64-encode the
// marshaled ssh.Signature.
func (d testDevice) sign(vaultID, nonce string) (string, error) {
	payload := []byte("crumbd-auth-v1|" + vaultID + "|" + nonce)
	sig, err := d.signer.Sign(rand.Reader, payload)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ssh.Marshal(sig)), nil
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.Migrate(sqlDB); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}

	cfg := config.Default()
	cfg.ChallengeTTL = 30 * time.Second

	st := store.New(sqlDB)
	handler := httpapi.NewRouter(st, cfg)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func doJSON(t *testing.T, method, url, token string, body, out any) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("failed to encode request body: %v", err)
		}
	}
	req, err := http.NewRequest(method, url, &buf)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if out != nil {
		defer resp.Body.Close()
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
	}
	return resp
}

// login runs the full SSH-signature challenge/response handshake for a
// device and returns its session token.
func login(t *testing.T, baseURL, vaultID string, d testDevice) string {
	t.Helper()

	var challengeResp struct {
		ChallengeID string `json:"challenge_id"`
		Nonce       string `json:"nonce"`
	}
	resp := doJSON(t, "POST", baseURL+"/api/v0/auth/challenge", "", map[string]string{
		"vault_id":    vaultID,
		"fingerprint": d.fingerprint,
	}, &challengeResp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("challenge request failed: status %d", resp.StatusCode)
	}

	sig, err := d.sign(vaultID, challengeResp.Nonce)
	if err != nil {
		t.Fatalf("failed to sign challenge: %v", err)
	}

	var verifyResp struct {
		SessionToken string `json:"session_token"`
	}
	resp = doJSON(t, "POST", baseURL+"/api/v0/auth/verify", "", map[string]string{
		"challenge_id": challengeResp.ChallengeID,
		"signature":    sig,
	}, &verifyResp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("verify request failed: status %d", resp.StatusCode)
	}
	if verifyResp.SessionToken == "" {
		t.Fatal("expected a session token")
	}
	return verifyResp.SessionToken
}

func TestFullSyncFlow(t *testing.T) {
	srv := newTestServer(t)
	deviceA := newTestDevice(t)
	deviceB := newTestDevice(t)

	// 1. Create vault + owner device A.
	var createResp struct {
		VaultID  string `json:"vault_id"`
		DeviceID string `json:"device_id"`
	}
	resp := doJSON(t, "POST", srv.URL+"/api/v0/vaults", "", map[string]string{
		"name":       "test-vault",
		"public_key": deviceA.publicKeyLine,
		"label":      "device-a",
	}, &createResp)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create vault failed: status %d", resp.StatusCode)
	}
	vaultID := createResp.VaultID

	// 2. Device A logs in.
	tokenA := login(t, srv.URL, vaultID, deviceA)

	// 3. Device A pushes an initial blob at version 0.
	var putResp struct{ Version int }
	resp = doJSON(t, "PUT", srv.URL+"/api/v0/vaults/"+vaultID+"/blob", tokenA, map[string]any{
		"expected_version": 0,
		"blob":             base64.StdEncoding.EncodeToString([]byte("ciphertext-v1")),
	}, &putResp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("initial push failed: status %d", resp.StatusCode)
	}
	if putResp.Version != 1 {
		t.Fatalf("expected version 1 after first push, got %d", putResp.Version)
	}

	// 4. Device A mints an invite for device B.
	var inviteResp struct {
		InviteCode string `json:"code"`
	}
	resp = doJSON(t, "POST", srv.URL+"/api/v0/vaults/"+vaultID+"/invites", tokenA, map[string]int{
		"max_uses": 1,
	}, &inviteResp)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create invite failed: status %d", resp.StatusCode)
	}

	// 5. Device B joins using the invite code and is immediately approved.
	var joinResp struct {
		DeviceID string `json:"device_id"`
		Status   string `json:"status"`
	}
	resp = doJSON(t, "POST", srv.URL+"/api/v0/vaults/"+vaultID+"/devices", "", map[string]string{
		"public_key":  deviceB.publicKeyLine,
		"label":       "device-b",
		"invite_code": inviteResp.InviteCode,
	}, &joinResp)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("join vault failed: status %d", resp.StatusCode)
	}
	if joinResp.Status != "approved" {
		t.Fatalf("expected device B to be auto-approved via invite, got status %q", joinResp.Status)
	}

	// 6. Device B logs in and pulls the current blob.
	tokenB := login(t, srv.URL, vaultID, deviceB)

	var getResp struct {
		Version int
		Blob    string
	}
	resp = doJSON(t, "GET", srv.URL+"/api/v0/vaults/"+vaultID+"/blob", tokenB, nil, &getResp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get blob failed: status %d", resp.StatusCode)
	}
	if getResp.Version != 1 {
		t.Fatalf("expected version 1, got %d", getResp.Version)
	}
	decoded, _ := base64.StdEncoding.DecodeString(getResp.Blob)
	if string(decoded) != "ciphertext-v1" {
		t.Fatalf("unexpected blob contents: %q", decoded)
	}

	// 7. Device B pushes based on version 1 -> succeeds, becomes version 2.
	resp = doJSON(t, "PUT", srv.URL+"/api/v0/vaults/"+vaultID+"/blob", tokenB, map[string]any{
		"expected_version": 1,
		"blob":             base64.StdEncoding.EncodeToString([]byte("ciphertext-v2-from-b")),
	}, &putResp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("device B push failed: status %d", resp.StatusCode)
	}
	if putResp.Version != 2 {
		t.Fatalf("expected version 2, got %d", putResp.Version)
	}

	// 8. Device A, still thinking the version is 1, pushes and must get a 409
	// with the fresher version/blob so it can merge and retry.
	var conflictResp struct {
		Version int
		Blob    string
	}
	resp = doJSON(t, "PUT", srv.URL+"/api/v0/vaults/"+vaultID+"/blob", tokenA, map[string]any{
		"expected_version": 1,
		"blob":             base64.StdEncoding.EncodeToString([]byte("ciphertext-v2-from-a")),
	}, &conflictResp)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 conflict, got status %d", resp.StatusCode)
	}
	if conflictResp.Version != 2 {
		t.Fatalf("expected conflict body to carry version 2, got %d", conflictResp.Version)
	}
	decoded, _ = base64.StdEncoding.DecodeString(conflictResp.Blob)
	if string(decoded) != "ciphertext-v2-from-b" {
		t.Fatalf("expected conflict body to carry device B's blob, got %q", decoded)
	}

	// 9. Device A retries against the fresh version and succeeds.
	resp = doJSON(t, "PUT", srv.URL+"/api/v0/vaults/"+vaultID+"/blob", tokenA, map[string]any{
		"expected_version": 2,
		"blob":             base64.StdEncoding.EncodeToString([]byte("merged-ciphertext")),
	}, &putResp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("device A retry push failed: status %d", resp.StatusCode)
	}
	if putResp.Version != 3 {
		t.Fatalf("expected version 3, got %d", putResp.Version)
	}

	// 10. Revoking device B invalidates its session immediately.
	resp = doJSON(t, "POST", srv.URL+"/api/v0/vaults/"+vaultID+"/devices/"+joinResp.DeviceID+"/revoke", tokenA, nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("revoke failed: status %d", resp.StatusCode)
	}
	resp = doJSON(t, "GET", srv.URL+"/api/v0/vaults/"+vaultID+"/blob", tokenB, nil, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected revoked device's session to be rejected, got status %d", resp.StatusCode)
	}
}

func TestWrongKeyCannotAuthenticate(t *testing.T) {
	srv := newTestServer(t)
	owner := newTestDevice(t)
	attacker := newTestDevice(t)

	var createResp struct {
		VaultID string `json:"vault_id"`
	}
	resp := doJSON(t, "POST", srv.URL+"/api/v0/vaults", "", map[string]string{
		"name":       "test-vault",
		"public_key": owner.publicKeyLine,
	}, &createResp)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create vault failed: status %d", resp.StatusCode)
	}

	var challengeResp struct {
		ChallengeID string `json:"challenge_id"`
		Nonce       string `json:"nonce"`
	}
	resp = doJSON(t, "POST", srv.URL+"/api/v0/auth/challenge", "", map[string]string{
		"vault_id":    createResp.VaultID,
		"fingerprint": owner.fingerprint,
	}, &challengeResp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("challenge failed: status %d", resp.StatusCode)
	}

	// The attacker signs the owner's challenge with their own key.
	sig, err := attacker.sign(createResp.VaultID, challengeResp.Nonce)
	if err != nil {
		t.Fatalf("failed to sign: %v", err)
	}

	resp = doJSON(t, "POST", srv.URL+"/api/v0/auth/verify", "", map[string]string{
		"challenge_id": challengeResp.ChallengeID,
		"signature":    sig,
	}, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected wrong-key signature to be rejected, got status %d", resp.StatusCode)
	}
}

// Because crumb's age encryption uses a single recipient per profile, every
// machine synced to a vault necessarily shares the SAME SSH keypair. A
// second machine "joining" with an already-registered fingerprint is the
// expected common case and must succeed idempotently, not collide with the
// vault's UNIQUE(vault_id, fingerprint) constraint.
func TestJoinWithSameFingerprintIsIdempotent(t *testing.T) {
	srv := newTestServer(t)
	owner := newTestDevice(t)

	var createResp struct {
		VaultID  string `json:"vault_id"`
		DeviceID string `json:"device_id"`
	}
	resp := doJSON(t, "POST", srv.URL+"/api/v0/vaults", "", map[string]string{
		"name":       "test-vault",
		"public_key": owner.publicKeyLine,
		"label":      "machine-a",
	}, &createResp)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create vault failed: status %d", resp.StatusCode)
	}

	// "Machine B" uses the very same keypair (as it must, to decrypt the
	// shared blob) and re-registers against the same vault with no invite.
	var rejoinResp struct {
		DeviceID string `json:"device_id"`
		Status   string `json:"status"`
	}
	resp = doJSON(t, "POST", srv.URL+"/api/v0/vaults/"+createResp.VaultID+"/devices", "", map[string]string{
		"public_key": owner.publicKeyLine,
		"label":      "machine-b",
	}, &rejoinResp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected idempotent re-registration to succeed with 200, got status %d", resp.StatusCode)
	}
	if rejoinResp.DeviceID != createResp.DeviceID {
		t.Fatalf("expected the same device id back, got %q want %q", rejoinResp.DeviceID, createResp.DeviceID)
	}
	if rejoinResp.Status != "approved" {
		t.Fatalf("expected the existing approved status to be returned, got %q", rejoinResp.Status)
	}

	// The device can still log in afterwards.
	token := login(t, srv.URL, createResp.VaultID, owner)
	if token == "" {
		t.Fatal("expected a session token")
	}
}

// A revoked key must not be able to silently re-register itself.
func TestJoinWithRevokedFingerprintIsRejected(t *testing.T) {
	srv := newTestServer(t)
	owner := newTestDevice(t)

	var createResp struct {
		VaultID  string `json:"vault_id"`
		DeviceID string `json:"device_id"`
	}
	resp := doJSON(t, "POST", srv.URL+"/api/v0/vaults", "", map[string]string{
		"name":       "test-vault",
		"public_key": owner.publicKeyLine,
	}, &createResp)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create vault failed: status %d", resp.StatusCode)
	}

	tokenOwner := login(t, srv.URL, createResp.VaultID, owner)
	resp = doJSON(t, "POST", srv.URL+"/api/v0/vaults/"+createResp.VaultID+"/devices/"+createResp.DeviceID+"/revoke", tokenOwner, nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("revoke failed: status %d", resp.StatusCode)
	}

	resp = doJSON(t, "POST", srv.URL+"/api/v0/vaults/"+createResp.VaultID+"/devices", "", map[string]string{
		"public_key": owner.publicKeyLine,
	}, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected a revoked key to be rejected on re-registration, got status %d", resp.StatusCode)
	}
}
