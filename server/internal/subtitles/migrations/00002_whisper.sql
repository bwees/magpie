-- +goose Up
ALTER TABLE subtitles_subtitles ADD COLUMN job_id TEXT;
ALTER TABLE subtitles_wanted ADD COLUMN whisper_attempts INTEGER NOT NULL DEFAULT 0;
CREATE INDEX subtitles_subtitles_job ON subtitles_subtitles (job_id);

-- +goose Down
DROP INDEX subtitles_subtitles_job;
ALTER TABLE subtitles_wanted DROP COLUMN whisper_attempts;
ALTER TABLE subtitles_subtitles DROP COLUMN job_id;
