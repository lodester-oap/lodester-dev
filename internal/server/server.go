// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Version is set at build time via -ldflags.
var Version = "0.0.1"

// New creates a configured HTTP server.
func New(addr string, logger *slog.Logger) *http.Server {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	// OAP discovery endpoints
	r.Get("/.well-known/oap/health", handleHealth)

	return &http.Server{
		Addr:    addr,
		Handler: r,
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"version": Version,
	})
}
