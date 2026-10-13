# Transcode

Runs GPU work (transcodes and Whisper subtitle generation) on remote Agents and hands verified results back for the Library to swap in.

- **Depends on (Readers):** `catalog.Reader` (original language, wanted languages), `library.Reader` (file, media info, `SourceForProcessing`). Never Subtitles or Acquisition.
- **Publishes:**
  - `transcode.JobProgressed` (ephemeral).
  - `JobQueued`, `JobStarted`, `TranscodeVerified`, `TranscodeSkipped`, `JobFailed`, `JobCancelled`, `WhisperCompleted`.
  - `AgentOnline`, `AgentOffline`.
- **Consumes:**
  - Events: `acquisition.CutoffMet`, `library.FileTranscoded`, `library.FileDeleted`, `library.TranscodeSwapRejected`, `catalog.TitleRemoved`.
  - Command: `transcode.RunAgentJob`.

Decisions:
- [13 Transcode policy](../../../.scratch/v1-spec/issues/13-transcode-policy.md)
- [14 Agent protocol](../../../.scratch/v1-spec/issues/14-agent-protocol.md)
- [05 GPU matrix](../../../.scratch/v1-spec/issues/05-gpu-encoder-matrix.md)
- [06 Streamed transcode](../../../.scratch/v1-spec/issues/06-network-streamed-transcode.md)
- [16 Subtitles](../../../.scratch/v1-spec/issues/16-subtitle-design.md)
- [26 Packaging](../../../.scratch/v1-spec/issues/26-packaging.md)
- [31 Import wizard](../../../.scratch/v1-spec/issues/31-import-wizard-and-model-gaps.md)

## Owns / Must not

| Owns | Must not |
|---|---|
| Agents, their devices and tested capabilities | Rename, move or replace library files (Library swaps) |
| Jobs (transcode, Whisper), attempts, leases | Write subtitle sidecars (Subtitles does) |
| Transcode target, quality preset, bitrate caps, skip thresholds | Decide cutoff or upgrades (Acquisition does) |
| Per-title and per-library transcode exclusions | Run work on the server host itself |
| Staging files under `.magpie-staging/` until Library swaps them | Read Acquisition or Subtitles tables, or depend on their Readers |
| Transfer cap, default slots per GPU | Store or rotate the agent shared secret (env-only: `MAGPIE_AGENT_SECRET`) |
| The set of media files eligible for transcode (fed by `CutoffMet`) | |

## Data model

