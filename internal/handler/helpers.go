// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

// uuidToString converts a pgtype.UUID to its string representation.
func uuidToString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// normalizeEmail normalizes an email address before hashing (DECISION-046).
// Domain part is lowercased; local part is kept as-is (per RFC 5321).
func normalizeEmail(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return email
	}
	return parts[0] + "@" + strings.ToLower(parts[1])
}
