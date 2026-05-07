-- +goose Up
ALTER TABLE feed_events RENAME TO feed_events_old;

CREATE TABLE IF NOT EXISTS feed_events (
  id TEXT PRIMARY KEY,
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
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  FOREIGN KEY (client_id) REFERENCES clients(id) ON DELETE CASCADE,
  FOREIGN KEY (actor_user_id) REFERENCES users(id) ON DELETE SET NULL,
  FOREIGN KEY (subject_user_id) REFERENCES users(id) ON DELETE SET NULL
);

INSERT INTO feed_events (
  id,
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
  read_at
)
SELECT
  substr(lower(hex(randomblob(16))), 1, 26),
  users.id,
  fe.event_type,
  fe.client_id,
  fe.actor_user_id,
  fe.subject_user_id,
  fe.source_type,
  fe.source_id,
  fe.file_path,
  fe.created_at,
  0,
  NULL
FROM feed_events_old fe
JOIN users ON users.role = 'admin';

INSERT INTO feed_events (
  id,
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
  read_at
)
SELECT
  substr(lower(hex(randomblob(16))), 1, 26),
  users.id,
  fe.event_type,
  fe.client_id,
  fe.actor_user_id,
  fe.subject_user_id,
  fe.source_type,
  fe.source_id,
  fe.file_path,
  fe.created_at,
  0,
  NULL
FROM feed_events_old fe
JOIN users ON users.role = 'manager'
WHERE fe.event_type = 'file_uploaded';

INSERT INTO feed_events (
  id,
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
  read_at
)
SELECT
  substr(lower(hex(randomblob(16))), 1, 26),
  uc.user_id,
  fe.event_type,
  fe.client_id,
  fe.actor_user_id,
  fe.subject_user_id,
  fe.source_type,
  fe.source_id,
  fe.file_path,
  fe.created_at,
  0,
  NULL
FROM feed_events_old fe
JOIN user_clients uc ON uc.client_id = fe.client_id
JOIN users ON users.id = uc.user_id
WHERE fe.event_type = 'file_uploaded'
  AND users.role IN ('client', 'customer');

DROP INDEX IF EXISTS idx_feed_events_source;

CREATE UNIQUE INDEX IF NOT EXISTS idx_feed_events_user_source ON feed_events(user_id, event_type, source_type, source_id);

CREATE INDEX IF NOT EXISTS idx_feed_events_user_created_at ON feed_events(user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_feed_events_client_created_at ON feed_events(client_id, created_at);

CREATE INDEX IF NOT EXISTS idx_feed_events_event_type_created_at ON feed_events(event_type, created_at);

CREATE INDEX IF NOT EXISTS idx_feed_events_actor_user_id ON feed_events(actor_user_id);

CREATE INDEX IF NOT EXISTS idx_feed_events_subject_user_id ON feed_events(subject_user_id);

DROP TABLE IF EXISTS feed_events_old;

-- +goose Down
ALTER TABLE feed_events RENAME TO feed_events_new;

CREATE TABLE IF NOT EXISTS feed_events (
  id TEXT PRIMARY KEY,
  event_type TEXT NOT NULL,
  client_id TEXT,
  actor_user_id TEXT,
  subject_user_id TEXT,
  source_type TEXT NOT NULL,
  source_id TEXT NOT NULL,
  file_path TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  FOREIGN KEY (client_id) REFERENCES clients(id) ON DELETE CASCADE,
  FOREIGN KEY (actor_user_id) REFERENCES users(id) ON DELETE SET NULL,
  FOREIGN KEY (subject_user_id) REFERENCES users(id) ON DELETE SET NULL
);

INSERT INTO feed_events (
  id,
  event_type,
  client_id,
  actor_user_id,
  subject_user_id,
  source_type,
  source_id,
  file_path,
  created_at
)
SELECT
  min(id),
  event_type,
  client_id,
  actor_user_id,
  subject_user_id,
  source_type,
  source_id,
  file_path,
  min(created_at)
FROM feed_events_new
GROUP BY event_type, client_id, actor_user_id, subject_user_id, source_type, source_id, file_path;

CREATE UNIQUE INDEX IF NOT EXISTS idx_feed_events_source ON feed_events(event_type, source_type, source_id);

CREATE INDEX IF NOT EXISTS idx_feed_events_client_created_at ON feed_events(client_id, created_at);

CREATE INDEX IF NOT EXISTS idx_feed_events_event_type_created_at ON feed_events(event_type, created_at);

CREATE INDEX IF NOT EXISTS idx_feed_events_actor_user_id ON feed_events(actor_user_id);

CREATE INDEX IF NOT EXISTS idx_feed_events_subject_user_id ON feed_events(subject_user_id);

DROP TABLE IF EXISTS feed_events_new;