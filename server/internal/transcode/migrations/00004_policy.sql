-- +goose Up
ALTER TABLE transcode_jobs ADD COLUMN policy_hash TEXT NOT NULL DEFAULT '';

CREATE TABLE transcode_passed_capabilities (
    capability TEXT PRIMARY KEY,
    agent_id   TEXT     NOT NULL,
    passed_at  DATETIME NOT NULL
);
INSERT INTO transcode_passed_capabilities (capability, agent_id, passed_at)
SELECT capability, MIN(agent_id), MIN(tested_at) FROM transcode_device_capabilities WHERE ok GROUP BY capability;

CREATE INDEX transcode_skips_reason ON transcode_skips (reason);

-- +goose Down
DROP INDEX transcode_skips_reason;
DROP TABLE transcode_passed_capabilities;
ALTER TABLE transcode_jobs DROP COLUMN policy_hash;
