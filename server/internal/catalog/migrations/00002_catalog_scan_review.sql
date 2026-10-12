-- +goose Up
CREATE TABLE catalog_scan_review (
    id           TEXT PRIMARY KEY,
    folder_path  TEXT     NOT NULL UNIQUE,
    kind         TEXT     NOT NULL CHECK (kind IN ('MOVIE', 'SERIES')),
    parsed_title TEXT     NOT NULL,
    parsed_year  INTEGER,
    tmdb_hint    INTEGER,
    candidates   TEXT     NOT NULL DEFAULT '[]',
    state        TEXT     NOT NULL CHECK (state IN ('PENDING', 'MATCHED', 'IGNORED')),
    title_id     TEXT,
    created_at   DATETIME NOT NULL,
    updated_at   DATETIME NOT NULL
);
CREATE INDEX catalog_scan_review_state ON catalog_scan_review (state, created_at);
CREATE INDEX catalog_scan_review_title ON catalog_scan_review (title_id);

-- +goose Down
DROP TABLE catalog_scan_review;
