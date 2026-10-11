-- +goose Up
CREATE TABLE catalog_titles (
    id                     TEXT PRIMARY KEY,
    kind                   TEXT NOT NULL CHECK (kind IN ('MOVIE', 'SERIES')),
    tmdb_id                INTEGER NOT NULL,
    tvdb_id                TEXT,
    imdb_id                TEXT,
    external_id_source     TEXT NOT NULL DEFAULT 'TMDB' CHECK (external_id_source IN ('TMDB', 'IMPORT')),
    name                   TEXT NOT NULL,
    original_name          TEXT NOT NULL DEFAULT '',
    aliases                TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(aliases)),
    year                   INTEGER NOT NULL DEFAULT 0,
    overview               TEXT NOT NULL DEFAULT '',
    original_language      TEXT NOT NULL DEFAULT '',
    runtime_minutes        INTEGER NOT NULL DEFAULT 0,
    series_status          TEXT NOT NULL DEFAULT '',
    profile_id             TEXT NOT NULL DEFAULT '',
    monitored              BOOLEAN NOT NULL DEFAULT FALSE,
    monitor_future_seasons BOOLEAN NOT NULL DEFAULT FALSE,
    episode_group_id       TEXT NOT NULL DEFAULT '',
    languages_override     TEXT CHECK (languages_override IS NULL OR json_valid(languages_override)),
    origin                 TEXT NOT NULL,
    requested_by           TEXT,
    request_id             TEXT,
    added_at               DATETIME NOT NULL,
    metadata_refreshed_at  DATETIME NOT NULL,
    release_theatrical_at  DATETIME,
    release_digital_at     DATETIME,
    release_physical_at    DATETIME,
    released               BOOLEAN NOT NULL DEFAULT FALSE,
    released_at            DATETIME,
    UNIQUE (kind, tmdb_id)
);
CREATE INDEX catalog_titles_name_year ON catalog_titles (name COLLATE NOCASE, year);

CREATE TABLE catalog_seasons (
    id        TEXT PRIMARY KEY,
    title_id  TEXT NOT NULL REFERENCES catalog_titles (id) ON DELETE CASCADE,
    number    INTEGER NOT NULL,
    monitored BOOLEAN NOT NULL DEFAULT FALSE,
    UNIQUE (title_id, number)
);

CREATE TABLE catalog_episodes (
    id              TEXT PRIMARY KEY,
    title_id        TEXT NOT NULL REFERENCES catalog_titles (id) ON DELETE CASCADE,
    tmdb_episode_id INTEGER NOT NULL UNIQUE,
    season_number   INTEGER NOT NULL,
    episode_number  INTEGER NOT NULL,
    aired_season    INTEGER NOT NULL,
    aired_number    INTEGER NOT NULL,
    name            TEXT NOT NULL DEFAULT '',
    air_date        DATETIME,
    runtime_minutes INTEGER NOT NULL DEFAULT 0,
    monitored       BOOLEAN NOT NULL DEFAULT FALSE,
    released        BOOLEAN NOT NULL DEFAULT FALSE,
    released_at     DATETIME
);
CREATE INDEX catalog_episodes_title_numbering ON catalog_episodes (title_id, season_number, episode_number);

CREATE TABLE catalog_images (
    title_id   TEXT NOT NULL REFERENCES catalog_titles (id) ON DELETE CASCADE,
    kind       TEXT NOT NULL CHECK (kind IN ('POSTER', 'BACKDROP', 'LOGO')),
    tmdb_path  TEXT NOT NULL,
    local_path TEXT NOT NULL,
    fetched_at DATETIME NOT NULL,
    PRIMARY KEY (title_id, kind)
);

CREATE TABLE catalog_languages (
    position         INTEGER PRIMARY KEY,
    language         TEXT NOT NULL UNIQUE,
    forced           BOOLEAN NOT NULL DEFAULT FALSE,
    hearing_impaired BOOLEAN NOT NULL DEFAULT FALSE
);
INSERT INTO catalog_languages (position, language) VALUES (0, 'en');

-- +goose Down
DROP TABLE catalog_languages;
DROP TABLE catalog_images;
DROP TABLE catalog_episodes;
DROP TABLE catalog_seasons;
DROP TABLE catalog_titles;
