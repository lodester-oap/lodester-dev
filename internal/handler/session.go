// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/lodester-oap/lodester/internal/crypto"
	"github.com/lodester-oap/lodester/internal/logging"
	"github.com/lodester-oap/lodester/internal/store"
)

// SessionHandler handles session (login) API endpoints.
type SessionHandler struct {
	queries *store.Queries
	logger  *slog.Logger
}

func NewSessionHandler(queries *store.Queries, logger *slog.Logger) *SessionHandler {
	return &SessionHandler{queries: queries, logger: logger}
}

type createSessionRequest struct {
	Email     string `json:"email"`
	LoginHash string `json:"login_hash"` // hex-encoded
}

type createSessionResponse struct {
	Token     string    `json:"token"`
	UserID    string    `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Create handles POST /api/v1/sessions.
func (h *SessionHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, CodeInvalidJSON, "invalid request body")
		return
	}

	if req.Email == "" || req.LoginHash == "" {
		writeError(w, http.StatusBadRequest, CodeMissingField, "email and login_hash are required")
		return
	}

	// Decode login_hash from hex
	loginHashBytes, err := hex.DecodeString(req.LoginHash)
	if err != nil {
		// Still return INVALID_CREDENTIALS to not leak info
		writeError(w, http.StatusUnauthorized, CodeInvalidCredentials, "invalid credentials")
		return
	}

	// Normalize email (DECISION-046) then hash
	normalized := normalizeEmail(req.Email)
	emailHash := sha256.Sum256([]byte(normalized))

	user, err := h.queries.GetUserByEmailHash(r.Context(), emailHash[:])
	if err != nil {
		// Timing attack protection: execute dummy Argon2id even when user not found.
		// This ensures response time is similar whether user exists or not.
		dummySalt := make([]byte, crypto.Argon2SaltLength)
		_, _, _ = crypto.HashLoginHash(loginHashBytes, dummySalt)

		h.logger.Warn("login failed: user not found", "email", logging.Redacted{})
		writeError(w, http.StatusUnauthorized, CodeInvalidCredentials, "invalid credentials")
		return
	}

	// Verify login_hash: re-derive server hash from received login_hash + stored salt,
	// then compare with constant-time comparison.
	serverHash, _, err := crypto.HashLoginHash(loginHashBytes, user.LoginSalt)
	if err != nil {
		h.logger.Error("failed to verify login_hash", "error", err)
		writeError(w, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	if !crypto.VerifyLoginHash(serverHash, user.LoginHash) {
		h.logger.Warn("login failed: invalid credentials", "email", logging.Redacted{})
		writeError(w, http.StatusUnauthorized, CodeInvalidCredentials, "invalid credentials")
		return
	}

	// Generate session token using crypto/rand (NEVER math/rand).
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		h.logger.Error("failed to generate session token", "error", err)
		writeError(w, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}
	token := base64.URLEncoding.EncodeToString(tokenBytes)
	tokenHash := sha256.Sum256(tokenBytes)

	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	_, err = h.queries.CreateSession(r.Context(), store.CreateSessionParams{
		UserID:    user.ID,
		TokenHash: tokenHash[:],
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
	if err != nil {
		h.logger.Error("failed to create session", "error", err)
		writeError(w, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	uid := uuidToString(user.ID)
	h.logger.Info("login success", "user_id", uid)

	writeJSON(w, http.StatusOK, createSessionResponse{
		Token:     token,
		UserID:    uid,
		ExpiresAt: expiresAt,
	})
}
