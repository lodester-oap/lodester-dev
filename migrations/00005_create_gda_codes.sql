-- +goose Up
-- +goose StatementBegin

-- gda_codes binds a public GDA identifier to a person row.
-- Per DECISION-053, GDA codes are PUBLIC (they carry no personal
-- information) and so may live in a plaintext table. The binding is
-- separate from persons so that codes can be rotated or retired
-- without disturbing the minimal persons row (DECISION-052).
CREATE TABLE gda_codes (
    code       TEXT PRIMARY KEY,
    person_id  UUID NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_gda_codes_person_id ON gda_codes(person_id);
CREATE INDEX idx_gda_codes_user_id ON gda_codes(user_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS gda_codes;
-- +goose StatementEnd
