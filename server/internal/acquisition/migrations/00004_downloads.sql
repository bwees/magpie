-- +goose Up
CREATE TABLE acquisition_downloads (
    id           TEXT PRIMARY KEY,
    title_id     TEXT     NOT NULL,
    episode_ids  TEXT     NOT NULL,
    search_id    TEXT,
    release_id   TEXT     NOT NULL DEFAULT '',
    release_name TEXT     NOT NULL,
    indexer_id   INTEGER  NOT NULL,
    indexer      TEXT     NOT NULL,
    info_hash    TEXT     NOT NULL,
    qbit_hash    TEXT     NOT NULL,
    quality      TEXT     NOT NULL,
    cf_score     INTEGER  NOT NULL,
    size_bytes   INTEGER  NOT NULL,
    state        TEXT     NOT NULL,
    progress     REAL     NOT NULL DEFAULT 0,
    seeding_mode TEXT     NOT NULL,
    trigger      TEXT     NOT NULL,
    override     BOOLEAN  NOT NULL,
    failure      TEXT     NOT NULL DEFAULT '',
    grabbed_at   DATETIME NOT NULL,
    completed_at DATETIME,
    removed_at   DATETIME
);

CREATE INDEX acquisition_downloads_title ON acquisition_downloads (title_id, grabbed_at);
CREATE INDEX acquisition_downloads_active ON acquisition_downloads (removed_at, grabbed_at);

-- +goose Down
DROP TABLE acquisition_downloads;