| Table | Key fields |
|---|---|
| `agents` | `id` (stable, agent-chosen ULID persisted on the agent volume), `hostname`, `version`, `ffmpeg_version`, `reported_mode` (the agent's local default from `Hello`), `mode_override` (`DOWNLOAD_FIRST` \| `DIRECT_READ`; wins when set), `slots_override`, `online`, `last_seen_at`, `scratch_free_bytes`, `throughput_bps` (EWMA), `draining` |
| `agent_devices` | `agent_id`, `device_id` (e.g. `cuda:0`, `/dev/dri/renderD128`), `vendor`, `name`, `driver`, `max_sessions` (NVIDIA GeForce 12, others unset), `healthy`, `retest_at` (set while unhealthy), `tested_at` |
| `device_capabilities` | `agent_id`, `device_id`, `capability` (`ENC_HEVC_10`, `ENC_AV1_10`, `DEC_H264_8`, … `WHISPER_VULKAN`), `ok`, `error`, `tested_at` |
| `eligible_files` | `media_file_id`, `title_id`, `origin` (`DOWNLOAD` \| `LIBRARY_SCAN` \| `MIGRATION`, from `CutoffMet.Origin`), `cutoff_met_at` |
| `jobs` | `id`, `kind` (`TRANSCODE` \| `WHISPER`), `media_file_id`, `title_id`, `origin`, `state`, `priority`, `attempt`, `not_before`, `target_codec`, `max_rate_kbps`, `hdr10`, `language` and `whisper_model` (`WHISPER` only), `agent_id`, `device_id`, `offered_at`, `lease_expires_at`, `prefetch`, `last_agent_id`, `avoid_agent_id`, `avoid_until`, `source_etag`, `transfer_etag`, `source_size`, `staging_path`, `spec` (JSON `JobSpec`), `result` (JSON `JobResult`), `output_sha256`, `output_size`, `progress`, `error`, `excluded_agents` |
| `job_attempts` | `job_id`, `attempt`, `agent_id`, `outcome`, `error`, `started_at`, `ended_at` |
| `exclusions` | `scope` (`TITLE` \| `ROOT_FOLDER`), `ref` (title ID, or the root folder path as `library.MediaFile.RootFolder` gives it, cleaned) |
| `skips` | `media_file_id`, `reason`, `detail`, `policy_hash`. One row per file: its latest skip. |
| `passed_capabilities` | `capability`, `agent_id`, `passed_at`: every capability any agent has ever passed (rule 6). Rows are never removed. |

- Tables carry the `transcode_` prefix like every context's (`transcode_agents`, `transcode_agent_devices`, `transcode_device_capabilities`, …).
- `jobs.policy_hash` is the policy hash a `TRANSCODE` job was queued (or last re-planned) under.
- The policy (see [Behaviour › Policy](#policy)) lives in the platform settings store under the key `transcode.policy`; the agent settings (`transfer_cap`, `default_slots_per_gpu`) under `transcode.agents`. `Target()` reads the policy.
- A job keeps two ETags: `source_etag` is Library's `ProcessingSource.ETag` at offer time, which `TranscodeVerified.SourceETag` carries for Library's own re-check; `transfer_etag` is the strong HTTP ETag from `FileStat` (`"<inode>-<size>-<mtime_ns>"`) that `SourceRef.ETag`, `ResumableJob.SourceETag` and the server's verification compare.
- `staging_path` (`<StagingDir>/<job>.mkv`, or `<job>.srt` for Whisper) is fixed at offer time, so a cancel can remove the staged file after Library forgot the media file.
- **Staging sweep:** the cleanup after a job ends can be skipped when the server stops, so at startup, before the scheduler's first pass, the server sweeps every staging dir a job was given. It removes files named like a job's output (`<job>.mkv`, `<job>.srt`) unless their job may still need them (any open state, or `COMPLETED` for a Whisper subtitle Subtitles hasn't moved), logging each removal at INFO with the path. Other files and a missing dir are left alone. Uploads write straight into `staging_path`; the server has no other upload scratch.
- `excluded_agents` gains an agent once it has two `FAILED` attempts at the job; lost leases don't count, since they are usually restarts the agent may resume from.

The calibration table (`preset × encoder → params`) ships as a versioned file in the server binary (`internal/transcode/calibration.json`) and isn't user data. See [Encoder choice](#policy).

## Query port

```go
package api // internal/transcode/api

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

type JobState string // QUEUED OFFERED LEASED FETCHING ENCODING CHECKING UPLOADING VERIFYING VERIFIED SWAPPED COMPLETED FAILED NOT_WORTH_IT CANCELLED
type JobKind string  // TRANSCODE | WHISPER
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

type QueueSummary struct { Counts map[JobState]int; EstimatedSavedBytes int64 }
type Target struct { Codec TargetCodec; Preset Preset; CapsMbps map[ResolutionClass]int } // HEVC_10 | AV1_10; 2160P | 1080P | 720P | SD
// Agent: ID, hostname, versions, Mode (DOWNLOAD_FIRST | DIRECT_READ), slots override, online, draining,
// last seen, scratch free, throughput, Devices (each with Capabilities) and the job IDs it holds.
```

- Job state constants are `StateQueued … StateCancelled` (`JobQueued`/`JobFailed` name events); `JobState.Active()` is the OFFERED..VERIFYING set.

## Events published

```go
// Durable (outbox).

// JobQueued: a job entered QUEUED for the first time (not on retries).
type JobQueued struct {
	Job       domain.JobID
	Kind      JobKind
	MediaFile domain.MediaFileID
	Title     domain.TitleID
	Origin    domain.Origin // TRANSCODE jobs: from CutoffMet
}

// JobStarted: an agent accepted the job (state LEASED), once per attempt.
type JobStarted struct {
	Job       domain.JobID
	Kind      JobKind
	MediaFile domain.MediaFileID
	Agent     domain.AgentID
	Attempt   int
}

type TranscodeVerified struct {
	Job        domain.JobID
	MediaFile  domain.MediaFileID
	StagedPath string // on the same filesystem as the media file
	SHA256     string
	SourceETag string // ETag of the source when the job was leased; Library must re-check before swapping
	Stats      TranscodeStats
}
type TranscodeStats struct {
	OldSize, NewSize  int64
	VideoCodec        string // hevc | av1
	BitDepth          int
	VMAFMean          *float64
	DurationSec       float64
	Agent             domain.AgentID
	Encoder           string // hevc_nvenc, av1_qsv, …
	DroppedAudio, DroppedSubs []int
}
type TranscodeSkipped struct {
	MediaFile domain.MediaFileID
	Reason    SkipReason // EXCLUDED | HDR_DYNAMIC | ALREADY_TARGET | LOW_BITRATE | NOT_WORTH_IT | NO_CAPABLE_AGENT
	Detail    string
}
type JobFailed struct {
	Job       domain.JobID
	Kind      JobKind
	MediaFile domain.MediaFileID
	Reason    string
	Final     bool            // true when retries are exhausted
	Language  domain.Language // WHISPER jobs: the language asked for; empty for TRANSCODE
}
type JobCancelled struct { // TRANSCODE jobs only
	Job       domain.JobID
	MediaFile domain.MediaFileID
	Reason    string
}
type WhisperCompleted struct {
	Job        domain.JobID
	MediaFile  domain.MediaFileID
	Language   domain.Language
	StagedPath string // .srt in .magpie-staging; Subtitles moves it
}
type AgentOnline  struct{ Agent domain.AgentID; Hostname string }
type AgentOffline struct{ Agent domain.AgentID; Since time.Time }

// Ephemeral (bus only).
type JobProgressed struct {
	Job       domain.JobID
	MediaFile domain.MediaFileID
	Stage     JobState
	Progress  float64 // 0..1
	Speed     float64 // encode speed × realtime, or bytes/s while transferring
	ETA       time.Duration
}
```

Every event is published in the same transaction as the job state change that causes it. `JobFailed` is published on every failed attempt; `Final` marks the last one. Aggregate ID: the media file for job, skip and Whisper events; the agent for `AgentOnline`/`AgentOffline`.

## Command accepted

```go
// RunAgentJob: the only command in v1 (api/commands.go). Durable, aggregate = media file.
type RunAgentJob struct {
	Kind      JobKind // WHISPER
	MediaFile domain.MediaFileID
	Language  domain.Language
	Model     string // large-v3-turbo (default) | large-v3 | medium | small
}
```

## Events and commands consumed

| Message | Reaction | Idempotency |
|---|---|---|
| `acquisition.CutoffMet` | Upsert `eligible_files`. Run [policy](#policy). Either queue a `TRANSCODE` job or record a skip and publish `TranscodeSkipped`. | Upsert on `media_file_id`. A non-terminal job already exists → no-op. |
| `library.FileTranscoded` | Job `VERIFIED → SWAPPED`. Delete the staging file if Library left it. Remove from `eligible_files`. | Already `SWAPPED` → no-op. |
| `library.TranscodeSwapRejected` | Job `VERIFIED → FAILED` with the reason; delete staging. `SOURCE_CHANGED` / `FILE_GONE`: no retry (a new `CutoffMet` re-queues). `HASH_MISMATCH` / `PROBE_FAILED`: retryable, attempt + 1 with backoff. Publish `JobFailed`. | Job not `VERIFIED` → no-op. |
| `library.FileDeleted` (any `Reason`, incl. `UPGRADED`) | Cancel non-terminal jobs for the file (send `Cancel` to the agent). Delete staging. Remove from `eligible_files`. | Cancelling a terminal job is a no-op. |
| `catalog.TitleRemoved` | Same as `FileDeleted` for every file of the title. Remove the exclusion. | Same. |
| Agent `Hello` / `CapabilitiesChanged` (protocol, not the bus) | Record each passed capability in `passed_capabilities`. When a target's encode capability is passed for the first time, queue a sweep of the `NO_CAPABLE_AGENT` skips. | The first pass only; later passes change nothing. |
| `transcode.RunAgentJob{Kind: WHISPER, MediaFile, Language, Model}` | Queue a `WHISPER` job at Whisper priority, with the file's origin and title. Every Whisper job ends in `WhisperCompleted` or a final `JobFailed`; an admin cancel counts as a final `JobFailed`. | Dedupe on (`media_file_id`, `language`, non-terminal). |

## External ports

Transcode has no third-party adapters. Its only external party is the Agent, through the [Agent protocol](#agent-protocol).

```go
// ports.go: used for staging and source checks
type FileStat interface {
	// Stat returns size, mtime and inode for an absolute path; ETag is derived from them.
	// A missing path returns an error matching fs.ErrNotExist.
	Stat(path string) (FileInfo, error)
}
type FileInfo struct { Size int64; ModTime time.Time; Inode, Device uint64 }

// MediaProber reads a staged output with ffprobe for the server's own check. The ffprobe adapter
// implements it next to Library's port of the same shape.
type MediaProber interface {
	Probe(ctx context.Context, path string) (libraryapi.MediaInfo, error)
}
```

- `adapters/disk.FileStat` implements `FileStat` on the server's disks.

## HTTP surface

Admin UI (session auth, Admin role):

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/transcode/jobs?state=&kind=&limit=` | `{jobs, counts, backlog, skipped}`: jobs agents hold, then queued ones in offer order, then finished ones newest first (default limit 200); `counts` per state over every job; `backlog` the queued jobs of `LIBRARY_SCAN`/`MIGRATION` files; `skipped` the eligible files per skip reason. Each job carries its title name, episode label, file name, origin, target (empty for Whisper), language (Whisper only), agent name, attempt of `maxAttempts`, progress, `prefetched` and error. `400` on an unknown state or kind |
| POST | `/api/transcode/jobs/{id}/cancel` | Cancel an unfinished job; `409` once finished, `404` unknown |
| POST | `/api/transcode/jobs/{id}/retry` | Re-queue a `FAILED` / `NOT_WORTH_IT` job at attempt 1, forgetting its attempts, exclusions and skip; `409` in any other state |
| GET | `/api/transcode/agents` | Agents, devices, capabilities |
| PATCH | `/api/transcode/agents/{id}` | `slots_override` (0–8, null = default), `mode`, `draining`. All three are sent together. `400` out of range, `404` unknown agent. |
| POST | `/api/transcode/agents/{id}/retest` | Ask the agent to re-run capability tests. `202`; `409` when the agent isn't connected. |
| GET, PUT | `/api/transcode/settings` | The policy (enabled, target, preset, caps, skip switches and thresholds, per-encoder overrides), transfer cap and default slots. `GET` also returns the calibration table (`encoders`: knob, range, value per preset). `PUT` replaces everything and `400`s anything outside the [12-settings](../12-settings.md#transcode) bounds before storing any of it; a change of the policy hash, or turning transcoding back on, queues a backlog sweep. |
| GET | `/api/transcode/secret` | `{configured, masked, secret}`: the shared secret from `MAGPIE_AGENT_SECRET`, masked for display and in full for the copy button. No rotate endpoint; rotating = change the env and restart. |
| GET | `/api/transcode/exclusions` | Every exclusion, `{scope, ref}` |
| PUT, DELETE | `/api/transcode/exclusions/{scope}/{ref}` | Exclude (`200` with the exclusion) or include (`204`) a title or root folder; `400` on an unknown scope. A title's files are evaluated at once (queued jobs cancelled, `EXCLUDED` skips); a root folder queues a sweep. A root folder `ref` is the URL-encoded path. |
| GET | `/api/transcode/backlog` | `{backlog, eligible, skipped}`: queued `LIBRARY_SCAN`/`MIGRATION` jobs, eligible files, skips per reason |
| POST | `/api/transcode/backlog/sweep` | `202` with the backlog counts. Queues a background re-evaluation of every eligible and skipped file against the current policy |

Agent endpoints (agent auth, see [Auth](#auth)):

| Method | Path | Purpose |
|---|---|---|
| GET (WebSocket) | `/api/transcode/agents/ws` | Control channel |
| GET | `/api/transcode/agents/jobs/{id}/source` | Source bytes (Range / If-Range) |
| HEAD | `/api/transcode/agents/jobs/{id}/output` | Committed upload offset |
| PUT | `/api/transcode/agents/jobs/{id}/output` | Upload bytes at an offset |

All agent HTTP endpoints return `409 Conflict` unless the job is currently leased to the calling agent. A job still `OFFERED` to the caller counts, since an agent starts fetching as it sends `JobAccept`. The byte endpoints are hidden from the OpenAPI spec like the socket: only agents call them, and `HEAD` has no fuego route. Paths and headers are constants in `agentproto` (`JobsPath`, `SourcePath`, `OutputPath`, `HeaderUploadOffset`, …).

## Agent protocol

### Auth

- Every request and the WebSocket upgrade send `Authorization: Bearer <shared secret>` and `X-Magpie-Agent: <agent id>`.
- Agent endpoints bypass Identity's session and setup gate (`httpserver.AsSelfAuthenticated`); the secret is their only check.
- A wrong secret returns `401`. The server doesn't start without `MAGPIE_AGENT_SECRET`; a test that builds the service with an empty secret gets `401` for every agent. An `X-Magpie-Agent` that isn't a ULID returns `400`. An unknown agent ID is registered on its first `Hello`.
- The first frame must be a `Hello` naming the same agent ID, within 10 s; otherwise the socket closes with a policy-violation status and nothing is stored.
- Agents are LAN/tailnet only; TLS is up to the operator's reverse proxy.

### Control channel

One WebSocket per agent, dialled by the agent. Text frames carry JSON:

```go
type Envelope struct {
	Type MessageType     `json:"type"` // UPPER_SNAKE message name: HELLO, HEARTBEAT, JOB_ACCEPT, …, WELCOME, JOB_OFFER, ACK
	ID   string          `json:"id"`   // ULID, for logs and Ack
	Body json.RawMessage `json:"body"`
}
```

- The socket path is `agentproto.ControlPath` (`/api/transcode/agents/ws`).
- The types live in `internal/transcode/api/agentproto`, shared by the server and `agent/`. Bodies use camelCase JSON fields.
- The server answers `CapabilitiesChanged` and `DeviceUnhealthy` with `Ack`. `Hello` is answered by `Welcome`. `CapabilitiesChanged` also gets a fresh `Welcome` before its `Ack`, so `Slots` covers devices tested after the `Hello`.
- The server registers the agent (and its `ResumableJobs`) before it sends `Welcome`, so a scheduling pass right after can offer it a job. No other frame goes out before `Welcome`.
- Enumerated string fields below are typed there (`Mode`, `Vendor`, `Capability`, `Stage`, `RejectReason`, `Outcome`, `JobKind`). Codec, encoder, container and Whisper model names stay ffmpeg/whisper.cpp spellings.

Agent → server:

```go
type Hello struct {
	AgentID       domain.AgentID
	Hostname      string
	Version       string // agent build
	FFmpegVersion string // jellyfin-ffmpeg version
	Mode          string // DOWNLOAD_FIRST | DIRECT_READ (local default; server override wins)
	ScratchFree   int64
	Devices       []DeviceReport
	ResumableJobs []ResumableJob // jobs whose scratch survived a restart
}
type DeviceReport struct {
	DeviceID     string // cuda:0, /dev/dri/renderD128
	Vendor       string // NVIDIA | INTEL | AMD | CPU (software encoders, only with MAGPIE_AGENT_CPU)
	Name, Driver string
	MaxSessions  int    // 0 = unknown
	Capabilities []CapabilityResult
}
type CapabilityResult struct {
	Capability string // ENC_HEVC_10 | ENC_AV1_10 | DEC_HEVC_10 | DEC_H264_8 | PIPELINE_HEVC_10 | WHISPER_VULKAN …
	OK         bool
	Error      string // e.g. "OpenEncodeSessionEx failed"
}
type ResumableJob struct{ Job domain.JobID; SourceETag string; BytesHave int64 }

type Heartbeat struct {
	ScratchFree int64
	Jobs        []JobHeartbeat // every job the agent holds, in any stage
}
type JobHeartbeat struct {
	Job      domain.JobID
	Stage    string  // FETCHING | ENCODING | CHECKING (only to a server whose Welcome lists it) | UPLOADING
	Progress float64 // 0..1 within the stage
	Speed    float64
	ETA      time.Duration
}

type JobAccept struct{ Job domain.JobID }
type JobReject struct{ Job domain.JobID; Reason string } // NO_SCRATCH | DEVICE_BUSY | DRAINING | CAPABILITY
type DeviceUnhealthy struct{ DeviceID, Error string }
type CapabilitiesChanged struct{ Devices []DeviceReport }

type JobResult struct {
	Job     domain.JobID
	Outcome string       // UPLOADED | NOT_WORTH_IT | FAILED
	Verify  VerifyReport // agent-side checks
	Error   string       // FAILED: SOURCE_CHANGED, VERIFY_FAILED (Verify says which check) or the error text
}
type VerifyReport struct {
	DecodeOK        bool     // ffmpeg -xerror -f null passed
	DurationDelta   float64  // seconds, output − source
	FrameDelta      int      // output − source (CFR sources only)
	StreamMapOK     bool     // codec, bit depth, kept tracks in order
	CopiedStreamsOK bool     // streamhash of copied audio/subs equals source
	HDR10Kept       *bool    // set when the source is HDR10
	VMAFMean        *float64 // sampled, nil when not run
	OutputSize      int64
	SHA256          string
}
```

Server → agent:

```go
// Welcome answers Hello, and is sent again whenever an admin changes the agent's overrides or the
// default slots per GPU, and after each CapabilitiesChanged.
type Welcome struct {
	HeartbeatEvery time.Duration // 10s
	LeaseTTL       time.Duration // 60s
	Mode           string        // effective transfer mode for this agent
	Slots          map[string]int // device_id -> concurrent jobs allowed
	Draining       bool          // accept no new jobs; false undoes an earlier Drain
	Stages         []string      // heartbeat stages the server takes; none from a server older than CHECKING
}
type JobOffer struct {
	Spec         JobSpec
	LeaseExpires time.Time
	Prefetch     bool // true: fetch the source only; the encode starts on a later Start message
}
type Start struct{ Job domain.JobID } // begin encoding a prefetched job
type Cancel struct{ Job domain.JobID; Reason string }
type Retest struct{ DeviceID string } // empty = all devices
type Drain struct{}                   // finish current jobs, accept no more
type Ack struct{ Of string }          // Envelope.ID being acknowledged

type JobSpec struct {
	Job        domain.JobID
	Kind       string // TRANSCODE | WHISPER
	DeviceID   string
	Source     SourceRef
	Transcode  *TranscodeParams
	Whisper    *WhisperParams
	OutputURL  string // /api/transcode/agents/jobs/{id}/output
	OutputName string // basename the server expects (job.mkv | job.srt)
}
type SourceRef struct {
	URL       string // /api/transcode/agents/jobs/{id}/source
	Size      int64
	ETag      string // strong ETag at lease time
	Container string // matroska | mov,mp4 | mpegts | avi
}
type TranscodeParams struct {
	Codec        string            // hevc | av1
	BitDepth     int               // 10
	Encoder      string            // hevc_nvenc | hevc_qsv | hevc_vaapi | av1_nvenc | …
	EncoderArgs  map[string]string // from calibration: cq, global_quality, qvbr_quality_level, preset, lookahead…
	MaxRateKbps  int               // per-resolution cap
	BufSizeKbps  int               // 2 × MaxRate
	KeepAudio    []int             // source stream indexes, in output order
	KeepSubs     []int
	HDR10        bool              // keep mastering metadata
	VMAFSamples  int               // 0 = skip sampled VMAF
	MinSavingPct int               // agent returns NOT_WORTH_IT below this
}
type WhisperParams struct {
	Language       domain.Language
	AudioStream    int    // source index of the first track in Language, else the default track, else the first
	StartOffsetSec float64 // audio start_time − video start_time; added to every cue
	Model          string // from RunAgentJob.Model: large-v3-turbo (default) | large-v3 | medium | small
}
```

### Timings and lease

- **Heartbeat:** the agent sends one every 10 s.
- **Lease extension:** each heartbeat that lists a job extends its lease to `now + 60 s`. This includes jobs in `FETCHING`, `CHECKING` and `UPLOADING`.
- **Expiry:** a lease that passes `lease_expires_at` returns the job to `QUEUED` with `attempt + 1` and publishes `JobFailed` (`Final` on the last attempt). It waits out no backoff: instead the job is not re-offered to the same agent for 2 minutes, unless that agent reconnects with a matching `ResumableJob` (same `SourceETag`).
- **Reject:** a `JobReject` returns the job to `QUEUED` without an attempt and keeps it from that agent for the same 2 minutes, so an agent short of scratch isn't offered it every tick.
  - `DEVICE_BUSY` instead takes the device out of scheduling until the agent's next heartbeat, `CapabilitiesChanged` or reconnect, and keeps nothing from the job. The server's slot count is wrong for that device (it expired leases the agent still runs, say, after a restart), so any other job offered there would be rejected too. The pause lives in memory and ends with a server restart.
- **Stray jobs:** a heartbeat that lists a job not leased (or offered) to that agent is answered with `Cancel`; so is a late `JobAccept` after the offer timed out.
- **Offer timeout:** an offer not accepted within 10 s returns the job to `QUEUED` without counting an attempt.
- **Disconnect:** WebSocket loss doesn't fail jobs on its own; only lease expiry does. When an agent's socket closes, or after 3 missed heartbeats, the agent is marked offline and `AgentOffline` is published. Reconnecting publishes `AgentOnline`.
  - A socket closing takes its agent offline at once, unless a newer socket from the same agent replaced it. Sockets the server closes as it stops leave their agents online.
  - A 5 s sweep finds agents not heard from for 3 × 10 s. `AgentOffline.Since` is the last heartbeat.
  - `AgentOnline` is published only when an agent goes from offline (or unknown) to online, on `Hello` or on a heartbeat. An agent whose socket was replaced, and one that reconnects within the 30 s window after a server restart, publish nothing.
- **Liveness:** a peer that vanishes without a close frame (a node that hard-reboots) leaves a TCP connection that keeps taking writes for many minutes, so each side checks the other.
  - The server drops a socket that sends no frame for 3 × 10 s, the same window as the sweep.
  - The agent pings the server every heartbeat interval and drops the socket when a pong doesn't come within 2 × 10 s.
  - The agent's dial times out after 10 s. It reconnects after 1 s, doubling to 60 s with ±20% jitter; the delay resets once a `Welcome` arrives.
- **Server stop:** the server closes every agent socket with a normal close (`1000`) and handles no frame after that. A socket dialled while the server stops gets `503`. The close waits at most 5 s for an agent that doesn't answer it. Agents reconnect with backoff; as with any disconnect, jobs keep their leases.
- **Unhealthy device:** the same sweep sends `Retest{DeviceID}` once `retest_at` passes and moves `retest_at` another 10 minutes on, so a retest that never answers is asked for again.

### Capability tests

The agent runs these on start and on `Retest` (`agent/probe`). It never trusts `ffmpeg -encoders`.

- **Startup:** the agent connects without waiting for the tests, which can take minutes. A `Hello` sent before they finish carries the FFmpeg version and no devices, so the agent gets no offers yet. When they finish, the agent sends `CapabilitiesChanged` with every device, and the server answers with a `Welcome` that sizes their slots. A reconnect's `Hello` carries the last results.
- An agent whose `Welcome` names no slots for a device leaves that device's limit to the server's offers. A server that predates the startup `CapabilitiesChanged` sends no new `Welcome`, so its agents would otherwise turn down every offer as `DEVICE_BUSY`.

- **Devices:** NVIDIA from `nvidia-smi --query-gpu=index,name,driver_version` (`cuda:N`; GeForce gets `MaxSessions` 12). Intel and AMD from `/sys/class/drm/renderD*/device/vendor` (`0x8086`, `0x1002`); name and driver come from the VA-API vendor string ffmpeg logs at verbose level. NVIDIA's render nodes are skipped.
- **Encode** (`ENC_HEVC_10`, `ENC_AV1_10`): 30 frames of 720p `testsrc2` converted to 10-bit, through `hevc_nvenc`/`av1_nvenc`, `hevc_qsv`/`av1_qsv` (Intel) or `hevc_vaapi`/`av1_vaapi` (AMD) to the null muxer.
- **Decode** (`DEC_H264_8`, `DEC_HEVC_10`): CPU-encoded sample clips decoded with `-hwaccel cuda|qsv|vaapi -hwaccel_output_format …`. ffmpeg falls back to the CPU and exits 0 when the hardware decode can't start, so a "Failed setup for format" warning also fails the test.
- **Pipeline** (`PIPELINE_HEVC_10`): the HEVC 10-bit sample decoded, scaled (`scale_cuda`, `vpp_qsv`, `scale_vaapi`) and re-encoded without leaving the GPU.
- **Whisper** (`WHISPER_VULKAN`): `whisper-cli` must run, and a Vulkan device must derive from the GPU (`-init_hw_device vulkan=vk@…`). There's no model run: models download on first use. The `MAGPIE_AGENT_CPU` device passes when `whisper-cli` runs, and transcribes without a GPU (`-ng`).
- Each result's `Error` is the first line ffmpeg logged, without its `[component @ 0x…]` prefix (e.g. `OpenEncodeSessionEx failed: unsupported device (2): (no details)`). Each test times out after 1 minute.
- A device is healthy when it passed at least one test; one that passed none is unhealthy and retested after 10 minutes.

### Byte transfer

- **Source:** `GET /api/transcode/agents/jobs/{id}/source`.
  - Served with `http.ServeContent` from `library.Reader.SourceForProcessing(id).Path` (read-only).
  - Strong ETag `"<inode>-<size>-<mtime_ns>"`. Range and If-Range are supported.
  - If the source changed since the lease, If-Range falls back to `200` with the full new file. The agent must compare the ETag, abort, and report `FAILED` with reason `SOURCE_CHANGED`.
- **Output:**
  - `HEAD .../output` returns `Upload-Offset: <committed bytes>`.
  - `PUT .../output` sends headers `Upload-Offset: <n>`, `Upload-Length: <total>`, and, on the last chunk, `Upload-Complete: ?1` plus `Content-Digest: sha-256=:<b64>:`.
  - The server writes at the offset into `<StagingDir>/<job>.<ext>`, with `StagingDir` from `library.Reader.SourceForProcessing` (`<root>/.magpie-staging/`), so the later rename stays on one filesystem.
  - Responses:
    - Offset mismatch: `409` with the current `Upload-Offset`.
    - Digest mismatch on completion: `422`; the staging file is truncated to 0. The agent uploads again from offset 0 (at most twice).
    - Success: `204` with the new `Upload-Offset`. `HEAD` also answers `204`.
    - A body that breaks off keeps what arrived; the agent asks `HEAD` and resumes. A body past `Upload-Length` is `400`.
  - The server hashes the staged file on the last chunk and records the digest; `JobResult{UPLOADED}` must name the same SHA-256 or the attempt fails.
- **Transfer cap:**
  - A server-wide semaphore (default 2) covers concurrent source downloads plus output uploads.
  - When full, the server returns `503` with `Retry-After: 5`. Uploads skip the queue ahead of downloads, and prefetch downloads go last.
  - Direct-read encodes hold a slot for the whole encode.
- **Direct read** (per-agent mode): FFmpeg reads `SourceRef.URL` itself with `-reconnect 1 -reconnect_on_network_error 1 -reconnect_delay_max 30` and the auth headers. Scratch only needs room for the output.
- **Prefetch:** in download-first mode, a busy slot may receive one `JobOffer{Prefetch: true}`. The agent downloads the source but keeps it at `FETCHING` until `Start`. A prefetched job holds a lease, so the agent heartbeats it.

### Agent-side job run

1. Fetch the source (download-first) or open the URL (direct read), checking the ETag.
2. **Transcode:** a full-GPU pipeline where supported (`-hwaccel X -hwaccel_output_format X`, device scaler, hardware encoder). Unsupported decode (e.g. H.264 10-bit) falls back to CPU decode plus `hwupload`. Output is MKV with `-map` from `KeepAudio`/`KeepSubs`, video re-encoded, the rest copied, and `-maxrate/-bufsize` from the spec.
3. Verify (`VerifyReport`), reported as stage `CHECKING`:
   - Decode check: `-xerror` full decode of the video and audio to the null muxer, with `-progress` like the encode, so `CHECKING` shows its own 0–100%.
     - It runs on the GPU (`-hwaccel X -hwaccel_output_format X`, frames never leave the device) when the device passed the decode test for the output's codec (`DEC_HEVC_10` for HEVC 10-bit); otherwise, and for AV1, on the CPU.
     - A GPU check that fails before ffmpeg writes its first frame runs again on the CPU: the device or its decoder didn't open, which says nothing about the file. One that fails after a frame found an error in the file and fails the check.
     - A server whose `Welcome` doesn't list `CHECKING` in `Stages` would cancel a job in a stage it doesn't know, so the agent reports the check to it as `ENCODING` at 100%.
   - Duration within ±1 s.
   - Frame count within ±1 for CFR sources.
   - Stream map matches the spec.
   - `streamhash` of copied streams equals the source.
   - HDR10 mastering metadata is present when `HDR10`.
   - Sampled VMAF over `VMAFSamples` 10-second windows.
   - Savings: if `(1 − out/src) × 100 < MinSavingPct`, return `NOT_WORTH_IT` without uploading.
   - An output that fails a check isn't uploaded: the agent sends `JobResult{FAILED, Error: VERIFY_FAILED, Verify}` and the server decides from `Verify` whether to retry.
   - The full-GPU pipeline needs the device to have passed the decode test for the source (`DEC_H264_8`, `DEC_HEVC_10`) and `PIPELINE_HEVC_10`. If that encode fails, the agent tries once more with the CPU decoding.
   - mp4 text subtitles (`mov_text`) are converted to SRT for Matroska and left out of the `streamhash` comparison.
4. Upload (resumable), then send `JobResult{Outcome: UPLOADED}`. The agent forgets the job before the result goes out, so no later heartbeat lists it; a result that can't be sent is kept and sent again after the next heartbeat or reconnect.
5. **Whisper:**
   - Extract `AudioStream` as 16 kHz mono PCM.
   - Run whisper.cpp (Vulkan backend, Silero VAD) with `Language` and `Model`, on the device's Vulkan index (`cuda:N` → N, `renderD128+N` → N). Progress is the end of the last printed segment over the source duration.
   - Models are `ggml-<Model>.bin` plus the Silero model in `MAGPIE_WHISPER_MODELS` (default `<scratch>/whisper-models`). A missing one is downloaded from whisper.cpp's Hugging Face repos on first use; a file already there is used as is.
   - Add `StartOffsetSec` to every cue and write SRT.
   - Upload as `job.srt` through the same offset-PUT endpoints. `JobResult.Verify` carries only `OutputSize` and `SHA256`; the server checks the SRT itself.

## Behaviour

### Policy

Settings (settings-store key `transcode.policy`):

| Setting | Default |
|---|---|
| `enabled` | on. Off queues nothing new and holds queued transcodes (Whisper jobs still run); running jobs finish. |
| `targetCodec` | `HEVC_10` (or `AV1_10`) |
| `preset` | `HIGH` (`ARCHIVE` \| `HIGH` \| `BALANCED` \| `SMALL`) |
| `capsMbps` | 2160p 25 · 1080p 10 · 720p 5 · SD 2 (1–200) |
| `skipAlreadyTarget` | on (rule 4) |
| `skipLowBitrate`, `lowBitratePct` (skip non-target sources below this % of their resolution cap) | on, 50 (10–100). With default caps: 2160p < 12.5, 1080p < 5, 720p < 2.5, SD < 1 Mbps |
| `minSavingPct` | 15 (0–90) |
| `encoderOverrides` | none; per ffmpeg encoder name, a quality value in that encoder's range replaces the calibrated one |
| VMAF samples | 6, fixed |
| `transfer_cap` | 2 (key `transcode.agents`) |
| `default_slots_per_gpu` | 1 (key `transcode.agents`) |
| Imported-library backlog | `LIBRARY_SCAN`/`MIGRATION`-origin jobs run only on slots left free by `DOWNLOAD`-origin jobs |

- Resolution classes go by the frame's width or height: 2160P from 3200×1800, 1080P from 1600×900, 720P from 1100×650, else SD.
- "Bitrate" is the video stream's own bitrate when the file tags it (`BPS`), else the whole file's. An unknown (0) bitrate never matches rule 5.
- The **policy hash** covers every setting above except `enabled` and the agent settings, plus the calibration table's version. Empty overrides hash like none.

Evaluation of one media file (on `CutoffMet`, a sweep, a settings change, an exclusion change, or the first agent to pass the target's encode test). The first matching rule wins:

| # | Condition (from `library.Reader` media info, own exclusions) | Result |
|---|---|---|
| 1 | Title or root folder excluded | Skip `EXCLUDED` |
| 2 | Source is Dolby Vision or HDR10+ | Skip `HDR_DYNAMIC` |
| 3 | Already transcoded by Magpie (a `SWAPPED` job), or found `NOT_WORTH_IT`, under the same policy hash | No-op |
| 4 | `skipAlreadyTarget`, video codec = target and bitrate ≤ cap for its resolution | Skip `ALREADY_TARGET` |
| 5 | `skipLowBitrate`, video codec ≠ target and bitrate < `lowBitratePct` % of the cap for its resolution | Skip `LOW_BITRATE` |
| 6 | No online or offline agent has ever passed the target capability (`passed_capabilities`) | Skip `NO_CAPABLE_AGENT`; re-evaluated when one first passes it (`Hello` or `CapabilitiesChanged`) |
| 7 | Otherwise | Queue a `TRANSCODE` job with `MaxRateKbps = cap(resolution)`, `HDR10` for HDR10 sources, and the policy hash |

- Rule 4 also means a target-codec file over its cap is re-encoded.
- A file whose job an agent holds (or the server verifies) isn't evaluated: it finishes under the policy it started with.
- A file with a `QUEUED` job is evaluated again: a skip cancels the job (reason "The transcode policy now skips the file: …"); a queue moves the job onto the current target, cap and hash.
- A skip is recorded with the policy hash and publishes `TranscodeSkipped` once: evaluating the file again to the same reason under the same hash publishes nothing. Exclusion and capability changes don't change the hash, so `EXCLUDED` and `NO_CAPABLE_AGENT` skips are re-derived on every evaluation.
- A file that leaves the library (`FileDeleted`, `TitleRemoved`) loses its eligible row and its skip.

**Backlog sweep:** a backlite task (`transcode.backlog-sweep`) evaluates the eligible files in media file order, 50 per task, and queues the next batch 2 s later (`EnqueueTxAt`). A sweep can be limited to one skip reason (the `NO_CAPABLE_AGENT` sweep). Each file is its own transaction; a shutdown stops between files and the task runs again later.

**Tracks to keep:**
- Keep audio and subtitle streams whose language is in the Title's wanted languages or is its original language (both from `catalog.Reader`). Computed when the job is offered, so a `catalog.LanguagesChanged` reaches queued jobs without a handler; running and finished jobs are not redone. A title Catalog no longer knows keeps every stream.
- Streams tagged `und` are kept.
- If no audio stream would remain, keep all audio.
- Image subtitles (PGS/VobSub) follow the same rule.

**Encoder choice:** the target codec plus the device vendor give the encoder (`hevc_nvenc`, `hevc_qsv`, `hevc_vaapi`, `av1_nvenc`, `av1_qsv`, `av1_vaapi`; `libx265`/`libsvtav1` for the `CPU` test device). `EncoderArgs` come from the calibration table: per encoder, its fixed options, its quality knob (`cq` for NVENC, `global_quality` for QSV and VAAPI QVBR, `crf` for the CPU encoders) with range, and the knob's value per preset; an override for the encoder replaces that value. Lower values are higher quality on every encoder, and Archive < High < Balanced < Small.

| Encoder | Fixed options | Archive | High | Balanced | Small |
|---|---|---|---|---|---|
| `hevc_nvenc` | `preset p6, tune hq, rc vbr, b:v 0, spatial-aq 1, temporal-aq 1, rc-lookahead 32` | cq 19 | 23 | 26 | 29 |
| `av1_nvenc` | same as `hevc_nvenc` | cq 24 | 28 | 32 | 36 |
| `hevc_qsv` | `preset slower`, `b:v` 1 kbps under the cap | global_quality 20 | 24 | 27 | 30 |
| `av1_qsv` | `preset slower` | global_quality 22 | 26 | 30 | 34 |
| `hevc_vaapi` | `rc_mode QVBR, compression_level 1` | global_quality 21 | 24 | 26 | 29 |
| `av1_vaapi` | `rc_mode QVBR, compression_level 1` | global_quality (1–255) 64 | 76 | 86 | 104 |
| `libx265` | `preset medium` | crf 18 | 21 | 24 | 27 |
| `libsvtav1` | `preset 8` | crf 22 | 28 | 32 | 38 |

Version 1 of the table was hand-set from vendor guidance. Version 2 fits the VAAPI rows by VMAF. Version 3 moves `hevc_qsv` to QVBR and fits it by VMAF. The NVENC, `av1_qsv` and CPU rows are still hand-set. A fitted table ships as a new version, which changes the policy hash.

**VAAPI fit (version 2).** Measured on an AMD RX 9060 XT (Mesa 26.0.8, jellyfin-ffmpeg 8.1.3) with Magpie's AMD pipeline and the default caps. Six 60 s stream-copied clips: 1080p60 H.264 handheld web video, 1080p H.264 animation, 1080p HEVC grainy 1999 film, 1080p HEVC effects-heavy film, 720p HEVC TV, 2160p HEVC HDR10. VMAF v0.6.1 against the clip, mean of the six:

| Preset | Reference | Target VMAF | `hevc_vaapi` | VMAF | `av1_vaapi` | VMAF |
|---|---|---|---|---|---|---|
| Archive | libx265 crf 18 / libsvtav1 crf 22 | 93.6 / 92.7 | 21 | 93.6 | 64 | 92.5 |
| High | crf 21 / crf 28 | 92.6 / 91.8 | 24 | 92.9 | 76 | 91.8 |
| Balanced | crf 24 / crf 32 | 91.1 / 90.8 | 26 | 91.5 | 86 | 91.2 |
| Small | crf 27 / crf 38 | 88.7 / 89.0 | 29 | 88.9 | 104 | 88.8 |

- `compression_level 1` selects the driver's quality preset. On `hevc_vaapi` at 24 it scores the same mean VMAF as the driver default in 10–55% fewer bits per clip. The default hit the cap on 2 of 6 clips at 24.
- `av1_vaapi` reads `global_quality` on a 0–255 scale. Below about 50 it encodes at or near the cap.
- At Archive, the 1080p60 clip still reaches the 10 Mbps cap. At matched VMAF, the VAAPI encoders need more bits than the CPU references on that clip.

**QSV fit (version 3).** Before version 3, every `hevc_qsv` encode ran CQP. ffmpeg's QSV wrapper picks the rate control from the options: `-maxrate` with `global_quality` and no `b:v` gives CQP, which ignores both the quality value and the cap. On the 1080p60 clip at 20 that gave 19.7 Mbps against a 10 Mbps cap.

- QVBR runs when `global_quality`, `b:v` and `-maxrate` are all set and `b:v` differs from `-maxrate`. When they are equal, ffmpeg picks CBR. So `b:v` is the cap less 1 kbps.
- With no cap, `hevc_qsv` gets no `b:v` and runs ICQ. Caps can't be 0 today.
- `-look_ahead` and LA_ICQ exist only on `h264_qsv`.
- QVBR spends more bits than ICQ at the same value, since it leans toward `b:v`. On the 720p clip at 24: ICQ 341 kbps, QVBR 768 kbps.
- `av1_qsv` keeps version 1's options and values. Neither tested Intel device encodes AV1, so it still runs CQP.

Measured on an Intel N95 (Alder Lake-N, iHD 26.3.5, jellyfin-ffmpeg 8.1.3) with Magpie's Intel pipeline, hardware decode and the default caps. The six clips are the same kinds as the VAAPI fit, cut fresh, so the libx265 targets were measured again on them. VMAF v0.6.1 against the clip, mean of the six, and the total of the six clips' bitrates:

| Preset | libx265 | VMAF | Total | `hevc_qsv` | VMAF | Total |
|---|---|---|---|---|---|---|
| Archive | crf 18 | 92.7 | 26.6 Mbps | 20 | 92.5 | 26.5 Mbps |
| High | crf 21 | 91.6 | 16.2 Mbps | 24 | 91.5 | 18.1 Mbps |
| Balanced | crf 24 | 90.1 | 9.3 Mbps | 27 | 89.9 | 11.6 Mbps |
| Small | crf 27 | 87.8 | 5.7 Mbps | 30 | 87.3 | 7.9 Mbps |

- Every clip stays under its cap on average. The highest is the 1080p60 clip at Archive: 9.1 Mbps against 10. Its peak 2 s window is 12.7 Mbps, inside the 2 × cap buffer; libx265 peaks the same way.
- The extra bits at High and below come mostly from the 1080p60 clip. QVBR keeps it at 4.5–8 Mbps, where libx265 drops to 2.9–6.1 Mbps.
- At Small, 29 scores 88.3 and 30 scores 87.3. 30 is the one with fewer bits.

### Scheduler

Runs on: a job queued, a slot freed, an agent online, a capability change, and a 5 s tick.

1. Free slots are `(agent, device)` pairs where:
   - running jobs < slots (override, else default);
   - running jobs < `max_sessions` when that is set;
   - the device is healthy;
   - the device hasn't rejected an offer as `DEVICE_BUSY` since its agent's last heartbeat (this also holds back prefetch offers and `Start` of a prefetched job);
   - the agent is online and not draining.
2. Order queued jobs by:
   1. Kind priority: `WHISPER` > `TRANSCODE` with origin `DOWNLOAD` > `TRANSCODE` with origin `LIBRARY_SCAN` or `MIGRATION`.
   2. Then queued time.
   3. Skip jobs with `not_before > now`.
3. For each job, pick the best free slot:
   - The device passed the encode capability for the target (`WHISPER_VULKAN` for Whisper).
   - The agent isn't in `excluded_agents`.
   - Scratch ≥ 2.2 × source size (download-first) or ≥ 1.2 × source size (direct read).
   - Among those, prefer a resumable match, then an agent other than the previous attempt's, then the highest `throughput_bps`, then the fewest running jobs.
4. `LIBRARY_SCAN`/`MIGRATION`-origin jobs only take a slot when no waiting job ahead of them (`DOWNLOAD` transcodes, Whisper) could use it, counting one kept from that agent only by the 2-minute avoid window. The same holds for prefetch offers. Jobs waiting out a retry backoff don't hold slots. While transcoding is turned off, `TRANSCODE` jobs aren't offered.
5. Send `JobOffer` and set state `OFFERED`. A busy download-first agent with no prefetched job may get one `Prefetch` offer for its next job.

### Job state machine

```
QUEUED → OFFERED → LEASED → FETCHING → ENCODING → CHECKING → UPLOADING → VERIFYING → VERIFIED → SWAPPED      (TRANSCODE)
                                                                        └→ COMPLETED                        (WHISPER, after WhisperCompleted)
ENCODING → UPLOADING       (WHISPER, which has no CHECKING)
any non-terminal → CANCELLED
OFFERED  → QUEUED          (reject / offer timeout, no attempt counted)
LEASED.. → QUEUED          (lease expiry / retryable failure, attempt+1, backoff)
LEASED.. → FAILED          (attempt 3 failed, or non-retryable)
CHECKING → NOT_WORTH_IT    (agent outcome; recorded as skip, TranscodeSkipped{NOT_WORTH_IT})
```

### Server-side verification (`UPLOADED` → `VERIFIED`)

These checks run as the backlite job `transcode.verify-upload`, queued in the transaction that moves the job to `VERIFYING`.

1. The upload digest matches `VerifyReport.SHA256`.
2. ffprobe on the staged file: codec and bit depth as specified, duration within ±1 s of the source.
3. The source ETag now (from `FileStat` on the `library.Reader` path) equals the leased `source_etag`. A mismatch fails with `SOURCE_CHANGED`: no retry, the job is re-evaluated later if `CutoffMet` arrives again.
4. Publish `TranscodeVerified`. Library re-checks the ETag, swaps atomically, and publishes `FileTranscoded` (job → `SWAPPED`) or `TranscodeSwapRejected` (job → `FAILED`, see consumed events).
5. Whisper: the upload digest matches, and the staged SRT parses and has ≥ 1 cue. Then publish `WhisperCompleted` and mark the job `COMPLETED`; the staged file stays for Subtitles to move. An invalid or empty SRT fails the job without a retry (Whisper would hear the same audio again). A file without audio fails at offer time.

### Retries

- **Retryable:** lease expiry, network, `DEVICE_BUSY`, encoder session errors, swap `HASH_MISMATCH`/`PROBE_FAILED`. Maximum 3 attempts with backoff 1 min, then 5 min, then 30 min (`not_before`), preferring a different agent each time. Then `FAILED` with `JobFailed{Final: true}`; Activity notifies admins.
- **Agent exclusion:** the same agent failing a job twice adds it to `excluded_agents` for that job.
- **Unhealthy device:** `DeviceUnhealthy` marks the device unhealthy and frees its slots. The server sends `Retest` after 10 minutes, and a passing retest marks it healthy.
- **Not retried:** `SOURCE_CHANGED`, verification failure of the stream map or HDR metadata. These are logged with the encoder, and the job ends `FAILED`.

## Test seeds

1. **Queue on cutoff:** given a 1080p H.264 file at 12 Mbps, target HEVC, cap 10 Mbps, when `CutoffMet` arrives, then a `TRANSCODE` job is `QUEUED` with `MaxRateKbps=10000`.
2. **Target under cap:** given a 1080p HEVC file at 8 Mbps, when `CutoffMet` arrives, then no job is created and `TranscodeSkipped{ALREADY_TARGET}` is published.
3. **Target over cap:** given a 2160p HEVC file at 38 Mbps (cap 25), when `CutoffMet` arrives, then a job is queued.
4. **Low-bitrate source:** given cap 1080p = 10 Mbps and `low_bitrate_pct = 50`, when `CutoffMet` arrives for a 1080p H.264 file at 4 Mbps, then `TranscodeSkipped{LOW_BITRATE}`; at 6 Mbps a job is queued.
5. **Dynamic HDR:** given a Dolby Vision source, when `CutoffMet` arrives, then `TranscodeSkipped{HDR_DYNAMIC}`, and an HDR10 source is queued with `HDR10=true`.
6. **Excluded title:** given an excluded title, when `CutoffMet` arrives, then `TranscodeSkipped{EXCLUDED}`.
7. **Redelivered CutoffMet:** given `CutoffMet` delivered twice, then exactly one job exists.
8. **Capability mismatch:** given target AV1, agent A with only `ENC_HEVC_10` ok and agent B with `ENC_AV1_10` ok, when the scheduler runs, then the job is offered to B only.
9. **No capable agent:** given target AV1 and no agent ever passed `ENC_AV1_10`, when `CutoffMet` arrives, then `TranscodeSkipped{NO_CAPABLE_AGENT}`. When an agent later reports `ENC_AV1_10` ok, then the file is queued.
10. **Session limit:** given an NVIDIA device with `MaxSessions=12` running 12 jobs, when a job is queued, then it isn't offered to that device.
11. **Lease expiry:** given a job `ENCODING` on agent A, when no heartbeat lists it for 60 s, then it returns to `QUEUED` with `attempt=2` and isn't offered to A for 2 minutes.
12. **Heartbeat during transfer:** given a job `UPLOADING` and heartbeats every 10 s for 5 minutes, then the lease never expires.
13. **Resume after restart:** given agent A restarting with `ResumableJob{job, sameETag}` within the 2-minute window, when the scheduler runs, then A gets the job back and the download resumes at its offset.
14. **Source changed mid-download:** given the source replaced during download, when the agent sends If-Range with the old ETag, then the server returns `200` with the new ETag and the agent reports `FAILED{SOURCE_CHANGED}`.
15. **Source changed before swap:** given an upload that completed but the source ETag changed, when the server verifies, then the job is `FAILED{SOURCE_CHANGED}` and no `TranscodeVerified` is published.
16. **Digest mismatch:** given a final PUT whose `Content-Digest` doesn't match, then the response is `422`, the staging file is truncated, and the agent may re-upload from offset 0.
17. **Offset mismatch:** given a committed offset of 4 MiB, when a PUT has `Upload-Offset: 8 MiB`, then `409` with `Upload-Offset: 4194304`.
18. **Agent verification failure:** given `CopiedStreamsOK=false`, when `JobResult` arrives, then the job is `FAILED` and nothing is uploaded or swapped.
19. **Not worth it:** given an output 9% smaller and `min_saving_pct=15`, when the agent returns `NOT_WORTH_IT`, then `TranscodeSkipped{NOT_WORTH_IT}` is published and a sweep doesn't re-queue it until the policy hash changes.
20. **Transfer cap:** given cap 2 with two active uploads, when a third agent requests a source, then `503 Retry-After: 5`. When an upload completes, then the waiting download gets the slot before any prefetch.
21. **Origin priority:** given one free slot, a `MIGRATION`-origin job queued earlier and a `DOWNLOAD`-origin job queued later, when the scheduler runs, then the `DOWNLOAD` job is offered.
22. **Retries exhausted:** given three failed attempts, then `JobFailed{Final: true}` and state `FAILED`. A manual retry re-queues it with `attempt=1`.
23. **File deleted mid-job:** given a job `ENCODING` and `library.FileDeleted`, then the agent receives `Cancel`, the job is `CANCELLED`, and the staging file is removed.
24. **Swap confirmed:** given a `VERIFIED` job, when `library.FileTranscoded` arrives twice, then the job is `SWAPPED` once and no error is raised.
25. **Whisper flow:** given `RunAgentJob{WHISPER, en}` and an agent with `WHISPER_VULKAN`, when the agent uploads a valid SRT, then `WhisperCompleted` with the staged path. An empty SRT gives `JobFailed`.
26. **Whisper offset:** given an audio `start_time` 4.0 s after video, then `WhisperParams.StartOffsetSec=4.0` and the agent's first cue time is shifted by +4.0 s.
27. **Wrong agent:** given a job leased to A, when agent B requests its source, then `409`.
28. **Bad secret:** given a wrong shared secret on the WebSocket upgrade, then `401` and no agent row is created.
29. **Lifecycle events:** given `CutoffMet` that queues a job, then `JobQueued{Origin}` is published once; when agent A accepts, `JobStarted{Agent: A, Attempt: 1}`; after a lease expiry and acceptance by B, `JobStarted{Agent: B, Attempt: 2}` and no second `JobQueued`.
30. **Swap rejected:** given a `VERIFIED` job, when `TranscodeSwapRejected{SOURCE_CHANGED}` arrives, then the job is `FAILED`, staging is deleted, and nothing is re-queued; with `HASH_MISMATCH` it returns to `QUEUED` with `attempt + 1`.
31. **Upgrade cancels:** given a job `ENCODING` for file F1, when `FileDeleted{F1, Reason: UPGRADED}` arrives, then the agent receives `Cancel` and the job is `CANCELLED`.
32. **Keep languages from Catalog:** given the Title's wanted languages [en] (Catalog override) and original language ja, when the job is offered for a file with en/ja/fr audio, then `KeepAudio` holds the en and ja streams only.
33. **Retry agent preference:** given attempt 1 failed on agent A and both A and B are free and capable, when the retry is scheduled after 1 min, then it is offered to B.
