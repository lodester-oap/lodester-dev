// SPDX-License-Identifier: AGPL-3.0-or-later

package vcard

import (
	"strings"
	"testing"
)

func TestExport_MinimalEnglishCard(t *testing.T) {
	card := Card{
		Names: []Name{{
			Family:      "Smith",
			Given:       "Alice",
			LanguageTag: "en-Latn",
		}},
		Addresses: []Address{{
			StreetAddress: "123 Main St",
			Locality:      "Springfield",
			Region:        "IL",
			PostalCode:    "62704",
			Country:       "US",
			LanguageTag:   "en-Latn",
		}},
		GDACode: "ABCD-EFGH-JKMN",
	}
	out := Export(card)

	mustContain(t, out, "BEGIN:VCARD\r\n")
	mustContain(t, out, "VERSION:3.0\r\n")
	mustContain(t, out, "FN:Alice Smith\r\n")
	mustContain(t, out, "N:Smith;Alice;;;\r\n")
	mustContain(t, out, "ADR:;;123 Main St;Springfield;IL;62704;US\r\n")
	mustContain(t, out, "X-GDA-CODE:ABCD-EFGH-JKMN\r\n")
	mustContain(t, out, "END:VCARD\r\n")
}

func TestExport_NoUTF8BOM(t *testing.T) {
	card := Card{
		Names: []Name{{Family: "山口", Given: "大翔", LanguageTag: "ja-Jpan"}},
	}
	out := Export(card)
	// Apple Contacts fails to parse vCard files that begin with U+FEFF
	// (empirical). The output must start with BEGIN:VCARD, not a BOM.
	if strings.HasPrefix(out, "\ufeff") {
		t.Fatal("output must not be prefixed with a UTF-8 BOM")
	}
	if !strings.HasPrefix(out, "BEGIN:VCARD\r\n") {
		t.Fatalf("output must start with BEGIN:VCARD, got %q...", out[:16])
	}
}

func TestExport_JapaneseFirstName(t *testing.T) {
	card := Card{
		Names: []Name{{
			Family:      "山口",
			Given:       "大翔",
			LanguageTag: "ja-Jpan",
		}},
	}
	out := Export(card)

	// CJK: "山口 大翔" (family first, space then given)
	if !strings.Contains(out, "FN:山口 大翔\r\n") {
		t.Fatalf("expected CJK-ordered FN, got:\n%s", out)
	}
	if !strings.Contains(out, "N:山口;大翔;;;\r\n") {
		t.Fatalf("expected N with family first, got:\n%s", out)
	}
}

func TestExport_MultiScriptDropsSecondary(t *testing.T) {
	card := Card{
		Names: []Name{
			{Family: "山口", Given: "大翔", LanguageTag: "ja-Jpan"},
			{Family: "Yamaguchi", Given: "Taketo", LanguageTag: "ja-Latn"},
		},
	}
	out := Export(card)

	// Only the primary N is emitted (vCard 3.0 has no ALTID).
	mustContain(t, out, "N:山口;大翔;;;\r\n")
	// FN still prefers the Latin variant for business-card compatibility.
	mustContain(t, out, "FN:Taketo Yamaguchi\r\n")
	// ALTID must never appear.
	if strings.Contains(out, "ALTID") {
		t.Fatalf("vCard 3.0 output must not contain ALTID, got:\n%s", out)
	}
	// The structured secondary N must not leak into the output.
	if strings.Contains(out, "N;") || strings.Contains(out, "Yamaguchi;Taketo") {
		t.Fatalf("secondary N variant leaked into output:\n%s", out)
	}
}

func TestExport_EscapesSpecialCharacters(t *testing.T) {
	card := Card{
		Names: []Name{{Family: "O;Brien", Given: "Sean, Jr."}},
		Note:  "Line 1\nLine 2",
	}
	out := Export(card)

	// Semicolons and commas must be escaped in structured values.
	if !strings.Contains(out, `N:O\;Brien;Sean\, Jr.;;;`) {
		t.Fatalf("expected escaped N field, got:\n%s", out)
	}
	// Newlines in NOTE escape to \n.
	if !strings.Contains(out, `NOTE:Line 1\nLine 2`) {
		t.Fatalf("expected escaped newline in NOTE, got:\n%s", out)
	}
}

func TestExport_CRLFLineEndings(t *testing.T) {
	card := Card{
		Names: []Name{{Family: "Test", Given: "User"}},
	}
	out := Export(card)
	if strings.Contains(out, "\n") && !strings.Contains(out, "\r\n") {
		t.Fatal("expected CRLF line endings per RFC 6350")
	}
	// Every raw \n must be preceded by \r (no stray LF).
	for i, c := range out {
		if c == '\n' && (i == 0 || out[i-1] != '\r') {
			t.Fatalf("LF not preceded by CR at index %d", i)
		}
	}
}

func TestExport_OmitsEmptyOptionalFields(t *testing.T) {
	card := Card{
		Names: []Name{{Family: "Test", Given: "User"}},
		Orgs:  []string{""}, // intentionally blank
	}
	out := Export(card)
	if strings.Contains(out, "ORG:") {
		t.Fatal("blank ORG should be omitted")
	}
	if strings.Contains(out, "X-GDA-CODE:") {
		t.Fatal("empty GDA code should be omitted")
	}
}

func mustContain(t *testing.T, out, substr string) {
	t.Helper()
	if !strings.Contains(out, substr) {
		t.Fatalf("output missing %q\n---\n%s", substr, out)
	}
}
