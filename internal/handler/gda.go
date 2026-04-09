// SPDX-License-Identifier: AGPL-3.0-or-later

// Package handler — gda.go exposes endpoints for minting and listing
// GDA codes (Global Distinct Address identifiers). GDA codes are public
// by design (DECISION-053) and never contain personal information, so
// they live outside the user's Vault in the `gda_codes` table.
package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/lodester-oap/lodester/internal/gda"
	"github.com/lodester-oap/lodester/internal/middleware"
	"github.com/lodester-oap/lodester/internal/store"
)

// maxGDARetries guards against a pathological scenario where the randomly
// generated code collides with an existing row. With 55 bits of entropy
// this is astronomically unlikely, but the retry loop costs nothing.
const maxGDARetries = 5

// GDAHandler handles GDA code endpoints.
type GDAHandler struct {
	queries *store.Queries
	logger  *slog.Logger
}

func NewGDAHandler(queries *store.Queries, logger *slog.Logger) *GDAHandler {
	return &GDAHandler{queries: queries, logger: logger}
}

type gdaCodeResponse struct {
	Code      string `json:"code"`       // formatted XXXX-XXXX-XXXX
	Raw       string `json:"raw"`        // bare 12-char form (server storage format)
	PersonID  string `json:"person_id"`
	CreatedAt string `json:"created_at"`
}

func toGDAResponse(c store.GdaCode) gdaCodeResponse {
	formatted, _ := gda.Format(c.Code)
	return gdaCodeResponse{
		Code:      formatted,
		Raw:       c.Code,
		PersonID:  uuidToString(c.PersonID),
		CreatedAt: c.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
	}
}

type createGDARequest struct {
	PersonID string `json:"person_id"`
}

// Create handles POST /api/v1/gda-codes.
// The caller supplies the person_id they own; the server mints a fresh
// code via crypto/rand and stores the binding.
func (h *GDAHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeInvalidCredentials, "not authenticated")
		return
	}

	var req createGDARequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, CodeInvalidJSON, "invalid request body")
		return
	}
	if req.PersonID == "" {
		writeError(w, http.StatusBadRequest, CodeMissingField, "person_id is required")
		return
	}

	personID, err := parseUUIDString(req.PersonID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PERSON_ID", "invalid person id")
		return
	}

	// Ownership check: the person must belong to the caller.
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

	// Mint a unique code, retrying on the vanishingly rare collision.
	var created store.GdaCode
	for attempt := 0; attempt < maxGDARetries; attempt++ {
		formatted, gerr := gda.Generate()
		if gerr != nil {
			h.logger.Error("gda generation failed", "error", gerr)
			writeError(w, http.StatusInternalServerError, CodeInternalError, "internal server error")
			return
		}
		rawCode, _ := gda.Normalize(formatted)

		created, err = h.queries.CreateGDACode(r.Context(), store.CreateGDACodeParams{
			Code:     rawCode,
			PersonID: personID,
			UserID:   userID,
		})
		if err == nil {
			break
		}
		// Any non-unique violation bubbles up as a different error depending
		// on the driver; simplest is to retry a few times, then surface.
		if attempt == maxGDARetries-1 {
			h.logger.Error("failed to persist gda code after retries", "error", err)
			writeError(w, http.StatusInternalServerError, CodeInternalError, "internal server error")
			return
		}
	}

	h.logger.Info("gda code created",
		"user_id", uuidToString(userID),
		"person_id", req.PersonID,
		"code", created.Code,
	)
	writeJSON(w, http.StatusCreated, toGDAResponse(created))
}

// ListByPerson handles GET /api/v1/persons/{id}/gda-codes.
func (h *GDAHandler) ListByPerson(w http.ResponseWriter, r *http.Request) {
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

	codes, err := h.queries.ListGDACodesByPersonID(r.Context(), store.ListGDACodesByPersonIDParams{
		PersonID: personID,
		UserID:   userID,
	})
	if err != nil {
		h.logger.Error("failed to list gda codes", "error", err)
		writeError(w, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	resp := make([]gdaCodeResponse, 0, len(codes))
	for _, c := range codes {
		resp = append(resp, toGDAResponse(c))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"codes": resp,
		"count": len(resp),
	})
}

// Delete handles DELETE /api/v1/gda-codes/{code}.
// The URL path parameter is the formatted or raw code; normalization is
// applied before the database lookup.
func (h *GDAHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeInvalidCredentials, "not authenticated")
		return
	}

	codeParam := chi.URLParam(r, "code")
	if err := gda.Verify(codeParam); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_GDA_CODE", "invalid or corrupted GDA code")
		return
	}
	raw, _ := gda.Normalize(codeParam)

	if err := h.queries.DeleteGDACode(r.Context(), store.DeleteGDACodeParams{
		Code:   raw,
		UserID: userID,
	}); err != nil {
		h.logger.Error("failed to delete gda code", "error", err)
		writeError(w, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	h.logger.Info("gda code deleted",
		"user_id", uuidToString(userID),
		"code", raw,
	)
	w.WriteHeader(http.StatusNoContent)
}
