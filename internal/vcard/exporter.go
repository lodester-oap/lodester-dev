// SPDX-License-Identifier: AGPL-3.0-or-later

// Package vcard implements a minimal RFC 2426 (vCard 3.0) exporter
// tailored to Lodester's Person / Address data model.
//
// Design notes:
//   - We emit vCard *3.0*, not 4.0, because Apple Contacts on iOS /
//     macOS reads 4.0 files with legacy encoding assumptions and
//     mojibakes CJK text. Empirically, 3.0 is what Apple's parser
//     treats as first class, and it is also what Google Contacts
//     exports by default. See docs/vcard-compatibility.md.
//   - Lodester runs client-side (the server never sees decrypted
//     personal data), so this package is intentionally pure and free of
//     HTTP dependencies. Callers construct a Card from in-memory values
//     already decrypted from the user's Vault.
//   - The X-GDA-CODE custom field is added so that vCard consumers who
//     understand Lodester can round-trip the address identifier. Other
//     consumers will treat it as an opaque extension and preserve it.
//   - Multi-script name / address variants: vCard 3.0 has no ALTID,
//     and Apple Contacts does not display ALTID siblings even under
//     vCard 4.0. We therefore emit only the primary (first) variant
//     and document the trade-off. Phase 1b may revisit this with
//     X-PHONETIC-* extensions or a per-user "preferred script" switch.
package vcard

import (
	"strings"
)

// Name represents a single script variant of a person's name.
// The LanguageTag is an optional BCP 47 value (e.g. "ja-Jpan", "ja-Latn").
type Name struct {
	Family      string `json:"family"`
	Given       string `json:"given"`
	Additional  string `json:"additional,omitempty"`
	Prefix      string `json:"prefix,omitempty"`
	Suffix      string `json:"suffix,omitempty"`
	LanguageTag string `json:"language_tag,omitempty"`
}

// Address represents one libaddressinput address letter set.
// Fields correspond to the vCard ADR structured value per RFC 6350 § 6.3.1.
type Address struct {
	PostOfficeBox   string `json:"post_office_box,omitempty"`
	ExtendedAddress string `json:"extended_address,omitempty"`
	StreetAddress   string `json:"street_address,omitempty"` // libaddressinput A, joined
	Locality        string `json:"locality,omitempty"`       // C
	Region          string `json:"region,omitempty"`         // S
	PostalCode      string `json:"postal_code,omitempty"`    // Z
	Country         string `json:"country,omitempty"`        // ISO 3166-1 alpha-2 or country name
	LanguageTag     string `json:"language_tag,omitempty"`   // BCP 47 script, e.g. "ja-Jpan"
}

// Card is a single Person plus its addresses, phones, and the minted
// GDA identifier. Only the Names slice is required — everything else
// may be empty.
type Card struct {
	Names     []Name
	Orgs      []string
	Phones    []string
	Emails    []string
	Addresses []Address
	Note      string
	GDACode   string // already formatted as XXXX-XXXX-XXXX
}

// Export serializes the card as a vCard 3.0 document using CRLF line
// endings as required by RFC 2426.
//
// We deliberately avoid a UTF-8 BOM. Apple Contacts fails to parse
// vCard files that begin with U+FEFF (empirically confirmed on iOS);
// without a BOM and with VERSION:3.0 it reads UTF-8 text correctly.
func Export(card Card) string {
	var b strings.Builder
	writeLine(&b, "BEGIN:VCARD")
	writeLine(&b, "VERSION:3.0")

	// FN is mandatory per RFC 2426 § 3.1.1. We pick the best readable
	// full name (preferring a Latin variant if one was supplied, for
	// broad business-card compatibility).
	fn := formatFN(card)
	writeLine(&b, "FN:"+escape(fn))

	// vCard 3.0 has no ALTID — only the primary (first) N is emitted.
	if len(card.Names) > 0 {
		writeLine(&b, "N:"+joinN(card.Names[0]))
	}

	for _, o := range card.Orgs {
		if o == "" {
			continue
		}
		writeLine(&b, "ORG:"+escape(o))
	}
	for _, p := range card.Phones {
		if p == "" {
			continue
		}
		writeLine(&b, "TEL;TYPE=voice:"+escape(p))
	}
	for _, e := range card.Emails {
		if e == "" {
			continue
		}
		writeLine(&b, "EMAIL:"+escape(e))
	}
	// vCard 3.0 has no ALTID — only the primary (first) ADR is emitted.
	// iOS also renders a LANGUAGE parameter as the field label, so we
	// emit a bare ADR regardless of the stored LanguageTag.
	if len(card.Addresses) > 0 {
		writeLine(&b, "ADR:"+joinADR(card.Addresses[0]))
	}
	if card.Note != "" {
		writeLine(&b, "NOTE:"+escape(card.Note))
	}
	if card.GDACode != "" {
		// Custom field. RFC 6350 § 6.10 permits X- prefixed experimental
		// properties. Consumers that do not recognise it SHOULD preserve
		// it verbatim.
		writeLine(&b, "X-GDA-CODE:"+escape(card.GDACode))
	}
	writeLine(&b, "END:VCARD")
	return b.String()
}

