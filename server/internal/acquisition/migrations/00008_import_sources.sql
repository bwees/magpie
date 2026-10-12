-- +goose Up
-- The Sonarr/Radarr record each imported format, profile and blocklist row came from, so a resumed migration reuses it.
CREATE TABLE acquisition_import_sources (
    source_key TEXT PRIMARY KEY,
    kind       TEXT NOT NULL CHECK (kind IN ('CUSTOM_FORMAT', 'QUALITY_PROFILE', 'BLOCKLIST')),
    target_id  TEXT NOT NULL
);

-- +goose Down
DROP TABLE acquisition_import_sources;
