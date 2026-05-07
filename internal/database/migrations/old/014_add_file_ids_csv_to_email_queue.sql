-- +goose Up
ALTER TABLE email_queue
ADD COLUMN file_ids_csv TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE email_queue
DROP COLUMN file_ids_csv;
