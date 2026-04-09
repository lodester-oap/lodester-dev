// SPDX-License-Identifier: AGPL-3.0-or-later

package handler_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/lodester-oap/lodester/internal/handler"
	"github.com/lodester-oap/lodester/internal/middleware"
	"github.com/lodester-oap/lodester/internal/store"
	"github.com/lodester-oap/lodester/internal/testutil"
)

// shareRouter wires the share handler the same way server.New does, so we
// cover both the authed routes and the public GET. The test user is
// established via loginAndGetToken (from vault_test.go) and authenticates
// via the Auth middleware on the protected group.
func shareRouter(queries *store.Queries) http.Handler {
	logger := slog.Default()
	r := chi.NewRouter()
	h := handler.NewShareHandler(queries, logger)
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/share/{id}", h.Get) // public
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(queries, logger))
			r.Post("/share", h.Create)
			r.Get("/share", h.List)
			r.Delete("/share/{id}", h.Delete)
		})
	})
	return r
}

func createTestShareLink(t *testing.T, queries *store.Queries, router http.Handler, token string, ttlSeconds int64) string {
	t.Helper()
	// 32 random-looking bytes is enough to look like real ciphertext.
	ct := make([]byte, 48)
	for i := range ct {
		ct[i] = byte(i + 1)
	}
	body, _ := json.Marshal(map[string]interface{}{
		"ciphertext":         ct,
		"expires_in_seconds": ttlSeconds,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/share", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("createTestShareLink: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	id, ok := resp["id"].(string)
	if !ok || id == "" {
		t.Fatalf("createTestShareLink: missing id in response: %v", resp)
	}
	return id
}

func TestShareCreate_Success(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "share-create@example.com")
	router := shareRouter(queries)

	body, _ := json.Marshal(map[string]interface{}{
		"ciphertext":         []byte("sample-ciphertext-blob-contents"),
		"expires_in_seconds": 3600,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/share", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["id"] == nil || resp["id"].(string) == "" {
		t.Errorf("expected non-empty id, got %v", resp["id"])
	}
	if resp["expires_at"] == nil {
		t.Errorf("expected expires_at, got nil")
	}
}

func TestShareCreate_DefaultTTL(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "share-default@example.com")
	router := shareRouter(queries)

	// Omit expires_in_seconds → default TTL (7 days)
	body, _ := json.Marshal(map[string]interface{}{
		"ciphertext": []byte("ciphertext"),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/share", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	expiresAt, err := time.Parse(time.RFC3339, resp["expires_at"].(string))
	if err != nil {
		t.Fatalf("parse expires_at: %v", err)
	}
	// Should be roughly 7 days from now (allow 1 minute slop)
	expected := time.Now().Add(7 * 24 * time.Hour)
	if diff := expiresAt.Sub(expected); diff > time.Minute || diff < -time.Minute {
		t.Errorf("expected ~7 days from now, got %v (diff %v)", expiresAt, diff)
	}
}

func TestShareCreate_MissingCiphertext(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "share-missing@example.com")
	router := shareRouter(queries)

	body, _ := json.Marshal(map[string]interface{}{
		"expires_in_seconds": 3600,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/share", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestShareCreate_InvalidJSON(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "share-badjson@example.com")
	router := shareRouter(queries)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/share", bytes.NewBufferString("{not json"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestShareCreate_NegativeExpiry(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "share-neg@example.com")
	router := shareRouter(queries)

	body, _ := json.Marshal(map[string]interface{}{
		"ciphertext":         []byte("x"),
		"expires_in_seconds": -1,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/share", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestShareCreate_PayloadTooLarge(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "share-big@example.com")
	router := shareRouter(queries)

	big := make([]byte, (64<<10)+1) // one byte over the cap
	body, _ := json.Marshal(map[string]interface{}{
		"ciphertext": big,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/share", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestShareGet_PublicSuccess(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "share-get-pub@example.com")
	router := shareRouter(queries)

	id := createTestShareLink(t, queries, router, token, 3600)

	// Public fetch — NO auth header
	req := httptest.NewRequest(http.MethodGet, "/api/v1/share/"+id, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("expected Cache-Control: no-store, got %q", got)
	}
	if got := rec.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Errorf("expected Referrer-Policy: no-referrer, got %q", got)
	}
	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["ciphertext"] == nil {
		t.Errorf("expected ciphertext in response")
	}
	if resp["expires_at"] == nil {
		t.Errorf("expected expires_at in response")
	}
	// Ensure user_id is NOT leaked to recipients
	if _, ok := resp["user_id"]; ok {
		t.Errorf("user_id must not be exposed to recipients")
	}
}

func TestShareGet_NotFound(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	router := shareRouter(queries)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/share/does-not-exist", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestShareGet_InvalidID(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	router := shareRouter(queries)

	longID := bytes.Repeat([]byte("x"), 65)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/share/"+string(longID), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestShareGet_Expired forces an already-expired row via a direct DB insert,
// then verifies the public endpoint returns 410 Gone.
func TestShareGet_Expired(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "share-expired@example.com")
	router := shareRouter(queries)
	_ = token // user creation side-effect

	// Find the user id so we can insert a share_links row directly.
	user, err := queries.GetUserByEmailHash(context.Background(), sha256ForEmail("share-expired@example.com"))
	if err != nil {
		t.Fatalf("GetUserByEmailHash: %v", err)
	}

	_, err = queries.CreateShareLink(context.Background(), store.CreateShareLinkParams{
		ID:         "expired-test-id",
		UserID:     user.ID,
		Ciphertext: []byte("stale"),
		// 1 hour in the past
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(-1 * time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateShareLink: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/share/expired-test-id", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusGone {
		t.Fatalf("expected 410, got %d: %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("SHARE_EXPIRED")) {
		t.Errorf("expected SHARE_EXPIRED in body, got %s", rec.Body.String())
	}
}

func TestShareList_OwnerOnly(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "share-list@example.com")
	router := shareRouter(queries)

	_ = createTestShareLink(t, queries, router, token, 3600)
	_ = createTestShareLink(t, queries, router, token, 7200)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/share", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if int(resp["count"].(float64)) != 2 {
		t.Errorf("expected count 2, got %v", resp["count"])
	}
	links, _ := resp["share_links"].([]interface{})
	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d", len(links))
	}
	// The listing view must NOT leak the ciphertext.
	for _, l := range links {
		m := l.(map[string]interface{})
		if _, ok := m["ciphertext"]; ok {
			t.Errorf("listing leaked ciphertext")
		}
	}
}

func TestShareDelete_Success(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "share-del@example.com")
	router := shareRouter(queries)

	id := createTestShareLink(t, queries, router, token, 3600)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/share/"+id, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	// Public GET should now 404
	req = httptest.NewRequest(http.MethodGet, "/api/v1/share/"+id, nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", rec.Code)
	}
}

func TestShareDelete_CrossUser(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	tokenA := loginAndGetToken(t, queries, "share-del-a@example.com")
	tokenB := loginAndGetToken(t, queries, "share-del-b@example.com")
	router := shareRouter(queries)

	// A creates a link; B tries to delete it.
	id := createTestShareLink(t, queries, router, tokenA, 3600)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/share/"+id, nil)
	req.Header.Set("Authorization", "Bearer "+tokenB)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// B should see 404 (not 403) so we don't leak existence.
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 cross-user, got %d: %s", rec.Code, rec.Body.String())
	}

	// A's link should still work.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/share/"+id, nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected link to still exist after cross-user delete attempt, got %d", rec.Code)
	}
}

func TestShareDelete_NotFound(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "share-del-404@example.com")
	router := shareRouter(queries)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/share/no-such-id", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestShareDelete_InvalidID(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "share-del-bad@example.com")
	router := shareRouter(queries)

	longID := bytes.Repeat([]byte("x"), 65)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/share/"+string(longID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// Direct-handler tests for the 401 branches. These mirror the pattern used
// in auth_branches_test.go — the point is to cover the defensive
// UserIDFromContext branches in share.go by calling the handler with a
// request that never passed through the Auth middleware.
func TestShareCreate_MissingContext(t *testing.T) {
	h := handler.NewShareHandler(nil, slog.Default())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/share", bytes.NewBufferString(`{"ciphertext":"x"}`))
	rec := httptest.NewRecorder()
	h.Create(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestShareList_MissingContext(t *testing.T) {
	h := handler.NewShareHandler(nil, slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/share", nil)
	rec := httptest.NewRecorder()
	h.List(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestShareDelete_MissingContext(t *testing.T) {
	h := handler.NewShareHandler(nil, slog.Default())
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/share/abc", nil)
	rec := httptest.NewRecorder()
	h.Delete(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

// sha256ForEmail mirrors what the handlers do when looking up users.
func sha256ForEmail(email string) []byte {
	h := sha256.Sum256([]byte(email))
	return h[:]
}
