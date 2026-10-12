-- +goose Up
CREATE TABLE library_root_folders (
    id         TEXT PRIMARY KEY,
    kind       TEXT     NOT NULL,
    path       TEXT     NOT NULL UNIQUE,
    device_id  INTEGER  NOT NULL,
    created_at DATETIME NOT NULL
);

CREATE TABLE library_media_files (
    id               TEXT PRIMARY KEY,
    title_id         TEXT     NOT NULL,
    root_folder_id   TEXT     NOT NULL REFERENCES library_root_folders (id),
    path             TEXT     NOT NULL,
    size             INTEGER  NOT NULL,
    quality          TEXT     NOT NULL,
    release_name     TEXT     NOT NULL,
    os_hash          TEXT     NOT NULL,
    media_info       TEXT     NOT NULL,
    etag             TEXT     NOT NULL,
    origin           TEXT     NOT NULL,
    imported_at      DATETIME NOT NULL,
    download_id      TEXT,
    info_hash        TEXT     NOT NULL DEFAULT '',
    transcoded       BOOLEAN  NOT NULL DEFAULT FALSE,
    transcoded_at    DATETIME,
    original_size    INTEGER,
    jellyfin_item_id TEXT,
    available_at     DATETIME
);

CREATE INDEX library_media_files_title ON library_media_files (title_id, imported_at);
CREATE INDEX library_media_files_download ON library_media_files (download_id);

CREATE TABLE library_media_file_episodes (
    media_file_id TEXT    NOT NULL REFERENCES library_media_files (id) ON DELETE CASCADE,
    episode_id    TEXT    NOT NULL,
    season        INTEGER NOT NULL,
    number        INTEGER NOT NULL,
    PRIMARY KEY (media_file_id, episode_id)
);

CREATE INDEX library_media_file_episodes_episode ON library_media_file_episodes (episode_id);

CREATE TABLE library_imports (
    download_id  TEXT PRIMARY KEY,
    title_id     TEXT     NOT NULL,
    release_name TEXT     NOT NULL,
    status       TEXT     NOT NULL,
    attempts     INTEGER  NOT NULL DEFAULT 0,
    last_error   TEXT     NOT NULL DEFAULT '',
    payload      TEXT     NOT NULL,
    files        TEXT     NOT NULL DEFAULT '[]',
    mapping      TEXT,
    created_at   DATETIME NOT NULL,
    updated_at   DATETIME NOT NULL
);

CREATE INDEX library_imports_status ON library_imports (status, updated_at);

-- +goose Down
DROP TABLE library_imports;
DROP TABLE library_media_file_episodes;
DROP TABLE library_media_files;
DROP TABLE library_root_folders;
