package api

import (
	"time"

	"magpie/internal/domain"
)

const (
	TypeJobQueued         = "transcode.JobQueued"
	TypeJobStarted        = "transcode.JobStarted"
	TypeJobProgressed     = "transcode.JobProgressed"
	TypeTranscodeVerified = "transcode.TranscodeVerified"
	TypeTranscodeSkipped  = "transcode.TranscodeSkipped"
	TypeJobFailed         = "transcode.JobFailed"
	TypeJobCancelled      = "transcode.JobCancelled"
	TypeWhisperCompleted  = "transcode.WhisperCompleted"
	TypeAgentOnline       = "transcode.AgentOnline"
	TypeAgentOffline      = "transcode.AgentOffline"
)

// SkipReason says why a file keeps its original encode.
type SkipReason string

const (
	SkipExcluded       SkipReason = "EXCLUDED"
	SkipHDRDynamic     SkipReason = "HDR_DYNAMIC"
	SkipAlreadyTarget  SkipReason = "ALREADY_TARGET"
	SkipLowBitrate     SkipReason = "LOW_BITRATE"
	SkipNotWorthIt     SkipReason = "NOT_WORTH_IT"
	SkipNoCapableAgent SkipReason = "NO_CAPABLE_AGENT"
)

// TranscodeStats.VideoCodec values are ffmpeg codec names.
const (
	VideoCodecHEVC = "hevc"
	VideoCodecAV1  = "av1"
)

// JobQueued is durable. A job entered QUEUED for the first time (not on retries).
type JobQueued struct {
	Job       domain.JobID       `json:"job"`
	Kind      JobKind            `json:"kind"`
	MediaFile domain.MediaFileID `json:"mediaFile"`
	Title     domain.TitleID     `json:"title"`
	Origin    domain.Origin      `json:"origin"` // TRANSCODE jobs: from CutoffMet
}

// JobStarted is durable. An agent accepted the job (state LEASED), once per attempt.
type JobStarted struct {
	Job       domain.JobID       `json:"job"`
	Kind      JobKind            `json:"kind"`
	MediaFile domain.MediaFileID `json:"mediaFile"`
	Agent     domain.AgentID     `json:"agent"`
	Attempt   int                `json:"attempt"`
}

// TranscodeVerified is durable. Library re-checks SourceETag before swapping.
type TranscodeVerified struct {
	Job        domain.JobID       `json:"job"`
	MediaFile  domain.MediaFileID `json:"mediaFile"`
	StagedPath string             `json:"stagedPath"` // on the same filesystem as the media file
	SHA256     string             `json:"sha256"`
	SourceETag string             `json:"sourceETag"` // ETag of the source when the job was leased
	Stats      TranscodeStats     `json:"stats"`
}

type TranscodeStats struct {
	OldSize      int64          `json:"oldSize"`
	NewSize      int64          `json:"newSize"`
	VideoCodec   string         `json:"videoCodec"` // VideoCodecHEVC | VideoCodecAV1
	BitDepth     int            `json:"bitDepth"`
	VMAFMean     *float64       `json:"vmafMean"`
	DurationSec  float64        `json:"durationSec"`
	Agent        domain.AgentID `json:"agent"`
	Encoder      string         `json:"encoder"` // hevc_nvenc, av1_qsv, …
	DroppedAudio []int          `json:"droppedAudio"`
	DroppedSubs  []int          `json:"droppedSubs"`
}

// TranscodeSkipped is durable. The file keeps its original encode.
type TranscodeSkipped struct {
	MediaFile domain.MediaFileID `json:"mediaFile"`
	Reason    SkipReason         `json:"reason"`
	Detail    string             `json:"detail"`
}

// JobFailed is durable and published on every failed attempt; Final marks the last one.
type JobFailed struct {
	Job       domain.JobID       `json:"job"`
	Kind      JobKind            `json:"kind"`
	MediaFile domain.MediaFileID `json:"mediaFile"`
	Reason    string             `json:"reason"`
	Final     bool               `json:"final"`    // true when retries are exhausted
	Language  domain.Language    `json:"language"` // WHISPER jobs; empty for TRANSCODE
}

