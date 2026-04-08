// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware_test

import (
	"crypto/sha256"
	"encoding/base64"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/lodester-oap/lodester/internal/crypto"
	"github.com/lodester-oap/lodester/internal/middleware"
	"github.com/lodester-oap/lodester/internal/store"
	"github.com/lodester-oap/lodester/internal/testutil"
)

// setupAuthTest creates a user with a session and returns the token.
func setupAuthTest(t *testing.T, queries *store.Queries, email string) string {
	t.Helper()
	emailHash := sha256.Sum256([]byte(email))
	loginHash := make([]byte, 32)
	serverHash, serverSalt, err := crypto.HashLoginHash(loginHash, nil)
	if err != nil {
		t.Fatalf("failed to hash login: %v", err)
	}

	user, err := queries.CreateUser(t.Context(), store.CreateUserParams{
		EmailHash: emailHash[:],
		KdfParams: []byte(`{"algorithm":"argon2id"}`),
		LoginHash: serverHash,
		LoginSalt: serverSalt,
	})
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	tokenBytes := make([]byte, 32)
	for i := range tokenBytes {
		tokenBytes[i] = byte(i + 50)
	}
	tokenHash := sha256.Sum256(tokenBytes)
	token := base64.URLEncoding.EncodeToString(tokenBytes)

	_, err = queries.CreateSession(t.Context(), store.CreateSessionParams{
		UserID:    user.ID,
		TokenHash: tokenHash[:],
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(7 * 24 * time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	return token
}

func protectedHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := middleware.UserIDFromContext(r.Context())
		if !ok {
			http.Error(w, "no user", http.StatusInternalServerError)
			return
		}
		if !userID.Valid {
			http.Error(w, "invalid user id", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}
}

func TestAuth_ValidToken(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := setupAuthTest(t, queries, "auth-ok@example.com")

	handler := middleware.Auth(queries, slog.Default())(protectedHandler())
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAuth_MissingHeader(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	handler := middleware.Auth(queries, slog.Default())(protectedHandler())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestAuth_InvalidToken(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	handler := middleware.Auth(queries, slog.Default())(protectedHandler())

	// Use a token that doesn't exist in the DB
	fakeToken := base64.URLEncoding.EncodeToString(make([]byte, 32))
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+fakeToken)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestAuth_MalformedHeader(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	handler := middleware.Auth(queries, slog.Default())(protectedHandler())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz") // not Bearer
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
