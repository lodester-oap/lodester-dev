// SPDX-License-Identifier: AGPL-3.0-or-later

package crypto

import (
	"encoding/binary"
	"encoding/json"
	"testing"
)

func makeBlob(header VaultHeader, ciphertext []byte) []byte {
	headerJSON, _ := json.Marshal(header)
	headerLen := make([]byte, 4)
	binary.BigEndian.PutUint32(headerLen, uint32(len(headerJSON)))
	blob := append(headerLen, headerJSON...)
	blob = append(blob, ciphertext...)
	return blob
}

func TestParseVaultBlob_Valid(t *testing.T) {
	header := VaultHeader{
		Version:   1,
		Algorithm: AlgAESGCM256,
		KDF:       KDFArgon2id,
		KDFParams: json.RawMessage(`{"memory":65536,"iterations":3,"parallelism":4}`),
		Nonce:     "dGVzdG5vbmNlMTIz", // base64url
		CTLen:     100,
	}
	ct := make([]byte, 100)
	blob := makeBlob(header, ct)

	parsed, ciphertext, err := ParseVaultBlob(blob)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.Version != 1 {
		t.Errorf("expected version 1, got %d", parsed.Version)
	}
	if parsed.Algorithm != AlgAESGCM256 {
		t.Errorf("expected algorithm %s, got %s", AlgAESGCM256, parsed.Algorithm)
	}
	if len(ciphertext) != 100 {
		t.Errorf("expected 100 bytes ciphertext, got %d", len(ciphertext))
	}
}

func TestParseVaultBlob_TooShort(t *testing.T) {
	_, _, err := ParseVaultBlob([]byte{0, 0})
	if err == nil {
		t.Fatal("expected error for short blob")
	}
}

func TestParseVaultBlob_InvalidHeaderLen(t *testing.T) {
	// Header length larger than blob
	blob := make([]byte, 8)
	binary.BigEndian.PutUint32(blob[:4], 9999)
	_, _, err := ParseVaultBlob(blob)
	if err == nil {
		t.Fatal("expected error for invalid header length")
	}
}

func TestParseVaultBlob_InvalidJSON(t *testing.T) {
	data := []byte("not json at all!!")
	headerLen := make([]byte, 4)
	binary.BigEndian.PutUint32(headerLen, uint32(len(data)))
	blob := append(headerLen, data...)
	_, _, err := ParseVaultBlob(blob)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestValidateHeader_Valid(t *testing.T) {
	h := &VaultHeader{
		Version:   1,
		Algorithm: AlgAESGCM256,
		KDF:       KDFArgon2id,
		Nonce:     "abc123",
	}
	if err := ValidateHeader(h); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateHeader_MissingFields(t *testing.T) {
	tests := []struct {
		name   string
		header VaultHeader
	}{
		{"missing algorithm", VaultHeader{Version: 1, KDF: "a", Nonce: "n"}},
		{"missing kdf", VaultHeader{Version: 1, Algorithm: "a", Nonce: "n"}},
		{"missing nonce", VaultHeader{Version: 1, Algorithm: "a", KDF: "k"}},
		{"bad version", VaultHeader{Version: 0, Algorithm: "a", KDF: "k", Nonce: "n"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateHeader(&tt.header); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}
