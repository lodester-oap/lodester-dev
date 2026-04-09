// SPDX-License-Identifier: AGPL-3.0-or-later

package handler_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/lodester-oap/lodester/internal/handler"
	"github.com/lodester-oap/lodester/internal/middleware"
	"github.com/lodester-oap/lodester/internal/store"
	"github.com/lodester-oap/lodester/internal/testutil"
)

func meRouter(queries *store.Queries) http.Handler {
	logger := slog.Default()
	r := chi.NewRouter()
	meHandler := handler.NewMeHandler(queries, logger)
	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth(queries, logger))
		r.Get("/api/v1/me", meHandler.Get)
	})
	return r
}

func TestMeGet_Authenticated(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "me-get@example.com")
	router := meRouter(queries)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["user_id"] == nil || resp["user_id"].(string) == "" {
		t.Error("expected user_id in response")
	}
	if resp["kdf_params"] == nil {
		t.Error("expected kdf_params in response")
	}
}

func TestMeGet_Unauthenticated(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	router := meRouter(queries)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}
