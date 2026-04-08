-- +goose Up
-- +goose StatementBegin

-- Vault table stores the encrypted blob per user.
-- The server NEVER decrypts this data (zero-knowledge).
-- "version" supports optimistic locking (DECISION-051).
CREATE TABLE vaults (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    data BYTEA NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_vaults_user_id ON vaults(user_id);

-- Auto-update updated_at on row modification.
CREATE OR REPLACE FUNCTION update_vault_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_vault_updated_at
    BEFORE UPDATE ON vaults
    FOR EACH ROW
    EXECUTE FUNCTION update_vault_timestamp();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS trg_vault_updated_at ON vaults;
DROP FUNCTION IF EXISTS update_vault_timestamp();
DROP TABLE IF EXISTS vaults;
-- +goose StatementEnd
