-- name: CreateUser :one
INSERT INTO users (id, created_at, updated_at, email, hashed_password)
VALUES (
    gen_random_uuid(),
    NOW(),
    NOW(),
    $1,
    $2
)
RETURNING *;

-- name: UserByEmail :one
SELECT * FROM users
WHERE users.email = $1;

-- name: DeleteUsers :exec
DELETE FROM users;
