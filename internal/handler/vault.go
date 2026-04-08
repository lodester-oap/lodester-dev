// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/lodester-oap/lodester/internal/crypto"
	"github.com/lodester-oap/lodester/internal/middleware"
	"github.com/lodester-oap/lodester/internal/store"
)

// Maximum vault blob size: 1 MB.
const maxVaultSize = 1 << 20

// VaultHandler handles vault API endpoints.
type VaultHandler struct {
	queries *store.Queries
	logger  *slog.Logger
}

func NewVaultHandler(queries *store.Queries, logger *slog.Logger) *VaultHandler {
	return &VaultHandler{queries: queries, logger: logger}
}

type vaultResponse struct {
	Data    []byte `json:"data"`
	Version int32  `json:"version"`
}

type putVaultRequest struct {
	Data    []byte `json:"data"`
	Version int32  `json:"version"` // expected current version (0 for first upload)
}

// Get handles GET /api/v1/vault.
func (h *VaultHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeInvalidCredentials, "not authenticated")
		return
	}

	vault, err := h.queries.GetVaultByUserID(r.Context(), userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No vault yet — return empty with version 0
			writeJSON(w, http.StatusOK, vaultResponse{
				Data:    []byte{},
				Version: 0,
			})
			return
		}
		h.logger.Error("failed to get vault", "error", err)
		writeError(w, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	writeJSON(w, http.StatusOK, vaultResponse{
		Data:    vault.Data,
		Version: vault.Version,
	})
}

// Put handles PUT /api/v1/vault.
// Optimistic locking: client must send the current version. If it doesn't match,
// the server returns 409 Conflict (DECISION-051).
func (h *VaultHandler) Put(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeInvalidCredentials, "not authenticated")
		return
	}

	// Read body with size limit
	body, err := io.ReadAll(io.LimitReader(r.Body, maxVaultSize+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeInvalidJSON, "failed to read request body")
		return
	}
	if len(body) > maxVaultSize {
		writeError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "vault data exceeds 1 MB limit")
		return
	}

	var req putVaultRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, CodeInvalidJSON, "invalid request body")
		return
	}

	if len(req.Data) == 0 {
		writeError(w, http.StatusBadRequest, CodeMissingField, "data is required")
		return
	}

	// Validate vault blob structure (basic header check)
	header, _, err := crypto.ParseVaultBlob(req.Data)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_VAULT_DATA", "invalid vault blob format: "+err.Error())
		return
	}
	if err := crypto.ValidateHeader(header); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_VAULT_DATA", "invalid vault header: "+err.Error())
		return
	}

	vault, err := h.queries.UpsertVault(r.Context(), store.UpsertVaultParams{
		UserID:  userID,
		Data:    req.Data,
		Version: req.Version,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Version mismatch — optimistic lock conflict (DECISION-051)
			writeError(w, http.StatusConflict, "VERSION_CONFLICT",
				"vault version conflict: another update occurred, re-fetch and retry")
			return
		}
		h.logger.Error("failed to upsert vault", "error", err)
		writeError(w, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	uid := uuidToString(userID)
	h.logger.Info("vault updated", "user_id", uid, "version", vault.Version)

	writeJSON(w, http.StatusOK, vaultResponse{
		Data:    vault.Data,
		Version: vault.Version,
	})
}
