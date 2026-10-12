-- +goose Up
-- The issues whose REGRAB already ran, so a repeated action for one issue blocklists and searches once.
CREATE TABLE acquisition_issue_regrabs (
    issue_id   TEXT PRIMARY KEY,
    handled_at DATETIME NOT NULL
);

-- +goose Down
DROP TABLE acquisition_issue_regrabs;
