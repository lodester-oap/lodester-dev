// SPDX-License-Identifier: AGPL-3.0-or-later

// Package gda implements the Global Distinct Address code format defined in
// DECISION-053.
//
// A GDA code is an 11-character Crockford Base32 random identifier followed by
// a single Luhn mod 32 check digit. For display and user input the 12
// characters are grouped into three blocks of four separated by hyphens:
//
//	XXXX-XXXX-XXXX
//
// Crockford Base32 was chosen because it removes the visually ambiguous
// letters I, L, O and U; Luhn mod 32 detects every single-character typo and
// most adjacent transpositions.
//
// GDA codes are PUBLIC identifiers. They MUST NOT encode any personal
// information — a separate mapping table on the server binds them to a
// person UUID, and the person's actual attributes live in the encrypted
// Vault blob (DECISION-052).
package gda

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
)

// Alphabet is the Crockford Base32 character set (no I, L, O, U).
const Alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// Base is the radix for the encoding and Luhn checksum.
const Base = 32

// RawLength is the number of random characters before the check digit.
const RawLength = 11

// TotalLength is the full code length including the check digit.
const TotalLength = RawLength + 1

// FormattedLength is the length of the canonical XXXX-XXXX-XXXX form.
const FormattedLength = TotalLength + 2

// ErrInvalidLength indicates the decoded input is not exactly TotalLength
// characters after normalization.
var ErrInvalidLength = errors.New("gda: code must be 12 characters after normalization")

// ErrInvalidCharacter indicates a character outside the Crockford Base32
// alphabet appeared in the input.
var ErrInvalidCharacter = errors.New("gda: invalid character in code")

// ErrChecksumMismatch indicates the Luhn mod 32 check digit did not match.
var ErrChecksumMismatch = errors.New("gda: checksum mismatch")

// charValues maps alphabet characters to their 0..31 values. Crockford's
// forgiving aliases are applied: O -> 0, I -> 1, L -> 1.
var charValues = buildCharValues()

func buildCharValues() map[byte]int {
	m := make(map[byte]int, 64)
	for i := 0; i < Base; i++ {
		c := Alphabet[i]
		m[c] = i
	}
	// Crockford forgiving aliases (decode only).
	m['O'] = 0
	m['I'] = 1
	m['L'] = 1
	return m
}

// Generate returns a freshly generated GDA code in the canonical 4-4-4
// hyphenated form. It uses crypto/rand for entropy and panics only if the
// operating system's RNG is unavailable (which would indicate a severe
// environment failure).
func Generate() (string, error) {
	raw, err := randomBase32(RawLength)
	if err != nil {
		return "", fmt.Errorf("gda: random source failure: %w", err)
	}
	check := luhnMod32(raw)
	code := raw + string(Alphabet[check])
	return Format(code)
}

// randomBase32 produces n Crockford Base32 characters from crypto/rand.
// Each position is drawn from a uniform 0..31 distribution by masking the
// low 5 bits of a random byte (which is already uniform, so no rejection
// sampling is needed).
func randomBase32(n int) (string, error) {
	if n <= 0 {
		return "", nil
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		out[i] = Alphabet[buf[i]&0x1F]
	}
	return string(out), nil
}

// luhnMod32 computes the Luhn mod 32 check digit for the given raw body.
// The body is assumed to consist entirely of uppercase canonical alphabet
// characters (as produced by randomBase32). The returned value is the
// numeric code point of the check character in [0, 32).
//
// Algorithm (per ISO/IEC 7812-1 generalized by IBM's "Luhn mod N"):
//
//	factor = 2
//	sum    = 0
//	for each code point right to left:
//	    addend = factor * codePoint
//	    factor = (factor == 2) ? 1 : 2
//	    addend = (addend / Base) + (addend % Base)
//	    sum   += addend
//	remainder = sum % Base
//	check     = (Base - remainder) % Base
func luhnMod32(body string) int {
	factor := 2
	sum := 0
	for i := len(body) - 1; i >= 0; i-- {
		cp, ok := charValues[body[i]]
		if !ok {
			// Caller is responsible; return a deterministic sentinel that will
			// make downstream verification fail rather than panicking.
			return -1
		}
		addend := factor * cp
		if factor == 2 {
			factor = 1
		} else {
			factor = 2
		}
		addend = (addend / Base) + (addend % Base)
		sum += addend
	}
	remainder := sum % Base
	return (Base - remainder) % Base
}

// Verify returns nil if the input is a syntactically valid GDA code (with or
// without hyphens, any case, Crockford forgiving aliases honoured) and its
// check digit matches.
func Verify(code string) error {
	norm, err := normalize(code)
	if err != nil {
		return err
	}
	body := norm[:RawLength]
	expected := Alphabet[luhnMod32(body)]
	if norm[RawLength] != expected {
		return ErrChecksumMismatch
	}
	return nil
}

// Normalize returns the 12-character canonical (no hyphens, uppercase)
// representation of a code. It does NOT verify the checksum.
func Normalize(code string) (string, error) {
	return normalize(code)
}

func normalize(code string) (string, error) {
	// Strip whitespace and hyphens, uppercase.
	var b strings.Builder
	b.Grow(TotalLength)
	for i := 0; i < len(code); i++ {
		c := code[i]
		switch {
		case c == '-' || c == ' ':
			continue
		case c >= 'a' && c <= 'z':
			c = c - 'a' + 'A'
		}
		if _, ok := charValues[c]; !ok {
			return "", ErrInvalidCharacter
		}
		b.WriteByte(c)
	}
	norm := b.String()
	if len(norm) != TotalLength {
		return "", ErrInvalidLength
	}
	return norm, nil
}

// Format inserts the canonical XXXX-XXXX-XXXX hyphens. The input may be
// either the bare 12-character code or the already-formatted form.
func Format(code string) (string, error) {
	norm, err := normalize(code)
	if err != nil {
		return "", err
	}
	return norm[0:4] + "-" + norm[4:8] + "-" + norm[8:12], nil
}
