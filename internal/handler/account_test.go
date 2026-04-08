// SPDX-License-Identifier: AGPL-3.0-or-later

package handler_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lodester-oap/lodester/internal/handler"
	"github.com/lodester-oap/lodester/internal/testutil"
)

// validLoginHash is 32 bytes hex-encoded (64 hex chars).
const validLoginHash = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

func TestCreateAccount_Success(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	h := handler.NewAccountHandler(queries, slog.Default())

	body := `{"email":"acct-ok@example.com","login_hash":"` + validLoginHash + `","kdf_params":{"algorithm":"argon2id","memory":65536}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["user_id"] == "" {
		t.Error("expected non-empty user_id")
	}
}

func TestCreateAccount_DuplicateEmail(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	h := handler.NewAccountHandler(queries, slog.Default())

	body := `{"email":"dup@example.com","login_hash":"` + validLoginHash + `","kdf_params":{"algorithm":"argon2id","memory":65536}}`

	// First creation should succeed
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("first creation: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	// Second creation with same email should return 409
	req = httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate: expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateAccount_InvalidJSON(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	h := handler.NewAccountHandler(queries, slog.Default())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCreateAccount_MissingFields(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	h := handler.NewAccountHandler(queries, slog.Default())

	tests := []struct {
		name string
		body string
	}{
		{"missing email", `{"login_hash":"` + validLoginHash + `","kdf_params":{"a":"b"}}`},
		{"missing login_hash", `{"email":"a@b.com","kdf_params":{"a":"b"}}`},
		{"missing kdf_params", `{"email":"a@b.com","login_hash":"` + validLoginHash + `"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			h.Create(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestCreateAccount_InvalidLoginHash(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	h := handler.NewAccountHandler(queries, slog.Default())

	// login_hash must be exactly 32 bytes hex-encoded (64 hex chars)
	body := `{"email":"bad-hash@example.com","login_hash":"tooshort","kdf_params":{"algorithm":"argon2id"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}
