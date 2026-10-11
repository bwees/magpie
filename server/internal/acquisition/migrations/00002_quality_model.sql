-- +goose Up
CREATE TABLE acquisition_qualities (
    id         INTEGER PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    source     TEXT NOT NULL,
    resolution INTEGER NOT NULL,
    modifier   TEXT NOT NULL
);

-- Radarr's 30 quality IDs.
INSERT INTO acquisition_qualities (id, name, source, resolution, modifier) VALUES
    (0, 'Unknown', 'UNKNOWN', 0, 'NONE'),
    (1, 'SDTV', 'TV', 480, 'NONE'),
    (2, 'DVD', 'DVD', 480, 'NONE'),
    (3, 'WEBDL-1080p', 'WEBDL', 1080, 'NONE'),
    (4, 'HDTV-720p', 'TV', 720, 'NONE'),
    (5, 'WEBDL-720p', 'WEBDL', 720, 'NONE'),
    (6, 'Bluray-720p', 'BLURAY', 720, 'NONE'),
    (7, 'Bluray-1080p', 'BLURAY', 1080, 'NONE'),
    (8, 'WEBDL-480p', 'WEBDL', 480, 'NONE'),
    (9, 'HDTV-1080p', 'TV', 1080, 'NONE'),
    (10, 'Raw-HD', 'TV', 1080, 'RAWHD'),
    (12, 'WEBRip-480p', 'WEBRIP', 480, 'NONE'),
    (14, 'WEBRip-720p', 'WEBRIP', 720, 'NONE'),
    (15, 'WEBRip-1080p', 'WEBRIP', 1080, 'NONE'),
    (16, 'HDTV-2160p', 'TV', 2160, 'NONE'),
    (17, 'WEBRip-2160p', 'WEBRIP', 2160, 'NONE'),
    (18, 'WEBDL-2160p', 'WEBDL', 2160, 'NONE'),
    (19, 'Bluray-2160p', 'BLURAY', 2160, 'NONE'),
    (20, 'Bluray-480p', 'BLURAY', 480, 'NONE'),
    (21, 'Bluray-576p', 'BLURAY', 576, 'NONE'),
    (22, 'BR-DISK', 'BLURAY', 1080, 'BRDISK'),
    (23, 'DVD-R', 'DVD', 480, 'REMUX'),
    (24, 'WORKPRINT', 'WORKPRINT', 0, 'NONE'),
    (25, 'CAM', 'CAM', 0, 'NONE'),
    (26, 'TELESYNC', 'TELESYNC', 0, 'NONE'),
    (27, 'TELECINE', 'TELECINE', 0, 'NONE'),
    (28, 'DVDSCR', 'DVD', 480, 'SCREENER'),
    (29, 'REGIONAL', 'DVD', 480, 'REGIONAL'),
    (30, 'Remux-1080p', 'BLURAY', 1080, 'REMUX'),
    (31, 'Remux-2160p', 'BLURAY', 2160, 'REMUX');

CREATE TABLE acquisition_quality_profiles (
    id                       TEXT PRIMARY KEY,
    name                     TEXT NOT NULL UNIQUE COLLATE NOCASE,
    upgrade_allowed          BOOLEAN NOT NULL,
    cutoff_item_id           INTEGER NOT NULL,
    min_format_score         INTEGER NOT NULL,
    cutoff_format_score      INTEGER NOT NULL,
    min_upgrade_format_score INTEGER NOT NULL,
    propers_mode             TEXT NOT NULL,
    preset                   TEXT,
    trash_profile_id         TEXT,
    trash_score_set          TEXT,
    created_at               DATETIME NOT NULL,
    updated_at               DATETIME NOT NULL
);

CREATE TABLE acquisition_quality_groups (
    profile_id  TEXT NOT NULL REFERENCES acquisition_quality_profiles (id) ON DELETE CASCADE,
    id          INTEGER NOT NULL CHECK (id >= 1000),
    name        TEXT NOT NULL,
    quality_ids TEXT NOT NULL,
    PRIMARY KEY (profile_id, id)
);

CREATE TABLE acquisition_quality_profile_items (
    profile_id           TEXT NOT NULL REFERENCES acquisition_quality_profiles (id) ON DELETE CASCADE,
    position             INTEGER NOT NULL,
    quality_id           INTEGER REFERENCES acquisition_qualities (id),
    group_id             INTEGER,
    allowed              BOOLEAN NOT NULL,
    min_mb_per_min       REAL NOT NULL,
    preferred_mb_per_min REAL NOT NULL,
    max_mb_per_min       REAL NOT NULL,
    PRIMARY KEY (profile_id, position),
    FOREIGN KEY (profile_id, group_id) REFERENCES acquisition_quality_groups (profile_id, id) ON DELETE CASCADE,
    CHECK ((quality_id IS NULL) <> (group_id IS NULL)),
    CHECK (min_mb_per_min >= 0 AND min_mb_per_min <= preferred_mb_per_min AND preferred_mb_per_min <= max_mb_per_min)
);

CREATE TABLE acquisition_custom_formats (
    id                    TEXT PRIMARY KEY,
    name                  TEXT NOT NULL UNIQUE COLLATE NOCASE,
    trash_id              TEXT UNIQUE,
    trash_scores          TEXT NOT NULL,
    include_when_renaming BOOLEAN NOT NULL,
    specs                 TEXT NOT NULL,
    created_at            DATETIME NOT NULL,
    updated_at            DATETIME NOT NULL
);

CREATE TABLE acquisition_profile_format_scores (
    profile_id    TEXT NOT NULL REFERENCES acquisition_quality_profiles (id) ON DELETE CASCADE,
    format_id     TEXT NOT NULL REFERENCES acquisition_custom_formats (id) ON DELETE CASCADE,
    score         INTEGER NOT NULL,
    user_override BOOLEAN NOT NULL,
    PRIMARY KEY (profile_id, format_id)
);

-- The guide profiles of the last TRaSH sync, listed by GET /trash/profiles and matched on sync.
CREATE TABLE acquisition_trash_profiles (
    trash_id  TEXT PRIMARY KEY,
    app       TEXT NOT NULL,
    name      TEXT NOT NULL,
    profile   TEXT NOT NULL,
    synced_at DATETIME NOT NULL
);

-- +goose Down
DROP TABLE acquisition_trash_profiles;
DROP TABLE acquisition_profile_format_scores;
DROP TABLE acquisition_custom_formats;
DROP TABLE acquisition_quality_profile_items;
DROP TABLE acquisition_quality_groups;
DROP TABLE acquisition_quality_profiles;
DROP TABLE acquisition_qualities;
