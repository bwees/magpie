-- +goose Up
CREATE TABLE activity_timeline (
    id          TEXT PRIMARY KEY,
    event_id    TEXT NOT NULL UNIQUE,
    title_id    TEXT NOT NULL,
    episode_ids TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(episode_ids)),
    type        TEXT NOT NULL,
    severity    TEXT NOT NULL,
    text        TEXT NOT NULL,
    detail      TEXT NOT NULL DEFAULT '',
    at          DATETIME NOT NULL
);
CREATE INDEX activity_timeline_title_at ON activity_timeline (title_id, at);

CREATE TABLE activity_history (
    id          TEXT PRIMARY KEY,
    event_id    TEXT NOT NULL UNIQUE,
    type        TEXT NOT NULL,
    severity    TEXT NOT NULL,
    title_id    TEXT,
    title_name  TEXT NOT NULL DEFAULT '',
    episode_ids TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(episode_ids)),
    summary     TEXT NOT NULL,
    detail      TEXT NOT NULL DEFAULT '',
    at          DATETIME NOT NULL
);
CREATE INDEX activity_history_at ON activity_history (at);

CREATE TABLE activity_pipeline (
    title_id       TEXT NOT NULL,
    episode_id     TEXT NOT NULL DEFAULT '',
    season         INTEGER NOT NULL DEFAULT 0,
    number         INTEGER NOT NULL DEFAULT 0,
    wanted         INTEGER NOT NULL DEFAULT 1,
    search         TEXT NOT NULL DEFAULT '',
    download       TEXT NOT NULL DEFAULT '',
    download_id    TEXT NOT NULL DEFAULT '',
    file          TEXT NOT NULL DEFAULT 'NONE',
    media_file_id  TEXT NOT NULL DEFAULT '',
    jellyfin       INTEGER NOT NULL DEFAULT 0,
    note           TEXT NOT NULL DEFAULT '',
    stage          TEXT NOT NULL,
    failed         INTEGER NOT NULL DEFAULT 0,
    detail         TEXT NOT NULL DEFAULT '',
    subs_chip      TEXT CHECK (subs_chip IS NULL OR json_valid(subs_chip)),
    transcode_chip TEXT CHECK (transcode_chip IS NULL OR json_valid(transcode_chip)),
    last_event_id  TEXT NOT NULL,
    updated_at     DATETIME NOT NULL,
    PRIMARY KEY (title_id, episode_id)
);
CREATE INDEX activity_pipeline_media_file ON activity_pipeline (media_file_id);

CREATE TABLE activity_attention (
    id               TEXT PRIMARY KEY,
    kind             TEXT NOT NULL,
    ref              TEXT NOT NULL,
    title_id         TEXT,
    text             TEXT NOT NULL,
    action           TEXT NOT NULL DEFAULT '',
    opened_at        DATETIME NOT NULL,
    opened_event_id  TEXT NOT NULL,
    cleared_at       DATETIME,
    cleared_event_id TEXT
);
CREATE UNIQUE INDEX activity_attention_open ON activity_attention (kind, ref) WHERE cleared_at IS NULL;
CREATE UNIQUE INDEX activity_attention_opened_by ON activity_attention (kind, opened_event_id);
CREATE UNIQUE INDEX activity_attention_cleared_by ON activity_attention (kind, cleared_event_id);

CREATE TABLE activity_notifications (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL,
    event_id   TEXT NOT NULL,
    kind       TEXT NOT NULL,
    title      TEXT NOT NULL,
    body       TEXT NOT NULL DEFAULT '',
    link       TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    read_at    DATETIME,
    UNIQUE (user_id, event_id, kind)
);
CREATE INDEX activity_notifications_user_created ON activity_notifications (user_id, created_at);

-- What a media file, download or search belongs to, for events that carry only that ID.
CREATE TABLE activity_refs (
    kind        TEXT NOT NULL,
    ref         TEXT NOT NULL,
    title_id    TEXT NOT NULL,
    episode_ids TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(episode_ids)),
    PRIMARY KEY (kind, ref)
);

-- Titles removed from the catalog. Events that arrive after the removal add nothing to their timeline or pipeline.
CREATE TABLE activity_removed_titles (
    title_id   TEXT PRIMARY KEY,
    removed_at DATETIME NOT NULL
);

-- +goose Down
DROP TABLE activity_removed_titles;
DROP TABLE activity_refs;
DROP TABLE activity_notifications;
DROP TABLE activity_attention;
DROP TABLE activity_pipeline;
DROP TABLE activity_history;
DROP TABLE activity_timeline;
