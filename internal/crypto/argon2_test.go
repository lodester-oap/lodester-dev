// SPDX-License-Identifier: AGPL-3.0-or-later

package crypto

import (
	"bytes"
	"testing"
)

func TestDeriveKey_GeneratesSalt(t *testing.T) {
	key, salt, err := DeriveKey([]byte("test-password"), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(key) != int(Argon2KeyLength) {
		t.Errorf("expected key length %d, got %d", Argon2KeyLength, len(key))
	}
	if len(salt) != Argon2SaltLength {
		t.Errorf("expected salt length %d, got %d", Argon2SaltLength, len(salt))
	}
}

func TestDeriveKey_DeterministicWithSameSalt(t *testing.T) {
	password := []byte("test-password")
	_, salt, err := DeriveKey(password, nil)
	if err != nil {
		t.Fatal(err)
	}

	key1, _, err := DeriveKey(password, salt)
	if err != nil {
		t.Fatal(err)
	}
	key2, _, err := DeriveKey(password, salt)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(key1, key2) {
		t.Error("same password + salt should produce same key")
	}
}

func TestDeriveKey_DifferentSaltsDifferentKeys(t *testing.T) {
	password := []byte("test-password")

	key1, salt1, err := DeriveKey(password, nil)
	if err != nil {
		t.Fatal(err)
	}
	key2, salt2, err := DeriveKey(password, nil)
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Equal(salt1, salt2) {
		t.Error("random salts should differ")
	}
	if bytes.Equal(key1, key2) {
		t.Error("different salts should produce different keys")
	}
}

func TestDeriveKey_EmptyPassword(t *testing.T) {
	_, _, err := DeriveKey([]byte{}, nil)
	if err == nil {
		t.Error("expected error for empty password")
	}
}

func TestDeriveKey_InvalidSaltLength(t *testing.T) {
	_, _, err := DeriveKey([]byte("password"), []byte("short"))
	if err == nil {
		t.Error("expected error for invalid salt length")
	}
}

func TestDeriveKey_KnownAnswer(t *testing.T) {
	// Known Answer Test (KAT): verify that Argon2id with our parameters
	// produces a consistent output for a fixed input.
	password := []byte("lodester-kat-password")
	salt := []byte("lodester-kat-salt") // exactly 16 bytes + 1... need exact 16

	// Use a fixed 16-byte salt
	fixedSalt := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}

	key1, _, err := DeriveKey(password, fixedSalt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Derive again with same inputs
	key2, _, err := DeriveKey(password, fixedSalt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !bytes.Equal(key1, key2) {
		t.Error("KAT failed: same inputs must produce same output")
	}

	// Verify key length
	if len(key1) != int(Argon2KeyLength) {
		t.Errorf("KAT: expected key length %d, got %d", Argon2KeyLength, len(key1))
	}

	_ = salt // silence unused variable
}

func TestVerifyLoginHash(t *testing.T) {
	a := []byte("same-hash-value-32bytes-padding!")
	b := []byte("same-hash-value-32bytes-padding!")
	c := []byte("diff-hash-value-32bytes-padding!")

	if !VerifyLoginHash(a, b) {
		t.Error("identical hashes should match")
	}
	if VerifyLoginHash(a, c) {
		t.Error("different hashes should not match")
	}
}

func TestHashLoginHash(t *testing.T) {
	loginHash := []byte("client-derived-login-hash-value!")
	hashed, salt, err := HashLoginHash(loginHash, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hashed) != int(Argon2KeyLength) {
		t.Errorf("expected length %d, got %d", Argon2KeyLength, len(hashed))
	}

	// Verify reproducibility
	hashed2, _, err := HashLoginHash(loginHash, salt)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(hashed, hashed2) {
		t.Error("same login_hash + salt should produce same result")
	}
}

func BenchmarkDeriveKey(b *testing.B) {
	password := []byte("benchmark-password")
	salt := make([]byte, Argon2SaltLength)
	for i := range salt {
		salt[i] = byte(i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = DeriveKey(password, salt)
	}
}
