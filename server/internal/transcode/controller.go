package transcode

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"time"

	"magpie/internal/domain"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/problem"
	"magpie/internal/transcode/api"
	"magpie/internal/transcode/api/agentproto"

	"github.com/go-fuego/fuego"
)

const (
	tagTranscode = "transcode"
	pathID       = "id"

	TypeAgentNotFound  problem.Type = "urn:magpie:problem:agent-not-found"
	TypeAgentNotOnline problem.Type = "urn:magpie:problem:agent-not-connected"
)

type AgentCapabilityResponse struct {
	Capability string    `json:"capability" validate:"required"` // ENC_HEVC_10 | ENC_AV1_10 | DEC_HEVC_10 | DEC_H264_8 | PIPELINE_HEVC_10 | WHISPER_VULKAN
	OK         bool      `json:"ok" validate:"required"`
	Error      string    `json:"error" validate:"required"`
	TestedAt   time.Time `json:"testedAt" validate:"required"`
}

type AgentDeviceResponse struct {
	ID           string                    `json:"id" validate:"required"`
	Vendor       string                    `json:"vendor" validate:"required"`
	Name         string                    `json:"name" validate:"required"`
	Driver       string                    `json:"driver" validate:"required"`
	MaxSessions  int                       `json:"maxSessions" validate:"required"` // 0 = unknown
	Healthy      bool                      `json:"healthy" validate:"required"`
	Slots        int                       `json:"slots" validate:"required"` // concurrent jobs allowed now
	Capabilities []AgentCapabilityResponse `json:"capabilities" validate:"required"`
}

type AgentResponse struct {
	ID               string           `json:"id" validate:"required"`
	Hostname         string           `json:"hostname" validate:"required"`
	Version          string           `json:"version" validate:"required"`
	FFmpegVersion    string           `json:"ffmpegVersion" validate:"required"`
	Mode             api.TransferMode `json:"mode" validate:"required"`
	SlotsOverride    *int             `json:"slotsOverride"` // null uses the default slots per GPU
	Online           bool             `json:"online" validate:"required"`
	Draining         bool             `json:"draining" validate:"required"`
	LastSeenAt       time.Time        `json:"lastSeenAt" validate:"required"`
	ScratchFreeBytes int64            `json:"scratchFreeBytes" validate:"required"`
	ThroughputBps    int64            `json:"throughputBps" validate:"required"` // 0 until the agent has transferred a job
	// LifetimeSavedBytes is what the agent's swapped transcodes saved.
	LifetimeSavedBytes int64                 `json:"lifetimeSavedBytes" validate:"required"`
	Devices            []AgentDeviceResponse `json:"devices" validate:"required"`
}

type UpdateAgentRequest struct {
	SlotsOverride *int             `json:"slotsOverride"` // 0–8; null uses the default
	Mode          api.TransferMode `json:"mode" validate:"required,oneof=DOWNLOAD_FIRST DIRECT_READ"`
	Draining      bool             `json:"draining"`
}

type RetestResponse struct {
	Agent string `json:"agent" validate:"required"`
}

type AgentSecretResponse struct {
	Configured bool   `json:"configured" validate:"required"`
	Masked     string `json:"masked" validate:"required"`
	Secret     string `json:"secret" validate:"required"` // for the Copy button
}

type TranscodeSettingsResponse struct {
	Enabled           bool                        `json:"enabled" validate:"required"`
	TargetCodec       api.TargetCodec             `json:"targetCodec" validate:"required"` // HEVC_10 | AV1_10
	Preset            api.Preset                  `json:"preset" validate:"required"`      // ARCHIVE | HIGH | BALANCED | SMALL
	BitrateCapsMbps   map[api.ResolutionClass]int `json:"bitrateCapsMbps" validate:"required"`
	SkipAlreadyTarget bool                        `json:"skipAlreadyTarget" validate:"required"`
	SkipLowBitrate    bool                        `json:"skipLowBitrate" validate:"required"`
	LowBitratePct     int                         `json:"lowBitratePct" validate:"required"`
	MinSavingPct      int                         `json:"minSavingPct" validate:"required"`
	// EncoderOverrides replaces the calibrated quality value of an encoder.
	EncoderOverrides   map[string]int `json:"encoderOverrides" validate:"required"`
	TransferCap        int            `json:"transferCap" validate:"required"`
	DefaultSlotsPerGPU int            `json:"defaultSlotsPerGpu" validate:"required"`
	// Encoders is the calibration table, read-only.
	Encoders []EncoderCalibrationResponse `json:"encoders" validate:"required"`
}

