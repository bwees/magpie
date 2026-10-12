-- +goose Up
-- Scored from the release name (else the path) and size at import; rescored only when the profile changes.
CREATE TABLE acquisition_file_scores (
    media_file_id TEXT PRIMARY KEY,
    title_id      TEXT     NOT NULL,
    episode_ids   TEXT     NOT NULL,
    quality       TEXT     NOT NULL,
    release_name  TEXT     NOT NULL,
    path          TEXT     NOT NULL,
    size_bytes    INTEGER  NOT NULL,
    cf_score      INTEGER  NOT NULL,
    profile_id    TEXT     NOT NULL,
    cutoff_met    BOOLEAN  NOT NULL,
    transcoded    BOOLEAN  NOT NULL DEFAULT FALSE,
    origin        TEXT     NOT NULL,
    evaluated_at  DATETIME NOT NULL
);

CREATE INDEX acquisition_file_scores_title ON acquisition_file_scores (title_id);
CREATE INDEX acquisition_file_scores_profile ON acquisition_file_scores (profile_id);

-- +goose Down
DROP TABLE acquisition_file_scores;
