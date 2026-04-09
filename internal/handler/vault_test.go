// SPDX-License-Identifier: AGPL-3.0-or-later

package handler_test

import (
	"bytes"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
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

// loginAndGetToken creates a test user, creates a session, and returns the Bearer token.
func loginAndGetToken(t *testing.T, queries *store.Queries, email string) string {
	t.Helper()
	loginHash := make([]byte, 32)
	for i := range loginHash {
		loginHash[i] = byte(i + 1)
	}
	createTestUser(t, queries, email, loginHash)

	// Create a session directly in the DB. Use crypto/rand so that multiple
	// calls within the same test produce distinct tokens (and therefore
	// distinct token_hash values, satisfying the unique constraint).
	tokenBytes := make([]byte, 32)
	if _, err := cryptorand.Read(tokenBytes); err != nil {
		t.Fatalf("failed to read random token bytes: %v", err)
	}
	tokenHash := sha256.Sum256(tokenBytes)
	token := base64.URLEncoding.EncodeToString(tokenBytes)

	emailHash := sha256.Sum256([]byte(email))
	user, err := queries.GetUserByEmailHash(t.Context(), emailHash[:])
	if err != nil {
		t.Fatalf("failed to get test user: %v", err)
	}

	_, err = queries.CreateSession(t.Context(), store.CreateSessionParams{
		UserID:    user.ID,
		TokenHash: tokenHash[:],
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(7 * 24 * time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}

	return token
}

// makeTestVaultBlob creates a valid vault blob for testing.
func makeTestVaultBlob() []byte {
	header := `{"v":1,"alg":"aes-gcm-256","kdf":"argon2id","kdf_params":{"memory":65536},"nonce":"dGVzdG5vbmNl","ct_len":16}`
	headerBytes := []byte(header)
	headerLen := make([]byte, 4)
	binary.BigEndian.PutUint32(headerLen, uint32(len(headerBytes)))
	blob := append(headerLen, headerBytes...)
	blob = append(blob, make([]byte, 16)...) // dummy ciphertext
	return blob
}

// authedRouter creates a chi router with the vault handler behind auth middleware.
func authedRouter(queries *store.Queries) http.Handler {
	logger := slog.Default()
	r := chi.NewRouter()
	vaultHandler := handler.NewVaultHandler(queries, logger)
	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth(queries, logger))
		r.Get("/api/v1/vault", vaultHandler.Get)
		r.Put("/api/v1/vault", vaultHandler.Put)
	})
	return r
}

func TestVaultGet_Empty(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "vault-empty@example.com")
	router := authedRouter(queries)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/vault", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["version"].(float64) != 0 {
		t.Errorf("expected version 0, got %v", resp["version"])
	}
}

func TestVaultPut_Success(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "vault-put@example.com")
	router := authedRouter(queries)

	blob := makeTestVaultBlob()
	blobB64 := base64.StdEncoding.EncodeToString(blob)
	body, _ := json.Marshal(map[string]interface{}{
		"data":    blobB64,
		"version": 0,
	})

	req := httptest.NewRequest(http.MethodPut, "/api/v1/vault", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["version"].(float64) != 1 {
		t.Errorf("expected version 1, got %v", resp["version"])
	}
}

func TestVaultPut_VersionConflict(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "vault-conflict@example.com")
	router := authedRouter(queries)

	blob := makeTestVaultBlob()
	blobB64 := base64.StdEncoding.EncodeToString(blob)

	// First PUT (version 0 → creates)
	body, _ := json.Marshal(map[string]interface{}{
		"data":    blobB64,
		"version": 0,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/vault", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("first PUT: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Second PUT with stale version (0 instead of 1) → 409
	body, _ = json.Marshal(map[string]interface{}{
		"data":    blobB64,
		"version": 0,
	})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/vault", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestVaultPut_InvalidBlob(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "vault-invalid@example.com")
	router := authedRouter(queries)

	// Send raw bytes that aren't a valid vault blob
	invalidBlob := base64.StdEncoding.EncodeToString([]byte("not a vault blob"))
	body, _ := json.Marshal(map[string]interface{}{
		"data":    invalidBlob,
		"version": 0,
	})

	req := httptest.NewRequest(http.MethodPut, "/api/v1/vault", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestVaultPut_MissingData(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "vault-nodata@example.com")
	router := authedRouter(queries)

	body := `{"version":0}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/vault", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestVaultPut_PayloadTooLarge exercises the size-limit branch in vault.Put.
func TestVaultPut_PayloadTooLarge(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "vault-huge@example.com")
	router := authedRouter(queries)

	// 2 MB of zero bytes — exceeds the 1 MB limit enforced by the handler.
	huge := make([]byte, 2<<20)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/vault", bytes.NewReader(huge))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestVaultPut_InvalidJSON exercises the json.Unmarshal error branch in
// vault.Put — the body is small (passes the size gate) but is not valid
// JSON.
func TestVaultPut_InvalidJSON(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "vault-badjson@example.com")
	router := authedRouter(queries)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/vault", bytes.NewBufferString("{not json"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestVaultPut_InvalidHeader exercises the crypto.ValidateHeader error
// branch. The blob parses cleanly (ParseVaultBlob returns OK) but the
// header JSON is missing required fields, so ValidateHeader rejects it.
func TestVaultPut_InvalidHeader(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "vault-badheader@example.com")
	router := authedRouter(queries)

	// Version 0 trips ValidateHeader's "unsupported schema version" check
	// while ParseVaultBlob itself remains satisfied.
	header := `{"v":0,"alg":"aes-gcm-256","kdf":"argon2id","kdf_params":{"memory":65536},"nonce":"dGVzdG5vbmNl","ct_len":16}`
	headerBytes := []byte(header)
	headerLen := make([]byte, 4)
	binary.BigEndian.PutUint32(headerLen, uint32(len(headerBytes)))
	blob := append(headerLen, headerBytes...)
	blob = append(blob, make([]byte, 16)...)

	blobB64 := base64.StdEncoding.EncodeToString(blob)
	body, _ := json.Marshal(map[string]interface{}{
		"data":    blobB64,
		"version": 0,
	})

	req := httptest.NewRequest(http.MethodPut, "/api/v1/vault", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestVaultGet_AfterPut(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "vault-getput@example.com")
	router := authedRouter(queries)

	// PUT vault
	blob := makeTestVaultBlob()
	blobB64 := base64.StdEncoding.EncodeToString(blob)
	body, _ := json.Marshal(map[string]interface{}{
		"data":    blobB64,
		"version": 0,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/vault", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("PUT: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// GET vault
	req = httptest.NewRequest(http.MethodGet, "/api/v1/vault", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["version"].(float64) != 1 {
		t.Errorf("expected version 1, got %v", resp["version"])
	}
	if resp["data"] == nil || resp["data"] == "" {
		t.Error("expected non-empty data")
	}
}