type EncoderCalibrationResponse struct {
	Encoder    string             `json:"encoder" validate:"required"`    // hevc_nvenc, av1_qsv, libx265, …
	QualityArg string             `json:"qualityArg" validate:"required"` // cq | global_quality | crf
	Min        int                `json:"min" validate:"required"`
	Max        int                `json:"max" validate:"required"`
	Presets    map[api.Preset]int `json:"presets" validate:"required"`
}

// UpdateTranscodeSettingsRequest replaces every setting; the bounds are in 12-settings.
type UpdateTranscodeSettingsRequest struct {
	Enabled            bool                        `json:"enabled"`
	TargetCodec        api.TargetCodec             `json:"targetCodec" validate:"required"`
	Preset             api.Preset                  `json:"preset" validate:"required"`
	BitrateCapsMbps    map[api.ResolutionClass]int `json:"bitrateCapsMbps" validate:"required"` // 1–200 each
	SkipAlreadyTarget  bool                        `json:"skipAlreadyTarget"`
	SkipLowBitrate     bool                        `json:"skipLowBitrate"`
	LowBitratePct      int                         `json:"lowBitratePct"`      // 10–100
	MinSavingPct       int                         `json:"minSavingPct"`       // 0–90
	EncoderOverrides   map[string]int              `json:"encoderOverrides"`   // within each encoder's range
	TransferCap        int                         `json:"transferCap"`        // 1–16
	DefaultSlotsPerGPU int                         `json:"defaultSlotsPerGpu"` // 1–4
}

type JobResponse struct {
	ID          string          `json:"id" validate:"required"`
	Kind        api.JobKind     `json:"kind" validate:"required"`  // TRANSCODE | WHISPER
	State       api.JobState    `json:"state" validate:"required"` // QUEUED … CANCELLED
	MediaFile   string          `json:"mediaFile" validate:"required"`
	Title       string          `json:"title" validate:"required"`
	TitleName   string          `json:"titleName" validate:"required"`
	Episodes    string          `json:"episodes" validate:"required"` // S01E02; empty for a movie
	FileName    string          `json:"fileName" validate:"required"`
	Origin      domain.Origin   `json:"origin" validate:"required"`      // DOWNLOAD | LIBRARY_SCAN | MIGRATION
	TargetCodec api.TargetCodec `json:"targetCodec" validate:"required"` // HEVC_10 | AV1_10; empty for Whisper
	Language    domain.Language `json:"language" validate:"required"`    // the subtitle Whisper writes; empty for a transcode
	Agent       *string         `json:"agent"`                           // running it, or the last attempt's
	AgentName   string          `json:"agentName" validate:"required"`
	Attempt     int             `json:"attempt" validate:"required"`
	MaxAttempts int             `json:"maxAttempts" validate:"required"`
	Progress    float64         `json:"progress" validate:"required"`   // 0..1 within the current state
	Prefetched  bool            `json:"prefetched" validate:"required"` // fetched ahead; encodes when its slot frees
	Error       string          `json:"error" validate:"required"`
	QueuedAt    time.Time       `json:"queuedAt" validate:"required"`
	NotBefore   *time.Time      `json:"notBefore"` // a retry waits until then
	FinishedAt  *time.Time      `json:"finishedAt"`
}

type JobListResponse struct {
	Jobs []JobResponse `json:"jobs" validate:"required"`
	// Counts holds the jobs per state; states without jobs are absent.
	Counts map[api.JobState]int `json:"counts" validate:"required"`
	// Backlog is the queued jobs of files found by a library scan or a migration.
	Backlog int `json:"backlog" validate:"required"`
	// Skipped counts eligible files kept as they are, per reason; reasons without files are absent.
	Skipped map[api.SkipReason]int `json:"skipped" validate:"required"`
}

const (
	queryState = "state"
	queryKind  = "kind"
	queryLimit = "limit"

	TypeJobNotFound  problem.Type = "urn:magpie:problem:job-not-found"
	TypeJobFinished  problem.Type = "urn:magpie:problem:job-finished"
	TypeJobNotFailed problem.Type = "urn:magpie:problem:job-not-failed"

	// The byte endpoints, relative to the API group.
	sourceRoute = "/transcode/agents/jobs/{" + pathJobID + "}/source"
	outputRoute = "/transcode/agents/jobs/{" + pathJobID + "}/output"
)

