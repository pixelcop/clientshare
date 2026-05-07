-- +goose Up
ALTER TABLE
  file_activity
ADD
  COLUMN file_id TEXT;

CREATE INDEX IF NOT EXISTS idx_file_activity_file_id ON file_activity(file_id);

ALTER TABLE
  feed_events
ADD
  COLUMN file_id TEXT;

CREATE INDEX IF NOT EXISTS idx_feed_events_file_id ON feed_events(file_id);

WITH unique_files AS (
  SELECT
    client_id,
    path,
    MIN(id) AS id
  FROM
    files
  GROUP BY
    client_id,
    path
  HAVING
    COUNT(*) = 1
)
UPDATE
  file_activity
SET
  file_id = (
    SELECT
      uf.id
    FROM
      unique_files uf
    WHERE
      uf.client_id = file_activity.client_id
      AND uf.path = file_activity.file_path
  )
WHERE
  file_id IS NULL
  AND file_path <> ''
  AND EXISTS (
    SELECT
      1
    FROM
      unique_files uf
    WHERE
      uf.client_id = file_activity.client_id
      AND uf.path = file_activity.file_path
  );

UPDATE
  feed_events
SET
  file_id = (
    SELECT
      fa.file_id
    FROM
      file_activity fa
    WHERE
      fa.id = feed_events.source_id
  )
WHERE
  file_id IS NULL
  AND source_type = 'file_activity'
  AND EXISTS (
    SELECT
      1
    FROM
      file_activity fa
    WHERE
      fa.id = feed_events.source_id
      AND fa.file_id IS NOT NULL
  );

-- +goose Down
ALTER TABLE
  feed_events RENAME TO feed_events_with_file_id;

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
  FOREIGN KEY (actor_user_id) REFERENCES users(id) ON DELETE
  SET
    NULL,
    FOREIGN KEY (subject_user_id) REFERENCES users(id) ON DELETE
  SET
    NULL
);

INSERT INTO
  feed_events (
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
FROM
  feed_events_with_file_id;

CREATE UNIQUE INDEX IF NOT EXISTS idx_feed_events_user_source ON feed_events(user_id, event_type, source_type, source_id);

CREATE INDEX IF NOT EXISTS idx_feed_events_user_created_at ON feed_events(user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_feed_events_client_created_at ON feed_events(client_id, created_at);

CREATE INDEX IF NOT EXISTS idx_feed_events_event_type_created_at ON feed_events(event_type, created_at);

CREATE INDEX IF NOT EXISTS idx_feed_events_actor_user_id ON feed_events(actor_user_id);

CREATE INDEX IF NOT EXISTS idx_feed_events_subject_user_id ON feed_events(subject_user_id);

DROP TABLE IF EXISTS feed_events_with_file_id;

ALTER TABLE
  file_activity RENAME TO file_activity_with_file_id;

CREATE TABLE IF NOT EXISTS file_activity (
  id TEXT PRIMARY KEY,
  client_id TEXT NOT NULL,
  user_id TEXT,
  secure_link_id TEXT,
  ACTION TEXT NOT NULL,
  file_path TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  FOREIGN KEY (client_id) REFERENCES clients(id),
  FOREIGN KEY (user_id) REFERENCES users(id),
  FOREIGN KEY (secure_link_id) REFERENCES secure_links(id)
);

INSERT INTO
  file_activity (
    id,
    client_id,
    user_id,
    secure_link_id,
    ACTION,
    file_path,
    created_at
  )
SELECT
  id,
  client_id,
  user_id,
  secure_link_id,
  ACTION,
  file_path,
  created_at
FROM
  file_activity_with_file_id;

CREATE INDEX IF NOT EXISTS idx_file_activity_client_id ON file_activity(client_id);

CREATE INDEX IF NOT EXISTS idx_file_activity_user_id ON file_activity(user_id);

CREATE INDEX IF NOT EXISTS idx_file_activity_secure_link_id ON file_activity(secure_link_id);

DROP TABLE IF EXISTS file_activity_with_file_id;
