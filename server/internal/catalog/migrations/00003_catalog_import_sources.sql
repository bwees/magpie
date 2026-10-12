-- +goose Up
-- The Sonarr/Radarr records a migration created or found each Title and episode for, so a resumed run reuses them.
CREATE TABLE catalog_import_sources (
    source_key TEXT PRIMARY KEY,
    kind       TEXT NOT NULL CHECK (kind IN ('TITLE', 'EPISODE')),
    target_id  TEXT NOT NULL
);

-- Sonarr's alternate titles, kept beside TMDB's aliases so a metadata refresh doesn't drop them.
ALTER TABLE catalog_titles ADD COLUMN imported_aliases TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE catalog_titles DROP COLUMN imported_aliases;
DROP TABLE catalog_import_sources;
