-- +goose Up
ALTER TABLE
  secure_links
ADD
  COLUMN email TEXT NOT NULL DEFAULT '';

-- +goose Down
-- SQLite does not support DROP COLUMN; manual migration needed if rollback required.
