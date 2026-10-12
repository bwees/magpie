-- +goose Up
-- The one Sonarr/Radarr import: its connections, choices, job state and dry-run plan, as one JSON document.
CREATE TABLE platform_migration (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    session    TEXT     NOT NULL CHECK (json_valid(session)),
    updated_at DATETIME NOT NULL
);

-- +goose Down
DROP TABLE platform_migration;
