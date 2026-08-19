package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"crumbd/internal/auth"
)

type challengeRequest struct {
	VaultID     string `json:"vault_id"`
	Fingerprint string `json:"fingerprint"`
}

type challengeResponse struct {
	ChallengeID string `json:"challenge_id"`
	Nonce       string `json:"nonce"`
	ExpiresAt   string `json:"expires_at"`
}

// handleAuthChallenge issues a fresh nonce for a registered, approved device
// to sign.
func (s *Server) handleAuthChallenge(w http.ResponseWriter, r *http.Request) {
	var req challengeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	device, err := s.store.GetDeviceByFingerprint(req.VaultID, req.Fingerprint)
	if err != nil || device.Status != "approved" {
		// Deliberately generic: don't distinguish unknown device from not-yet-approved.
		writeError(w, http.StatusForbidden, "forbidden", "device is unknown or not approved")
		return
	}

	nonce, err := auth.NewNonce()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to generate challenge")
		return
	}

	expiresAt := time.Now().Add(s.cfg.ChallengeTTL)
	challengeID, err := s.store.CreateChallenge(req.VaultID, device.ID, nonce, expiresAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to create challenge")
		return
	}

	writeJSON(w, http.StatusOK, challengeResponse{
		ChallengeID: challengeID,
		Nonce:       nonce,
		ExpiresAt:   expiresAt.UTC().Format(time.RFC3339),
	})
}

type verifyRequest struct {
	ChallengeID string `json:"challenge_id"`
	Signature   string `json:"signature"`
}

type verifyResponse struct {
	SessionToken string `json:"session_token"`
	ExpiresAt    string `json:"expires_at"`
}

// handleAuthVerify checks a device's signature over a previously issued
// challenge and, if valid, mints a bearer session.
func (s *Server) handleAuthVerify(w http.ResponseWriter, r *http.Request) {
	var req verifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	challenge, err := s.store.ConsumeChallenge(req.ChallengeID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired challenge")
		return
	}

	device, err := s.store.GetDevice(challenge.DeviceID)
	if err != nil || device.Status != "approved" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "device is unknown or not approved")
		return
	}

	if err := auth.VerifyChallengeResponse(device.PublicKey, challenge.VaultID, challenge.Nonce, req.Signature); err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "signature verification failed")
		return
	}

	token, hash, err := auth.NewSessionToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to create session")
		return
	}

	expiresAt := time.Now().Add(s.cfg.SessionTTL)
	if err := s.store.CreateSession(challenge.VaultID, device.ID, hash, expiresAt); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to create session")
		return
	}

	writeJSON(w, http.StatusOK, verifyResponse{
		SessionToken: token,
		ExpiresAt:    expiresAt.UTC().Format(time.RFC3339),
	})
}