type Controller struct {
	service   *AgentService
	jobs      *JobService
	socket    *AgentSocket
	transfers *Transfers
	secret    AgentSecret
}

func NewController(service *AgentService, jobs *JobService, socket *AgentSocket, transfers *Transfers, secret AgentSecret) *Controller {
	return &Controller{service: service, jobs: jobs, socket: socket, transfers: transfers, secret: secret}
}

func (c *Controller) Register(s *fuego.Server) {
	fuego.GetStd(s, strings.TrimPrefix(agentproto.ControlPath, httpserver.APIPrefix), c.socket.ServeHTTP, fuego.OptionHide())
	fuego.GetStd(s, sourceRoute, c.transfers.ServeSource, fuego.OptionHide())
	fuego.Handle(s, outputRoute, http.HandlerFunc(c.output))

	fuego.Get(s, "/transcode/jobs", c.listJobs,
		fuego.OptionOperationID("listTranscodeJobs"),
		fuego.OptionSummary("Jobs agents hold, then queued jobs in offer order, then finished ones (admin)"),
		fuego.OptionTags(tagTranscode),
		fuego.OptionQueryArray(queryState, "Only jobs in these states", reflect.String),
		fuego.OptionQuery(queryKind, "Only TRANSCODE or WHISPER jobs"),
		fuego.OptionQueryInt(queryLimit, "At most this many jobs (default 200)"),
	)
	fuego.Post(s, "/transcode/jobs/{id}/cancel", c.cancelJob,
		fuego.OptionOperationID("cancelTranscodeJob"),
		fuego.OptionSummary("Cancel an unfinished job; its agent drops it (admin)"),
		fuego.OptionTags(tagTranscode),
		fuego.OptionPath(pathID, "Job ID"),
	)
	fuego.Post(s, "/transcode/jobs/{id}/retry", c.retryJob,
		fuego.OptionOperationID("retryTranscodeJob"),
		fuego.OptionSummary("Queue a FAILED or NOT_WORTH_IT job again from its first attempt (admin)"),
		fuego.OptionTags(tagTranscode),
		fuego.OptionPath(pathID, "Job ID"),
	)
	fuego.Delete(s, "/transcode/jobs/{id}", c.dismissJob,
		fuego.OptionOperationID("dismissTranscodeJob"),
		fuego.OptionSummary("Delete a FAILED or NOT_WORTH_IT job (admin)"),
		fuego.OptionTags(tagTranscode),
		fuego.OptionPath(pathID, "Job ID"),
		fuego.OptionDefaultStatusCode(http.StatusNoContent),
	)

	fuego.Get(s, "/transcode/agents", c.listAgents,
		fuego.OptionOperationID("listAgents"),
		fuego.OptionSummary("Agents with devices and tested capabilities, by hostname (admin)"),
		fuego.OptionTags(tagTranscode),
	)
	fuego.Patch(s, "/transcode/agents/{id}", c.updateAgent,
		fuego.OptionOperationID("updateAgent"),
		fuego.OptionSummary("Set an agent's slots override, transfer mode and draining (admin)"),
		fuego.OptionTags(tagTranscode),
		fuego.OptionPath(pathID, "Agent ID"),
	)
	fuego.Post(s, "/transcode/agents/{id}/retest", c.retestAgent,
		fuego.OptionOperationID("retestAgent"),
		fuego.OptionSummary("Ask a connected agent to re-run its capability tests (admin)"),
		fuego.OptionTags(tagTranscode),
		fuego.OptionPath(pathID, "Agent ID"),
		fuego.OptionDefaultStatusCode(http.StatusAccepted),
	)
	fuego.Get(s, "/transcode/secret", c.getSecret,
		fuego.OptionOperationID("getAgentSecret"),
		fuego.OptionSummary("The agent shared secret from MAGPIE_AGENT_SECRET, masked and for copying (admin)"),
		fuego.OptionTags(tagTranscode),
	)
	fuego.Get(s, "/transcode/settings", c.getSettings,
		fuego.OptionOperationID("getTranscodeSettings"),
		fuego.OptionSummary("Transcode settings (admin)"),
		fuego.OptionTags(tagTranscode),
	)
	fuego.Put(s, "/transcode/settings", c.updateSettings,
		fuego.OptionOperationID("updateTranscodeSettings"),
		fuego.OptionSummary("Update transcode settings; a policy change re-evaluates the backlog (admin)"),
		fuego.OptionTags(tagTranscode),
	)
	c.registerBacklog(s)
}

