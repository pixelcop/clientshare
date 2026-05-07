-- +goose Up
ALTER TABLE
  email_queue
ADD
  COLUMN html_body TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE
  email_queue DROP COLUMN html_body;
