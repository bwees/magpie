package api

import "magpie/internal/domain"

const TypeRunAgentJob = "transcode.RunAgentJob"

// Whisper model names, as whisper.cpp spells them.
const (
	WhisperModelLargeV3Turbo = "large-v3-turbo"
	WhisperModelLargeV3      = "large-v3"
	WhisperModelMedium       = "medium"
	WhisperModelSmall        = "small"
)

// RunAgentJob is the only command in v1. Subtitles sends it with Kind WHISPER; Transcode queues the job
// and replies with WhisperCompleted or JobFailed. Deduped on (MediaFile, Language) while a job is open.
type RunAgentJob struct {
	Kind      JobKind            `json:"kind"`
	MediaFile domain.MediaFileID `json:"mediaFile"`
	Language  domain.Language    `json:"language"`
	Model     string             `json:"model"` // WhisperModelLargeV3Turbo by default
}

func (RunAgentJob) Type() string          { return TypeRunAgentJob }
func (c RunAgentJob) AggregateID() string { return string(c.MediaFile) }
func (RunAgentJob) Durable() bool         { return true }
