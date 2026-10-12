-- +goose Up
-- A file counts as available once it is in the library, without waiting for Jellyfin.
UPDATE library_media_files SET available_at = imported_at WHERE available_at IS NULL;
ALTER TABLE library_media_files DROP COLUMN jellyfin_item_id;

-- +goose Down
ALTER TABLE library_media_files ADD COLUMN jellyfin_item_id TEXT;
