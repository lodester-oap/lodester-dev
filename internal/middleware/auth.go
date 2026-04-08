// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/lodester-oap/lodester/internal/store"
)

type contextKey string

const userIDKey contextKey = "user_id"

// UserIDFromContext extracts the authenticated user's ID from the request context.
func UserIDFromContext(ctx context.Context) (pgtype.UUID, bool) {
	uid, ok := ctx.Value(userIDKey).(pgtype.UUID)
	return uid, ok
}

// Auth returns middleware that validates Bearer tokens against the sessions table.
func Auth(queries *store.Queries, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				writeAuthError(w, "missing or invalid authorization header")
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")
			tokenBytes, err := base64.URLEncoding.DecodeString(token)
			if err != nil {
				writeAuthError(w, "invalid token format")
				return
			}

			tokenHash := sha256.Sum256(tokenBytes)
			session, err := queries.GetSessionByTokenHash(r.Context(), tokenHash[:])
			if err != nil {
				// Token not found or expired — same error to prevent enumeration
				logger.Debug("auth failed: session not found or expired")
				writeAuthError(w, "invalid or expired token")
				return
			}

			ctx := context.WithValue(r.Context(), userIDKey, session.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func writeAuthError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{
			"code":    "UNAUTHORIZED",
			"message": message,
		},
	})
}
