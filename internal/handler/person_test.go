// SPDX-License-Identifier: AGPL-3.0-or-later

package handler_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/lodester-oap/lodester/internal/gda"
	"github.com/lodester-oap/lodester/internal/handler"
	"github.com/lodester-oap/lodester/internal/middleware"
	"github.com/lodester-oap/lodester/internal/store"
	"github.com/lodester-oap/lodester/internal/testutil"
)

// personAPIRouter mounts the Person / GDA / vCard handlers behind auth.
func personAPIRouter(queries *store.Queries) http.Handler {
	logger := slog.Default()
	r := chi.NewRouter()
	ph := handler.NewPersonHandler(queries, logger)
	gh := handler.NewGDAHandler(queries, logger)
	vh := handler.NewVCardHandler(queries, logger)
	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth(queries, logger))
		r.Post("/api/v1/persons", ph.Create)
		r.Get("/api/v1/persons", ph.List)
		r.Delete("/api/v1/persons/{id}", ph.Delete)
		r.Post("/api/v1/gda-codes", gh.Create)
		r.Get("/api/v1/persons/{id}/gda-codes", gh.ListByPerson)
		r.Delete("/api/v1/gda-codes/{code}", gh.Delete)
		r.Post("/api/v1/vcard", vh.Export)
	})
	return r
}

func doJSON(t *testing.T, router http.Handler, method, path, token string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestPersonCreate_ReturnsMinimalFields(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "person-create@example.com")
	router := personAPIRouter(queries)

	rec := doJSON(t, router, http.MethodPost, "/api/v1/persons", token, map[string]interface{}{})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["id"] == nil || resp["id"].(string) == "" {
		t.Fatal("expected non-empty id in response")
	}
	// Crucially, there MUST be no personal fields in the response
	// (DECISION-052). Only id + timestamps.
	forbidden := []string{"name", "names", "family", "given", "address", "phone", "email", "note"}
	for _, f := range forbidden {
		if _, ok := resp[f]; ok {
			t.Errorf("response must not contain %q field (zero-knowledge contract)", f)
		}
	}
}

func TestPersonList_OnlyOwnersPersons(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	routerA := personAPIRouter(queries)
	tokenA := loginAndGetToken(t, queries, "person-list-a@example.com")
	tokenB := loginAndGetToken(t, queries, "person-list-b@example.com")

	// User A creates two persons.
	doJSON(t, routerA, http.MethodPost, "/api/v1/persons", tokenA, map[string]interface{}{})
	doJSON(t, routerA, http.MethodPost, "/api/v1/persons", tokenA, map[string]interface{}{})
	// User B creates one person.
	doJSON(t, routerA, http.MethodPost, "/api/v1/persons", tokenB, map[string]interface{}{})

	// User A sees exactly 2.
	rec := doJSON(t, routerA, http.MethodGet, "/api/v1/persons", tokenA, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if count := int(resp["count"].(float64)); count != 2 {
		t.Errorf("user A expected 2 persons, got %d", count)
	}

	// User B sees exactly 1.
	rec = doJSON(t, routerA, http.MethodGet, "/api/v1/persons", tokenB, nil)
	json.NewDecoder(rec.Body).Decode(&resp)
	if count := int(resp["count"].(float64)); count != 1 {
		t.Errorf("user B expected 1 person, got %d", count)
	}
}

func TestPersonDelete_CannotDeleteOtherUsersPerson(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	router := personAPIRouter(queries)
	tokenA := loginAndGetToken(t, queries, "person-del-a@example.com")
	tokenB := loginAndGetToken(t, queries, "person-del-b@example.com")

	// A creates a person.
	rec := doJSON(t, router, http.MethodPost, "/api/v1/persons", tokenA, map[string]interface{}{})
	var created map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&created)
	personID := created["id"].(string)

	// B tries to delete it — must get 404 (not 403, to avoid enumeration).
	rec = doJSON(t, router, http.MethodDelete, "/api/v1/persons/"+personID, tokenB, nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for cross-user delete, got %d", rec.Code)
	}

	// A can still delete it.
	rec = doJSON(t, router, http.MethodDelete, "/api/v1/persons/"+personID, tokenA, nil)
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204 for owner delete, got %d", rec.Code)
	}
}

func TestGDACreate_VerifyRoundTrip(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "gda-create@example.com")
	router := personAPIRouter(queries)

	// Create a person first.
	rec := doJSON(t, router, http.MethodPost, "/api/v1/persons", token, map[string]interface{}{})
	var created map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&created)
	personID := created["id"].(string)

	// Mint a GDA code.
	rec = doJSON(t, router, http.MethodPost, "/api/v1/gda-codes", token, map[string]string{
		"person_id": personID,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var gdaResp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&gdaResp)
	code := gdaResp["code"].(string)

	// The returned code must be verifiable.
	if err := gda.Verify(code); err != nil {
		t.Fatalf("returned code fails verification: %v (%q)", err, code)
	}
	// And must be in canonical 4-4-4 form.
	if len(code) != gda.FormattedLength || code[4] != '-' || code[9] != '-' {
		t.Errorf("expected XXXX-XXXX-XXXX, got %q", code)
	}

	// Listing by person returns our one code.
	rec = doJSON(t, router, http.MethodGet, "/api/v1/persons/"+personID+"/gda-codes", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", rec.Code)
	}
	var listResp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&listResp)
	if count := int(listResp["count"].(float64)); count != 1 {
		t.Errorf("expected 1 code, got %d", count)
	}
}

