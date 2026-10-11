-- +goose Up
CREATE TABLE requests_requests (
    id             TEXT PRIMARY KEY,
    user_id        TEXT NOT NULL,
    kind           TEXT NOT NULL CHECK (kind IN ('MOVIE', 'SERIES')),
    tmdb_id        INTEGER NOT NULL,
    title_id       TEXT,
    scope          TEXT NOT NULL CHECK (scope IN ('MOVIE', 'ALL_SEASONS', 'SEASONS')),
    seasons        TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(seasons)),
    profile_id     TEXT NOT NULL,
    status         TEXT NOT NULL CHECK (status IN ('PENDING', 'APPROVED', 'PROCESSING', 'PARTIALLY_AVAILABLE', 'AVAILABLE', 'DECLINED', 'FAILED')),
    created_at     DATETIME NOT NULL,
    decided_by     TEXT,
    decided_at     DATETIME,
    decline_reason TEXT,
    failure_reason TEXT,
    available_at   DATETIME
);
CREATE INDEX requests_requests_user ON requests_requests (user_id, created_at);
CREATE INDEX requests_requests_tmdb ON requests_requests (kind, tmdb_id);
CREATE INDEX requests_requests_title ON requests_requests (title_id);
CREATE INDEX requests_requests_status ON requests_requests (status);

-- A movie's one item has no episode; the expression index keeps it unique too.
CREATE TABLE requests_request_items (
    request_id   TEXT NOT NULL REFERENCES requests_requests (id) ON DELETE CASCADE,
    episode_id   TEXT,
    available_at DATETIME
);
CREATE UNIQUE INDEX requests_request_items_key ON requests_request_items (request_id, COALESCE(episode_id, ''));

-- +goose Down
DROP TABLE requests_request_items;
DROP TABLE requests_requests;
