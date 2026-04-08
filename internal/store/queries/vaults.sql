-- name: GetVaultByUserID :one
SELECT * FROM vaults
WHERE user_id = $1
LIMIT 1;

-- name: UpsertVault :one
-- Creates or updates the vault. On update, requires version match (optimistic locking, DECISION-051).
-- The version is incremented automatically.
INSERT INTO vaults (user_id, data, version)
VALUES ($1, $2, 1)
ON CONFLICT (user_id)
DO UPDATE SET
    data = $2,
    version = vaults.version + 1
WHERE vaults.version = $3
RETURNING *;
