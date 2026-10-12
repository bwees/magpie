-- +goose Up
-- Library now counts a file as available once it is in the library, so no row waits for Jellyfin.
UPDATE activity_pipeline SET stage = 'AVAILABLE', detail = ''
WHERE file = 'PRESENT' AND jellyfin = 0 AND stage = 'IMPORTING';
UPDATE activity_pipeline SET jellyfin = 1 WHERE file = 'PRESENT' AND jellyfin = 0;

-- +goose Down
