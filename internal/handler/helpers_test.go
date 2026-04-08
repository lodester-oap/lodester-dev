// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestNormalizeEmail(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"user@Example.COM", "user@example.com"},
		{"User@example.com", "User@example.com"},
		{"test@GMAIL.COM", "test@gmail.com"},
		{"natsign", "natsign"}, // no @ sign
		{"a@b", "a@b"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeEmail(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeEmail(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestUuidToString(t *testing.T) {
	// Valid UUID
	valid := pgtype.UUID{
		Bytes: [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
			0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
		Valid: true,
	}
	got := uuidToString(valid)
	if got == "" {
		t.Error("expected non-empty string for valid UUID")
	}

	// Invalid UUID
	invalid := pgtype.UUID{Valid: false}
	got = uuidToString(invalid)
	if got != "" {
		t.Errorf("expected empty string for invalid UUID, got %q", got)
	}
}
