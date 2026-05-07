-- +goose Up
ALTER TABLE
  clients
ADD
  COLUMN id_ulid TEXT;

ALTER TABLE
  clients
ADD
  COLUMN root_folder_id_ulid TEXT;

ALTER TABLE
  users
ADD
  COLUMN id_ulid TEXT;

ALTER TABLE
  users
ADD
  COLUMN client_id_ulid TEXT;

ALTER TABLE
  files
ADD
  COLUMN id_ulid TEXT;

ALTER TABLE
  files
ADD
  COLUMN client_id_ulid TEXT;

ALTER TABLE
  files
ADD
  COLUMN user_id_ulid TEXT;

ALTER TABLE
  files
ADD
  COLUMN folder_id_ulid TEXT;

ALTER TABLE
  secure_links
ADD
  COLUMN id_ulid TEXT;

ALTER TABLE
  secure_links
ADD
  COLUMN client_id_ulid TEXT;

ALTER TABLE
  secure_links
ADD
  COLUMN created_by_ulid TEXT;

ALTER TABLE
  file_activity
ADD
  COLUMN id_ulid TEXT;

ALTER TABLE
  file_activity
ADD
  COLUMN client_id_ulid TEXT;

ALTER TABLE
  file_activity
ADD
  COLUMN user_id_ulid TEXT;

ALTER TABLE
  file_activity
ADD
  COLUMN secure_link_id_ulid TEXT;

ALTER TABLE
  email_queue
ADD
  COLUMN id_ulid TEXT;

ALTER TABLE
  email_queue
ADD
  COLUMN client_id_ulid TEXT;

ALTER TABLE
  password_reset_tokens
ADD
  COLUMN id_ulid TEXT;

ALTER TABLE
  password_reset_tokens
ADD
  COLUMN user_id_ulid TEXT;

CREATE UNIQUE INDEX idx_clients_id_ulid ON clients(id_ulid);

CREATE UNIQUE INDEX idx_users_id_ulid ON users(id_ulid);

CREATE UNIQUE INDEX idx_files_id_ulid ON files(id_ulid);

CREATE UNIQUE INDEX idx_secure_links_id_ulid ON secure_links(id_ulid);

CREATE UNIQUE INDEX idx_file_activity_id_ulid ON file_activity(id_ulid);

CREATE UNIQUE INDEX idx_email_queue_id_ulid ON email_queue(id_ulid);

CREATE UNIQUE INDEX idx_password_reset_tokens_id_ulid ON password_reset_tokens(id_ulid);

CREATE INDEX idx_users_client_id_ulid ON users(client_id_ulid);

CREATE INDEX idx_files_client_id_ulid ON files(client_id_ulid);

CREATE INDEX idx_files_user_id_ulid ON files(user_id_ulid);

CREATE INDEX idx_files_folder_id_ulid ON files(folder_id_ulid);

CREATE INDEX idx_secure_links_client_id_ulid ON secure_links(client_id_ulid);

CREATE INDEX idx_secure_links_created_by_ulid ON secure_links(created_by_ulid);

CREATE INDEX idx_file_activity_client_id_ulid ON file_activity(client_id_ulid);

CREATE INDEX idx_file_activity_user_id_ulid ON file_activity(user_id_ulid);

CREATE INDEX idx_file_activity_secure_link_id_ulid ON file_activity(secure_link_id_ulid);

CREATE INDEX idx_email_queue_client_id_ulid ON email_queue(client_id_ulid);

CREATE INDEX idx_password_reset_tokens_user_id_ulid ON password_reset_tokens(user_id_ulid);

-- +goose Down
DROP INDEX IF EXISTS idx_password_reset_tokens_user_id_ulid;

DROP INDEX IF EXISTS idx_email_queue_client_id_ulid;

DROP INDEX IF EXISTS idx_file_activity_secure_link_id_ulid;

DROP INDEX IF EXISTS idx_file_activity_user_id_ulid;

DROP INDEX IF EXISTS idx_file_activity_client_id_ulid;

DROP INDEX IF EXISTS idx_secure_links_created_by_ulid;

DROP INDEX IF EXISTS idx_secure_links_client_id_ulid;

DROP INDEX IF EXISTS idx_files_folder_id_ulid;

DROP INDEX IF EXISTS idx_files_user_id_ulid;

DROP INDEX IF EXISTS idx_files_client_id_ulid;

DROP INDEX IF EXISTS idx_users_client_id_ulid;

DROP INDEX IF EXISTS idx_password_reset_tokens_id_ulid;

DROP INDEX IF EXISTS idx_email_queue_id_ulid;

DROP INDEX IF EXISTS idx_file_activity_id_ulid;

DROP INDEX IF EXISTS idx_secure_links_id_ulid;

DROP INDEX IF EXISTS idx_files_id_ulid;

DROP INDEX IF EXISTS idx_users_id_ulid;

DROP INDEX IF EXISTS idx_clients_id_ulid;

-- SQLite does not support DROP COLUMN directly; manual rollback required.
