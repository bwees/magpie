-- +goose Up
CREATE TABLE subtitles_files (
    media_file_id TEXT PRIMARY KEY,
    title_id      TEXT     NOT NULL,
    settled       BOOLEAN  NOT NULL DEFAULT TRUE,
    imported_at   DATETIME NOT NULL
);

CREATE INDEX subtitles_files_title ON subtitles_files (title_id, media_file_id);

CREATE TABLE subtitles_subtitles (
    id               TEXT PRIMARY KEY,
    media_file_id    TEXT     NOT NULL,
    language         TEXT     NOT NULL,
    forced           BOOLEAN  NOT NULL,
    sdh              BOOLEAN  NOT NULL,
    source           TEXT     NOT NULL,
    stream_index     INTEGER,
    provider_file_id INTEGER,
    release_name     TEXT     NOT NULL DEFAULT '',
    score            INTEGER,
    max_score        INTEGER,
    synced           BOOLEAN  NOT NULL DEFAULT FALSE,
    sync_offset_ms   INTEGER,
    path             TEXT     NOT NULL DEFAULT '',
    added_at         DATETIME NOT NULL,
    upgrade_until    DATETIME
);

CREATE INDEX subtitles_subtitles_file ON subtitles_subtitles (media_file_id);
CREATE INDEX subtitles_subtitles_path ON subtitles_subtitles (path);
CREATE INDEX subtitles_subtitles_upgrade ON subtitles_subtitles (upgrade_until);

CREATE TABLE subtitles_wanted (
    media_file_id     TEXT     NOT NULL,
    language          TEXT     NOT NULL,
    forced            BOOLEAN  NOT NULL,
    hearing_impaired  BOOLEAN  NOT NULL,
    position          INTEGER  NOT NULL,
    status            TEXT     NOT NULL,
    source            TEXT,
    reason            TEXT     NOT NULL DEFAULT '',
    priority          TEXT     NOT NULL,
    next_attempt_at   DATETIME,
    candidate         TEXT,
    created_at        DATETIME NOT NULL,
    PRIMARY KEY (media_file_id, language, forced)
);

CREATE INDEX subtitles_wanted_due ON subtitles_wanted (status, next_attempt_at);

CREATE TABLE subtitles_blocklist (
    media_file_id    TEXT    NOT NULL,
    provider_file_id INTEGER NOT NULL,
    PRIMARY KEY (media_file_id, provider_file_id)
);

CREATE TABLE subtitles_opensubtitles_session (
    id                  INTEGER PRIMARY KEY CHECK (id = 1),
    downloads_remaining INTEGER,
    reset_at            DATETIME,
    vip                 BOOLEAN  NOT NULL DEFAULT FALSE,
    token_expires_at    DATETIME,
    paused_until        DATETIME,
    problem             TEXT     NOT NULL DEFAULT ''
);

-- +goose Down
DROP TABLE subtitles_opensubtitles_session;
DROP TABLE subtitles_blocklist;
DROP TABLE subtitles_wanted;
DROP TABLE subtitles_subtitles;
DROP TABLE subtitles_files;
