-- name: CreateUser :one
INSERT INTO users (
    email_hash, kdf_params, login_hash, login_salt
) VALUES (
    $1, $2, $3, $4
) RETURNING *;

-- name: GetUserByEmailHash :one
SELECT * FROM users
WHERE email_hash = $1
LIMIT 1;

-- name: GetUserByID :one
SELECT * FROM users
WHERE id = $1
LIMIT 1;
