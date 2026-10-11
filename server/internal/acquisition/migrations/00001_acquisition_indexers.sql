-- +goose Up
CREATE TABLE acquisition_indexers (
    prowlarr_id           INTEGER PRIMARY KEY,
    name                  TEXT     NOT NULL,
    privacy               TEXT     NOT NULL,
    caps                  TEXT     NOT NULL,
    status                TEXT     NOT NULL,
    last_error            TEXT     NOT NULL DEFAULT '',
    failing_since         DATETIME,
    average_response_ms   INTEGER  NOT NULL DEFAULT 0,
    last_ok_at            DATETIME,
    synced_at             DATETIME NOT NULL,
    priority              INTEGER  NOT NULL,
    min_seeders           INTEGER  NOT NULL,
    seeding_mode_override TEXT     NOT NULL,
    seed_ratio            REAL,
    seed_days             INTEGER
);

-- +goose Down
DROP TABLE acquisition_indexers;
