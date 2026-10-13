package transcode

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"magpie/internal/domain"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/db"
	"magpie/internal/platform/jobs"
	"magpie/internal/platform/settings"
	"magpie/internal/transcode/api"
	"magpie/internal/transcode/api/agentproto"

	"gorm.io/gorm"
)

var ErrInvalidSetting = errors.New("transcode: setting out of range")

// AgentOverrides are the per-agent settings an admin edits on Activity → Agents.
type AgentOverrides struct {
	// SlotsOverride replaces the default slots per GPU; nil uses the default.
	SlotsOverride *int
	Mode          api.TransferMode
	Draining      bool
}

var transferModes = []api.TransferMode{api.TransferDownloadFirst, api.TransferDirectRead}

func (o AgentOverrides) validate() error {
	if o.SlotsOverride != nil && (*o.SlotsOverride < MinSlotsOverride || *o.SlotsOverride > MaxSlotsOverride) {
		return fmt.Errorf("%w: slots must be %d–%d", ErrInvalidSetting, MinSlotsOverride, MaxSlotsOverride)
	}
	if !slices.Contains(transferModes, o.Mode) {
		return fmt.Errorf("%w: unknown transfer mode %q", ErrInvalidSetting, o.Mode)
	}
	return nil
}

func (s AgentSettings) validate() error {
	if s.TransferCap < MinTransferCap || s.TransferCap > MaxTransferCap {
		return fmt.Errorf("%w: transfer cap must be %d–%d", ErrInvalidSetting, MinTransferCap, MaxTransferCap)
	}
	if s.DefaultSlotsPerGPU < MinSlotsPerGPU || s.DefaultSlotsPerGPU > MaxSlotsPerGPU {
		return fmt.Errorf("%w: default jobs per GPU must be %d–%d", ErrInvalidSetting, MinSlotsPerGPU, MaxSlotsPerGPU)
	}
	return nil
}

// AgentService registers agents, tracks their liveness and capabilities, and pushes admin changes to them.
type AgentService struct {
	db      *db.DB
	bus     *bus.Bus
	repo    *AgentRepository
	hub     *AgentHub
	store   *settings.Store
	queue   jobs.Enqueuer
	timings Timings
	now     func() time.Time
}

