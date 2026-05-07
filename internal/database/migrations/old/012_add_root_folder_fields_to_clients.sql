-- +goose Up
ALTER TABLE clients ADD COLUMN root_folder_id INTEGER;
ALTER TABLE clients ADD COLUMN root_folder_public_id TEXT;

UPDATE clients
SET root_folder_id = (
  SELECT f.id
  FROM files f
  WHERE f.client_id = clients.id
    AND f.path = clients.folder_path
    AND f.type = 'folder'
    AND f.deleted_at IS NULL
  LIMIT 1
),
root_folder_public_id = (
  SELECT f.public_id
  FROM files f
  WHERE f.client_id = clients.id
    AND f.path = clients.folder_path
    AND f.type = 'folder'
    AND f.deleted_at IS NULL
  LIMIT 1
)
WHERE root_folder_id IS NULL
   OR root_folder_public_id IS NULL;

-- +goose Down
-- SQLite does not support DROP COLUMN; manual migration needed if rollback required.
