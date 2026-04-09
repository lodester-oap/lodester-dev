// SPDX-License-Identifier: AGPL-3.0-or-later

package handler_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lodester-oap/lodester/internal/crypto"
	"github.com/lodester-oap/lodester/internal/handler"
	"github.com/lodester-oap/lodester/internal/store"
	"github.com/lodester-oap/lodester/internal/testutil"
)

// createTestUser inserts a user directly into the DB for session tests.
// loginHash is the raw 32-byte client-derived login hash.
func createTestUser(t *testing.T, queries *store.Queries, email string, loginHash []byte) {
	t.Helper()
	emailHash := sha256.Sum256([]byte(email))

	// Server-side Argon2id hash (same as what account.Create does)
	serverHash, serverSalt, err := crypto.HashLoginHash(loginHash, nil)
	if err != nil {
		t.Fatalf("failed to hash login_hash: %v", err)
	}

	_, err = queries.CreateUser(t.Context(), store.CreateUserParams{
		EmailHash: emailHash[:],
		KdfParams: []byte(`{"algorithm":"argon2id"}`),
		LoginHash: serverHash,
		LoginSalt: serverSalt,
	})
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
}

func TestCreateSession_Success(t *testing.T) {
	queries := testutil.SetupTestQueries(t)

	// Create user with known login hash
	loginHash := make([]byte, 32)
	for i := range loginHash {
		loginHash[i] = byte(i)
	}
	email := "session-ok@example.com"
	createTestUser(t, queries, email, loginHash)

	h := handler.NewSessionHandler(queries, slog.Default())
	body := `{"email":"session-ok@example.com","login_hash":"000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["token"] == "" {
		t.Error("expected non-empty token")
	}

	// Verify expiry is roughly 7 days from now
	expiresStr, ok := resp["expires_at"].(string)
	if !ok {
		t.Fatal("expires_at is not a string")
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, expiresStr)
	if err != nil {
		t.Fatalf("failed to parse expires_at: %v", err)
	}
	diff := time.Until(expiresAt)
	if diff < 6*24*time.Hour || diff > 8*24*time.Hour {
		t.Errorf("expected ~7 days expiry, got %v", diff)
	}
}

func TestCreateSession_WrongPassword(t *testing.T) {
	queries := testutil.SetupTestQueries(t)

	loginHash := make([]byte, 32)
	for i := range loginHash {
		loginHash[i] = byte(i)
	}
	email := "wrong-pw@example.com"
	createTestUser(t, queries, email, loginHash)

	h := handler.NewSessionHandler(queries, slog.Default())
	// Send a different login_hash
	body := `{"email":"wrong-pw@example.com","login_hash":"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateSession_NonexistentUser(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	h := handler.NewSessionHandler(queries, slog.Default())

	body := `{"email":"nobody@example.com","login_hash":"` + validLoginHash + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	// Should return 401, NOT 404 (don't reveal user existence)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestCreateSession_InvalidJSON(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	h := handler.NewSessionHandler(queries, slog.Default())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewBufferString("bad"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// TestCreateSession_NonHexLoginHash exercises the hex.DecodeString error
// branch. The request body is well-formed JSON with a login_hash that is
// 64 characters long but contains non-hex runes, so the decoder fails.
// The response must still be 401 (not 400) to avoid leaking whether the
// input format was the problem.
func TestCreateSession_NonHexLoginHash(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	h := handler.NewSessionHandler(queries, slog.Default())

	body := `{"email":"nonhex@example.com","login_hash":"zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for non-hex login_hash, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateSession_MissingFields(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	h := handler.NewSessionHandler(queries, slog.Default())

	tests := []struct {
		name string
		body string
	}{
		{"missing email", `{"login_hash":"` + validLoginHash + `"}`},
		{"missing login_hash", `{"email":"a@b.com"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			h.Create(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}
