-- +goose Up
-- +goose StatementBegin
ALTER TABLE users ADD COLUMN login_salt BYTEA;
COMMENT ON COLUMN users.login_salt IS 'Server-side Argon2id salt for login_hash';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE users DROP COLUMN IF EXISTS login_salt;
-- +goose StatementEnd
