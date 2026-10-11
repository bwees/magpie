-- +goose Up
CREATE TABLE activity_notification_urls (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL,
    label        TEXT NOT NULL,
    url          TEXT NOT NULL,
    kinds        TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(kinds)),
    created_at   DATETIME NOT NULL,
    last_sent_at DATETIME,
    last_error   TEXT NOT NULL DEFAULT ''
);
CREATE INDEX activity_notification_urls_user ON activity_notification_urls (user_id, created_at);

-- +goose Down
DROP TABLE activity_notification_urls;
