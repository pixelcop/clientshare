-- +goose Up
ALTER TABLE
  files
ADD
  COLUMN public_id TEXT;

CREATE UNIQUE INDEX idx_files_public_id ON files(public_id);

-- +goose Down
DROP INDEX IF EXISTS idx_files_public_id;

-- SQLite does not support DROP COLUMN directly; manual migration needed if rollback required.
