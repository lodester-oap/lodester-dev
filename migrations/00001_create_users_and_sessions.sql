-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email_hash BYTEA NOT NULL UNIQUE,
    handle VARCHAR(64) UNIQUE,             -- Phase 1b: custom handles, NULL in MVP
    kdf_params JSONB NOT NULL,
    login_hash BYTEA NOT NULL,
    mfa_enabled BOOLEAN NOT NULL DEFAULT FALSE,  -- Phase 1b: MFA support
    mfa_secret BYTEA,                             -- Phase 1b: TOTP secret
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_users_email_hash ON users(email_hash);
CREATE UNIQUE INDEX idx_users_handle_lower ON users(LOWER(handle)) WHERE handle IS NOT NULL;

CREATE TABLE sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash BYTEA NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ
);

CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE INDEX idx_sessions_token_hash ON sessions(token_hash);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
-- +goose StatementEnd
