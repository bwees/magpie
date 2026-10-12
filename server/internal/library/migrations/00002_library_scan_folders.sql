-- +goose Up
CREATE TABLE library_scan_folders (
    path           TEXT PRIMARY KEY,
    root_folder_id TEXT     NOT NULL,
    kind           TEXT     NOT NULL,
    status         TEXT     NOT NULL CHECK (status IN ('FOUND', 'LINKED', 'IGNORED')),
    title_id       TEXT,
    found_at       DATETIME NOT NULL,
    updated_at     DATETIME NOT NULL
);

ALTER TABLE library_media_files ADD COLUMN rename_problem TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE library_media_files DROP COLUMN rename_problem;
DROP TABLE library_scan_folders;
