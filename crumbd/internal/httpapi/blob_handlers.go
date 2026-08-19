package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"

	"crumbd/internal/store"
)

type blobResponse struct {
	Version   int    `json:"version"`
	Blob      string `json:"blob"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// handleGetBlob returns the vault's current version and blob.
func (s *Server) handleGetBlob(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r.Context())
	vaultID := r.PathValue("vault_id")
	if sess.VaultID != vaultID {
		writeError(w, http.StatusForbidden, "forbidden", "session does not belong to this vault")
		return
	}

	version, blob, updatedAt, err := s.store.GetBlob(vaultID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "vault not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to read blob")
		return
	}

	writeJSON(w, http.StatusOK, blobResponse{
		Version:   version,
		Blob:      base64.StdEncoding.EncodeToString(blob),
		UpdatedAt: updatedAt,
	})
}

type putBlobRequest struct {
	ExpectedVersion int    `json:"expected_version"`
	Blob            string `json:"blob"`
}

type putBlobResponse struct {
	Version int `json:"version"`
}

// handlePutBlob performs a compare-and-swap update of the vault's blob: it
// succeeds only if the vault's current version still equals
// ExpectedVersion. On conflict, it responds 409 with the vault's actual
// current version/blob so the client can merge and retry without a second
// round trip.
func (s *Server) handlePutBlob(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r.Context())
	vaultID := r.PathValue("vault_id")
	if sess.VaultID != vaultID {
		writeError(w, http.StatusForbidden, "forbidden", "session does not belong to this vault")
		return
	}

	var req putBlobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	blob, err := base64.StdEncoding.DecodeString(req.Blob)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "blob is not valid base64")
		return
	}
	if int64(len(blob)) > s.cfg.MaxBlobSize {
		writeError(w, http.StatusRequestEntityTooLarge, "blob_too_large", "blob exceeds the server's maximum size")
		return
	}

	newVersion, err := s.store.CompareAndSwapBlob(vaultID, req.ExpectedVersion, blob)
	if err != nil {
		if errors.Is(err, store.ErrVersionConflict) {
			currentVersion, currentBlob, _, getErr := s.store.GetBlob(vaultID)
			if getErr != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", "failed to read current blob")
				return
			}
			writeJSON(w, http.StatusConflict, blobResponse{
				Version: currentVersion,
				Blob:    base64.StdEncoding.EncodeToString(currentBlob),
			})
			return
		}
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "vault not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to update blob")
		return
	}

	writeJSON(w, http.StatusOK, putBlobResponse{Version: newVersion})
}
