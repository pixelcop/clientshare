-- +goose Up
CREATE TABLE IF NOT EXISTS tenants (
  id TEXT PRIMARY KEY,
  slug TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS tenant_domains (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  domain TEXT NOT NULL UNIQUE,
  kind TEXT NOT NULL DEFAULT 'base_url',
  is_primary INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_tenant_domains_tenant_id ON tenant_domains(tenant_id);

CREATE TABLE IF NOT EXISTS tenant_settings (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL UNIQUE,
  site_title TEXT NOT NULL DEFAULT '',
  logo_path TEXT NOT NULL DEFAULT '',
  primary_color TEXT NOT NULL DEFAULT '',
  public_base_url TEXT NOT NULL DEFAULT '',
  invite_welcome_text TEXT NOT NULL DEFAULT '',
  secure_link_default_expiry_days INTEGER NOT NULL DEFAULT 90,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS tenant_entitlements (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL UNIQUE,
  STATUS TEXT NOT NULL DEFAULT 'active',
  plan_code TEXT NOT NULL DEFAULT 'self_hosted',
  max_users INTEGER NOT NULL DEFAULT 0,
  max_storage_bytes INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
);

-- +goose Down
DROP TABLE IF EXISTS tenant_entitlements;

DROP TABLE IF EXISTS tenant_settings;

DROP TABLE IF EXISTS tenant_domains;

DROP TABLE IF EXISTS tenants;
