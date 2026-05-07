-- +goose Up
CREATE TABLE file_activity (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    client_id INTEGER NOT NULL,
    user_id INTEGER,
    secure_link_id INTEGER,
    action TEXT NOT NULL,
    file_path TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (client_id) REFERENCES clients(id),
    FOREIGN KEY (user_id) REFERENCES users(id),
    FOREIGN KEY (secure_link_id) REFERENCES secure_links(id)
);

-- +goose Down
DROP TABLE IF EXISTS file_activity;
