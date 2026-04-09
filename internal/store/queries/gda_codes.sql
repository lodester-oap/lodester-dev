-- name: CreateGDACode :one
-- Persists a freshly generated GDA code bound to (person_id, user_id).
-- The canonical 12-character (no hyphens, uppercase) form is stored.
INSERT INTO gda_codes (code, person_id, user_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetGDACodeByCode :one
SELECT * FROM gda_codes
WHERE code = $1
LIMIT 1;

-- name: ListGDACodesByPersonID :many
SELECT * FROM gda_codes
WHERE person_id = $1 AND user_id = $2
ORDER BY created_at ASC;

-- name: ListGDACodesByUserID :many
SELECT * FROM gda_codes
WHERE user_id = $1
ORDER BY created_at ASC;

-- name: DeleteGDACode :exec
DELETE FROM gda_codes
WHERE code = $1 AND user_id = $2;
