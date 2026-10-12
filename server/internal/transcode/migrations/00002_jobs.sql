-- +goose Up
CREATE TABLE transcode_eligible_files (
    media_file_id TEXT PRIMARY KEY,
    title_id      TEXT     NOT NULL,
    origin        TEXT     NOT NULL,
    cutoff_met_at DATETIME NOT NULL
);
CREATE INDEX transcode_eligible_files_title ON transcode_eligible_files (title_id);

CREATE TABLE transcode_jobs (
    id               TEXT PRIMARY KEY,
    kind             TEXT     NOT NULL,
    media_file_id    TEXT     NOT NULL,
    title_id         TEXT     NOT NULL,
    origin           TEXT     NOT NULL,
    state            TEXT     NOT NULL,
    priority         INTEGER  NOT NULL,
    attempt          INTEGER  NOT NULL,
    not_before       DATETIME,
    target_codec     TEXT     NOT NULL DEFAULT '',
    max_rate_kbps    INTEGER  NOT NULL DEFAULT 0,
    hdr10            INTEGER  NOT NULL DEFAULT 0,
    agent_id         TEXT,
    device_id        TEXT,
    offered_at       DATETIME,
    lease_expires_at DATETIME,
    prefetch         INTEGER  NOT NULL DEFAULT 0,
    last_agent_id    TEXT,
    avoid_agent_id   TEXT,
    avoid_until      DATETIME,
    excluded_agents  TEXT     NOT NULL DEFAULT '[]' CHECK (json_valid(excluded_agents)),
    source_etag      TEXT     NOT NULL DEFAULT '',
    transfer_etag    TEXT     NOT NULL DEFAULT '',
    source_size      INTEGER  NOT NULL DEFAULT 0,
    staging_path     TEXT     NOT NULL DEFAULT '',
    spec            TEXT CHECK (spec IS NULL OR json_valid(spec)),
    result           TEXT CHECK (result IS NULL OR json_valid(result)),
    output_sha256    TEXT     NOT NULL DEFAULT '',
    output_size      INTEGER  NOT NULL DEFAULT 0,
    progress         REAL     NOT NULL DEFAULT 0,
    error            TEXT     NOT NULL DEFAULT '',
    queued_at        DATETIME NOT NULL,
    updated_at       DATETIME NOT NULL,
    finished_at      DATETIME
);
CREATE INDEX transcode_jobs_state ON transcode_jobs (state, priority, queued_at);
CREATE INDEX transcode_jobs_media_file ON transcode_jobs (media_file_id, queued_at);
CREATE INDEX transcode_jobs_title ON transcode_jobs (title_id);
CREATE INDEX transcode_jobs_agent ON transcode_jobs (agent_id);

CREATE TABLE transcode_job_attempts (
    job_id     TEXT     NOT NULL REFERENCES transcode_jobs (id) ON DELETE CASCADE,
    attempt    INTEGER  NOT NULL,
    agent_id   TEXT     NOT NULL,
    outcome    TEXT     NOT NULL DEFAULT '',
    error      TEXT     NOT NULL DEFAULT '',
    started_at DATETIME NOT NULL,
    ended_at   DATETIME,
    PRIMARY KEY (job_id, attempt)
);

CREATE TABLE transcode_exclusions (
    scope TEXT NOT NULL,
    ref   TEXT NOT NULL,
    PRIMARY KEY (scope, ref)
);

CREATE TABLE transcode_skips (
    media_file_id TEXT PRIMARY KEY,
    reason        TEXT     NOT NULL,
    detail        TEXT     NOT NULL DEFAULT '',
    policy_hash   TEXT     NOT NULL,
    skipped_at    DATETIME NOT NULL
);

-- +goose Down
DROP TABLE transcode_skips;
DROP TABLE transcode_exclusions;
DROP TABLE transcode_job_attempts;
DROP TABLE transcode_jobs;
DROP TABLE transcode_eligible_files;
