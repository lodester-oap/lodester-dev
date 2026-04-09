-- +goose Up
-- +goose StatementBegin

-- share_links stores ephemeral, end-to-end encrypted capability URLs
-- (DECISION-055). The server stores only opaque ciphertext; the decryption
-- key lives in the URL fragment and is never transmitted to the server.
--
-- The 'id' column is an application-generated URL-safe random token (see
-- internal/handler/share.go for generation). It is NOT a UUID so that the
-- resulting URL is compact enough for QR codes and short manual sharing.
--
-- Expiration policy: DECISION-056.
--   * Server returns 410 Gone once expires_at is in the past.
--   * Rows are physically removed 30 days after expiration (manual for MVP,
--     automated job in Phase 1b).
CREATE TABLE share_links (
    id          TEXT PRIMARY KEY,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    ciphertext  BYTEA NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_share_links_user_id ON share_links(user_id);
CREATE INDEX idx_share_links_expires_at ON share_links(expires_at);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS share_links;
-- +goose StatementEnd
