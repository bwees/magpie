-- +goose Up
ALTER TABLE subtitles_files ADD COLUMN episode_ids TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(episode_ids));

CREATE TABLE subtitles_issue_reruns (
    issue_id   TEXT PRIMARY KEY,
    handled_at DATETIME NOT NULL
);

-- +goose Down
DROP TABLE subtitles_issue_reruns;
ALTER TABLE subtitles_files DROP COLUMN episode_ids;
