-- name: CreatePerson :one
-- Creates a minimal person row owned by the given user.
-- All sensitive fields (name, addresses, phone, notes) live in the Vault (DECISION-052).
INSERT INTO persons (user_id)
VALUES ($1)
RETURNING *;

-- name: GetPersonByID :one
SELECT * FROM persons
WHERE id = $1
LIMIT 1;

-- name: ListPersonsByUserID :many
SELECT * FROM persons
WHERE user_id = $1
ORDER BY created_at ASC;

-- name: DeletePerson :exec
-- Deletes a person owned by the given user. Returns no rows; callers must check
-- the affected row count to detect missing or foreign persons.
DELETE FROM persons
WHERE id = $1 AND user_id = $2;

-- name: TouchPerson :one
-- Bumps updated_at (e.g. after vault rewrite affecting this person).
UPDATE persons
SET updated_at = NOW()
WHERE id = $1 AND user_id = $2
RETURNING *;
