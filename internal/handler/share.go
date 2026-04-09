// SPDX-License-Identifier: AGPL-3.0-or-later

// Package handler — share.go implements the capability-URL sharing API.
//
// Architecture (DECISION-055 / DECISION-056):
//
//   - The client encrypts the payload (a JSON bundle of one or more
//     Vault entries — typically a single Person + addresses) with AES-GCM
//     using a random 256-bit key generated in the browser.
//   - The server stores ONLY the ciphertext, the opaque id, and
//     expires_at. It never sees the key.
//   - The URL returned to the user contains the key in the fragment
//     (`#k=<base64url>`). Browsers do not transmit fragments, so the
//     server never learns the key even through access logs.
//   - Expiration is enforced by the server (410 Gone). Physical rows
//     persist for 30 days past expiration (manual cleanup for MVP).
//
// Security caveats (surfaced to the user in web/share.html, not enforced
// by this handler): share URLs leak via browser history, bookmarks,
// screenshots, and any system that logs full URLs. The UI must warn
// recipients before decryption.
package handler

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/lodester-oap/lodester/internal/middleware"
	"github.com/lodester-oap/lodester/internal/store"
)

// maxShareCiphertextSize bounds the stored ciphertext to 64 KB
// (DECISION-056). A single Person bundle is typically <2 KB; the cap
// exists to prevent abuse of the share endpoint as general-purpose
// untrusted blob storage.
const maxShareCiphertextSize = 64 << 10

// shareIDBytes — 16 random bytes → 22-char base64url id.
// Short enough for a QR code, long enough (128 bits) that brute-force
// enumeration is infeasible.
const shareIDBytes = 16

// defaultShareTTL is used when the client omits expires_in_seconds.
// DECISION-056: 7 days.
const defaultShareTTL = 7 * 24 * time.Hour

// maxShareTTL caps user-requested TTLs. Clients may pass 0 to mean
// "no expiration", but in practice we still bound at 1 year to keep
// garbage collection tractable.
const maxShareTTL = 365 * 24 * time.Hour

// ShareHandler handles POST/GET/DELETE /api/v1/share and
// the public GET /api/v1/share/{id} lookup.
type ShareHandler struct {
	queries *store.Queries
	logger  *slog.Logger
}

func NewShareHandler(queries *store.Queries, logger *slog.Logger) *ShareHandler {
	return &ShareHandler{queries: queries, logger: logger}
}

// createShareRequest is the body for POST /api/v1/share.
// ciphertext is the AES-GCM output (nonce || ciphertext || tag), already
// base64-encoded by the JSON layer since it is a []byte field.
type createShareRequest struct {
	Ciphertext       []byte `json:"ciphertext"`
	ExpiresInSeconds int64  `json:"expires_in_seconds"`
}

// createShareResponse returns the opaque id and the absolute expiry.
// The URL itself is assembled client-side (the client holds the key).
type createShareResponse struct {
	ID        string `json:"id"`
	ExpiresAt string `json:"expires_at"`
	CreatedAt string `json:"created_at"`
}

// shareListItem omits ciphertext — the owner listing is for audit/revoke,
// not for re-downloading the payload.
type shareListItem struct {
	ID        string `json:"id"`
	ExpiresAt string `json:"expires_at"`
	CreatedAt string `json:"created_at"`
	Expired   bool   `json:"expired"`
}

// publicShareResponse is the body for the unauthenticated GET
// /api/v1/share/{id}. It deliberately omits user_id so the server
// never correlates recipients with owners.
type publicShareResponse struct {
	Ciphertext []byte `json:"ciphertext"`
	ExpiresAt  string `json:"expires_at"`
}

