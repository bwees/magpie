-- +goose Up
-- ETags no longer hash the device, which an NFS share changes on every mount. Library stamps the
-- cleared ones again from disk when it starts.
UPDATE library_media_files SET etag = '';

-- +goose Down
