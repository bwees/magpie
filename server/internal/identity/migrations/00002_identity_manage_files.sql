-- +goose Up
ALTER TABLE identity_users ADD COLUMN manage_files BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE identity_users DROP COLUMN manage_files;
