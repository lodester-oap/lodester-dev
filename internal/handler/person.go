// SPDX-License-Identifier: AGPL-3.0-or-later

// Package handler — person.go exposes the minimal Person CRUD API.
//
// IMPORTANT (DECISION-052): The server MUST NOT accept or store any person
// attributes beyond ownership metadata (id, user_id, timestamps). All
// personal data — names, addresses, phone numbers, notes — lives inside
// the user's encrypted Vault blob. Adding fields to this table would
// break Lodester's zero-knowledge guarantee.
package handler

import (
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/lodester-oap/lodester/internal/middleware"
	"github.com/lodester-oap/lodester/internal/store"
)

// PersonHandler handles person CRUD endpoints.
type PersonHandler struct {
	queries *store.Queries
	logger  *slog.Logger
}

func NewPersonHandler(queries *store.Queries, logger *slog.Logger) *PersonHandler {
	return &PersonHandler{queries: queries, logger: logger}
}

// personResponse is the wire shape for a person row.
// No sensitive fields — by design. See DECISION-052.
type personResponse struct {
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func toPersonResponse(p store.Person) personResponse {
	return personResponse{
		ID:        uuidToString(p.ID),
		CreatedAt: p.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: p.UpdatedAt.Time.Format("2006-01-02T15:04:05Z"),
	}
}

// Create handles POST /api/v1/persons.
// The request body is intentionally empty — the client stores all person
// attributes inside the Vault blob and only asks the server to mint an ID.
func (h *PersonHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeInvalidCredentials, "not authenticated")
		return
	}

	person, err := h.queries.CreatePerson(r.Context(), userID)
	if err != nil {
		h.logger.Error("failed to create person", "error", err)
		writeError(w, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	h.logger.Info("person created",
		"user_id", uuidToString(userID),
		"person_id", uuidToString(person.ID),
	)
	writeJSON(w, http.StatusCreated, toPersonResponse(person))
}

// List handles GET /api/v1/persons.
func (h *PersonHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeInvalidCredentials, "not authenticated")
		return
	}

	persons, err := h.queries.ListPersonsByUserID(r.Context(), userID)
	if err != nil {
		h.logger.Error("failed to list persons", "error", err)
		writeError(w, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	resp := make([]personResponse, 0, len(persons))
	for _, p := range persons {
		resp = append(resp, toPersonResponse(p))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"persons": resp,
		"count":   len(resp),
	})
}

// Delete handles DELETE /api/v1/persons/{id}.
// The server does not know what the person contained — removal is
// purely a bookkeeping operation. The client is responsible for
// scrubbing the corresponding entry from the Vault blob before/after.
func (h *PersonHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeInvalidCredentials, "not authenticated")
		return
	}

	idStr := chi.URLParam(r, "id")
	personID, err := parseUUIDString(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PERSON_ID", "invalid person id")
		return
	}

	// Check ownership first — we want a 404 for both "not found" and
	// "owned by someone else" (no enumeration leak).
	existing, err := h.queries.GetPersonByID(r.Context(), personID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "PERSON_NOT_FOUND", "person not found")
			return
		}
		h.logger.Error("failed to load person", "error", err)
		writeError(w, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}
	if !uuidEqual(existing.UserID, userID) {
		writeError(w, http.StatusNotFound, "PERSON_NOT_FOUND", "person not found")
		return
	}

	if err := h.queries.DeletePerson(r.Context(), store.DeletePersonParams{
		ID:     personID,
		UserID: userID,
	}); err != nil {
		h.logger.Error("failed to delete person", "error", err)
		writeError(w, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	h.logger.Info("person deleted",
		"user_id", uuidToString(userID),
		"person_id", idStr,
	)
	w.WriteHeader(http.StatusNoContent)
}

// parseUUIDString parses a canonical UUID string (with or without hyphens)
// into a pgtype.UUID value.
func parseUUIDString(s string) (pgtype.UUID, error) {
	var out pgtype.UUID
	// Accept canonical 8-4-4-4-12 form; strip hyphens.
	if len(s) == 36 && s[8] == '-' && s[13] == '-' && s[18] == '-' && s[23] == '-' {
		s = s[0:8] + s[9:13] + s[14:18] + s[19:23] + s[24:]
	}
	if len(s) != 32 {
		return out, errors.New("uuid: wrong length")
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return out, err
	}
	copy(out.Bytes[:], b)
	out.Valid = true
	return out, nil
}

// uuidEqual reports whether two pgtype.UUID values hold the same bytes.
func uuidEqual(a, b pgtype.UUID) bool {
	if !a.Valid || !b.Valid {
		return false
	}
	return a.Bytes == b.Bytes
}
