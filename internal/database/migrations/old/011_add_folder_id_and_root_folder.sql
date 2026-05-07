-- +goose Up
ALTER TABLE files ADD COLUMN folder_id INTEGER;

INSERT INTO files (client_id, user_id, filename, path, size, uploaded_at, type, public_id, folder_id)
SELECT
  c.id,
  NULL,
  c.folder_path,
  c.folder_path,
  0,
  CURRENT_TIMESTAMP,
  'folder',
  lower(hex(randomblob(16))),
  NULL
FROM clients c
WHERE NOT EXISTS (
  SELECT 1
  FROM files f
  WHERE f.client_id = c.id
    AND f.path = c.folder_path
    AND f.type = 'folder'
    AND f.deleted_at IS NULL
);

-- +goose Down
DELETE FROM files
WHERE type = 'folder'
  AND path IN (SELECT folder_path FROM clients);

-- SQLite does not support DROP COLUMN; manual migration needed if rollback required.
