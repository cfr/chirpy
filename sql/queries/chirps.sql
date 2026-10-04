-- name: CreateChirp :one
INSERT INTO chirps (id, created_at, updated_at, body, user_id)
VALUES (
    gen_random_uuid(),
    NOW(),
    NOW(),
    $1,
    $2
)
RETURNING *;

-- name: GetChirp :one
SELECT * FROM chirps
WHERE id = $1;

-- name: DeleteChirp :execrows
DELETE FROM chirps
WHERE id = $1 AND user_id = $2;

-- name: GetChirps :many
SELECT * FROM chirps
ORDER BY created_at ASC;

-- name: DeleteChirps :exec
DELETE FROM chirps;
