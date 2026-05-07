-- +goose Up
CREATE TABLE IF NOT EXISTS invite_tokens (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  email TEXT NOT NULL,
  token_hash TEXT NOT NULL,
  expires_at DATETIME NOT NULL,
  used_at DATETIME,
  invited_by TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  FOREIGN KEY (invited_by) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_invite_tokens_user_id ON invite_tokens(user_id);

CREATE INDEX IF NOT EXISTS idx_invite_tokens_invited_by ON invite_tokens(invited_by);

CREATE UNIQUE INDEX IF NOT EXISTS idx_invite_tokens_token_hash ON invite_tokens(token_hash);

-- +goose Down
DROP TABLE IF EXISTS invite_tokens;
