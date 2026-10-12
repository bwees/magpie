-- +goose Up
CREATE TABLE issues (
    id             TEXT PRIMARY KEY,
    user_id        TEXT     NOT NULL,
    title_id       TEXT     NOT NULL,
    episode_id     TEXT,
    kind           TEXT     NOT NULL CHECK (kind IN ('VIDEO', 'AUDIO', 'SUBTITLE', 'OTHER')),
    text           TEXT     NOT NULL,
    status         TEXT     NOT NULL CHECK (status IN ('OPEN', 'ACTION_PENDING', 'RESOLVED')),
    pending_action TEXT CHECK (pending_action IN ('REGRAB', 'RERUN_SUBTITLES')),
    created_at     DATETIME NOT NULL,
    resolved_at    DATETIME,
    resolved_by    TEXT
);
CREATE INDEX issues_status ON issues (status, created_at);
CREATE INDEX issues_title ON issues (title_id, status);
CREATE INDEX issues_user ON issues (user_id, created_at);

CREATE TABLE issue_comments (
    id         TEXT PRIMARY KEY,
    issue_id   TEXT     NOT NULL REFERENCES issues (id) ON DELETE CASCADE,
    user_id    TEXT     NOT NULL,
    body       TEXT     NOT NULL,
    created_at DATETIME NOT NULL
);
CREATE INDEX issue_comments_issue ON issue_comments (issue_id, created_at);

-- +goose Down
DROP TABLE issue_comments;
DROP TABLE issues;
