-- +goose Up
-- When each fetching download last moved forward or waited in qBittorrent's queue, so a restart keeps its
-- stall and metadata clocks.
CREATE TABLE acquisition_download_marks (
    download_id TEXT PRIMARY KEY,
    progress    REAL     NOT NULL,
    at          DATETIME NOT NULL
);

-- +goose Down
DROP TABLE acquisition_download_marks;
