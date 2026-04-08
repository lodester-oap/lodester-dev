// SPDX-License-Identifier: AGPL-3.0-or-later

package testutil

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lodester-oap/lodester/internal/store"
)

// SetupTestDB returns a pgxpool.Pool connected to the test database.
// It skips the test if TEST_DATABASE_URL is not set.
func SetupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL or DATABASE_URL not set, skipping DB test")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("failed to connect to test DB: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// SetupTestQueries returns a store.Queries backed by the test database.
// It also cleans up users and sessions tables before returning.
func SetupTestQueries(t *testing.T) *store.Queries {
	t.Helper()
	pool := SetupTestDB(t)

	// Clean tables for test isolation
	ctx := context.Background()
	_, err := pool.Exec(ctx, "DELETE FROM sessions")
	if err != nil {
		t.Fatalf("failed to clean sessions: %v", err)
	}
	_, err = pool.Exec(ctx, "DELETE FROM users")
	if err != nil {
		t.Fatalf("failed to clean users: %v", err)
	}

	return store.New(pool)
}
