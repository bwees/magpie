-- +goose Up
CREATE TABLE platform_schedules (
    name        TEXT PRIMARY KEY,
    next_run_at DATETIME NOT NULL
);

-- +goose Down
DROP TABLE platform_schedules;
