-- +goose Up
ALTER TABLE
  files
ADD
  COLUMN deleted_at DATETIME;

-- +goose Down
ALTER TABLE
  files DROP COLUMN deleted_at;
