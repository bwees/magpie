// Package api is Transcode's public contract: the job Reader, the events Transcode publishes and the
// RunAgentJob command it accepts. The agent protocol lives in agentproto.
// Spec: docs/spec/contexts/transcode.md.
package api

import (
	"context"
	"errors"
	"slices"
	"time"

	"magpie/internal/domain"
)

type JobKind string

const (
	JobKindTranscode JobKind = "TRANSCODE"
	JobKindWhisper   JobKind = "WHISPER"
)

type JobState string

const (
	StateQueued     JobState = "QUEUED"
	StateOffered    JobState = "OFFERED"
	StateLeased     JobState = "LEASED"
	StateFetching   JobState = "FETCHING"
	StateEncoding   JobState = "ENCODING"
	StateChecking   JobState = "CHECKING" // the agent checks its output before the upload; VERIFYING is the server's check
	StateUploading  JobState = "UPLOADING"
	StateVerifying  JobState = "VERIFYING"
	StateVerified   JobState = "VERIFIED"
	StateSwapped    JobState = "SWAPPED"
	StateCompleted  JobState = "COMPLETED"
	StateFailed     JobState = "FAILED"
	StateNotWorthIt JobState = "NOT_WORTH_IT"
	StateCancelled  JobState = "CANCELLED"
)

var activeStates = []JobState{StateOffered, StateLeased, StateFetching, StateEncoding, StateChecking, StateUploading, StateVerifying}

// Active reports whether s is one of OFFERED through VERIFYING, the states ActiveJobs returns.
func (s JobState) Active() bool { return slices.Contains(activeStates, s) }

var ErrNotFound = errors.New("transcode: not found")

// Reader is read-only and backed by Transcode's repository.
type Reader interface {
	// Job returns one job or ErrNotFound.
	Job(ctx context.Context, id domain.JobID) (Job, error)
	// JobsForMediaFile returns every job for the file, newest first. An unknown file returns an empty slice.
	JobsForMediaFile(ctx context.Context, id domain.MediaFileID) ([]Job, error)
	// ActiveJobs returns jobs in OFFERED..VERIFYING, ordered by priority (highest first) then age (oldest first).
	ActiveJobs(ctx context.Context) ([]Job, error)
	// QueueSummary returns counts per state and the estimated bytes saved for queued transcodes.
	QueueSummary(ctx context.Context) (QueueSummary, error)
	// Agents returns all known agents with devices, capabilities and current jobs, ordered by hostname.
	Agents(ctx context.Context) ([]Agent, error)
	// IsExcluded reports whether a title is excluded from transcoding. Unknown titles are not excluded.
	IsExcluded(ctx context.Context, title domain.TitleID) (bool, error)
	// Target returns the active transcode target (codec, preset, caps).
	Target(ctx context.Context) (Target, error)
}

type Job struct {
	ID         domain.JobID
	Kind       JobKind
	MediaFile  domain.MediaFileID
	State      JobState
	Priority   int // higher is offered first
	Agent      *domain.AgentID
	Attempt    int
	Progress   float64 // 0..1, last durable value
	Error      string
	QueuedAt   time.Time
	FinishedAt *time.Time
}

type QueueSummary struct {
	Counts map[JobState]int // states with no jobs are absent
	// EstimatedSavedBytes is the expected size reduction of every QUEUED transcode job.
	EstimatedSavedBytes int64
}

// TransferMode is how an agent reads the source.
type TransferMode string

const (
	TransferDownloadFirst TransferMode = "DOWNLOAD_FIRST"
	TransferDirectRead    TransferMode = "DIRECT_READ"
)

type Agent struct {
	ID               domain.AgentID
	Hostname         string
	Version          string
	FFmpegVersion    string
	Mode             TransferMode
	SlotsOverride    *int
	Online           bool
	Draining         bool
	LastSeenAt       time.Time
	ScratchFreeBytes int64
	ThroughputBps    int64 // EWMA
	Devices          []Device
	Jobs             []domain.JobID // jobs the agent currently holds
}

type Device struct {
	ID           string // cuda:0, /dev/dri/renderD128
	Vendor       string // NVIDIA | INTEL | AMD
	Name         string
	Driver       string
	MaxSessions  int // 0 = unknown
	Healthy      bool
	Capabilities []Capability
}

type Capability struct {
	Name     string // ENC_HEVC_10 | ENC_AV1_10 | DEC_H264_8 | … | WHISPER_VULKAN
	OK       bool
	Error    string
	TestedAt time.Time
}

type TargetCodec string

const (
	TargetHEVC10 TargetCodec = "HEVC_10"
	TargetAV110  TargetCodec = "AV1_10"
)

type Preset string

const (
	PresetArchive  Preset = "ARCHIVE"
	PresetHigh     Preset = "HIGH"
	PresetBalanced Preset = "BALANCED"
	PresetSmall    Preset = "SMALL"
)

// ResolutionClass groups sources for bitrate caps.
type ResolutionClass string

const (
	Resolution2160 ResolutionClass = "2160P"
	Resolution1080 ResolutionClass = "1080P"
	Resolution720  ResolutionClass = "720P"
	ResolutionSD   ResolutionClass = "SD"
)

type Target struct {
	Codec    TargetCodec
	Preset   Preset
	CapsMbps map[ResolutionClass]int
}
