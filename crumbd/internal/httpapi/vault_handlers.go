package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"crumbd/internal/auth"
	"crumbd/internal/config"
	"crumbd/internal/store"
)

type createVaultRequest struct {
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
	Label     string `json:"label"`
}

type createVaultResponse struct {
	VaultID  string `json:"vault_id"`
	DeviceID string `json:"device_id"`
}

// handleCreateVault creates a new vault and registers the caller's public
// key as its first (owner) device. Gated by the server's registration mode.
func (s *Server) handleCreateVault(w http.ResponseWriter, r *http.Request) {
	switch config.RegistrationMode(s.cfg.RegistrationMode) {
	case config.RegistrationClosed:
		writeError(w, http.StatusForbidden, "registration_closed", "vault registration is disabled on this server")
		return
	case config.RegistrationToken:
		presented := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(presented), []byte(s.cfg.RegistrationToken)) != 1 {
			writeError(w, http.StatusForbidden, "registration_forbidden", "invalid registration token")
			return
		}
	case config.RegistrationOpen:
		// no gate
	}

	var req createVaultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}
	if req.Name == "" || req.PublicKey == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "name and public_key are required")
		return
	}

	fingerprint, err := auth.Fingerprint(req.PublicKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid SSH public key")
		return
	}

	vaultID, err := s.store.CreateVault(req.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to create vault")
		return
	}

	deviceID, err := s.store.CreateDevice(vaultID, req.PublicKey, fingerprint, req.Label, "approved", "owner")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to register device")
		return
	}

	writeJSON(w, http.StatusCreated, createVaultResponse{VaultID: vaultID, DeviceID: deviceID})
}

type createInviteRequest struct {
	MaxUses    int `json:"max_uses"`
	TTLSeconds int `json:"ttl_seconds"`
}

type createInviteResponse struct {
	InviteCode string `json:"code"`
	ExpiresAt  string `json:"expires_at"`
}

// handleCreateInvite mints a one-time invite code for adding a new device to
// the caller's vault. The plaintext code is returned exactly once; only its
// hash is ever stored.
func (s *Server) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r.Context())
	vaultID := r.PathValue("vault_id")
	if sess.VaultID != vaultID {
		writeError(w, http.StatusForbidden, "forbidden", "session does not belong to this vault")
		return
	}

	var req createInviteRequest
	_ = json.NewDecoder(r.Body).Decode(&req) // all fields optional, defaults below
	if req.MaxUses <= 0 {
		req.MaxUses = 1
	}
	ttl := time.Duration(req.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = s.cfg.InviteTTL
	}

	code, err := auth.NewNonce() // 32 random bytes, base64 — reused as a high-entropy invite code
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to generate invite code")
		return
	}

	expiresAt := time.Now().Add(ttl)
	if _, err := s.store.CreateInvite(vaultID, auth.HashToken(code), req.MaxUses, expiresAt); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to create invite")
		return
	}

	writeJSON(w, http.StatusCreated, createInviteResponse{
		InviteCode: code,
		ExpiresAt:  expiresAt.UTC().Format(time.RFC3339),
	})
}

type joinVaultRequest struct {
	PublicKey  string `json:"public_key"`
	Label      string `json:"label"`
	InviteCode string `json:"invite_code"`
}

type joinVaultResponse struct {
	DeviceID string `json:"device_id"`
	Status   string `json:"status"`
}

// handleCreateDevice registers a new device against an existing vault:
// approved immediately with a valid invite code, otherwise left pending for
// an existing device owner to approve.
func (s *Server) handleCreateDevice(w http.ResponseWriter, r *http.Request) {
	vaultID := r.PathValue("vault_id")
	exists, err := s.store.VaultExists(vaultID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to look up vault")
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "not_found", "vault not found")
		return
	}

	var req joinVaultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}
	if req.PublicKey == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "public_key is required")
		return
	}

	fingerprint, err := auth.Fingerprint(req.PublicKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid SSH public key")
		return
	}

	// crumb's age encryption uses a single recipient per profile, so every
	// synced machine necessarily shares the same SSH keypair - a second
	// "device" joining with an already-registered fingerprint is the
	// expected common case, not an error. Re-registering it is a no-op that
	// returns the existing device rather than colliding on the vault's
	// UNIQUE(vault_id, fingerprint) constraint.
	if existing, err := s.store.GetDeviceByFingerprint(vaultID, fingerprint); err == nil {
		if existing.Status == "revoked" {
			writeError(w, http.StatusForbidden, "forbidden", "this key has been revoked from this vault")
			return
		}
		writeJSON(w, http.StatusOK, joinVaultResponse{DeviceID: existing.ID, Status: existing.Status})
		return
	}

	status := "pending"
	if req.InviteCode != "" {
		if err := s.store.ConsumeInvite(vaultID, auth.HashToken(req.InviteCode)); err != nil {
			writeError(w, http.StatusForbidden, "invalid_invite", "invite code is invalid or expired")
			return
		}
		status = "approved"
	}

	deviceID, err := s.store.CreateDevice(vaultID, req.PublicKey, fingerprint, req.Label, status, "member")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to register device")
		return
	}

	writeJSON(w, http.StatusCreated, joinVaultResponse{DeviceID: deviceID, Status: status})
}

type deviceView struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Fingerprint string `json:"fingerprint"`
	Status      string `json:"status"`
	Role        string `json:"role"`
	CreatedAt   string `json:"created_at"`
}

// handleListDevices lists every device registered against the caller's vault.
func (s *Server) handleListDevices(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r.Context())
	vaultID := r.PathValue("vault_id")
	if sess.VaultID != vaultID {
		writeError(w, http.StatusForbidden, "forbidden", "session does not belong to this vault")
		return
	}

	devices, err := s.store.ListDevices(vaultID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to list devices")
		return
	}

	views := make([]deviceView, 0, len(devices))
	for _, d := range devices {
		views = append(views, deviceView{
			ID: d.ID, Label: d.Label, Fingerprint: d.Fingerprint,
			Status: d.Status, Role: d.Role, CreatedAt: d.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": views})
}

// handleApproveDevice approves a pending device on the caller's vault.
func (s *Server) handleApproveDevice(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r.Context())
	vaultID := r.PathValue("vault_id")
	if sess.VaultID != vaultID {
		writeError(w, http.StatusForbidden, "forbidden", "session does not belong to this vault")
		return
	}

	deviceID := r.PathValue("device_id")
	if err := s.store.ApproveDevice(vaultID, deviceID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "device not found or not pending")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to approve device")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "approved"})
}

// handleRevokeDevice revokes a device's access to the caller's vault,
// invalidating its sessions immediately.
func (s *Server) handleRevokeDevice(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r.Context())
	vaultID := r.PathValue("vault_id")
	if sess.VaultID != vaultID {
		writeError(w, http.StatusForbidden, "forbidden", "session does not belong to this vault")
		return
	}

	deviceID := r.PathValue("device_id")
	if err := s.store.RevokeDevice(vaultID, deviceID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "device not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to revoke device")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "revoked"})
}
