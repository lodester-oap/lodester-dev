// SPDX-License-Identifier: AGPL-3.0-or-later

package handler_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lodester-oap/lodester/internal/handler"
	"github.com/lodester-oap/lodester/internal/testutil"
)

// These tests exercise the defensive `UserIDFromContext` branches in every
// handler that requires authentication. The production routing guarantees
// the middleware has populated the context, so these branches are only
// reachable by calling the handler methods directly without the Auth
// middleware. Without these tests the branches remain uncovered and the
// overall coverage suffers for no reason — the code paths themselves are
// cheap to exercise.

func newRecorder(method, path string) (*httptest.ResponseRecorder, *http.Request) {
	req := httptest.NewRequest(method, path, nil)
	return httptest.NewRecorder(), req
}

func assertUnauthorized(t *testing.T, rec *httptest.ResponseRecorder, label string) {
	t.Helper()
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("%s: expected 401, got %d: %s", label, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "INVALID_CREDENTIALS") {
		t.Errorf("%s: expected INVALID_CREDENTIALS in body, got %s", label, rec.Body.String())
	}
}

func TestMeGet_MissingContext(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	h := handler.NewMeHandler(queries, slog.Default())

	rec, req := newRecorder(http.MethodGet, "/api/v1/me")
	h.Get(rec, req)
	assertUnauthorized(t, rec, "me.Get")
}

func TestPersonCreate_MissingContext(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	h := handler.NewPersonHandler(queries, slog.Default())

	rec, req := newRecorder(http.MethodPost, "/api/v1/persons")
	h.Create(rec, req)
	assertUnauthorized(t, rec, "person.Create")
}

func TestPersonList_MissingContext(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	h := handler.NewPersonHandler(queries, slog.Default())

	rec, req := newRecorder(http.MethodGet, "/api/v1/persons")
	h.List(rec, req)
	assertUnauthorized(t, rec, "person.List")
}

func TestPersonDelete_MissingContext(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	h := handler.NewPersonHandler(queries, slog.Default())

	// chi URL params are never read because the unauth branch fires first.
	rec, req := newRecorder(http.MethodDelete, "/api/v1/persons/whatever")
	h.Delete(rec, req)
	assertUnauthorized(t, rec, "person.Delete")
}

func TestGDACreate_MissingContext(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	h := handler.NewGDAHandler(queries, slog.Default())

	rec, req := newRecorder(http.MethodPost, "/api/v1/gda-codes")
	h.Create(rec, req)
	assertUnauthorized(t, rec, "gda.Create")
}

func TestGDAListByPerson_MissingContext(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	h := handler.NewGDAHandler(queries, slog.Default())

	rec, req := newRecorder(http.MethodGet, "/api/v1/persons/whatever/gda-codes")
	h.ListByPerson(rec, req)
	assertUnauthorized(t, rec, "gda.ListByPerson")
}

func TestGDADelete_MissingContext(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	h := handler.NewGDAHandler(queries, slog.Default())

	rec, req := newRecorder(http.MethodDelete, "/api/v1/gda-codes/ABCD-EFGH-JKMN")
	h.Delete(rec, req)
	assertUnauthorized(t, rec, "gda.Delete")
}

func TestVaultGet_MissingContext(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	h := handler.NewVaultHandler(queries, slog.Default())

	rec, req := newRecorder(http.MethodGet, "/api/v1/vault")
	h.Get(rec, req)
	assertUnauthorized(t, rec, "vault.Get")
}

func TestVaultPut_MissingContext(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	h := handler.NewVaultHandler(queries, slog.Default())

	rec, req := newRecorder(http.MethodPut, "/api/v1/vault")
	h.Put(rec, req)
	assertUnauthorized(t, rec, "vault.Put")
}

func TestVCardExport_MissingContext(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	h := handler.NewVCardHandler(queries, slog.Default())

	rec, req := newRecorder(http.MethodPost, "/api/v1/vcard")
	h.Export(rec, req)
	assertUnauthorized(t, rec, "vcard.Export")
}
