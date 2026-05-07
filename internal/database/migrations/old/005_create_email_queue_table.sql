-- +goose Up
CREATE TABLE email_queue (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    recipient TEXT NOT NULL,
    subject TEXT NOT NULL,
    body TEXT NOT NULL,
    client_id INTEGER,
    event_type TEXT NOT NULL,
    scheduled_for DATETIME NOT NULL,
    sent_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (client_id) REFERENCES clients(id)
);

-- +goose Down
DROP TABLE IF EXISTS email_queue;
