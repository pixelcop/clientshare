-- +goose Up
ALTER TABLE
  file_activity
ADD
  COLUMN upload_batch_id TEXT;

CREATE INDEX IF NOT EXISTS idx_file_activity_upload_batch_id ON file_activity(upload_batch_id);

-- +goose Down
ALTER TABLE
  file_activity RENAME TO file_activity_with_upload_batch_id;

CREATE TABLE IF NOT EXISTS file_activity (
  id TEXT PRIMARY KEY,
  client_id TEXT NOT NULL,
  user_id TEXT,
  secure_link_id TEXT,
  file_id TEXT,
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
    file_id,
    ACTION,
    file_path,
    created_at
  )
SELECT
  id,
  client_id,
  user_id,
  secure_link_id,
  file_id,
  ACTION,
  file_path,
  created_at
FROM
  file_activity_with_upload_batch_id;

CREATE INDEX IF NOT EXISTS idx_file_activity_client_id ON file_activity(client_id);

CREATE INDEX IF NOT EXISTS idx_file_activity_user_id ON file_activity(user_id);

CREATE INDEX IF NOT EXISTS idx_file_activity_secure_link_id ON file_activity(secure_link_id);

CREATE INDEX IF NOT EXISTS idx_file_activity_file_id ON file_activity(file_id);

DROP TABLE IF EXISTS file_activity_with_upload_batch_id;
