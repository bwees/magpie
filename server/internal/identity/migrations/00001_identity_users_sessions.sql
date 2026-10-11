-- +goose Up
CREATE TABLE identity_users (
    id                   TEXT PRIMARY KEY,
    username             TEXT NOT NULL UNIQUE COLLATE NOCASE,
    display_name         TEXT NOT NULL,
    role                 TEXT NOT NULL CHECK (role IN ('ADMIN', 'REQUESTER')),
    jellyfin_user_id     TEXT UNIQUE,
    is_local_admin       BOOLEAN NOT NULL DEFAULT FALSE,
    password_hash        TEXT,
    auto_approve_movies  BOOLEAN NOT NULL DEFAULT FALSE,
    auto_approve_tv      BOOLEAN NOT NULL DEFAULT FALSE,
    view_others_requests BOOLEAN NOT NULL DEFAULT FALSE,
    report_issues        BOOLEAN NOT NULL DEFAULT FALSE,
    disabled             BOOLEAN NOT NULL DEFAULT FALSE,
    created_at           DATETIME NOT NULL,
    last_login_at        DATETIME
);
CREATE UNIQUE INDEX identity_users_single_local_admin ON identity_users (is_local_admin) WHERE is_local_admin;

CREATE TABLE identity_sessions (
    token_hash   TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES identity_users (id) ON DELETE CASCADE,
    created_at   DATETIME NOT NULL,
    expires_at   DATETIME NOT NULL,
    last_seen_at DATETIME NOT NULL,
    user_agent   TEXT NOT NULL DEFAULT ''
);
CREATE INDEX identity_sessions_user ON identity_sessions (user_id);

-- +goose Down
DROP TABLE identity_sessions;
DROP TABLE identity_users;
