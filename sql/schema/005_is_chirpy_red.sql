-- +goose Up
ALTER TABLE users
ADD is_chirpy_red BOOL NOT NULL
DEFAULT FALSE;

-- +goose Down
ALTER TABLE users
DROP COLUMN is_chirpy_red;
