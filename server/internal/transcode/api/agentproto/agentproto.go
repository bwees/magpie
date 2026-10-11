// Package agentproto is the control-channel protocol between the server and transcode agents.
// Both binaries import it. Spec: docs/spec/contexts/transcode.md#agent-protocol.
package agentproto

import (
	"encoding/json"
	"time"

	"magpie/internal/domain"
)

// ControlPath is the control WebSocket, relative to the server URL.
const ControlPath = "/api/transcode/agents/ws"

// Auth headers sent on every agent request and on the WebSocket upgrade.
const (
	HeaderAuthorization = "Authorization"
	HeaderAgentID       = "X-Magpie-Agent"
	BearerPrefix        = "Bearer "
)

// Envelope is one text frame on the control WebSocket.
type Envelope struct {
	Type MessageType     `json:"type"`
	ID   string          `json:"id"` // ULID, for logs and Ack
	Body json.RawMessage `json:"body"`
}

// MessageType names the body an Envelope carries.
type MessageType string

// Agent → server.
const (
	TypeHello               MessageType = "HELLO"
	TypeHeartbeat           MessageType = "HEARTBEAT"
	TypeJobAccept           MessageType = "JOB_ACCEPT"
	TypeJobReject           MessageType = "JOB_REJECT"
	TypeDeviceUnhealthy     MessageType = "DEVICE_UNHEALTHY"
	TypeCapabilitiesChanged MessageType = "CAPABILITIES_CHANGED"
	TypeJobResult           MessageType = "JOB_RESULT"
)

// Server → agent.
const (
	TypeWelcome  MessageType = "WELCOME"
	TypeJobOffer MessageType = "JOB_OFFER"
	TypeStart    MessageType = "START"
	TypeCancel   MessageType = "CANCEL"
	TypeRetest   MessageType = "RETEST"
	TypeDrain    MessageType = "DRAIN"
	TypeAck      MessageType = "ACK"
)

type Mode string

const (
	ModeDownloadFirst Mode = "DOWNLOAD_FIRST"
	ModeDirectRead    Mode = "DIRECT_READ"
)

type Vendor string

const (
	VendorNVIDIA Vendor = "NVIDIA"
	VendorIntel  Vendor = "INTEL"
	VendorAMD    Vendor = "AMD"
)

// Capability is one tested device feature.
type Capability string

const (
	CapEncHEVC10      Capability = "ENC_HEVC_10"
	CapEncAV110       Capability = "ENC_AV1_10"
	CapDecHEVC10      Capability = "DEC_HEVC_10"
	CapDecH2648       Capability = "DEC_H264_8"
	CapPipelineHEVC10 Capability = "PIPELINE_HEVC_10"
	CapWhisperVulkan  Capability = "WHISPER_VULKAN"
)

type JobKind string

const (
	KindTranscode JobKind = "TRANSCODE"
	KindWhisper   JobKind = "WHISPER"
)

// Stage is where an agent-held job is.
type Stage string

const (
	StageFetching  Stage = "FETCHING"
	StageEncoding  Stage = "ENCODING"
	StageUploading Stage = "UPLOADING"
)

type RejectReason string

const (
	RejectNoScratch  RejectReason = "NO_SCRATCH"
	RejectDeviceBusy RejectReason = "DEVICE_BUSY"
	RejectDraining   RejectReason = "DRAINING"
	RejectCapability RejectReason = "CAPABILITY"
)

type Outcome string

const (
	OutcomeUploaded   Outcome = "UPLOADED"
	OutcomeNotWorthIt Outcome = "NOT_WORTH_IT"
	OutcomeFailed     Outcome = "FAILED"
)

// ErrorSourceChanged is the JobResult.Error an agent reports when the source ETag changed under it.
const ErrorSourceChanged = "SOURCE_CHANGED"

type Hello struct {
	AgentID       domain.AgentID `json:"agentId"`
	Hostname      string         `json:"hostname"`
	Version       string         `json:"version"`       // agent build
	FFmpegVersion string         `json:"ffmpegVersion"` // jellyfin-ffmpeg version
	Mode          Mode           `json:"mode"`          // local default; server override wins
	ScratchFree   int64          `json:"scratchFree"`
	Devices       []DeviceReport `json:"devices"`
	ResumableJobs []ResumableJob `json:"resumableJobs"` // jobs whose scratch survived a restart
}

type DeviceReport struct {
	DeviceID     string             `json:"deviceId"` // cuda:0, /dev/dri/renderD128
	Vendor       Vendor             `json:"vendor"`
	Name         string             `json:"name"`
	Driver       string             `json:"driver"`
	MaxSessions  int                `json:"maxSessions"` // 0 = unknown
	Capabilities []CapabilityResult `json:"capabilities"`
}

type CapabilityResult struct {
	Capability Capability `json:"capability"`
	OK         bool       `json:"ok"`
	Error      string     `json:"error"` // e.g. "OpenEncodeSessionEx failed"
}

type ResumableJob struct {
	Job        domain.JobID `json:"job"`
	SourceETag string       `json:"sourceETag"`
	BytesHave  int64        `json:"bytesHave"`
}

type Heartbeat struct {
	ScratchFree int64          `json:"scratchFree"`
	Jobs        []JobHeartbeat `json:"jobs"` // every job the agent holds, in any stage
}

