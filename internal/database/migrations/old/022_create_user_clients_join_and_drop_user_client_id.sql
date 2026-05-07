-- +goose Up
PRAGMA foreign_keys = OFF;

CREATE TABLE IF NOT EXISTS user_clients (
  user_id TEXT NOT NULL,
  client_id TEXT NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, client_id),
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  FOREIGN KEY (client_id) REFERENCES clients(id) ON DELETE CASCADE
);

INSERT
  OR IGNORE INTO user_clients (user_id, client_id, created_at)
SELECT
  id,
  client_id,
  COALESCE(updated_at, created_at, CURRENT_TIMESTAMP)
FROM
  users
WHERE
  client_id IS NOT NULL
  AND TRIM(client_id) <> '';

CREATE TABLE users_new (
  id TEXT PRIMARY KEY,
  email TEXT UNIQUE NOT NULL,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL,
  name TEXT,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);

INSERT INTO
  users_new (
    id,
    email,
    password_hash,
    role,
    name,
    created_at,
    updated_at
  )
SELECT
  id,
  email,
  password_hash,
  role,
  name,
  created_at,
  updated_at
FROM
  users;

DROP TABLE users;

ALTER TABLE
  users_new RENAME TO users;

CREATE INDEX IF NOT EXISTS idx_user_clients_client_id ON user_clients(client_id);

CREATE INDEX IF NOT EXISTS idx_user_clients_user_id ON user_clients(user_id);

PRAGMA foreign_keys = ON;

-- +goose Down
PRAGMA foreign_keys = OFF;

CREATE TABLE users_old (
  id TEXT PRIMARY KEY,
  email TEXT UNIQUE NOT NULL,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL,
  name TEXT,
  client_id TEXT,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY (client_id) REFERENCES clients(id)
);

INSERT INTO
  users_old (
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
  u.id,
  u.email,
  u.password_hash,
  u.role,
  u.name,
  (
    SELECT
      uc.client_id
    FROM
      user_clients uc
    WHERE
      uc.user_id = u.id
    ORDER BY
      uc.created_at ASC
    LIMIT
      1
  ) AS client_id,
  u.created_at,
  u.updated_at
FROM
  users u;

DROP TABLE users;

ALTER TABLE
  users_old RENAME TO users;

DROP TABLE IF EXISTS user_clients;

PRAGMA foreign_keys = ON;
