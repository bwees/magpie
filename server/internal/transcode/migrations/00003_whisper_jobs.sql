-- +goose Up
ALTER TABLE transcode_jobs ADD COLUMN language TEXT NOT NULL DEFAULT '';
ALTER TABLE transcode_jobs ADD COLUMN whisper_model TEXT NOT NULL DEFAULT '';
CREATE INDEX transcode_jobs_whisper ON transcode_jobs (media_file_id, kind, language, state);

-- +goose Down
DROP INDEX transcode_jobs_whisper;
ALTER TABLE transcode_jobs DROP COLUMN whisper_model;
ALTER TABLE transcode_jobs DROP COLUMN language;
