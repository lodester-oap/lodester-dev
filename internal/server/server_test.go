// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lodester-oap/lodester/web"
)

func TestHealthEndpoint(t *testing.T) {
	logger := slog.Default()
	srv := New(":0", logger, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/oap/health", nil)
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("expected status=ok, got %q", body["status"])
	}
	if body["version"] != Version {
		t.Errorf("expected version=%s, got %q", Version, body["version"])
	}
}

func TestStaticFileServing(t *testing.T) {
	logger := slog.Default()
	srv := New(":0", logger, nil, web.FS())

	// GET / should serve index.html
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /: expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Lodester") {
		t.Error("GET /: expected body to contain 'Lodester'")
	}

	// GET /static/lodester-client.js should serve the JS file
	req = httptest.NewRequest(http.MethodGet, "/static/lodester-client.js", nil)
	rec = httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /static/lodester-client.js: expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "deriveLoginHash") {
		t.Error("GET /static/lodester-client.js: expected JS content")
	}
}
