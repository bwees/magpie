-- +goose Up
ALTER TABLE catalog_titles ADD COLUMN allow_upgrades BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE catalog_titles DROP COLUMN allow_upgrades;
