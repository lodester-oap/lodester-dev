// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/lodester-oap/lodester/internal/middleware"
	"github.com/lodester-oap/lodester/internal/store"
)

// MeHandler handles the /me endpoint.
type MeHandler struct {
	queries *store.Queries
	logger  *slog.Logger
}

func NewMeHandler(queries *store.Queries, logger *slog.Logger) *MeHandler {
	return &MeHandler{queries: queries, logger: logger}
}

type meResponse struct {
	UserID    string          `json:"user_id"`
	KDFParams json.RawMessage `json:"kdf_params"`
	CreatedAt string          `json:"created_at"`
}

// Get handles GET /api/v1/me.
func (h *MeHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeInvalidCredentials, "not authenticated")
		return
	}

	user, err := h.queries.GetUserByID(r.Context(), userID)
	if err != nil {
		h.logger.Error("failed to get user", "error", err)
		writeError(w, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	writeJSON(w, http.StatusOK, meResponse{
		UserID:    uuidToString(user.ID),
		KDFParams: user.KdfParams,
		CreatedAt: user.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
	})
}
