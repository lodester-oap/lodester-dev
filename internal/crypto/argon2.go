// SPDX-License-Identifier: AGPL-3.0-or-later

package crypto

import (
	"crypto/rand"
	"errors"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters (DECISION-045).
// OWASP 2024 recommended, matches Bitwarden defaults.
const (
	Argon2Memory      uint32 = 65536 // 64 MB
	Argon2Iterations  uint32 = 3
	Argon2Parallelism uint8  = 4
	Argon2SaltLength         = 16 // bytes
	Argon2KeyLength   uint32 = 32 // bytes
)

// DeriveKey derives a key from password using Argon2id.
// If salt is nil, a random salt is generated using crypto/rand.
func DeriveKey(password, salt []byte) (key, usedSalt []byte, err error) {
	if len(password) == 0 {
		return nil, nil, errors.New("password is required")
	}

	if salt == nil {
		salt = make([]byte, Argon2SaltLength)
		if _, err := rand.Read(salt); err != nil {
			return nil, nil, err
		}
	} else if len(salt) != Argon2SaltLength {
		return nil, nil, errors.New("invalid salt length")
	}

	key = argon2.IDKey(
		password, salt,
		Argon2Iterations, Argon2Memory, Argon2Parallelism, Argon2KeyLength,
	)
	return key, salt, nil
}
