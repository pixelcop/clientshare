-- +goose NO TRANSACTION
-- +goose Up
PRAGMA foreign_keys = OFF;

CREATE TABLE clients_tenanted (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  name TEXT NOT NULL,
  folder_path TEXT NOT NULL,
  root_folder_id TEXT,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
  FOREIGN KEY (root_folder_id) REFERENCES files_tenanted(id)
);

CREATE TABLE users_tenanted (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  email TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL,
  name TEXT,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
);

INSERT INTO
  users_tenanted (
    id,
    tenant_id,
    email,
    password_hash,
    role,
    name,
    created_at,
    updated_at
  )
SELECT
  id,
  '01JPMT0NX5K8J9Q4S7V2W3X6YZ',
  email,
  password_hash,
  role,
  name,
  created_at,
  updated_at
FROM
  users;

CREATE TABLE files_tenanted (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  client_id TEXT NOT NULL,
  user_id TEXT,
  folder_id TEXT,
  filename TEXT NOT NULL,
  path TEXT NOT NULL,
  TYPE TEXT NOT NULL DEFAULT 'file',
  size INTEGER NOT NULL,
  uploaded_at DATETIME NOT NULL,
  deleted_at DATETIME,
  FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
  FOREIGN KEY (client_id) REFERENCES clients_tenanted(id),
  FOREIGN KEY (user_id) REFERENCES users_tenanted(id),
  FOREIGN KEY (folder_id) REFERENCES files_tenanted(id)
);

INSERT INTO
  clients_tenanted (
    id,
    tenant_id,
    name,
    folder_path,
    root_folder_id,
    created_at,
    updated_at
  )
SELECT
  id,
  '01JPMT0NX5K8J9Q4S7V2W3X6YZ',
  name,
  folder_path,
  NULL,
  created_at,
  updated_at
FROM
  clients;