func (c *Controller) listAgents(ctx fuego.ContextNoBody) ([]AgentResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return nil, err
	}
	agents, err := c.service.Agents(ctx.Context())
	if err != nil {
		return nil, err
	}
	defaultSlots, err := c.service.DefaultSlots(ctx.Context())
	if err != nil {
		return nil, err
	}
	saved, err := c.jobs.SavedBytes(ctx.Context())
	if err != nil {
		return nil, err
	}
	response := make([]AgentResponse, 0, len(agents))
	for _, agent := range agents {
		r := agentResponse(agent, defaultSlots)
		r.LifetimeSavedBytes = saved[agent.ID]
		response = append(response, r)
	}
	return response, nil
}

func (c *Controller) updateAgent(ctx fuego.ContextWithBody[UpdateAgentRequest]) (AgentResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return AgentResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return AgentResponse{}, err
	}
	id := domain.AgentID(ctx.PathParam(pathID))
	agent, err := c.service.Update(ctx.Context(), id, AgentOverrides(body))
	if errors.Is(err, ErrAgentNotFound) {
		return AgentResponse{}, problem.New(TypeAgentNotFound, http.StatusNotFound, "no agent "+string(id))
	}
	if errors.Is(err, ErrInvalidSetting) {
		return AgentResponse{}, problem.New(problem.TypeBadRequest, http.StatusBadRequest, err.Error())
	}
	if err != nil {
		return AgentResponse{}, err
	}
	defaultSlots, err := c.service.DefaultSlots(ctx.Context())
	if err != nil {
		return AgentResponse{}, err
	}
	saved, err := c.jobs.SavedBytes(ctx.Context())
	if err != nil {
		return AgentResponse{}, err
	}
	response := agentResponse(agent, defaultSlots)
	response.LifetimeSavedBytes = saved[agent.ID]
	return response, nil
}

func (c *Controller) retestAgent(ctx fuego.ContextNoBody) (RetestResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return RetestResponse{}, err
	}
	id := ctx.PathParam(pathID)
	err := c.service.Retest(ctx.Context(), domain.AgentID(id))
	if errors.Is(err, ErrAgentNotConnected) {
		return RetestResponse{}, problem.New(TypeAgentNotOnline, http.StatusConflict, "agent "+id+" is not connected")
	}
	if err != nil {
		return RetestResponse{}, err
	}
	return RetestResponse{Agent: id}, nil
}

func (c *Controller) getSecret(ctx fuego.ContextNoBody) (AgentSecretResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return AgentSecretResponse{}, err
	}
	return AgentSecretResponse{Configured: c.secret != "", Masked: c.secret.Masked(), Secret: string(c.secret)}, nil
}

func (c *Controller) getSettings(ctx fuego.ContextNoBody) (TranscodeSettingsResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return TranscodeSettingsResponse{}, err
	}
	return c.settings(ctx.Context())
}

func (c *Controller) settings(ctx context.Context) (TranscodeSettingsResponse, error) {
	agents, err := c.service.Settings(ctx)
	if err != nil {
		return TranscodeSettingsResponse{}, err
	}
	policy, err := c.jobs.Policy(ctx)
	if err != nil {
		return TranscodeSettingsResponse{}, err
	}
	overrides := policy.EncoderOverrides
	if overrides == nil {
		overrides = map[string]int{}
	}
	return TranscodeSettingsResponse{
		Enabled: policy.Enabled, TargetCodec: policy.TargetCodec, Preset: policy.Preset, BitrateCapsMbps: policy.CapsMbps,
		SkipAlreadyTarget: policy.SkipAlreadyTarget, SkipLowBitrate: policy.SkipLowBitrate, LowBitratePct: policy.LowBitratePct,
		MinSavingPct: policy.MinSavingPct, EncoderOverrides: overrides,
		TransferCap: agents.TransferCap, DefaultSlotsPerGPU: agents.DefaultSlotsPerGPU,
		Encoders: encoderCalibrations(),
	}, nil
}

