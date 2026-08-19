// Package httpapi wires crumbd's HTTP routes: vault/device/invite
// management, SSH-signature auth, and the versioned-blob sync endpoints.
package httpapi

import (
	"net/http"

	"crumbd/internal/config"
	"crumbd/internal/store"
)

// Server holds the dependencies HTTP handlers need.
type Server struct {
	store *store.Store
	cfg   config.Config
}

// NewRouter builds crumbd's complete http.Handler.
func NewRouter(st *store.Store, cfg config.Config) http.Handler {
	s := &Server{store: st, cfg: cfg}

	registrationLimiter := newRateLimiter(10) // per IP, per minute
	authLimiter := newRateLimiter(10)         // per IP, per minute
	recordLimiter := newRateLimiter(60)       // per device, per minute

	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.Handle("POST /api/v0/vaults",
		registrationLimiter.middleware(clientIP)(http.HandlerFunc(s.handleCreateVault)))
	mux.Handle("POST /api/v0/vaults/{vault_id}/devices",
		registrationLimiter.middleware(clientIP)(http.HandlerFunc(s.handleCreateDevice)))

	mux.Handle("POST /api/v0/vaults/{vault_id}/invites",
		http.HandlerFunc(requireSession(st, s.handleCreateInvite)))
	mux.Handle("GET /api/v0/vaults/{vault_id}/devices",
		http.HandlerFunc(requireSession(st, s.handleListDevices)))
	mux.Handle("POST /api/v0/vaults/{vault_id}/devices/{device_id}/approve",
		http.HandlerFunc(requireSession(st, s.handleApproveDevice)))
	mux.Handle("POST /api/v0/vaults/{vault_id}/devices/{device_id}/revoke",
		http.HandlerFunc(requireSession(st, s.handleRevokeDevice)))

	mux.Handle("POST /api/v0/auth/challenge",
		registrationLimiter.middleware(clientIP)(http.HandlerFunc(s.handleAuthChallenge)))
	mux.Handle("POST /api/v0/auth/verify",
		authLimiter.middleware(clientIP)(http.HandlerFunc(s.handleAuthVerify)))

	mux.HandleFunc("GET /api/v0/vaults/{vault_id}/blob",
		requireSession(st, recordLimiter.limitFunc(sessionDeviceKey, s.handleGetBlob)))
	mux.HandleFunc("PUT /api/v0/vaults/{vault_id}/blob",
		requireSession(st, recordLimiter.limitFunc(sessionDeviceKey, s.handlePutBlob)))

	var handler http.Handler = mux
	handler = http.MaxBytesHandler(handler, s.cfg.MaxBlobSize+4096) // headroom for JSON/base64 envelope
	handler = loggingMiddleware(handler)
	handler = recoverMiddleware(handler)
	return handler
}

// sessionDeviceKey rate-limits by device once a session is already
// resolvable, falling back to IP for pre-auth requests on the same path
// pattern (defensive; in practice requireSession always runs downstream).
func sessionDeviceKey(r *http.Request) string {
	if sess := sessionFromContext(r.Context()); sess != nil {
		return sess.DeviceID
	}
	return clientIP(r)
}
