-- +goose Up
ALTER TABLE clients ADD COLUMN public_id TEXT;
ALTER TABLE users ADD COLUMN public_id TEXT;
ALTER TABLE email_queue ADD COLUMN public_id TEXT;
ALTER TABLE file_activity ADD COLUMN public_id TEXT;

CREATE UNIQUE INDEX idx_clients_public_id ON clients(public_id);
CREATE UNIQUE INDEX idx_users_public_id ON users(public_id);
CREATE UNIQUE INDEX idx_email_queue_public_id ON email_queue(public_id);
CREATE UNIQUE INDEX idx_file_activity_public_id ON file_activity(public_id);

-- +goose Down
DROP INDEX IF EXISTS idx_file_activity_public_id;
DROP INDEX IF EXISTS idx_email_queue_public_id;
DROP INDEX IF EXISTS idx_users_public_id;
DROP INDEX IF EXISTS idx_clients_public_id;

-- SQLite does not support DROP COLUMN directly; manual migration needed if rollback required.