type JobHeartbeat struct {
	Job      domain.JobID  `json:"job"`
	Stage    Stage         `json:"stage"`
	Progress float64       `json:"progress"` // 0..1 within the stage
	Speed    float64       `json:"speed"`
	ETA      time.Duration `json:"eta"`
}

type JobAccept struct {
	Job domain.JobID `json:"job"`
}

type JobReject struct {
	Job    domain.JobID `json:"job"`
	Reason RejectReason `json:"reason"`
}

type DeviceUnhealthy struct {
	DeviceID string `json:"deviceId"`
	Error    string `json:"error"`
}

type CapabilitiesChanged struct {
	Devices []DeviceReport `json:"devices"`
}

type JobResult struct {
	Job     domain.JobID `json:"job"`
	Outcome Outcome      `json:"outcome"`
	Verify  VerifyReport `json:"verify"` // agent-side checks
	Error   string       `json:"error"`
}

type VerifyReport struct {
	DecodeOK        bool     `json:"decodeOk"`        // ffmpeg -xerror -f null passed
	DurationDelta   float64  `json:"durationDelta"`   // seconds, output − source
	FrameDelta      int      `json:"frameDelta"`      // output − source (CFR sources only)
	StreamMapOK     bool     `json:"streamMapOk"`     // codec, bit depth, kept tracks in order
	CopiedStreamsOK bool     `json:"copiedStreamsOk"` // streamhash of copied audio/subs equals source
	HDR10Kept       *bool    `json:"hdr10Kept"`       // set when the source is HDR10
	VMAFMean        *float64 `json:"vmafMean"`        // sampled, nil when not run
	OutputSize      int64    `json:"outputSize"`
	SHA256          string   `json:"sha256"`
}

// Welcome answers Hello. The server sends it again whenever an admin changes the agent's overrides.
type Welcome struct {
	HeartbeatEvery time.Duration  `json:"heartbeatEvery"`
	LeaseTTL       time.Duration  `json:"leaseTtl"`
	Mode           Mode           `json:"mode"`     // effective transfer mode for this agent
	Slots          map[string]int `json:"slots"`    // device_id -> concurrent jobs allowed
	Draining       bool           `json:"draining"` // accept no new jobs
}

type JobOffer struct {
	Spec         JobSpec   `json:"spec"`
	LeaseExpires time.Time `json:"leaseExpires"`
	Prefetch     bool      `json:"prefetch"` // true: fetch the source only; the encode starts on a later Start
}

// Start begins encoding a prefetched job.
type Start struct {
	Job domain.JobID `json:"job"`
}

type Cancel struct {
	Job    domain.JobID `json:"job"`
	Reason string       `json:"reason"`
}

// Retest asks for capability tests again. An empty DeviceID means all devices.
type Retest struct {
	DeviceID string `json:"deviceId"`
}

// Drain asks the agent to finish current jobs and accept no more.
type Drain struct{}

// Ack acknowledges the Envelope whose ID is Of.
type Ack struct {
	Of string `json:"of"`
}

type JobSpec struct {
	Job        domain.JobID     `json:"job"`
	Kind       JobKind          `json:"kind"`
	DeviceID   string           `json:"deviceId"`
	Source     SourceRef        `json:"source"`
	Transcode  *TranscodeParams `json:"transcode"`
	Whisper    *WhisperParams   `json:"whisper"`
	OutputURL  string           `json:"outputUrl"`  // /api/transcode/agents/jobs/{id}/output
	OutputName string           `json:"outputName"` // basename the server expects (job.mkv | job.srt)
}

type SourceRef struct {
	URL       string `json:"url"` // /api/transcode/agents/jobs/{id}/source
	Size      int64  `json:"size"`
	ETag      string `json:"etag"`      // strong ETag at lease time
	Container string `json:"container"` // ffprobe format name: matroska | mov,mp4 | mpegts | avi
}

// TranscodeParams carries ffmpeg names verbatim: codec hevc | av1, encoder hevc_nvenc | hevc_qsv | …
type TranscodeParams struct {
	Codec        string            `json:"codec"`
	BitDepth     int               `json:"bitDepth"`
	Encoder      string            `json:"encoder"`
	EncoderArgs  map[string]string `json:"encoderArgs"` // from calibration: cq, global_quality, preset, …
	MaxRateKbps  int               `json:"maxRateKbps"` // per-resolution cap
	BufSizeKbps  int               `json:"bufSizeKbps"` // 2 × MaxRate
	KeepAudio    []int             `json:"keepAudio"`   // source stream indexes, in output order
	KeepSubs     []int             `json:"keepSubs"`
	HDR10        bool              `json:"hdr10"`        // keep mastering metadata
	VMAFSamples  int               `json:"vmafSamples"`  // 0 = skip sampled VMAF
	MinSavingPct int               `json:"minSavingPct"` // agent returns NOT_WORTH_IT below this
}

type WhisperParams struct {
	Language       domain.Language `json:"language"`
	AudioStream    int             `json:"audioStream"`    // source index of the original-language track
	StartOffsetSec float64         `json:"startOffsetSec"` // audio start_time − video start_time; added to every cue
	Model          string          `json:"model"`          // from RunAgentJob.Model
}
