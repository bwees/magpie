-- +goose Up
-- The Sonarr/Radarr file each migrated media file came from, so a resumed migration reuses it.
CREATE TABLE library_import_sources (
    source_key    TEXT PRIMARY KEY,
    media_file_id TEXT NOT NULL
);

-- +goose Down
DROP TABLE library_import_sources;
