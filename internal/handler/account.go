// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/lodester-oap/lodester/internal/crypto"
	"github.com/lodester-oap/lodester/internal/logging"
	"github.com/lodester-oap/lodester/internal/store"
)

// AccountHandler handles account-related API endpoints.
type AccountHandler struct {
	queries *store.Queries
	logger  *slog.Logger
}

func NewAccountHandler(queries *store.Queries, logger *slog.Logger) *AccountHandler {
	return &AccountHandler{queries: queries, logger: logger}
}

type createAccountRequest struct {
	Email     string          `json:"email"`
	LoginHash string          `json:"login_hash"` // hex-encoded, client-derived via Bitwarden method
	KDFParams json.RawMessage `json:"kdf_params"`
}

type createAccountResponse struct {
	UserID string `json:"user_id"`
}

// Create handles POST /api/v1/accounts.
func (h *AccountHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, CodeInvalidJSON, "invalid request body")
		return
	}

	if req.Email == "" || req.LoginHash == "" || len(req.KDFParams) == 0 {
		writeError(w, http.StatusBadRequest, CodeMissingField, "email, login_hash, and kdf_params are required")
		return
	}

	// Basic email validation
	if !strings.Contains(req.Email, "@") {
		writeError(w, http.StatusBadRequest, CodeInvalidEmail, "invalid email format")
		return
	}

	// Decode login_hash from hex
	loginHashBytes, err := hex.DecodeString(req.LoginHash)
	if err != nil || len(loginHashBytes) != int(crypto.Argon2KeyLength) {
		writeError(w, http.StatusBadRequest, CodeInvalidLoginHash, "login_hash must be 32 bytes hex-encoded")
		return
	}

	// Normalize email (DECISION-046) then hash for PII protection
	normalized := normalizeEmail(req.Email)
	emailHash := sha256.Sum256([]byte(normalized))

	// Server-side Argon2id hash of the login_hash for additional protection
	serverHash, serverSalt, err := crypto.HashLoginHash(loginHashBytes, nil)
	if err != nil {
		h.logger.Error("failed to hash login_hash", "error", err)
		writeError(w, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	user, err := h.queries.CreateUser(r.Context(), store.CreateUserParams{
		EmailHash: emailHash[:],
		KdfParams: req.KDFParams,
		LoginHash: serverHash,
		LoginSalt: serverSalt,
	})
	if err != nil {
		// Check for unique constraint violation (duplicate email)
		if strings.Contains(err.Error(), "23505") {
			writeError(w, http.StatusConflict, CodeAccountExists, "account already exists")
			return
		}
		h.logger.Error("failed to create user", "error", err, "email", logging.Redacted{})
		writeError(w, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	uid := uuidToString(user.ID)
	h.logger.Info("user created", "user_id", uid, "email", logging.Redacted{})

	writeJSON(w, http.StatusCreated, createAccountResponse{UserID: uid})
}