// JobCancelled is durable: a transcode job stopped before it finished. A cancelled Whisper job publishes a
// final JobFailed instead, since Subtitles waits for one.
type JobCancelled struct {
	Job       domain.JobID       `json:"job"`
	MediaFile domain.MediaFileID `json:"mediaFile"`
	Reason    string             `json:"reason"`
}

// WhisperCompleted is durable. Subtitles moves the staged SRT to its sidecar path.
type WhisperCompleted struct {
	Job        domain.JobID       `json:"job"`
	MediaFile  domain.MediaFileID `json:"mediaFile"`
	Language   domain.Language    `json:"language"`
	StagedPath string             `json:"stagedPath"` // .srt in .magpie-staging
}

// AgentOnline is durable.
type AgentOnline struct {
	Agent    domain.AgentID `json:"agent"`
	Hostname string         `json:"hostname"`
}

// AgentOffline is durable.
type AgentOffline struct {
	Agent domain.AgentID `json:"agent"`
	Since time.Time      `json:"since"`
}

// JobProgressed is ephemeral: bus only, never stored.
type JobProgressed struct {
	Job       domain.JobID       `json:"job"`
	MediaFile domain.MediaFileID `json:"mediaFile"`
	Stage     JobState           `json:"stage"`
	Progress  float64            `json:"progress"` // 0..1
	Speed     float64            `json:"speed"`    // encode speed × realtime, or bytes/s while transferring
	ETA       time.Duration      `json:"eta"`
}

func (JobQueued) Type() string          { return TypeJobQueued }
func (e JobQueued) AggregateID() string { return string(e.MediaFile) }
func (JobQueued) Durable() bool         { return true }

func (JobStarted) Type() string          { return TypeJobStarted }
func (e JobStarted) AggregateID() string { return string(e.MediaFile) }
func (JobStarted) Durable() bool         { return true }

func (TranscodeVerified) Type() string          { return TypeTranscodeVerified }
func (e TranscodeVerified) AggregateID() string { return string(e.MediaFile) }
func (TranscodeVerified) Durable() bool         { return true }

func (TranscodeSkipped) Type() string          { return TypeTranscodeSkipped }
func (e TranscodeSkipped) AggregateID() string { return string(e.MediaFile) }
func (TranscodeSkipped) Durable() bool         { return true }

func (JobFailed) Type() string          { return TypeJobFailed }
func (e JobFailed) AggregateID() string { return string(e.MediaFile) }
func (JobFailed) Durable() bool         { return true }

func (JobCancelled) Type() string          { return TypeJobCancelled }
func (e JobCancelled) AggregateID() string { return string(e.MediaFile) }
func (JobCancelled) Durable() bool         { return true }

func (WhisperCompleted) Type() string          { return TypeWhisperCompleted }
func (e WhisperCompleted) AggregateID() string { return string(e.MediaFile) }
func (WhisperCompleted) Durable() bool         { return true }

func (AgentOnline) Type() string          { return TypeAgentOnline }
func (e AgentOnline) AggregateID() string { return string(e.Agent) }
func (AgentOnline) Durable() bool         { return true }

func (AgentOffline) Type() string          { return TypeAgentOffline }
func (e AgentOffline) AggregateID() string { return string(e.Agent) }
func (AgentOffline) Durable() bool         { return true }

func (JobProgressed) Type() string          { return TypeJobProgressed }
func (e JobProgressed) AggregateID() string { return string(e.MediaFile) }
func (JobProgressed) Durable() bool         { return false }

// Messages lists every event and command this package declares, for the bus registry and the live schemas.
func Messages() []domain.Message {
	return []domain.Message{
		JobQueued{}, JobStarted{}, JobProgressed{}, TranscodeVerified{}, TranscodeSkipped{},
		JobFailed{}, JobCancelled{}, WhisperCompleted{}, AgentOnline{}, AgentOffline{},
		RunAgentJob{},
	}
}
