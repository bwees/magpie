-- +goose Up
-- The Sonarr/Radarr records a migration created or found each Title and episode for, so a resumed run reuses them.
CREATE TABLE catalog_import_sources (
    source_key TEXT PRIMARY KEY,
    kind       TEXT NOT NULL CHECK (kind IN ('TITLE', 'EPISODE')),
    target_id  TEXT NOT NULL
);

-- +goose Down
DROP TABLE catalog_import_sources;