func TestGDACreate_RejectsCrossUserPerson(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	router := personAPIRouter(queries)
	tokenA := loginAndGetToken(t, queries, "gda-a@example.com")
	tokenB := loginAndGetToken(t, queries, "gda-b@example.com")

	// A creates a person.
	rec := doJSON(t, router, http.MethodPost, "/api/v1/persons", tokenA, map[string]interface{}{})
	var created map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&created)
	personID := created["id"].(string)

	// B tries to mint a code for A's person.
	rec = doJSON(t, router, http.MethodPost, "/api/v1/gda-codes", tokenB, map[string]string{
		"person_id": personID,
	})
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for cross-user mint, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestVCardExport_ContainsExpectedFields(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "vcard@example.com")
	router := personAPIRouter(queries)

	body := map[string]interface{}{
		"names": []map[string]interface{}{
			{"family": "山口", "given": "大翔", "language_tag": "ja-Jpan"},
			{"family": "Yamaguchi", "given": "Taketo", "language_tag": "en-Latn"},
		},
		"addresses": []map[string]interface{}{
			{
				"street_address": "永田町 1-7-1",
				"locality":       "千代田区",
				"region":         "東京都",
				"postal_code":    "100-8914",
				"country":        "JP",
				"language_tag":   "ja-Jpan",
			},
		},
		"phones":   []string{"+81-3-3581-5111"},
		"gda_code": "ABCD-EFGH-JKMN",
		"filename": "taketo.vcf",
	}
	rec := doJSON(t, router, http.MethodPost, "/api/v1/vcard", token, body)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/vcard") {
		t.Errorf("expected text/vcard content type, got %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "taketo.vcf") {
		t.Errorf("expected filename in disposition, got %q", cd)
	}
	out := rec.Body.String()
	for _, want := range []string{
		"BEGIN:VCARD",
		"VERSION:4.0",
		"FN:",
		"N:山口;大翔;;;",
		"ADR;LANGUAGE=ja-Jpan:;;永田町 1-7-1;千代田区;東京都;100-8914;JP",
		"TEL;TYPE=voice:+81-3-3581-5111",
		"X-GDA-CODE:ABCD-EFGH-JKMN",
		"END:VCARD",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("vCard missing %q\n---\n%s", want, out)
		}
	}
}

func TestVCardExport_RequiresName(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "vcard-noname@example.com")
	router := personAPIRouter(queries)

	rec := doJSON(t, router, http.MethodPost, "/api/v1/vcard", token, map[string]interface{}{})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 when names missing, got %d", rec.Code)
	}
}

func TestGDADelete_HappyPath(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "gda-del@example.com")
	router := personAPIRouter(queries)

	// Create a person + mint a code.
	rec := doJSON(t, router, http.MethodPost, "/api/v1/persons", token, map[string]interface{}{})
	var person map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&person)
	personID := person["id"].(string)

	rec = doJSON(t, router, http.MethodPost, "/api/v1/gda-codes", token, map[string]string{
		"person_id": personID,
	})
	var gdaResp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&gdaResp)
	formatted := gdaResp["code"].(string)

	// Delete the code using the formatted form.
	rec = doJSON(t, router, http.MethodDelete, "/api/v1/gda-codes/"+formatted, token, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	// List should now be empty.
	rec = doJSON(t, router, http.MethodGet, "/api/v1/persons/"+personID+"/gda-codes", token, nil)
	var listResp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&listResp)
	if count := int(listResp["count"].(float64)); count != 0 {
		t.Errorf("expected 0 codes after delete, got %d", count)
	}
}

func TestGDADelete_RejectsMalformedCode(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "gda-del-bad@example.com")
	router := personAPIRouter(queries)

	rec := doJSON(t, router, http.MethodDelete, "/api/v1/gda-codes/UUUU-UUUU-UUUU", token, nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for malformed code (U is not in alphabet), got %d", rec.Code)
	}
}

func TestPersonDelete_InvalidUUID(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "person-del-bad@example.com")
	router := personAPIRouter(queries)

	rec := doJSON(t, router, http.MethodDelete, "/api/v1/persons/not-a-uuid", token, nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad UUID, got %d", rec.Code)
	}
}

func TestGDACreate_InvalidPersonIDFormat(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "gda-bad@example.com")
	router := personAPIRouter(queries)

	rec := doJSON(t, router, http.MethodPost, "/api/v1/gda-codes", token, map[string]string{
		"person_id": "not-a-uuid",
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad person_id, got %d", rec.Code)
	}
}

func TestGDACreate_MissingPersonID(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "gda-miss@example.com")
	router := personAPIRouter(queries)

	rec := doJSON(t, router, http.MethodPost, "/api/v1/gda-codes", token, map[string]string{})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing person_id, got %d", rec.Code)
	}
}

func TestVCardExport_InvalidJSON(t *testing.T) {
	queries := testutil.SetupTestQueries(t)
	token := loginAndGetToken(t, queries, "vcard-bad@example.com")
	router := personAPIRouter(queries)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/vcard", bytes.NewReader([]byte("{not json")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", rec.Code)
	}
}
