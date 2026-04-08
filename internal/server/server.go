// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/lodester-oap/lodester/internal/handler"
	"github.com/lodester-oap/lodester/internal/store"
)

// Version is set at build time via -ldflags.
var Version = "0.0.1"

// New creates a configured HTTP server.
// If queries is nil, only the health endpoint is registered (useful for tests).
func New(addr string, logger *slog.Logger, queries *store.Queries) *http.Server {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	// OAP discovery endpoints
	r.Get("/.well-known/oap/health", handleHealth)

	// API v1 — requires DB
	if queries != nil {
		accountHandler := handler.NewAccountHandler(queries, logger)
		sessionHandler := handler.NewSessionHandler(queries, logger)

		r.Route("/api/v1", func(r chi.Router) {
			r.Post("/accounts", accountHandler.Create)
			r.Post("/sessions", sessionHandler.Create)
		})
	}

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