func encoderCalibrations() []EncoderCalibrationResponse {
	all := Calibrations()
	response := make([]EncoderCalibrationResponse, 0, len(all))
	for _, encoder := range slices.Sorted(maps.Keys(all)) {
		cal := all[encoder]
		response = append(response, EncoderCalibrationResponse{
			Encoder: encoder, QualityArg: cal.Quality.Arg, Min: cal.Quality.Min, Max: cal.Quality.Max, Presets: cal.Presets,
		})
	}
	return response
}

// updateSettings checks the policy and the agent settings before storing either.
func (c *Controller) updateSettings(ctx fuego.ContextWithBody[UpdateTranscodeSettingsRequest]) (TranscodeSettingsResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return TranscodeSettingsResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return TranscodeSettingsResponse{}, err
	}
	policy := Policy{
		Enabled: body.Enabled, TargetCodec: body.TargetCodec, Preset: body.Preset, CapsMbps: body.BitrateCapsMbps,
		SkipAlreadyTarget: body.SkipAlreadyTarget, SkipLowBitrate: body.SkipLowBitrate, LowBitratePct: body.LowBitratePct,
		MinSavingPct: body.MinSavingPct, EncoderOverrides: body.EncoderOverrides,
	}
	agents := AgentSettings{TransferCap: body.TransferCap, DefaultSlotsPerGPU: body.DefaultSlotsPerGPU}
	err = errors.Join(policy.validate(), agents.validate())
	if err == nil {
		err = c.service.UpdateSettings(ctx.Context(), agents)
	}
	if err == nil {
		err = c.jobs.UpdatePolicy(ctx.Context(), policy)
	}
	if errors.Is(err, ErrInvalidSetting) {
		return TranscodeSettingsResponse{}, problem.New(problem.TypeBadRequest, http.StatusBadRequest, err.Error())
	}
	if err != nil {
		return TranscodeSettingsResponse{}, err
	}
	return c.settings(ctx.Context())
}

// output takes HEAD for the committed offset and PUT for a chunk; the mux pattern has no method so both
// reach it.
func (c *Controller) output(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodHead:
		c.transfers.OutputOffset(w, r)
	case http.MethodPut:
		c.transfers.PutOutput(w, r)
	default:
		w.Header().Set("Allow", http.MethodHead+", "+http.MethodPut)
		problem.Write(w, r, problem.New(problem.TypeBadRequest, http.StatusMethodNotAllowed, "use HEAD or PUT"))
	}
}

var jobStates = []api.JobState{
	api.StateQueued, api.StateOffered, api.StateLeased, api.StateFetching, api.StateEncoding, api.StateChecking,
	api.StateUploading, api.StateVerifying, api.StateVerified, api.StateSwapped, api.StateCompleted, api.StateFailed,
	api.StateNotWorthIt, api.StateCancelled,
}

func (c *Controller) listJobs(ctx fuego.ContextNoBody) (JobListResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return JobListResponse{}, err
	}
	var filter JobFilter
	for _, raw := range ctx.QueryParamArr(queryState) {
		for state := range strings.SplitSeq(raw, ",") {
			if !slices.Contains(jobStates, api.JobState(state)) {
				return JobListResponse{}, problem.New(problem.TypeBadRequest, http.StatusBadRequest, "unknown state "+state)
			}
			filter.States = append(filter.States, api.JobState(state))
		}
	}
	if kind := api.JobKind(ctx.QueryParam(queryKind)); kind != "" {
		if kind != api.JobKindTranscode && kind != api.JobKindWhisper {
			return JobListResponse{}, problem.New(problem.TypeBadRequest, http.StatusBadRequest, "kind must be TRANSCODE or WHISPER")
		}
		filter.Kind = kind
	}
	if ctx.QueryParam(queryLimit) != "" {
		limit, err := ctx.QueryParamIntErr(queryLimit)
		if err != nil || limit < 1 {
			return JobListResponse{}, problem.New(problem.TypeBadRequest, http.StatusBadRequest, "limit must be a positive number")
		}
		filter.Limit = limit
	}
	list, err := c.jobs.Jobs(ctx.Context(), filter)
	if err != nil {
		return JobListResponse{}, err
	}
	backlog, err := c.jobs.Backlog(ctx.Context())
	if err != nil {
		return JobListResponse{}, err
	}
	response := JobListResponse{
		Jobs: make([]JobResponse, 0, len(list.Jobs)), Counts: list.Counts, Backlog: backlog.Backlog, Skipped: backlog.Skipped,
	}
	for _, view := range list.Jobs {
		response.Jobs = append(response.Jobs, jobResponse(view))
	}
	return response, nil
}