// formatFN picks the best human-readable full name for the FN field.
// Order: first non-empty Latin or unspecified language, else first variant.
func formatFN(card Card) string {
	if len(card.Names) == 0 {
		return ""
	}
	// Prefer a Latin variant for broad compatibility, otherwise first.
	for _, n := range card.Names {
		if strings.Contains(strings.ToLower(n.LanguageTag), "latn") {
			return joinFN(n)
		}
	}
	return joinFN(card.Names[0])
}

// joinFN composes a single human-readable "Given Family" (or family-first
// for CJK scripts) string.
func joinFN(n Name) string {
	if n.Family == "" && n.Given == "" {
		return strings.TrimSpace(n.Prefix + " " + n.Suffix)
	}
	// Heuristic: CJK scripts write family first, no separator.
	if isCJKLanguage(n.LanguageTag) {
		return strings.TrimSpace(n.Family + " " + n.Given)
	}
	parts := []string{n.Prefix, n.Given, n.Additional, n.Family, n.Suffix}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " ")
}

// joinN builds the structured N value per RFC 6350 § 6.2.2:
// N:Family;Given;Additional;Prefix;Suffix
func joinN(n Name) string {
	return strings.Join([]string{
		escape(n.Family),
		escape(n.Given),
		escape(n.Additional),
		escape(n.Prefix),
		escape(n.Suffix),
	}, ";")
}

// joinADR builds the structured ADR value per RFC 6350 § 6.3.1:
// ADR:PostOfficeBox;ExtendedAddress;StreetAddress;Locality;Region;PostalCode;Country
func joinADR(a Address) string {
	return strings.Join([]string{
		escape(a.PostOfficeBox),
		escape(a.ExtendedAddress),
		escape(a.StreetAddress),
		escape(a.Locality),
		escape(a.Region),
		escape(a.PostalCode),
		escape(a.Country),
	}, ";")
}

func isCJKLanguage(tag string) bool {
	l := strings.ToLower(tag)
	switch {
	case strings.HasPrefix(l, "ja-jpan"),
		strings.HasPrefix(l, "zh-hans"),
		strings.HasPrefix(l, "zh-hant"),
		strings.HasPrefix(l, "ko-hang"),
		strings.HasPrefix(l, "ko-kore"):
		return true
	}
	// Bare language codes: ja / zh / ko default to CJK if no script tag.
	if l == "ja" || l == "zh" || l == "ko" {
		return true
	}
	return false
}

// escape implements the text-value escape rules from RFC 6350 § 3.4:
//   - backslash   → \\
//   - comma       → \,
//   - semicolon   → \;
//   - newline     → \n
func escape(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`,`, `\,`,
		`;`, `\;`,
		"\r\n", `\n`,
		"\n", `\n`,
		"\r", `\n`,
	)
	return r.Replace(s)
}

// writeLine appends a line with the mandatory CRLF line terminator.
// RFC 6350 recommends (but does not mandate) folding long lines at 75
// octets; callers with long strings may add folding if needed. For
// Lodester's current scope this is kept simple.
func writeLine(b *strings.Builder, line string) {
	b.WriteString(line)
	b.WriteString("\r\n")
}
