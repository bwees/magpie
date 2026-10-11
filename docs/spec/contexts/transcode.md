# Transcode

Runs GPU work (transcodes and Whisper subtitle generation) on remote Agents and hands verified results back for the Library to swap in.

- **Depends on (Readers):** `catalog.Reader` (original language, wanted languages), `library.Reader` (file, media info, `SourceForProcessing`). Never Subtitles or Acquisition.
- **Publishes:**
  - `transcode.JobProgressed` (ephemeral).
  - `JobQueued`, `JobStarted`, `TranscodeVerified`, `TranscodeSkipped`, `JobFailed`, `WhisperCompleted`.
  - `AgentOnline`, `AgentOffline`.
- **Consumes:**
  - Events: `acquisition.CutoffMet`, `library.FileTranscoded`, `library.FileDeleted`, `library.TranscodeSwapRejected`, `catalog.TitleRemoved`, `catalog.LanguagesChanged`.
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
| `jobs` | `id`, `kind` (`TRANSCODE` \| `WHISPER`), `media_file_id`, `state`, `priority`, `attempt`, `not_before`, `agent_id`, `device_id`, `lease_expires_at`, `source_etag`, `source_size`, `spec` (JSON `JobSpec`), `result` (JSON), `excluded_agents` |
| `job_attempts` | `job_id`, `attempt`, `agent_id`, `outcome`, `error`, `started_at`, `ended_at` |
| `exclusions` | `scope` (`TITLE` \| `ROOT_FOLDER`), `ref` |
| `skips` | `media_file_id`, `reason`, `policy_hash`. A file isn't re-evaluated until the policy changes. |
| `transcode_settings` | singleton (see [Behaviour › Policy](#policy)) |

- Tables carry the `transcode_` prefix like every context's (`transcode_agents`, `transcode_agent_devices`, `transcode_device_capabilities`, …).
- The agent settings (`transfer_cap`, `default_slots_per_gpu`) live in the platform settings store under the key `transcode.agents`, not in `transcode_settings`.

The calibration table (`preset × encoder → params`) ships as a versioned file in the server binary and isn't user data.

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

type JobState string // QUEUED OFFERED LEASED FETCHING ENCODING UPLOADING VERIFYING VERIFIED SWAPPED COMPLETED FAILED NOT_WORTH_IT CANCELLED
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
	Final     bool // true when retries are exhausted
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
| `catalog.LanguagesChanged` | Drop the cached `KeepAudio`/`KeepSubs` of `QUEUED` jobs for the affected title(s); they are recomputed at offer time. Running and finished jobs are not redone. | Idempotent by nature. |
| `transcode.RunAgentJob{Kind: WHISPER, MediaFile, Language, Model}` | Queue a `WHISPER` job at Whisper priority. | Dedupe on (`media_file_id`, `language`, non-terminal). |

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
```

## HTTP surface

Admin UI (session auth, Admin role):

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/transcode/jobs?state=&kind=` | List jobs |
| POST | `/api/transcode/jobs/{id}/cancel` | Cancel a job |
| POST | `/api/transcode/jobs/{id}/retry` | Re-queue a `FAILED` / `NOT_WORTH_IT` job and clear the skip |
| GET | `/api/transcode/agents` | Agents, devices, capabilities |
| PATCH | `/api/transcode/agents/{id}` | `slots_override` (0–8, null = default), `mode`, `draining`. All three are sent together. `400` out of range, `404` unknown agent. |
| POST | `/api/transcode/agents/{id}/retest` | Ask the agent to re-run capability tests. `202`; `409` when the agent isn't connected. |
| GET, PUT | `/api/transcode/settings` | Target, preset, caps, thresholds, transfer cap, default slots |
| GET | `/api/transcode/secret` | `{configured, masked, secret}`: the shared secret from `MAGPIE_AGENT_SECRET`, masked for display and in full for the copy button. No rotate endpoint; rotating = change the env and restart. |
| PUT, DELETE | `/api/transcode/exclusions/{scope}/{ref}` | Exclude or include a title or root folder |
| POST | `/api/transcode/backlog/sweep` | Re-evaluate every eligible and skipped file against the current policy |

Agent endpoints (agent auth, see [Auth](#auth)):

| Method | Path | Purpose |
|---|---|---|
| GET (WebSocket) | `/api/transcode/agents/ws` | Control channel |
| GET | `/api/transcode/agents/jobs/{id}/source` | Source bytes (Range / If-Range) |
| HEAD | `/api/transcode/agents/jobs/{id}/output` | Committed upload offset |
| PUT | `/api/transcode/agents/jobs/{id}/output` | Upload bytes at an offset |

All agent HTTP endpoints return `409 Conflict` unless the job is currently leased to the calling agent.

## Agent protocol

### Auth

- Every request and the WebSocket upgrade send `Authorization: Bearer <shared secret>` and `X-Magpie-Agent: <agent id>`.
- A wrong secret returns `401`, and so does every agent while `MAGPIE_AGENT_SECRET` is unset. An `X-Magpie-Agent` that isn't a ULID returns `400`. An unknown agent ID is registered on its first `Hello`.
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
- The server answers `CapabilitiesChanged` and `DeviceUnhealthy` with `Ack`. `Hello` is answered by `Welcome`.
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
	Vendor       string // NVIDIA | INTEL | AMD
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
	Stage    string  // FETCHING | ENCODING | UPLOADING
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
	Error   string
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
// default slots per GPU.
type Welcome struct {
	HeartbeatEvery time.Duration // 10s
	LeaseTTL       time.Duration // 60s
	Mode           string        // effective transfer mode for this agent
	Slots          map[string]int // device_id -> concurrent jobs allowed
	Draining       bool          // accept no new jobs; false undoes an earlier Drain
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
	AudioStream    int    // source index of the original-language track
	StartOffsetSec float64 // audio start_time − video start_time; added to every cue
	Model          string // from RunAgentJob.Model: large-v3-turbo (default) | large-v3 | medium | small
}
```

### Timings and lease

- **Heartbeat:** the agent sends one every 10 s.
- **Lease extension:** each heartbeat that lists a job extends its lease to `now + 60 s`. This includes jobs in `FETCHING` and `UPLOADING`.
- **Expiry:** a lease that passes `lease_expires_at` returns the job to `QUEUED` with `attempt + 1`. The job is not re-offered to the same agent for 2 minutes, unless that agent reconnects with a matching `ResumableJob` (same `SourceETag`).
- **Offer timeout:** an offer not accepted within 10 s returns the job to `QUEUED` without counting an attempt.
- **Disconnect:** WebSocket loss doesn't fail jobs on its own; only lease expiry does. After 3 missed heartbeats the agent is marked offline and `AgentOffline` is published. Reconnecting publishes `AgentOnline`.
  - A 5 s sweep finds agents not heard from for 3 × 10 s. `AgentOffline.Since` is the last heartbeat.
  - `AgentOnline` is published only when an agent goes from offline (or unknown) to online, on `Hello` or on a heartbeat. A quick reconnect within the 30 s window publishes nothing.
- **Unhealthy device:** the same sweep sends `Retest{DeviceID}` once `retest_at` passes and moves `retest_at` another 10 minutes on, so a retest that never answers is asked for again.

### Capability tests

The agent runs these on start and on `Retest` (`agent/probe`). It never trusts `ffmpeg -encoders`.

- **Devices:** NVIDIA from `nvidia-smi --query-gpu=index,name,driver_version` (`cuda:N`; GeForce gets `MaxSessions` 12). Intel and AMD from `/sys/class/drm/renderD*/device/vendor` (`0x8086`, `0x1002`); name and driver come from the VA-API vendor string ffmpeg logs at verbose level. NVIDIA's render nodes are skipped.
- **Encode** (`ENC_HEVC_10`, `ENC_AV1_10`): 30 frames of 720p `testsrc2` converted to 10-bit, through `hevc_nvenc`/`av1_nvenc`, `hevc_qsv`/`av1_qsv` (Intel) or `hevc_vaapi`/`av1_vaapi` (AMD) to the null muxer.
- **Decode** (`DEC_H264_8`, `DEC_HEVC_10`): CPU-encoded sample clips decoded with `-hwaccel cuda|qsv|vaapi -hwaccel_output_format …`. ffmpeg falls back to the CPU and exits 0 when the hardware decode can't start, so a "Failed setup for format" warning also fails the test.
- **Pipeline** (`PIPELINE_HEVC_10`): the HEVC 10-bit sample decoded, scaled (`scale_cuda`, `vpp_qsv`, `scale_vaapi`) and re-encoded without leaving the GPU.
- **Whisper** (`WHISPER_VULKAN`): `whisper-cli` must run, and a Vulkan device must derive from the GPU (`-init_hw_device vulkan=vk@…`). Ticket 26 may add a model run.
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
    - Digest mismatch on completion: `422`; the staging file is truncated to 0.
    - Success: `204`.
- **Transfer cap:**
  - A server-wide semaphore (default 2) covers concurrent source downloads plus output uploads.
  - When full, the server returns `503` with `Retry-After: 5`. Uploads skip the queue ahead of downloads, and prefetch downloads go last.
  - Direct-read encodes hold a slot for the whole encode.
- **HTTP/2:** the server sets the per-stream and per-connection receive window to 16 MiB. Agents may use HTTP/1.1 for bulk transfers.
- **Direct read** (per-agent mode): FFmpeg reads `SourceRef.URL` itself with `-reconnect 1 -reconnect_on_network_error 1 -reconnect_delay_max 30` and the auth headers. Scratch only needs room for the output.
- **Prefetch:** in download-first mode, a busy slot may receive one `JobOffer{Prefetch: true}`. The agent downloads the source but keeps it at `FETCHING` until `Start`. A prefetched job holds a lease, so the agent heartbeats it.

### Agent-side job run

1. Fetch the source (download-first) or open the URL (direct read), checking the ETag.
2. **Transcode:** a full-GPU pipeline where supported (`-hwaccel X -hwaccel_output_format X`, device scaler, hardware encoder). Unsupported decode (e.g. H.264 10-bit) falls back to CPU decode plus `hwupload`. Output is MKV with `-map` from `KeepAudio`/`KeepSubs`, video re-encoded, the rest copied, and `-maxrate/-bufsize` from the spec.
3. Verify (`VerifyReport`):
   - Decode check: `-xerror` full decode.
   - Duration within ±1 s.
   - Frame count within ±1 for CFR sources.
   - Stream map matches the spec.
   - `streamhash` of copied streams equals the source.
   - HDR10 mastering metadata is present when `HDR10`.
   - Sampled VMAF over `VMAFSamples` 10-second windows.
   - Savings: if `(1 − out/src) × 100 < MinSavingPct`, return `NOT_WORTH_IT` without uploading.
4. Upload (resumable), then send `JobResult{Outcome: UPLOADED}`.
5. **Whisper:**
   - Extract `AudioStream` as 16 kHz mono PCM.
   - Run whisper.cpp (Vulkan backend, Silero VAD) with `Language`.
   - Add `StartOffsetSec` to every cue and write SRT.
   - Upload as `job.srt`.

## Behaviour

### Policy

Settings (`transcode_settings`):

| Setting | Default |
|---|---|
| `target_codec` | `HEVC_10` (or `AV1_10`) |
| `preset` | `HIGH` (`ARCHIVE` \| `HIGH` \| `BALANCED` \| `SMALL`) |
| `bitrate_caps_mbps` | 2160p 25 · 1080p 10 · 720p 5 · SD 2 (1–200) |
| `low_bitrate_pct` (skip non-target sources below this % of their resolution cap) | 50 (10–100). With default caps: 2160p < 12.5, 1080p < 5, 720p < 2.5, SD < 1 Mbps |
| `min_saving_pct` | 15 |
| `vmaf_samples` | 6 |
| `transfer_cap` | 2 |
| `default_slots_per_gpu` | 1 |
| Imported-library backlog | `LIBRARY_SCAN`/`MIGRATION`-origin jobs run only on slots left free by `DOWNLOAD`-origin jobs |

Evaluation of one media file (on `CutoffMet`, a sweep, or a settings change). The first matching rule wins:

| # | Condition (from `library.Reader` media info, own exclusions) | Result |
|---|---|---|
| 1 | Title or root folder excluded | Skip `EXCLUDED` |
| 2 | Source is Dolby Vision or HDR10+ | Skip `HDR_DYNAMIC` |
| 3 | Already transcoded by Magpie under the same policy hash | No-op |
| 4 | Video codec = target and bitrate ≤ cap for its resolution | Skip `ALREADY_TARGET` |
| 5 | Video codec ≠ target and bitrate < `low_bitrate_pct` % of the cap for its resolution | Skip `LOW_BITRATE` |
| 6 | No online or offline agent has ever passed the target capability | Skip `NO_CAPABLE_AGENT`; re-evaluated on `CapabilitiesChanged` |
| 7 | Otherwise | Queue a `TRANSCODE` job with `MaxRateKbps = cap(resolution)` |

Rule 4 also means a target-codec file over its cap is re-encoded.

**Tracks to keep:**
- Keep audio and subtitle streams whose language is in the Title's wanted languages or is its original language (both from `catalog.Reader`). Computed when the job is offered.
- Streams tagged `und` are kept.
- If no audio stream would remain, keep all audio.
- Image subtitles (PGS/VobSub) follow the same rule.

**Encoder choice:** the target codec plus the device vendor give the encoder (`hevc_nvenc`, `hevc_qsv`, `hevc_vaapi`, `av1_nvenc`, `av1_qsv`, `av1_vaapi`). `EncoderArgs` come from the calibration table (VMAF-fitted per encoder and preset; values are produced at build time).

### Scheduler

Runs on: a job queued, a slot freed, an agent online, a capability change, and a 5 s tick.

1. Free slots are `(agent, device)` pairs where:
   - running jobs < slots (override, else default);
   - running jobs < `max_sessions` when that is set;
   - the device is healthy;
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
4. `LIBRARY_SCAN`/`MIGRATION`-origin jobs only take a slot when no `DOWNLOAD`-origin job could use it.
5. Send `JobOffer` and set state `OFFERED`. A busy download-first agent with no prefetched job may get one `Prefetch` offer for its next job.

### Job state machine

```
QUEUED → OFFERED → LEASED → FETCHING → ENCODING → UPLOADING → VERIFYING → VERIFIED → SWAPPED      (TRANSCODE)
                                                             └→ COMPLETED                        (WHISPER, after WhisperCompleted)
any non-terminal → CANCELLED
OFFERED  → QUEUED          (reject / offer timeout, no attempt counted)
LEASED.. → QUEUED          (lease expiry / retryable failure, attempt+1, backoff)
LEASED.. → FAILED          (attempt 3 failed, or non-retryable)
ENCODING → NOT_WORTH_IT    (agent outcome; recorded as skip, TranscodeSkipped{NOT_WORTH_IT})
```

### Server-side verification (`UPLOADED` → `VERIFIED`)

1. The upload digest matches `VerifyReport.SHA256`.
2. ffprobe on the staged file: codec and bit depth as specified, duration within ±1 s of the source.
3. The source ETag now (from `FileStat` on the `library.Reader` path) equals the leased `source_etag`. A mismatch fails with `SOURCE_CHANGED`: no retry, the job is re-evaluated later if `CutoffMet` arrives again.
4. Publish `TranscodeVerified`. Library re-checks the ETag, swaps atomically, and publishes `FileTranscoded` (job → `SWAPPED`) or `TranscodeSwapRejected` (job → `FAILED`, see consumed events).
5. Whisper: the staged SRT parses and has ≥ 1 cue. Then publish `WhisperCompleted` and mark the job `COMPLETED`.

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
