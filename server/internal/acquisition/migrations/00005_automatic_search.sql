-- +goose Up
CREATE TABLE acquisition_pending_releases (
    id           TEXT PRIMARY KEY,
    title_id     TEXT     NOT NULL,
    episode_ids  TEXT     NOT NULL,
    release_key  TEXT     NOT NULL,
    indexer_id   INTEGER  NOT NULL,
    release      TEXT     NOT NULL,
    trigger      TEXT     NOT NULL,
    reason       TEXT     NOT NULL,
    publish_at   DATETIME NOT NULL,
    added_at     DATETIME NOT NULL
);

CREATE UNIQUE INDEX acquisition_pending_releases_key ON acquisition_pending_releases (title_id, release_key);

-- A queued search job's key: (title, episodes, trigger). A second job for the same key is not queued.
CREATE TABLE acquisition_search_jobs (
    key       TEXT PRIMARY KEY,
    queued_at DATETIME NOT NULL
);

-- +goose Down
DROP TABLE acquisition_search_jobs;
DROP TABLE acquisition_pending_releases;
