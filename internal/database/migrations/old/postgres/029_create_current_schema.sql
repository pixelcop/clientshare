-- +goose Up
CREATE TABLE tenants (
  id VARCHAR(26) PRIMARY KEY,
  slug VARCHAR(255) NOT NULL UNIQUE,
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE tenant_domains (
  id VARCHAR(26) PRIMARY KEY,
  tenant_id VARCHAR(26) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  domain VARCHAR(255) NOT NULL UNIQUE,
  kind VARCHAR(64) NOT NULL DEFAULT 'base_url',
  is_primary BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_tenant_domains_tenant_id ON tenant_domains(tenant_id);

CREATE TABLE tenant_settings (
  id VARCHAR(26) PRIMARY KEY,
  tenant_id VARCHAR(26) NOT NULL UNIQUE REFERENCES tenants(id) ON DELETE CASCADE,
  site_title TEXT NOT NULL DEFAULT '',
  logo_path TEXT NOT NULL DEFAULT '',
  primary_color TEXT NOT NULL DEFAULT '',
  public_base_url TEXT NOT NULL DEFAULT '',
  invite_welcome_text TEXT NOT NULL DEFAULT '',
  secure_link_default_expiry_days INTEGER NOT NULL DEFAULT 90,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE tenant_entitlements (
  id VARCHAR(26) PRIMARY KEY,
  tenant_id VARCHAR(26) NOT NULL UNIQUE REFERENCES tenants(id) ON DELETE CASCADE,
  STATUS VARCHAR(64) NOT NULL DEFAULT 'active',
  plan_code VARCHAR(64) NOT NULL DEFAULT 'self_hosted',
  max_users BIGINT NOT NULL DEFAULT 0,
  max_storage_bytes BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE clients (
  id VARCHAR(26) PRIMARY KEY,
  tenant_id VARCHAR(26) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  folder_path VARCHAR(512) NOT NULL,
  root_folder_id VARCHAR(26),
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE users (
  id VARCHAR(26) PRIMARY KEY,
  tenant_id VARCHAR(26) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  email VARCHAR(320) NOT NULL,
  password_hash TEXT NOT NULL,
  role VARCHAR(64) NOT NULL,
  name TEXT,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE files (
  id VARCHAR(26) PRIMARY KEY,
  tenant_id VARCHAR(26) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  client_id VARCHAR(26) NOT NULL REFERENCES clients(id),
  user_id VARCHAR(26) REFERENCES users(id),
  folder_id VARCHAR(26) REFERENCES files(id),
  filename TEXT NOT NULL,
  path TEXT NOT NULL,
  TYPE VARCHAR(16) NOT NULL DEFAULT 'file',
  size BIGINT NOT NULL,
  uploaded_at TIMESTAMPTZ NOT NULL,
  deleted_at TIMESTAMPTZ
);

ALTER TABLE
  clients
ADD
  CONSTRAINT fk_clients_root_folder_id FOREIGN KEY (root_folder_id) REFERENCES files(id);

CREATE TABLE secure_links (
  id VARCHAR(26) PRIMARY KEY,
  tenant_id VARCHAR(26) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  client_id VARCHAR(26) NOT NULL REFERENCES clients(id),
  token VARCHAR(128) NOT NULL UNIQUE,
  email TEXT NOT NULL DEFAULT '',
  expires_at TIMESTAMPTZ NOT NULL,
  access_type VARCHAR(64) NOT NULL,
  created_by VARCHAR(26) NOT NULL REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL,
  last_accessed TIMESTAMPTZ
);

CREATE TABLE file_activity (
  id VARCHAR(26) PRIMARY KEY,
  tenant_id VARCHAR(26) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  client_id VARCHAR(26) NOT NULL REFERENCES clients(id),
  user_id VARCHAR(26) REFERENCES users(id),
  secure_link_id VARCHAR(26) REFERENCES secure_links(id),
  ACTION VARCHAR(64) NOT NULL,
  file_path TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  file_id VARCHAR(26),
  upload_batch_id VARCHAR(64)
);

CREATE TABLE email_queue (
  id VARCHAR(26) PRIMARY KEY,
  tenant_id VARCHAR(26) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  recipient VARCHAR(320) NOT NULL,
  subject TEXT NOT NULL,
  body TEXT NOT NULL,
  html_body TEXT NOT NULL DEFAULT '',
  file_ids_csv TEXT NOT NULL DEFAULT '',
  client_id VARCHAR(26) REFERENCES clients(id),
  event_type VARCHAR(64) NOT NULL,
  scheduled_for TIMESTAMPTZ NOT NULL,
  sent_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE password_reset_tokens (
  id VARCHAR(26) PRIMARY KEY,
  tenant_id VARCHAR(26) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  user_id VARCHAR(26) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash VARCHAR(128) NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  used_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE invite_tokens (
  id VARCHAR(26) PRIMARY KEY,
  tenant_id VARCHAR(26) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  user_id VARCHAR(26) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  email VARCHAR(320) NOT NULL DEFAULT '',
  token_hash VARCHAR(128) NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  used_at TIMESTAMPTZ,
  invited_by VARCHAR(26) NOT NULL REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE user_clients (
  tenant_id VARCHAR(26) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  user_id VARCHAR(26) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  client_id VARCHAR(26) NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, client_id)
);

CREATE TABLE feed_events (
  id VARCHAR(26) PRIMARY KEY,
  tenant_id VARCHAR(26) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  user_id VARCHAR(26) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  event_type VARCHAR(64) NOT NULL,
  client_id VARCHAR(26) REFERENCES clients(id) ON DELETE CASCADE,
  actor_user_id VARCHAR(26) REFERENCES users(id) ON DELETE
  SET
    NULL,
    subject_user_id VARCHAR(26) REFERENCES users(id) ON DELETE
  SET
    NULL,
    source_type VARCHAR(64) NOT NULL,
    source_id VARCHAR(128) NOT NULL,
    file_path TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    is_read BOOLEAN NOT NULL DEFAULT FALSE,
    read_at TIMESTAMPTZ,
    file_id VARCHAR(26)
);

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

-- +goose Down
DROP TABLE IF EXISTS feed_events;

DROP TABLE IF EXISTS user_clients;

DROP TABLE IF EXISTS invite_tokens;

DROP TABLE IF EXISTS password_reset_tokens;

DROP TABLE IF EXISTS email_queue;

DROP TABLE IF EXISTS file_activity;

DROP TABLE IF EXISTS secure_links;

DROP TABLE IF EXISTS files;

DROP TABLE IF EXISTS users;

DROP TABLE IF EXISTS clients;

DROP TABLE IF EXISTS tenant_entitlements;

DROP TABLE IF EXISTS tenant_settings;

DROP TABLE IF EXISTS tenant_domains;

DROP TABLE IF EXISTS tenants;
