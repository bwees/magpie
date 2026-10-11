-- +goose Up
CREATE TABLE platform_settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL CHECK (json_valid(value)),
    updated_at DATETIME NOT NULL
);

-- +goose Down
DROP TABLE platform_settings;
