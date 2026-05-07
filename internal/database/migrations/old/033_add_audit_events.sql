-- +goose Up
CREATE TABLE IF NOT EXISTS audit_events (
    id TEXT NOT NULL PRIMARY KEY,
    tenant_id TEXT,
    actor TEXT NOT NULL,
    ACTION TEXT NOT NULL,
    target_id TEXT,
    payload TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_audit_events_tenant_id ON audit_events(tenant_id);

CREATE INDEX IF NOT EXISTS idx_audit_events_created_at ON audit_events(created_at);

-- +goose Down
DROP TABLE IF EXISTS audit_events;
