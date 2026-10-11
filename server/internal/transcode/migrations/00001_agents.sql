-- +goose Up
CREATE TABLE transcode_agents (
    id                 TEXT PRIMARY KEY,
    hostname           TEXT NOT NULL,
    version            TEXT NOT NULL,
    ffmpeg_version     TEXT NOT NULL,
    reported_mode      TEXT NOT NULL,
    mode_override      TEXT,
    slots_override     INTEGER,
    online             INTEGER NOT NULL DEFAULT 0,
    draining           INTEGER NOT NULL DEFAULT 0,
    last_seen_at       DATETIME NOT NULL,
    scratch_free_bytes INTEGER NOT NULL DEFAULT 0,
    throughput_bps     INTEGER NOT NULL DEFAULT 0,
    created_at         DATETIME NOT NULL
);
CREATE INDEX transcode_agents_online_last_seen ON transcode_agents (online, last_seen_at);

CREATE TABLE transcode_agent_devices (
    agent_id     TEXT NOT NULL REFERENCES transcode_agents (id) ON DELETE CASCADE,
    device_id    TEXT NOT NULL,
    vendor       TEXT NOT NULL,
    name         TEXT NOT NULL,
    driver       TEXT NOT NULL,
    max_sessions INTEGER NOT NULL DEFAULT 0,
    healthy      INTEGER NOT NULL DEFAULT 1,
    retest_at    DATETIME,
    tested_at    DATETIME NOT NULL,
    PRIMARY KEY (agent_id, device_id)
);

CREATE TABLE transcode_device_capabilities (
    agent_id   TEXT NOT NULL,
    device_id  TEXT NOT NULL,
    capability TEXT NOT NULL,
    ok         INTEGER NOT NULL,
    error      TEXT NOT NULL DEFAULT '',
    tested_at  DATETIME NOT NULL,
    PRIMARY KEY (agent_id, device_id, capability),
    FOREIGN KEY (agent_id, device_id) REFERENCES transcode_agent_devices (agent_id, device_id) ON DELETE CASCADE
);

-- +goose Down
DROP TABLE transcode_device_capabilities;
DROP TABLE transcode_agent_devices;
DROP TABLE transcode_agents;
