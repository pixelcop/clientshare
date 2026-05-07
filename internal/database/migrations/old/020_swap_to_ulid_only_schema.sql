-- +goose Up
PRAGMA foreign_keys = OFF;

CREATE TABLE clients_ulid (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  folder_path TEXT UNIQUE NOT NULL,
  root_folder_id TEXT,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY (root_folder_id) REFERENCES files(id)
);

INSERT INTO
  clients_ulid (
    id,
    name,
    folder_path,
    root_folder_id,
    created_at,
    updated_at
  )
SELECT
  id_ulid,
  name,
  folder_path,
  root_folder_id_ulid,
  created_at,
  updated_at
FROM
  clients;

CREATE TABLE users_ulid (
  id TEXT PRIMARY KEY,
  email TEXT UNIQUE NOT NULL,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL,
  name TEXT,
  client_id TEXT,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY (client_id) REFERENCES clients_ulid(id)
);

INSERT INTO
  users_ulid (
    id,
    email,
    password_hash,
    role,
    name,
    client_id,
    created_at,
    updated_at
  )
SELECT
  id_ulid,
  email,
  password_hash,
  role,
  name,
  client_id_ulid,
  created_at,
  updated_at
FROM
  users;

CREATE TABLE files_ulid (
  id TEXT PRIMARY KEY,
  client_id TEXT NOT NULL,
  user_id TEXT,
  folder_id TEXT,
  filename TEXT NOT NULL,
  path TEXT NOT NULL,
  TYPE TEXT NOT NULL DEFAULT 'file',
  size INTEGER NOT NULL,
  uploaded_at DATETIME NOT NULL,
  deleted_at DATETIME,
  FOREIGN KEY (client_id) REFERENCES clients_ulid(id),
  FOREIGN KEY (user_id) REFERENCES users_ulid(id),
  FOREIGN KEY (folder_id) REFERENCES files_ulid(id)
);

INSERT INTO
  files_ulid (
    id,
    client_id,
    user_id,
    folder_id,
    filename,
    path,
    TYPE,
    size,
    uploaded_at,
    deleted_at
  )
SELECT
  id_ulid,
  client_id_ulid,
  user_id_ulid,
  folder_id_ulid,
  filename,
  path,
  TYPE,
  size,
  uploaded_at,
  deleted_at
FROM
  files;

CREATE TABLE secure_links_ulid (
  id TEXT PRIMARY KEY,
  client_id TEXT NOT NULL,
  token TEXT UNIQUE NOT NULL,
  email TEXT NOT NULL DEFAULT '',
  expires_at DATETIME NOT NULL,
  access_type TEXT NOT NULL,
  created_by TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  last_accessed DATETIME,
  FOREIGN KEY (client_id) REFERENCES clients_ulid(id),
  FOREIGN KEY (created_by) REFERENCES users_ulid(id)
);

INSERT INTO
  secure_links_ulid (
    id,
    client_id,
    token,
    email,
    expires_at,
    access_type,
    created_by,
    created_at,
    last_accessed
  )
SELECT
  id_ulid,
  client_id_ulid,
  token,
  email,
  expires_at,
  access_type,
  created_by_ulid,
  created_at,
  last_accessed
FROM
  secure_links;

CREATE TABLE file_activity_ulid (
  id TEXT PRIMARY KEY,
  client_id TEXT NOT NULL,
  user_id TEXT,
  secure_link_id TEXT,
  ACTION TEXT NOT NULL,
  file_path TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  FOREIGN KEY (client_id) REFERENCES clients_ulid(id),
  FOREIGN KEY (user_id) REFERENCES users_ulid(id),
  FOREIGN KEY (secure_link_id) REFERENCES secure_links_ulid(id)
);

INSERT INTO
  file_activity_ulid (
    id,
    client_id,
    user_id,
    secure_link_id,
    ACTION,
    file_path,
    created_at
  )
SELECT
  id_ulid,
  client_id_ulid,
  user_id_ulid,
  secure_link_id_ulid,
  ACTION,
  file_path,
  created_at
FROM
  file_activity;

CREATE TABLE email_queue_ulid (
  id TEXT PRIMARY KEY,
  recipient TEXT NOT NULL,
  subject TEXT NOT NULL,
  body TEXT NOT NULL,
  html_body TEXT NOT NULL DEFAULT '',
  file_ids_csv TEXT NOT NULL DEFAULT '',
  client_id TEXT,
  event_type TEXT NOT NULL,
  scheduled_for DATETIME NOT NULL,
  sent_at DATETIME,
  created_at DATETIME NOT NULL,
  FOREIGN KEY (client_id) REFERENCES clients_ulid(id)
);

INSERT INTO
  email_queue_ulid (
    id,
    recipient,
    subject,
    body,
    html_body,
    file_ids_csv,
    client_id,
    event_type,
    scheduled_for,
    sent_at,
    created_at
  )
SELECT
  id_ulid,
  recipient,
  subject,
  body,
  html_body,
  file_ids_csv,
  client_id_ulid,
  event_type,
  scheduled_for,
  sent_at,
  created_at
FROM
  email_queue;

CREATE TABLE password_reset_tokens_ulid (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  token_hash TEXT NOT NULL,
  expires_at DATETIME NOT NULL,
  used_at DATETIME,
  created_at DATETIME NOT NULL,
  FOREIGN KEY (user_id) REFERENCES users_ulid(id) ON DELETE CASCADE
);

INSERT INTO
  password_reset_tokens_ulid (
    id,
    user_id,
    token_hash,
    expires_at,
    used_at,
    created_at
  )
SELECT
  id_ulid,
  user_id_ulid,
  token_hash,
  expires_at,
  used_at,
  created_at
FROM
  password_reset_tokens;

DROP TABLE password_reset_tokens;

DROP TABLE email_queue;

DROP TABLE file_activity;

DROP TABLE secure_links;

DROP TABLE files;

DROP TABLE users;

DROP TABLE clients;

ALTER TABLE
  clients_ulid RENAME TO clients;

ALTER TABLE
  users_ulid RENAME TO users;

ALTER TABLE
  files_ulid RENAME TO files;

ALTER TABLE
  secure_links_ulid RENAME TO secure_links;

ALTER TABLE
  file_activity_ulid RENAME TO file_activity;

ALTER TABLE
  email_queue_ulid RENAME TO email_queue;

ALTER TABLE
  password_reset_tokens_ulid RENAME TO password_reset_tokens;

CREATE INDEX idx_users_client_id ON users(client_id);

CREATE INDEX idx_files_client_id ON files(client_id);

CREATE INDEX idx_files_user_id ON files(user_id);

CREATE INDEX idx_files_folder_id ON files(folder_id);

CREATE INDEX idx_secure_links_client_id ON secure_links(client_id);

CREATE INDEX idx_secure_links_created_by ON secure_links(created_by);

CREATE INDEX idx_file_activity_client_id ON file_activity(client_id);

CREATE INDEX idx_file_activity_user_id ON file_activity(user_id);

CREATE INDEX idx_file_activity_secure_link_id ON file_activity(secure_link_id);

CREATE INDEX idx_email_queue_client_id ON email_queue(client_id);

CREATE INDEX idx_password_reset_tokens_user_id ON password_reset_tokens(user_id);

CREATE UNIQUE INDEX idx_password_reset_tokens_token_hash ON password_reset_tokens(token_hash);

PRAGMA foreign_keys = ON;

-- +goose Down
-- Not supported: ULID-only schema swap is irreversible in SQLite without full backup restore.
SELECT
  1;
