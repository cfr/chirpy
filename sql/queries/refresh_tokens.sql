-- name: SaveRefreshToken :one
INSERT INTO refresh_tokens (token, created_at, updated_at, user_id, expires_at, revoked_at)
VALUES (
    $1,
    NOW(),
    NOW(),
    $2,
    $3,
    NULL
)
ON CONFLICT(user_id)
DO UPDATE SET
    token = EXCLUDED.token,
    updated_at = NOW(),
    expires_at = EXCLUDED.expires_at,
    revoked_at = NULL
RETURNING *;

-- name: GetRefreshToken :one
SELECT * FROM refresh_tokens
WHERE refresh_tokens.token = $1;

-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens
SET revoked_at = NOW(), updated_at = NOW()
WHERE token = $1;

-- name: DeleteRefreshTokens :exec
DELETE FROM refresh_tokens;