func NewAgentService(d *db.DB, b *bus.Bus, repo *AgentRepository, hub *AgentHub, store *settings.Store, queue jobs.Enqueuer, timings Timings) *AgentService {
	return &AgentService{
		db:      d,
		bus:     b,
		repo:    repo,
		hub:     hub,
		store:   store,
		queue:   queue,
		timings: timings,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

// rememberPassed records the capabilities the devices passed. The first agent to pass a target's test
// encode sweeps the files skipped for want of one.
func (s *AgentService) rememberPassed(tx *gorm.DB, id domain.AgentID, devices []agentproto.DeviceReport) error {
	first, err := s.repo.recordPassed(tx, id, devices, s.now())
	if err != nil || !liftsNoCapableAgent(first) {
		return err
	}
	return s.queue.EnqueueTx(tx, sweepTask{Reason: api.SkipNoCapableAgent})
}

// Connect registers the agent on its first Hello, refreshes it on later ones, and returns its Welcome.
// An agent that was offline publishes AgentOnline.
func (s *AgentService) Connect(ctx context.Context, hello agentproto.Hello) (agentproto.Welcome, error) {
	var welcome agentproto.Welcome
	err := s.db.Write(ctx, func(tx *gorm.DB) error {
		wasOnline, err := s.repo.register(tx, hello, s.now(), s.timings.RetestDelay)
		if err != nil {
			return err
		}
		if err := s.rememberPassed(tx, hello.AgentID, hello.Devices); err != nil {
			return err
		}
		if !wasOnline {
			if err := s.bus.Publish(tx, bus.EventOf(api.AgentOnline{Agent: hello.AgentID, Hostname: hello.Hostname})); err != nil {
				return err
			}
		}
		welcome, err = s.welcomeTx(tx, hello.AgentID)
		return err
	})
	return welcome, err
}

// Heartbeat marks the agent seen. An agent that had gone offline publishes AgentOnline again.
func (s *AgentService) Heartbeat(ctx context.Context, id domain.AgentID, heartbeat agentproto.Heartbeat) error {
	return s.db.Write(ctx, func(tx *gorm.DB) error {
		wasOnline, err := s.repo.seen(tx, id, heartbeat.ScratchFree, s.now())
		if err != nil || wasOnline {
			return err
		}
		row, err := s.repo.row(tx, id)
		if err != nil {
			return err
		}
		return s.bus.Publish(tx, bus.EventOf(api.AgentOnline{Agent: id, Hostname: row.Hostname}))
	})
}

// CapabilitiesChanged stores the results of a test run and sends a Welcome with the devices' slots. A
// device that passes again becomes healthy.
func (s *AgentService) CapabilitiesChanged(ctx context.Context, id domain.AgentID, changed agentproto.CapabilitiesChanged) error {
	var welcome agentproto.Welcome
	err := s.db.Write(ctx, func(tx *gorm.DB) error {
		if err := s.repo.saveDevices(tx, id, changed.Devices, s.now(), s.timings.RetestDelay); err != nil {
			return err
		}
		if err := s.rememberPassed(tx, id, changed.Devices); err != nil {
			return err
		}
		var err error
		welcome, err = s.welcomeTx(tx, id)
		return err
	})
	if err != nil {
		return err
	}
	s.hub.clearBusy(id)
	s.push(ctx, id, agentproto.TypeWelcome, welcome)
	return nil
}

// Disconnected takes an agent offline once its socket closes, unless it has reconnected since.
func (s *AgentService) Disconnected(ctx context.Context, id domain.AgentID) error {
	return s.db.Write(ctx, func(tx *gorm.DB) error {
		if s.hub.connected(id) {
			return nil
		}
		row, err := s.repo.row(tx, id)
		if err != nil || !row.Online {
			return err
		}
		return s.takeOffline(tx, row)
	})
}

func (s *AgentService) takeOffline(tx *gorm.DB, agent agentRow) error {
	if err := s.repo.setOffline(tx, domain.AgentID(agent.ID)); err != nil {
		return err
	}
	return s.bus.Publish(tx, bus.EventOf(api.AgentOffline{Agent: domain.AgentID(agent.ID), Since: agent.LastSeenAt}))
}

// DeviceUnhealthy frees the device's slots. The sweep asks for a retest after RetestDelay.
func (s *AgentService) DeviceUnhealthy(ctx context.Context, id domain.AgentID, report agentproto.DeviceUnhealthy) error {
	slog.WarnContext(ctx, "transcode: device unhealthy", "agent", id, "device", report.DeviceID, "error", report.Error)
	return s.db.Write(ctx, func(tx *gorm.DB) error {
		return s.repo.markUnhealthy(tx, SlotKey{Agent: id, Device: report.DeviceID}, s.now().Add(s.timings.RetestDelay))
	})
}

// Sweep takes silent agents offline (publishing AgentOffline) and asks for retests that are due.
func (s *AgentService) Sweep(ctx context.Context) error {
	now := s.now()
	var retests []SlotKey
	err := s.db.Write(ctx, func(tx *gorm.DB) error {
		silent, err := s.repo.silentAgents(tx, now.Add(-offlineAfter(s.timings)))
		if err != nil {
			return err
		}
		for _, agent := range silent {
			if err := s.takeOffline(tx, agent); err != nil {
				return err
			}
		}
		retests, err = s.repo.dueRetests(tx, now, now.Add(s.timings.RetestDelay))
		return err
	})
	if err != nil {
		return err
	}
	for _, key := range retests {
		err := s.hub.Send(ctx, key.Agent, agentproto.TypeRetest, agentproto.Retest{DeviceID: key.Device})
		if err != nil && !errors.Is(err, ErrAgentNotConnected) {
			slog.WarnContext(ctx, "transcode: sending retest", "agent", key.Agent, "device", key.Device, "error", err)
		}
	}
	return nil
}

// Retest asks a connected agent to run its capability tests again on every device.
func (s *AgentService) Retest(ctx context.Context, id domain.AgentID) error {
	return s.hub.Send(ctx, id, agentproto.TypeRetest, agentproto.Retest{})
}

// Update stores an agent's overrides and sends them to it. Turning draining on also sends Drain.
func (s *AgentService) Update(ctx context.Context, id domain.AgentID, overrides AgentOverrides) (api.Agent, error) {
	if err := overrides.validate(); err != nil {
		return api.Agent{}, err
	}
	var before, after api.Agent
	var welcome agentproto.Welcome
	err := s.db.Write(ctx, func(tx *gorm.DB) error {
		var err error
		if before, err = s.repo.agentTx(tx, id); err != nil {
			return err
		}
		if err := s.repo.updateOverrides(tx, id, overrides); err != nil {
			return err
		}
		if after, err = s.repo.agentTx(tx, id); err != nil {
			return err
		}
		welcome, err = s.welcomeTx(tx, id)
		return err
	})
	if err != nil {
		return api.Agent{}, err
	}

	s.push(ctx, id, agentproto.TypeWelcome, welcome)
	if after.Draining && !before.Draining {
		s.push(ctx, id, agentproto.TypeDrain, agentproto.Drain{})
	}
	return after, nil
}

// Agents returns every agent, ordered by hostname.
func (s *AgentService) Agents(ctx context.Context) ([]api.Agent, error) {
	return s.repo.Agents(ctx)
}

func (s *AgentService) Settings(ctx context.Context) (AgentSettings, error) {
	return agentSettingsKey.Get(ctx, s.store)
}

// UpdateSettings stores the agent settings and sends every connected agent its new slots.
func (s *AgentService) UpdateSettings(ctx context.Context, updated AgentSettings) error {
	if err := updated.validate(); err != nil {
		return err
	}
	welcomes := map[domain.AgentID]agentproto.Welcome{}
	err := s.db.Write(ctx, func(tx *gorm.DB) error {
		if err := agentSettingsKey.SetTx(tx, updated); err != nil {
			return err
		}
		for _, id := range s.hub.Connected() {
			welcome, err := s.welcomeTx(tx, id)
			if err != nil {
				return err
			}
			welcomes[id] = welcome
		}
		return nil
	})
	if err != nil {
		return err
	}
	for id, welcome := range welcomes {
		s.push(ctx, id, agentproto.TypeWelcome, welcome)
	}
	return nil
}

// DefaultSlots is the slots per GPU for agents without an override.
func (s *AgentService) DefaultSlots(ctx context.Context) (int, error) {
	current, err := s.Settings(ctx)
	return current.DefaultSlotsPerGPU, err
}

func (s *AgentService) welcomeTx(tx *gorm.DB, id domain.AgentID) (agentproto.Welcome, error) {
	agent, err := s.repo.agentTx(tx, id)
	if err != nil {
		return agentproto.Welcome{}, err
	}
	current, err := agentSettingsKey.GetTx(tx)
	if err != nil {
		return agentproto.Welcome{}, err
	}
	slots := make(map[string]int, len(agent.Devices))
	for _, device := range agent.Devices {
		slots[device.ID] = SlotLimit(agent, device, current.DefaultSlotsPerGPU)
	}
	return agentproto.Welcome{
		HeartbeatEvery: s.timings.HeartbeatEvery,
		LeaseTTL:       s.timings.LeaseTTL,
		Mode:           agentproto.Mode(agent.Mode),
		Slots:          slots,
		Draining:       agent.Draining,
	}, nil
}

// push sends a frame if the agent is connected; an offline agent gets the change in its next Welcome.
func (s *AgentService) push(ctx context.Context, id domain.AgentID, messageType agentproto.MessageType, body any) {
	err := s.hub.Send(ctx, id, messageType, body)
	if err != nil && !errors.Is(err, ErrAgentNotConnected) {
		slog.WarnContext(ctx, "transcode: pushing to agent", "agent", id, "type", messageType, "error", err)
	}
}
