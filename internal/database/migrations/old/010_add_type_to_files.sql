-- +goose Up
ALTER TABLE
  files
ADD
  COLUMN TYPE TEXT NOT NULL DEFAULT 'file';

UPDATE
  files
SET
  TYPE = 'file'
WHERE
  TYPE IS NULL
  OR TYPE = '';

-- +goose Down
ALTER TABLE
  files DROP COLUMN TYPE;

-- SQLite does not support DROP COLUMN directly; manual migration needed if rollback required.