INSERT INTO
  files_tenanted (
    id,
    tenant_id,
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
  id,
  '01JPMT0NX5K8J9Q4S7V2W3X6YZ',
  client_id,
  user_id,
  folder_id,
  filename,
  path,
  TYPE,
  size,
  uploaded_at,
  deleted_at
FROM
  files;

UPDATE
  clients_tenanted
SET
  root_folder_id = (
    SELECT
      clients.root_folder_id
    FROM
      clients
    WHERE
      clients.id = clients_tenanted.id
  )
WHERE
  EXISTS (
    SELECT
      1
    FROM
      clients
    WHERE
      clients.id = clients_tenanted.id
      AND clients.root_folder_id IS NOT NULL
  );

CREATE TABLE secure_links_tenanted (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  client_id TEXT NOT NULL,
  token TEXT UNIQUE NOT NULL,
  email TEXT NOT NULL DEFAULT '',
  expires_at DATETIME NOT NULL,
  access_type TEXT NOT NULL,
  created_by TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  last_accessed DATETIME,
  FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
  FOREIGN KEY (client_id) REFERENCES clients_tenanted(id),
  FOREIGN KEY (created_by) REFERENCES users_tenanted(id)
);

INSERT INTO
  secure_links_tenanted (
    id,
    tenant_id,
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
  id,
  '01JPMT0NX5K8J9Q4S7V2W3X6YZ',
  client_id,
  token,
  email,
  expires_at,
  access_type,
  created_by,
  created_at,
  last_accessed
FROM
  secure_links;

CREATE TABLE file_activity_tenanted (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  client_id TEXT NOT NULL,
  user_id TEXT,
  secure_link_id TEXT,
  ACTION TEXT NOT NULL,
  file_path TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  file_id TEXT,
  upload_batch_id TEXT,
  FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
  FOREIGN KEY (client_id) REFERENCES clients_tenanted(id),
  FOREIGN KEY (user_id) REFERENCES users_tenanted(id),
  FOREIGN KEY (secure_link_id) REFERENCES secure_links_tenanted(id)
);

INSERT INTO
  file_activity_tenanted (
    id,
    tenant_id,
    client_id,
    user_id,
    secure_link_id,
    ACTION,
    file_path,
    created_at,
    file_id,
    upload_batch_id
  )
SELECT
  id,
  '01JPMT0NX5K8J9Q4S7V2W3X6YZ',
  client_id,
  user_id,
  secure_link_id,
  ACTION,
  file_path,
  created_at,
  file_id,
  upload_batch_id
FROM
  file_activity;

CREATE TABLE email_queue_tenanted (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
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
  FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
  FOREIGN KEY (client_id) REFERENCES clients_tenanted(id)
);

INSERT INTO
  email_queue_tenanted (
    id,
    tenant_id,
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
  id,
  '01JPMT0NX5K8J9Q4S7V2W3X6YZ',
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
FROM
  email_queue;

CREATE TABLE password_reset_tokens_tenanted (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  token_hash TEXT NOT NULL,
  expires_at DATETIME NOT NULL,
  used_at DATETIME,
  created_at DATETIME NOT NULL,
  FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
  FOREIGN KEY (user_id) REFERENCES users_tenanted(id) ON DELETE CASCADE
);

INSERT INTO
  password_reset_tokens_tenanted (
    id,
    tenant_id,
    user_id,
    token_hash,
    expires_at,
    used_at,
    created_at
  )
SELECT
  id,
  '01JPMT0NX5K8J9Q4S7V2W3X6YZ',
  user_id,
  token_hash,
  expires_at,
  used_at,
  created_at
FROM
  password_reset_tokens;

CREATE TABLE invite_tokens_tenanted (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  email TEXT NOT NULL,
  token_hash TEXT NOT NULL,
  expires_at DATETIME NOT NULL,
  used_at DATETIME,
  invited_by TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
  FOREIGN KEY (user_id) REFERENCES users_tenanted(id) ON DELETE CASCADE,
  FOREIGN KEY (invited_by) REFERENCES users_tenanted(id)
);

INSERT INTO
  invite_tokens_tenanted (
    id,
    tenant_id,
    user_id,
    email,
    token_hash,
    expires_at,
    used_at,
    invited_by,
    created_at
  )
SELECT
  id,
  '01JPMT0NX5K8J9Q4S7V2W3X6YZ',
  user_id,
  email,
  token_hash,
  expires_at,
  used_at,
  invited_by,
  created_at
FROM
  invite_tokens;

CREATE TABLE user_clients_tenanted (
  tenant_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  client_id TEXT NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, client_id),
  FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
  FOREIGN KEY (user_id) REFERENCES users_tenanted(id) ON DELETE CASCADE,
  FOREIGN KEY (client_id) REFERENCES clients_tenanted(id) ON DELETE CASCADE
);

INSERT INTO
  user_clients_tenanted (
    tenant_id,
    user_id,
    client_id,
    created_at
  )
SELECT
  '01JPMT0NX5K8J9Q4S7V2W3X6YZ',
  user_id,
  client_id,
  created_at
FROM
  user_clients;

CREATE TABLE feed_events_tenanted (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  client_id TEXT,
  actor_user_id TEXT,
  subject_user_id TEXT,
  source_type TEXT NOT NULL,
  source_id TEXT NOT NULL,
  file_path TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  is_read INTEGER NOT NULL DEFAULT 0,
  read_at DATETIME,
  file_id TEXT,
  FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
  FOREIGN KEY (user_id) REFERENCES users_tenanted(id) ON DELETE CASCADE,
  FOREIGN KEY (client_id) REFERENCES clients_tenanted(id) ON DELETE CASCADE,
  FOREIGN KEY (actor_user_id) REFERENCES users_tenanted(id) ON DELETE
  SET
    NULL,
    FOREIGN KEY (subject_user_id) REFERENCES users_tenanted(id) ON DELETE
  SET
    NULL
);

INSERT INTO
  feed_events_tenanted (
    id,
    tenant_id,
    user_id,
    event_type,
    client_id,
    actor_user_id,
    subject_user_id,
    source_type,
    source_id,
    file_path,
    created_at,
    is_read,
    read_at,
    file_id
  )
SELECT
  id,
  '01JPMT0NX5K8J9Q4S7V2W3X6YZ',
  user_id,
  event_type,
  client_id,
  actor_user_id,
  subject_user_id,
  source_type,
  source_id,
  file_path,
  created_at,
  is_read,
  read_at,
  file_id
FROM
  feed_events;

DROP TABLE feed_events;

DROP TABLE user_clients;

DROP TABLE invite_tokens;

DROP TABLE password_reset_tokens;

DROP TABLE email_queue;

DROP TABLE file_activity;

DROP TABLE secure_links;

DROP TABLE files;

DROP TABLE users;

DROP TABLE clients;

ALTER TABLE
  clients_tenanted RENAME TO clients;

ALTER TABLE
  users_tenanted RENAME TO users;

ALTER TABLE
  files_tenanted RENAME TO files;

ALTER TABLE
  secure_links_tenanted RENAME TO secure_links;

ALTER TABLE
  file_activity_tenanted RENAME TO file_activity;

ALTER TABLE
  email_queue_tenanted RENAME TO email_queue;

ALTER TABLE
  password_reset_tokens_tenanted RENAME TO password_reset_tokens;

ALTER TABLE
  invite_tokens_tenanted RENAME TO invite_tokens;

ALTER TABLE
  user_clients_tenanted RENAME TO user_clients;

ALTER TABLE
  feed_events_tenanted RENAME TO feed_events;

CREATE INDEX idx_users_tenant_id ON users(tenant_id);

CREATE UNIQUE INDEX idx_users_tenant_email ON users(tenant_id, email);

CREATE INDEX idx_clients_tenant_id ON clients(tenant_id);

CREATE UNIQUE INDEX idx_clients_tenant_folder_path ON clients(tenant_id, folder_path);

CREATE INDEX idx_files_tenant_id ON files(tenant_id);

CREATE INDEX idx_files_client_id ON files(client_id);

CREATE INDEX idx_files_user_id ON files(user_id);

CREATE INDEX idx_files_folder_id ON files(folder_id);

CREATE INDEX idx_secure_links_tenant_id ON secure_links(tenant_id);

CREATE INDEX idx_secure_links_client_id ON secure_links(client_id);

CREATE INDEX idx_secure_links_created_by ON secure_links(created_by);

CREATE INDEX idx_file_activity_tenant_id ON file_activity(tenant_id);

CREATE INDEX idx_file_activity_client_id ON file_activity(client_id);

CREATE INDEX idx_file_activity_user_id ON file_activity(user_id);

CREATE INDEX idx_file_activity_secure_link_id ON file_activity(secure_link_id);

CREATE INDEX idx_file_activity_file_id ON file_activity(file_id);

CREATE INDEX idx_file_activity_upload_batch_id ON file_activity(upload_batch_id);

CREATE INDEX idx_email_queue_tenant_id ON email_queue(tenant_id);

CREATE INDEX idx_email_queue_client_id ON email_queue(client_id);

CREATE INDEX idx_password_reset_tokens_tenant_id ON password_reset_tokens(tenant_id);

CREATE INDEX idx_password_reset_tokens_user_id ON password_reset_tokens(user_id);

CREATE UNIQUE INDEX idx_password_reset_tokens_token_hash ON password_reset_tokens(token_hash);

CREATE INDEX idx_invite_tokens_tenant_id ON invite_tokens(tenant_id);

CREATE INDEX idx_invite_tokens_user_id ON invite_tokens(user_id);

CREATE INDEX idx_invite_tokens_invited_by ON invite_tokens(invited_by);

CREATE UNIQUE INDEX idx_invite_tokens_token_hash ON invite_tokens(token_hash);

CREATE INDEX idx_user_clients_tenant_id ON user_clients(tenant_id);

CREATE INDEX idx_user_clients_client_id ON user_clients(client_id);

CREATE INDEX idx_user_clients_user_id ON user_clients(user_id);

CREATE INDEX idx_feed_events_tenant_id ON feed_events(tenant_id);

CREATE UNIQUE INDEX idx_feed_events_user_source ON feed_events(
  tenant_id,
  user_id,
  event_type,
  source_type,
  source_id
);

CREATE INDEX idx_feed_events_user_created_at ON feed_events(tenant_id, user_id, created_at DESC);

CREATE INDEX idx_feed_events_file_id ON feed_events(file_id);

PRAGMA foreign_keys = ON;

-- +goose Down
-- Not supported: tenant foundation rebuild is irreversible in SQLite without full backup restore.
SELECT
  1;
