// SPDX-License-Identifier: AGPL-3.0-or-later

// Package crypto — vault_header.go defines the ciphertext header schema (DECISION-049).
//
// The vault blob stored on the server is opaque: the server NEVER decrypts it.
// This header definition exists so that the server can perform basic structural
// validation on upload (to reject obviously corrupt data) and for documentation.
//
// Wire format: [4-byte header_len (big-endian)] [header JSON] [ciphertext]
//
// Header JSON:
//
//	{
//	  "v": 1,
//	  "alg": "aes-gcm-256",
//	  "kdf": "argon2id",
//	  "kdf_params": {"memory":65536,"iterations":3,"parallelism":4},
//	  "nonce": "<base64url>",
//	  "ct_len": 12345
//	}
//
// The "nonce" field stores the AES-GCM nonce (12 bytes, base64url-encoded).
// Each encryption operation MUST generate a fresh nonce via crypto/rand.
// NEVER reuse a nonce with the same key — this is catastrophic for AES-GCM.
package crypto

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
)

// VaultHeaderVersion is the current schema version.
const VaultHeaderVersion = 1

// Supported algorithms.
const (
	AlgAESGCM256 = "aes-gcm-256"
	KDFArgon2id  = "argon2id"
)

// VaultHeader is the ciphertext header (DECISION-049).
type VaultHeader struct {
	Version   int             `json:"v"`
	Algorithm string          `json:"alg"`
	KDF       string          `json:"kdf"`
	KDFParams json.RawMessage `json:"kdf_params"`
	Nonce     string          `json:"nonce"`  // base64url-encoded
	CTLen     int             `json:"ct_len"` // ciphertext length in bytes
}

// ParseVaultBlob extracts the header from a vault blob.
// It does NOT decrypt the ciphertext — the server has no key.
func ParseVaultBlob(blob []byte) (*VaultHeader, []byte, error) {
	if len(blob) < 4 {
		return nil, nil, errors.New("vault blob too short")
	}

	headerLen := int(binary.BigEndian.Uint32(blob[:4]))
	if headerLen <= 0 || headerLen > 1024*64 {
		return nil, nil, fmt.Errorf("invalid header length: %d", headerLen)
	}
	if len(blob) < 4+headerLen {
		return nil, nil, errors.New("vault blob shorter than declared header length")
	}

	var header VaultHeader
	if err := json.Unmarshal(blob[4:4+headerLen], &header); err != nil {
		return nil, nil, fmt.Errorf("invalid header JSON: %w", err)
	}

	ciphertext := blob[4+headerLen:]
	return &header, ciphertext, nil
}

// ValidateHeader performs basic structural validation on a vault header.
// The server cannot verify cryptographic correctness (it has no key),
// but it can reject obviously malformed data.
func ValidateHeader(h *VaultHeader) error {
	if h.Version < 1 {
		return fmt.Errorf("unsupported schema version: %d", h.Version)
	}
	if h.Algorithm == "" {
		return errors.New("missing algorithm")
	}
	if h.KDF == "" {
		return errors.New("missing KDF")
	}
	if h.Nonce == "" {
		return errors.New("missing nonce")
	}
	return nil
}
