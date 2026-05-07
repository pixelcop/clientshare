-- +goose Up
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
  FOREIGN KEY (actor_user_id) REFERENCES users(id) ON DELETE
  SET
    NULL,
    FOREIGN KEY (subject_user_id) REFERENCES users(id) ON DELETE
  SET
    NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_feed_events_source ON feed_events(event_type, source_type, source_id);

CREATE INDEX IF NOT EXISTS idx_feed_events_client_created_at ON feed_events(client_id, created_at);

CREATE INDEX IF NOT EXISTS idx_feed_events_event_type_created_at ON feed_events(event_type, created_at);

CREATE INDEX IF NOT EXISTS idx_feed_events_actor_user_id ON feed_events(actor_user_id);

CREATE INDEX IF NOT EXISTS idx_feed_events_subject_user_id ON feed_events(subject_user_id);

-- +goose Down
DROP TABLE IF EXISTS feed_events;