// Create handles POST /api/v1/share.
func (h *ShareHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeInvalidCredentials, "not authenticated")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxShareCiphertextSize*2+1024))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeInvalidJSON, "failed to read request body")
		return
	}

	var req createShareRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, CodeInvalidJSON, "invalid request body")
		return
	}

	if len(req.Ciphertext) == 0 {
		writeError(w, http.StatusBadRequest, CodeMissingField, "ciphertext is required")
		return
	}
	if len(req.Ciphertext) > maxShareCiphertextSize {
		writeError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE",
			"ciphertext exceeds 64 KB limit")
		return
	}

	// Resolve the TTL. 0 → default, negative → reject, very large → cap.
	ttl := defaultShareTTL
	if req.ExpiresInSeconds > 0 {
		ttl = time.Duration(req.ExpiresInSeconds) * time.Second
		if ttl > maxShareTTL {
			ttl = maxShareTTL
		}
	} else if req.ExpiresInSeconds < 0 {
		writeError(w, http.StatusBadRequest, "INVALID_EXPIRY",
			"expires_in_seconds must be zero or positive")
		return
	}

	id, err := generateShareID()
	if err != nil {
		h.logger.Error("failed to generate share id", "error", err)
		writeError(w, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	expiresAt := time.Now().Add(ttl).UTC()

	row, err := h.queries.CreateShareLink(r.Context(), store.CreateShareLinkParams{
		ID:         id,
		UserID:     userID,
		Ciphertext: req.Ciphertext,
		ExpiresAt:  pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
	if err != nil {
		h.logger.Error("failed to create share link", "error", err)
		writeError(w, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	h.logger.Info("share link created",
		"user_id", uuidToString(userID),
		"share_id", row.ID,
		"expires_at", row.ExpiresAt.Time.Format(time.RFC3339),
	)

	writeJSON(w, http.StatusCreated, createShareResponse{
		ID:        row.ID,
		ExpiresAt: row.ExpiresAt.Time.UTC().Format(time.RFC3339),
		CreatedAt: row.CreatedAt.Time.UTC().Format(time.RFC3339),
	})
}

// List handles GET /api/v1/share — owner-only, for audit/revoke.
func (h *ShareHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeInvalidCredentials, "not authenticated")
		return
	}

	rows, err := h.queries.ListShareLinksByUserID(r.Context(), userID)
	if err != nil {
		h.logger.Error("failed to list share links", "error", err)
		writeError(w, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	now := time.Now().UTC()
	items := make([]shareListItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, shareListItem{
			ID:        row.ID,
			ExpiresAt: row.ExpiresAt.Time.UTC().Format(time.RFC3339),
			CreatedAt: row.CreatedAt.Time.UTC().Format(time.RFC3339),
			Expired:   row.ExpiresAt.Time.Before(now),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"share_links": items,
		"count":       len(items),
	})
}

// Get handles the unauthenticated GET /api/v1/share/{id}.
// This is the only endpoint exposed to recipients; it returns the
// ciphertext so the browser can decrypt using the fragment key.
// The handler MUST set no-store and no-referrer headers so that the
// key in the fragment is not leaked onward (DECISION-055).
func (h *ShareHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || len(id) > 64 {
		writeError(w, http.StatusBadRequest, "INVALID_SHARE_ID", "invalid share id")
		return
	}

	// Headers first so even the error branches carry them.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")

	row, err := h.queries.GetShareLinkByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "SHARE_NOT_FOUND", "share link not found")
			return
		}
		h.logger.Error("failed to load share link", "error", err)
		writeError(w, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	if row.ExpiresAt.Time.Before(time.Now()) {
		// 410 Gone per DECISION-056 — distinguishable from 404 so the
		// recipient UI can explain "this link has expired" rather than
		// "no such link".
		writeError(w, http.StatusGone, "SHARE_EXPIRED", "share link has expired")
		return
	}

	writeJSON(w, http.StatusOK, publicShareResponse{
		Ciphertext: row.Ciphertext,
		ExpiresAt:  row.ExpiresAt.Time.UTC().Format(time.RFC3339),
	})
}

// Delete handles DELETE /api/v1/share/{id}. Owner-only; returns 204 on
// success, 404 if the link does not belong to the caller.
func (h *ShareHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeInvalidCredentials, "not authenticated")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" || len(id) > 64 {
		writeError(w, http.StatusBadRequest, "INVALID_SHARE_ID", "invalid share id")
		return
	}

	// Confirm ownership before deletion so that we return 404 instead
	// of silently succeeding on someone else's link.
	row, err := h.queries.GetShareLinkByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "SHARE_NOT_FOUND", "share link not found")
			return
		}
		h.logger.Error("failed to load share link", "error", err)
		writeError(w, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}
	if !uuidEqual(row.UserID, userID) {
		writeError(w, http.StatusNotFound, "SHARE_NOT_FOUND", "share link not found")
		return
	}

	if err := h.queries.DeleteShareLink(r.Context(), store.DeleteShareLinkParams{
		ID:     id,
		UserID: userID,
	}); err != nil {
		h.logger.Error("failed to delete share link", "error", err)
		writeError(w, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	h.logger.Info("share link revoked",
		"user_id", uuidToString(userID),
		"share_id", id,
	)
	w.WriteHeader(http.StatusNoContent)
}

// generateShareID returns a URL-safe random id (base64url, no padding).
func generateShareID() (string, error) {
	buf := make([]byte, shareIDBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
