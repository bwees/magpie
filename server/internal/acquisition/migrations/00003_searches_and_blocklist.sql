-- +goose Up
CREATE TABLE acquisition_searches (
    id           TEXT PRIMARY KEY,
    title_id     TEXT     NOT NULL,
    episode_ids  TEXT     NOT NULL,
    trigger      TEXT     NOT NULL,
    started_at   DATETIME NOT NULL,
    completed_at DATETIME,
    accepted     INTEGER  NOT NULL DEFAULT 0,
    total        INTEGER  NOT NULL DEFAULT 0
);

CREATE INDEX acquisition_searches_title ON acquisition_searches (title_id, started_at);

CREATE TABLE acquisition_blocklist (
    id           TEXT PRIMARY KEY,
    title_id     TEXT     NOT NULL,
    info_hash    TEXT     NOT NULL DEFAULT '',
    release_name TEXT     NOT NULL,
    indexer      TEXT     NOT NULL DEFAULT '',
    reason       TEXT     NOT NULL,
    at           DATETIME NOT NULL,
    expires_at   DATETIME
);

CREATE INDEX acquisition_blocklist_title ON acquisition_blocklist (title_id, at);

-- +goose Down
DROP TABLE acquisition_blocklist;
DROP TABLE acquisition_searches;
