// SPDX-License-Identifier: AGPL-3.0-or-later

// Package handler — vcard.go exposes a vCard 4.0 exporter endpoint.
//
// IMPORTANT (DECISION-052): The server must not know the contents of a
// person. The client therefore decrypts the Vault locally and POSTs the
// (already-plain) card data to this endpoint, which simply formats it
// as a vCard blob and streams it back with the appropriate content
// headers. Nothing is persisted. The request never crosses the network
// in encrypted form because the plaintext is short-lived and the HTTPS
// transport is trusted.
//
// An alternative would be to generate the vCard entirely client-side in
// JavaScript. That is a fine future optimization; for M4 we keep the
// formatting logic server-side so there is a single, well-tested
// reference implementation that other OAP clients can reuse.
package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/lodester-oap/lodester/internal/middleware"
	"github.com/lodester-oap/lodester/internal/store"
	"github.com/lodester-oap/lodester/internal/vcard"
)

// VCardHandler handles vCard export.
type VCardHandler struct {
	queries *store.Queries
	logger  *slog.Logger
}

func NewVCardHandler(queries *store.Queries, logger *slog.Logger) *VCardHandler {
	return &VCardHandler{queries: queries, logger: logger}
}

type exportRequest struct {
	Names     []vcard.Name    `json:"names"`
	Orgs      []string        `json:"orgs,omitempty"`
	Phones    []string        `json:"phones,omitempty"`
	Emails    []string        `json:"emails,omitempty"`
	Addresses []vcard.Address `json:"addresses,omitempty"`
	Note      string          `json:"note,omitempty"`
	GDACode   string          `json:"gda_code,omitempty"`
	Filename  string          `json:"filename,omitempty"`
}

// Export handles POST /api/v1/vcard.
// The body is the already-decrypted card payload; the response is the
// formatted vCard 4.0 document with a `.vcf` attachment disposition.
func (h *VCardHandler) Export(w http.ResponseWriter, r *http.Request) {
	if _, ok := middleware.UserIDFromContext(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, CodeInvalidCredentials, "not authenticated")
		return
	}

	var req exportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, CodeInvalidJSON, "invalid request body")
		return
	}
	if len(req.Names) == 0 {
		writeError(w, http.StatusBadRequest, CodeMissingField, "at least one name is required")
		return
	}

	card := vcard.Card{
		Names:     req.Names,
		Orgs:      req.Orgs,
		Phones:    req.Phones,
		Emails:    req.Emails,
		Addresses: req.Addresses,
		Note:      req.Note,
		GDACode:   req.GDACode,
	}
	body := vcard.Export(card)

	filename := req.Filename
	if filename == "" {
		filename = "person.vcf"
	}

	w.Header().Set("Content-Type", "text/vcard; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+sanitizeFilename(filename)+`"`)
	_, _ = w.Write([]byte(body))
}

// sanitizeFilename strips characters that are unsafe in an HTTP
// Content-Disposition filename. Only a small ASCII subset is allowed;
// everything else is replaced with an underscore.
func sanitizeFilename(s string) string {
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9',
			c == '-' || c == '_' || c == '.':
			b = append(b, c)
		default:
			b = append(b, '_')
		}
	}
	if len(b) == 0 {
		return "person.vcf"
	}
	return string(b)
}
