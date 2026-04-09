-- +goose Up
-- +goose StatementBegin

-- Persons table holds only the minimal metadata needed for ownership and
-- referential integrity. All sensitive fields (names, phone numbers, notes,
-- and the linked addresses) live inside the user's encrypted Vault blob.
-- Per DECISION-052, the server MUST remain ignorant of person content.
--
-- Marcus design: id / user_id / created_at / updated_at only.
CREATE TABLE persons (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_persons_user_id ON persons(user_id);

-- Generic updated_at trigger function for tables added from M4 onward.
-- The earlier vault-specific function (update_vault_timestamp) is kept as-is
-- per DECISION-047 (existing migrations must not be rewritten).
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_persons_updated_at
    BEFORE UPDATE ON persons
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS trg_persons_updated_at ON persons;
DROP TABLE IF EXISTS persons;
DROP FUNCTION IF EXISTS set_updated_at();
-- +goose StatementEnd
