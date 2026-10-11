package transcode

import (
	"errors"
	"net/http"
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
	// LifetimeSavedBytes is what the agent's verified transcodes saved. Jobs arrive in ticket 24.
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
	TransferCap        int `json:"transferCap" validate:"required"`
	DefaultSlotsPerGPU int `json:"defaultSlotsPerGpu" validate:"required"`
}

type UpdateTranscodeSettingsRequest struct {
	TransferCap        int `json:"transferCap" validate:"required"`        // 1–16
	DefaultSlotsPerGPU int `json:"defaultSlotsPerGpu" validate:"required"` // 1–4
}

type Controller struct {
	service *AgentService
	socket  *AgentSocket
	secret  AgentSecret
}

func NewController(service *AgentService, socket *AgentSocket, secret AgentSecret) *Controller {
	return &Controller{service: service, socket: socket, secret: secret}
}

func (c *Controller) Register(s *fuego.Server) {
	fuego.GetStd(s, strings.TrimPrefix(agentproto.ControlPath, httpserver.APIPrefix), c.socket.ServeHTTP, fuego.OptionHide())

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
		fuego.OptionSummary("Update transcode settings (admin)"),
		fuego.OptionTags(tagTranscode),
	)
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
	response := make([]AgentResponse, 0, len(agents))
	for _, agent := range agents {
		response = append(response, agentResponse(agent, defaultSlots))
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
	return agentResponse(agent, defaultSlots), nil
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
	current, err := c.service.Settings(ctx.Context())
	return TranscodeSettingsResponse(current), err
}

func (c *Controller) updateSettings(ctx fuego.ContextWithBody[UpdateTranscodeSettingsRequest]) (TranscodeSettingsResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return TranscodeSettingsResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return TranscodeSettingsResponse{}, err
	}
	err = c.service.UpdateSettings(ctx.Context(), AgentSettings(body))
	if errors.Is(err, ErrInvalidSetting) {
		return TranscodeSettingsResponse{}, problem.New(problem.TypeBadRequest, http.StatusBadRequest, err.Error())
	}
	if err != nil {
		return TranscodeSettingsResponse{}, err
	}
	return TranscodeSettingsResponse(body), nil
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