func (c *Controller) cancelJob(ctx fuego.ContextNoBody) (JobResponse, error) {
	return c.act(ctx, func(id domain.JobID) error { return c.jobs.Cancel(ctx.Context(), id, ReasonCancelled) })
}

func (c *Controller) retryJob(ctx fuego.ContextNoBody) (JobResponse, error) {
	return c.act(ctx, func(id domain.JobID) error { return c.jobs.Retry(ctx.Context(), id) })
}

func (c *Controller) dismissJob(ctx fuego.ContextNoBody) (any, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return nil, err
	}
	id := domain.JobID(ctx.PathParam(pathID))
	return nil, jobProblem(id, c.jobs.Dismiss(ctx.Context(), id))
}

func (c *Controller) act(ctx fuego.ContextNoBody, action func(domain.JobID) error) (JobResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return JobResponse{}, err
	}
	id := domain.JobID(ctx.PathParam(pathID))
	if err := jobProblem(id, action(id)); err != nil {
		return JobResponse{}, err
	}
	view, err := c.jobs.Job(ctx.Context(), id)
	if err != nil {
		return JobResponse{}, err
	}
	return jobResponse(view), nil
}

func jobProblem(id domain.JobID, err error) error {
	switch {
	case errors.Is(err, ErrJobNotFound):
		return problem.New(TypeJobNotFound, http.StatusNotFound, "no job "+string(id))
	case errors.Is(err, ErrJobFinished):
		return problem.New(TypeJobFinished, http.StatusConflict, "the job has already finished")
	case errors.Is(err, ErrJobNotFailed):
		return problem.New(TypeJobNotFailed, http.StatusConflict, "only a failed or not-worth-it job can be retried or dismissed")
	}
	return err
}

func jobResponse(v JobView) JobResponse {
	var agent *string
	if v.Agent != nil {
		id := string(*v.Agent)
		agent = &id
	}
	return JobResponse{
		ID: string(v.ID), Kind: v.Kind, State: v.State, MediaFile: string(v.MediaFile), Title: string(v.Title),
		TitleName: v.TitleName, Episodes: v.Episodes, FileName: v.FileName, Origin: v.Origin, TargetCodec: v.TargetCodec,
		Language: v.Language, Agent: agent, AgentName: v.AgentName, Attempt: v.Attempt, MaxAttempts: MaxAttempts, Progress: v.Progress,
		Prefetched: v.Prefetched, Error: v.Error, QueuedAt: v.QueuedAt, NotBefore: v.NotBefore, FinishedAt: v.FinishedAt,
	}
}

func agentResponse(agent api.Agent, defaultSlots int) AgentResponse {
	devices := make([]AgentDeviceResponse, 0, len(agent.Devices))
	for _, d := range agent.Devices {
		capabilities := make([]AgentCapabilityResponse, 0, len(d.Capabilities))
		for _, c := range d.Capabilities {
			capabilities = append(capabilities, AgentCapabilityResponse{Capability: c.Name, OK: c.OK, Error: c.Error, TestedAt: c.TestedAt})
		}
		devices = append(devices, AgentDeviceResponse{
			ID:           d.ID,
			Vendor:       d.Vendor,
			Name:         d.Name,
			Driver:       d.Driver,
			MaxSessions:  d.MaxSessions,
			Healthy:      d.Healthy,
			Slots:        SlotLimit(agent, d, defaultSlots),
			Capabilities: capabilities,
		})
	}
	return AgentResponse{
		ID:               string(agent.ID),
		Hostname:         agent.Hostname,
		Version:          agent.Version,
		FFmpegVersion:    agent.FFmpegVersion,
		Mode:             agent.Mode,
		SlotsOverride:    agent.SlotsOverride,
		Online:           agent.Online,
		Draining:         agent.Draining,
		LastSeenAt:       agent.LastSeenAt,
		ScratchFreeBytes: agent.ScratchFreeBytes,
		ThroughputBps:    agent.ThroughputBps,
		Devices:          devices,
	}
}
