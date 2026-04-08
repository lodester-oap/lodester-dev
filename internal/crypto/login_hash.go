// SPDX-License-Identifier: AGPL-3.0-or-later

package crypto

import (
	"crypto/subtle"
)

// HashLoginHash hashes the client-provided login_hash with a server-side
// Argon2id for additional protection. This means that even if the DB is
// leaked, the login_hash cannot be used directly.
func HashLoginHash(loginHash, salt []byte) (hashed, usedSalt []byte, err error) {
	return DeriveKey(loginHash, salt)
}

// VerifyLoginHash compares received login hash against expected using
// constant-time comparison to prevent timing attacks.
func VerifyLoginHash(received, expected []byte) bool {
	return subtle.ConstantTimeCompare(received, expected) == 1
}
