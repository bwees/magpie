-- +goose Up
-- An unmonitored episode or movie without a file is no longer a failure.
UPDATE activity_pipeline SET stage = 'REQUESTED', failed = 0, detail = ''
WHERE wanted = 0 AND file = 'NONE' AND download NOT IN ('ACTIVE', 'DONE');

-- +goose Down
