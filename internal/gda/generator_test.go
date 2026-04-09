// SPDX-License-Identifier: AGPL-3.0-or-later

package gda

import (
	"strings"
	"testing"
)

func TestGenerate_Format(t *testing.T) {
	for i := 0; i < 200; i++ {
		code, err := Generate()
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if len(code) != FormattedLength {
			t.Fatalf("expected length %d, got %d (%q)", FormattedLength, len(code), code)
		}
		if code[4] != '-' || code[9] != '-' {
			t.Fatalf("expected hyphens at 4 and 9, got %q", code)
		}
		if err := Verify(code); err != nil {
			t.Fatalf("freshly generated code failed verification: %v (%q)", err, code)
		}
	}
}

func TestGenerate_Uniqueness(t *testing.T) {
	// 1000 generations should be distinct with overwhelming probability
	// (55 bits of entropy).
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		code, err := Generate()
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if _, dup := seen[code]; dup {
			t.Fatalf("duplicate code generated: %q", code)
		}
		seen[code] = struct{}{}
	}
}

func TestGenerate_AlphabetOnly(t *testing.T) {
	for i := 0; i < 100; i++ {
		code, err := Generate()
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		for j := 0; j < len(code); j++ {
			c := code[j]
			if c == '-' {
				continue
			}
			if !strings.ContainsRune(Alphabet, rune(c)) {
				t.Fatalf("character %q not in alphabet (code=%q)", c, code)
			}
		}
	}
}

func TestVerify_RejectsSingleCharChange(t *testing.T) {
	code, err := Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// Flip each non-hyphen position to every other alphabet character and
	// confirm Verify rejects every mutation.
	for i := 0; i < len(code); i++ {
		if code[i] == '-' {
			continue
		}
		for j := 0; j < len(Alphabet); j++ {
			if Alphabet[j] == code[i] {
				continue
			}
			mutated := []byte(code)
			mutated[i] = Alphabet[j]
			if err := Verify(string(mutated)); err == nil {
				t.Fatalf("mutation at %d accepted: original=%q mutated=%q",
					i, code, string(mutated))
			}
		}
	}
}

func TestVerify_AcceptsCrockfordAliases(t *testing.T) {
	code, err := Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// The raw code only contains canonical chars, but user input may
	// substitute O->0 and I/L->1. These should still verify only if the
	// canonical form is unchanged — i.e. only when the substituted char
	// matches the canonical char.
	// Smoke test: lowercased input must verify the same way.
	if err := Verify(strings.ToLower(code)); err != nil {
		t.Fatalf("lowercase input rejected: %v (%q)", err, code)
	}
	// With hyphens removed.
	if err := Verify(strings.ReplaceAll(code, "-", "")); err != nil {
		t.Fatalf("unhyphenated input rejected: %v (%q)", err, code)
	}
	// With extra spaces.
	spaced := strings.ReplaceAll(code, "-", "  ")
	if err := Verify(spaced); err != nil {
		t.Fatalf("spaced input rejected: %v (%q)", err, spaced)
	}
}

func TestVerify_LengthErrors(t *testing.T) {
	cases := []string{
		"",
		"AAAA-AAAA-AAA",      // one short
		"AAAA-AAAA-AAAAA",    // one long
		"AAAA-AAAA-AAAA-AAA", // way long
	}
	for _, c := range cases {
		if err := Verify(c); err == nil {
			t.Errorf("expected error for %q, got nil", c)
		}
	}
}

func TestVerify_InvalidCharacter(t *testing.T) {
	// 'U' is deliberately excluded from Crockford Base32.
	if err := Verify("UUUU-UUUU-UUUU"); err == nil {
		t.Fatal("expected error for 'U' characters, got nil")
	}
}

func TestNormalize_StripsHyphensAndCase(t *testing.T) {
	got, err := Normalize("abcd-efgh-jkmn")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if got != "ABCDEFGHJKMN" {
		t.Fatalf("expected ABCDEFGHJKMN, got %q", got)
	}
}

func TestLuhnMod32_KnownVector(t *testing.T) {
	// Hand-computed fixed vector so regressions are caught even if
	// the algorithm is accidentally rewritten.
	//
	// body = "00000000000" (all zeros) → every addend = 0 → check = 0 ('0').
	if c := luhnMod32("00000000000"); c != 0 {
		t.Fatalf("luhnMod32(\"0...0\") = %d, want 0", c)
	}
	// A different fixed vector: body = "1" → factor=2, cp=1, addend=2,
	// sum=2, remainder=2, check=30 ('Y').
	if c := luhnMod32("1"); c != 30 {
		t.Fatalf("luhnMod32(\"1\") = %d, want 30", c)
	}
}

func TestFormat_FromBareCode(t *testing.T) {
	// Use a known-valid code: "00000000000" + check digit '0' = all zeros.
	got, err := Format("000000000000")
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	if got != "0000-0000-0000" {
		t.Fatalf("expected 0000-0000-0000, got %q", got)
	}
}
