-- +goose Up
CREATE TABLE tenants (
  id VARCHAR(26) NOT NULL,
  slug VARCHAR(255) NOT NULL,
  name VARCHAR(255) NOT NULL,
  created_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY idx_tenants_slug (slug)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci;

CREATE TABLE tenant_domains (
  id VARCHAR(26) NOT NULL,
  tenant_id VARCHAR(26) NOT NULL,
  domain VARCHAR(255) NOT NULL,
  kind VARCHAR(64) NOT NULL DEFAULT 'base_url',
  is_primary BOOLEAN NOT NULL DEFAULT FALSE,
  created_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY idx_tenant_domains_domain (domain),
  KEY idx_tenant_domains_tenant_id (tenant_id),
  CONSTRAINT fk_tenant_domains_tenant_id FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci;

CREATE TABLE tenant_settings (
  id VARCHAR(26) NOT NULL,
  tenant_id VARCHAR(26) NOT NULL,
  site_title VARCHAR(255) NOT NULL DEFAULT '',
  logo_path VARCHAR(1024) NOT NULL DEFAULT '',
  primary_color VARCHAR(32) NOT NULL DEFAULT '',
  public_base_url VARCHAR(1024) NOT NULL DEFAULT '',
  invite_welcome_text VARCHAR(2048) NOT NULL DEFAULT '',
  secure_link_default_expiry_days INT NOT NULL DEFAULT 90,
  created_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY idx_tenant_settings_tenant_id (tenant_id),
  CONSTRAINT fk_tenant_settings_tenant_id FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci;

CREATE TABLE tenant_entitlements (
  id VARCHAR(26) NOT NULL,
  tenant_id VARCHAR(26) NOT NULL,
  STATUS VARCHAR(64) NOT NULL DEFAULT 'active',
  plan_code VARCHAR(64) NOT NULL DEFAULT 'self_hosted',
  max_users BIGINT NOT NULL DEFAULT 0,
  max_storage_bytes BIGINT NOT NULL DEFAULT 0,
  created_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY idx_tenant_entitlements_tenant_id (tenant_id),
  CONSTRAINT fk_tenant_entitlements_tenant_id FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci;

CREATE TABLE clients (
  id VARCHAR(26) NOT NULL,
  tenant_id VARCHAR(26) NOT NULL,
  name VARCHAR(255) NOT NULL,
  folder_path VARCHAR(512) NOT NULL,
  root_folder_id VARCHAR(26) DEFAULT NULL,
  created_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  PRIMARY KEY (id),
  KEY idx_clients_tenant_id (tenant_id),
  UNIQUE KEY idx_clients_tenant_folder_path (tenant_id, folder_path),
  CONSTRAINT fk_clients_tenant_id FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci;

CREATE TABLE users (
  id VARCHAR(26) NOT NULL,
  tenant_id VARCHAR(26) NOT NULL,
  email VARCHAR(320) NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  role VARCHAR(64) NOT NULL,
  name VARCHAR(255) DEFAULT NULL,
  created_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  PRIMARY KEY (id),
  KEY idx_users_tenant_id (tenant_id),
  UNIQUE KEY idx_users_tenant_email (tenant_id, email),
  CONSTRAINT fk_users_tenant_id FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci;

CREATE TABLE files (
  id VARCHAR(26) NOT NULL,
  tenant_id VARCHAR(26) NOT NULL,
  client_id VARCHAR(26) NOT NULL,
  user_id VARCHAR(26) DEFAULT NULL,
  folder_id VARCHAR(26) DEFAULT NULL,
  filename VARCHAR(1024) NOT NULL,
  path VARCHAR(2048) NOT NULL,
  TYPE VARCHAR(16) NOT NULL DEFAULT 'file',
  size BIGINT NOT NULL,
  uploaded_at DATETIME(6) NOT NULL,
  deleted_at DATETIME(6) DEFAULT NULL,
  PRIMARY KEY (id),
  KEY idx_files_tenant_id (tenant_id),
  KEY idx_files_client_id (client_id),
  KEY idx_files_user_id (user_id),
  KEY idx_files_folder_id (folder_id),
  CONSTRAINT fk_files_tenant_id FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
  CONSTRAINT fk_files_client_id FOREIGN KEY (client_id) REFERENCES clients(id),
  CONSTRAINT fk_files_user_id FOREIGN KEY (user_id) REFERENCES users(id),
  CONSTRAINT fk_files_folder_id FOREIGN KEY (folder_id) REFERENCES files(id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci;

ALTER TABLE
  clients
ADD
  CONSTRAINT fk_clients_root_folder_id FOREIGN KEY (root_folder_id) REFERENCES files(id);

CREATE TABLE secure_links (
  id VARCHAR(26) NOT NULL,
  tenant_id VARCHAR(26) NOT NULL,
  client_id VARCHAR(26) NOT NULL,
  token VARCHAR(128) NOT NULL,
  email VARCHAR(320) NOT NULL DEFAULT '',
  expires_at DATETIME(6) NOT NULL,
  access_type VARCHAR(64) NOT NULL,
  created_by VARCHAR(26) NOT NULL,
  created_at DATETIME(6) NOT NULL,
  last_accessed DATETIME(6) DEFAULT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY idx_secure_links_token (token),
  KEY idx_secure_links_tenant_id (tenant_id),
  KEY idx_secure_links_client_id (client_id),
  KEY idx_secure_links_created_by (created_by),
  CONSTRAINT fk_secure_links_tenant_id FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
  CONSTRAINT fk_secure_links_client_id FOREIGN KEY (client_id) REFERENCES clients(id),
  CONSTRAINT fk_secure_links_created_by FOREIGN KEY (created_by) REFERENCES users(id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci;

CREATE TABLE file_activity (
  id VARCHAR(26) NOT NULL,
  tenant_id VARCHAR(26) NOT NULL,
  client_id VARCHAR(26) NOT NULL,
  user_id VARCHAR(26) DEFAULT NULL,
  secure_link_id VARCHAR(26) DEFAULT NULL,
  ACTION VARCHAR(64) NOT NULL,
  file_path VARCHAR(2048) NOT NULL,
  created_at DATETIME(6) NOT NULL,
  file_id VARCHAR(26) DEFAULT NULL,
  upload_batch_id VARCHAR(64) DEFAULT NULL,
  PRIMARY KEY (id),
  KEY idx_file_activity_tenant_id (tenant_id),
  KEY idx_file_activity_client_id (client_id),
  KEY idx_file_activity_user_id (user_id),
  KEY idx_file_activity_secure_link_id (secure_link_id),
  KEY idx_file_activity_file_id (file_id),
  KEY idx_file_activity_upload_batch_id (upload_batch_id),
  CONSTRAINT fk_file_activity_tenant_id FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
  CONSTRAINT fk_file_activity_client_id FOREIGN KEY (client_id) REFERENCES clients(id),
  CONSTRAINT fk_file_activity_user_id FOREIGN KEY (user_id) REFERENCES users(id),
  CONSTRAINT fk_file_activity_secure_link_id FOREIGN KEY (secure_link_id) REFERENCES secure_links(id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci;

CREATE TABLE email_queue (
  id VARCHAR(26) NOT NULL,
  tenant_id VARCHAR(26) NOT NULL,
  recipient VARCHAR(320) NOT NULL,
  subject VARCHAR(512) NOT NULL,
  body LONGTEXT NOT NULL,
  html_body LONGTEXT NOT NULL,
  file_ids_csv LONGTEXT NOT NULL,
  client_id VARCHAR(26) DEFAULT NULL,
  event_type VARCHAR(64) NOT NULL,
  scheduled_for DATETIME(6) NOT NULL,
  sent_at DATETIME(6) DEFAULT NULL,
  created_at DATETIME(6) NOT NULL,
  PRIMARY KEY (id),
  KEY idx_email_queue_tenant_id (tenant_id),
  KEY idx_email_queue_client_id (client_id),
  CONSTRAINT fk_email_queue_tenant_id FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
  CONSTRAINT fk_email_queue_client_id FOREIGN KEY (client_id) REFERENCES clients(id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci;

CREATE TABLE password_reset_tokens (
  id VARCHAR(26) NOT NULL,
  tenant_id VARCHAR(26) NOT NULL,
  user_id VARCHAR(26) NOT NULL,
  token_hash VARCHAR(128) NOT NULL,
  expires_at DATETIME(6) NOT NULL,
  used_at DATETIME(6) DEFAULT NULL,
  created_at DATETIME(6) NOT NULL,
  PRIMARY KEY (id),
  KEY idx_password_reset_tokens_tenant_id (tenant_id),
  KEY idx_password_reset_tokens_user_id (user_id),
  UNIQUE KEY idx_password_reset_tokens_token_hash (token_hash),
  CONSTRAINT fk_password_reset_tokens_tenant_id FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
  CONSTRAINT fk_password_reset_tokens_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci;

CREATE TABLE invite_tokens (
  id VARCHAR(26) NOT NULL,
  tenant_id VARCHAR(26) NOT NULL,
  user_id VARCHAR(26) NOT NULL,
  email VARCHAR(320) NOT NULL DEFAULT '',
  token_hash VARCHAR(128) NOT NULL,
  expires_at DATETIME(6) NOT NULL,
  used_at DATETIME(6) DEFAULT NULL,
  invited_by VARCHAR(26) NOT NULL,
  created_at DATETIME(6) NOT NULL,
  PRIMARY KEY (id),
  KEY idx_invite_tokens_tenant_id (tenant_id),
  KEY idx_invite_tokens_user_id (user_id),
  KEY idx_invite_tokens_invited_by (invited_by),
  UNIQUE KEY idx_invite_tokens_token_hash (token_hash),
  CONSTRAINT fk_invite_tokens_tenant_id FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
  CONSTRAINT fk_invite_tokens_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_invite_tokens_invited_by FOREIGN KEY (invited_by) REFERENCES users(id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci;

CREATE TABLE user_clients (
  tenant_id VARCHAR(26) NOT NULL,
  user_id VARCHAR(26) NOT NULL,
  client_id VARCHAR(26) NOT NULL,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (user_id, client_id),
  KEY idx_user_clients_tenant_id (tenant_id),
  KEY idx_user_clients_client_id (client_id),
  KEY idx_user_clients_user_id (user_id),
  CONSTRAINT fk_user_clients_tenant_id FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
  CONSTRAINT fk_user_clients_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_user_clients_client_id FOREIGN KEY (client_id) REFERENCES clients(id) ON DELETE CASCADE
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci;

CREATE TABLE feed_events (
  id VARCHAR(26) NOT NULL,
  tenant_id VARCHAR(26) NOT NULL,
  user_id VARCHAR(26) NOT NULL,
  event_type VARCHAR(64) NOT NULL,
  client_id VARCHAR(26) DEFAULT NULL,
  actor_user_id VARCHAR(26) DEFAULT NULL,
  subject_user_id VARCHAR(26) DEFAULT NULL,
  source_type VARCHAR(64) NOT NULL,
  source_id VARCHAR(128) NOT NULL,
  file_path VARCHAR(2048) NOT NULL DEFAULT '',
  created_at DATETIME(6) NOT NULL,
  is_read BOOLEAN NOT NULL DEFAULT FALSE,
  read_at DATETIME(6) DEFAULT NULL,
  file_id VARCHAR(26) DEFAULT NULL,
  PRIMARY KEY (id),
  KEY idx_feed_events_tenant_id (tenant_id),
  UNIQUE KEY idx_feed_events_user_source (
    tenant_id,
    user_id,
    event_type,
    source_type,
    source_id
  ),
  KEY idx_feed_events_user_created_at (tenant_id, user_id, created_at DESC),
  KEY idx_feed_events_file_id (file_id),
  CONSTRAINT fk_feed_events_tenant_id FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
  CONSTRAINT fk_feed_events_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_feed_events_client_id FOREIGN KEY (client_id) REFERENCES clients(id) ON DELETE CASCADE,
  CONSTRAINT fk_feed_events_actor_user_id FOREIGN KEY (actor_user_id) REFERENCES users(id) ON DELETE
  SET
    NULL,
    CONSTRAINT fk_feed_events_subject_user_id FOREIGN KEY (subject_user_id) REFERENCES users(id) ON DELETE
  SET
    NULL
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci;

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
