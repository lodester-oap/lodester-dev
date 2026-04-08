// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/lodester-oap/lodester/internal/handler"
	"github.com/lodester-oap/lodester/internal/middleware"
	"github.com/lodester-oap/lodester/internal/store"
)

// Version is set at build time via -ldflags.
var Version = "0.0.1"

// New creates a configured HTTP server.
// If queries is nil, only the health endpoint is registered (useful for tests).
// If webFS is non-nil, static files are served from it under /static/ and / serves index.html.
func New(addr string, logger *slog.Logger, queries *store.Queries, webFS fs.FS) *http.Server {
	r := chi.NewRouter()

	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)

	// OAP discovery endpoints
	r.Get("/.well-known/oap/health", handleHealth)

	// API v1 — requires DB
	if queries != nil {
		accountHandler := handler.NewAccountHandler(queries, logger)
		sessionHandler := handler.NewSessionHandler(queries, logger)
		meHandler := handler.NewMeHandler(queries, logger)
		vaultHandler := handler.NewVaultHandler(queries, logger)

		r.Route("/api/v1", func(r chi.Router) {
			// Public routes
			r.Post("/accounts", accountHandler.Create)
			r.Post("/sessions", sessionHandler.Create)

			// Authenticated routes
			r.Group(func(r chi.Router) {
				r.Use(middleware.Auth(queries, logger))
				r.Get("/me", meHandler.Get)
				r.Get("/vault", vaultHandler.Get)
				r.Put("/vault", vaultHandler.Put)
			})
		})
	}

	// Static files and SPA fallback
	if webFS != nil {
		fileServer := http.FileServer(http.FS(webFS))
		r.Handle("/static/*", http.StripPrefix("/static/", fileServer))
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFileFS(w, r, webFS, "index.html")
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
