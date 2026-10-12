package transcode

import (
	"context"
	"errors"
	"slices"
	"time"

	"magpie/internal/domain"
	"magpie/internal/platform/db"
	"magpie/internal/transcode/api"
	"magpie/internal/transcode/api/agentproto"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	agentsTable       = "transcode_agents"
	devicesTable      = "transcode_agent_devices"
	capabilitiesTable = "transcode_device_capabilities"
)

var (
	ErrAgentNotFound  = errors.New("transcode: agent not found")
	ErrDeviceNotFound = errors.New("transcode: device not found")
)

type agentRow struct {
	ID               string `gorm:"primaryKey"`
	Hostname         string
	Version          string
	FFmpegVersion    string `gorm:"column:ffmpeg_version"`
	ReportedMode     string
	ModeOverride     *string
	SlotsOverride    *int
	Online           bool
	Draining         bool
	LastSeenAt       time.Time
	ScratchFreeBytes int64
	ThroughputBps    int64
	CreatedAt        time.Time
}

func (agentRow) TableName() string { return agentsTable }

type deviceRow struct {
	AgentID     string `gorm:"primaryKey"`
	DeviceID    string `gorm:"primaryKey"`
	Vendor      string
	Name        string
	Driver      string
	MaxSessions int
	Healthy     bool
	RetestAt    *time.Time
	TestedAt    time.Time
}

func (deviceRow) TableName() string { return devicesTable }

type capabilityRow struct {
	AgentID    string `gorm:"primaryKey"`
	DeviceID   string `gorm:"primaryKey"`
	Capability string `gorm:"primaryKey"`
	OK         bool   `gorm:"column:ok"`
	Error      string
	TestedAt   time.Time
}

func (capabilityRow) TableName() string { return capabilitiesTable }

// AgentRepository stores agents, their devices and tested capabilities. Writes take the caller's
// transaction from db.Write so events publish with them.
type AgentRepository struct {
	db *db.DB
}

func NewAgentRepository(d *db.DB) *AgentRepository {
	return &AgentRepository{db: d}
}

// register creates or refreshes the agent from its Hello and replaces its devices. It reports whether
// the agent was already online.
func (r *AgentRepository) register(tx *gorm.DB, hello agentproto.Hello, now time.Time, retestDelay time.Duration) (wasOnline bool, err error) {
	var existing agentRow
	err = tx.Where("id = ?", hello.AgentID).Take(&existing).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		existing = agentRow{ID: string(hello.AgentID), CreatedAt: now}
	case err != nil:
		return false, err
	default:
		wasOnline = existing.Online
	}

	existing.Hostname = hello.Hostname
	existing.Version = hello.Version
	existing.FFmpegVersion = hello.FFmpegVersion
	existing.ReportedMode = string(hello.Mode)
	existing.ScratchFreeBytes = hello.ScratchFree
	existing.Online = true
	existing.LastSeenAt = now
	if err := tx.Save(&existing).Error; err != nil {
		return false, err
	}

	if err := tx.Where("agent_id = ?", hello.AgentID).Delete(&deviceRow{}).Error; err != nil {
		return false, err
	}
	return wasOnline, r.saveDevices(tx, hello.AgentID, hello.Devices, now, retestDelay)
}

// saveDevices upserts each reported device with its capabilities. A device that passed nothing is
// unhealthy and gets a retest after retestDelay.
func (r *AgentRepository) saveDevices(tx *gorm.DB, agent domain.AgentID, reports []agentproto.DeviceReport, now time.Time, retestDelay time.Duration) error {
	for _, report := range reports {
		healthy := slices.ContainsFunc(report.Capabilities, func(c agentproto.CapabilityResult) bool { return c.OK })
		device := deviceRow{
			AgentID:     string(agent),
			DeviceID:    report.DeviceID,
			Vendor:      string(report.Vendor),
			Name:        report.Name,
			Driver:      report.Driver,
			MaxSessions: report.MaxSessions,
			Healthy:     healthy,
			TestedAt:    now,
		}
		if !healthy {
			retestAt := now.Add(retestDelay)
			device.RetestAt = &retestAt
		}
		if err := tx.Save(&device).Error; err != nil {
			return err
		}
		if err := tx.Where("agent_id = ? AND device_id = ?", agent, report.DeviceID).Delete(&capabilityRow{}).Error; err != nil {
			return err
		}
		for _, result := range report.Capabilities {
			row := capabilityRow{
				AgentID:    string(agent),
				DeviceID:   report.DeviceID,
				Capability: string(result.Capability),
				OK:         result.OK,
				Error:      result.Error,
				TestedAt:   now,
			}
			if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// recordPassed remembers each capability the reports passed and returns those no agent had passed before.
func (r *AgentRepository) recordPassed(tx *gorm.DB, agent domain.AgentID, reports []agentproto.DeviceReport, now time.Time) ([]agentproto.Capability, error) {
	var first []agentproto.Capability
	for _, report := range reports {
		for _, result := range report.Capabilities {
			if !result.OK {
				continue
			}
			row := passedCapabilityRow{Capability: result.Capability, AgentID: agent, PassedAt: now}
			created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
			if created.Error != nil {
				return nil, created.Error
			}
			if created.RowsAffected > 0 {
				first = append(first, result.Capability)
			}
		}
	}
	return first, nil
}

// seen records a heartbeat and reports whether the agent was already online.
func (r *AgentRepository) seen(tx *gorm.DB, id domain.AgentID, scratchFree int64, now time.Time) (wasOnline bool, err error) {
	row, err := r.row(tx, id)
	if err != nil {
		return false, err
	}
	return row.Online, tx.Model(&agentRow{}).Where("id = ?", id).
		Updates(map[string]any{"online": true, "last_seen_at": now, "scratch_free_bytes": scratchFree}).Error
}

// markUnhealthy takes a device out of scheduling until a retest at retestAt passes.
func (r *AgentRepository) markUnhealthy(tx *gorm.DB, key SlotKey, retestAt time.Time) error {
	result := tx.Model(&deviceRow{}).Where("agent_id = ? AND device_id = ?", key.Agent, key.Device).
		Updates(map[string]any{"healthy": false, "retest_at": retestAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrDeviceNotFound
	}
	return nil
}

// silentAgents returns online agents not heard from since cutoff.
func (r *AgentRepository) silentAgents(tx *gorm.DB, cutoff time.Time) ([]agentRow, error) {
	var rows []agentRow
	err := tx.Where("online AND last_seen_at < ?", cutoff).Find(&rows).Error
	return rows, err
}

func (r *AgentRepository) setOffline(tx *gorm.DB, id domain.AgentID) error {
	return tx.Model(&agentRow{}).Where("id = ?", id).Update("online", false).Error
}

// dueRetests returns unhealthy devices of online agents whose retest time has come, and moves each
// one's next retest to next so a retest that never answers is asked for again.
func (r *AgentRepository) dueRetests(tx *gorm.DB, now, next time.Time) ([]SlotKey, error) {
	var rows []deviceRow
	err := tx.Joins("JOIN "+agentsTable+" ON "+agentsTable+".id = "+devicesTable+".agent_id").
		Where(agentsTable+".online AND NOT "+devicesTable+".healthy AND "+devicesTable+".retest_at <= ?", now).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	keys := make([]SlotKey, 0, len(rows))
	for _, row := range rows {
		if err := tx.Model(&row).Update("retest_at", next).Error; err != nil {
			return nil, err
		}
		keys = append(keys, SlotKey{Agent: domain.AgentID(row.AgentID), Device: row.DeviceID})
	}
	return keys, nil
}

func (r *AgentRepository) updateOverrides(tx *gorm.DB, id domain.AgentID, o AgentOverrides) error {
	mode := string(o.Mode)
	result := tx.Model(&agentRow{}).Where("id = ?", id).
		Updates(map[string]any{"slots_override": o.SlotsOverride, "mode_override": &mode, "draining": o.Draining})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrAgentNotFound
	}
	return nil
}

func (r *AgentRepository) row(tx *gorm.DB, id domain.AgentID) (agentRow, error) {
	var row agentRow
	err := tx.Where("id = ?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row, ErrAgentNotFound
	}
	return row, err
}

// Agents returns every agent with devices and capabilities, ordered by hostname.
func (r *AgentRepository) Agents(ctx context.Context) ([]api.Agent, error) {
	return r.agents(r.db.Read(ctx), nil)
}

// agentTx reads one agent inside a write transaction.
func (r *AgentRepository) agentTx(tx *gorm.DB, id domain.AgentID) (api.Agent, error) {
	agents, err := r.agents(tx, &id)
	if err != nil {
		return api.Agent{}, err
	}
	if len(agents) == 0 {
		return api.Agent{}, ErrAgentNotFound
	}
	return agents[0], nil
}

func (r *AgentRepository) agents(conn *gorm.DB, only *domain.AgentID) ([]api.Agent, error) {
	agentQuery, deviceQuery, capabilityQuery := conn, conn, conn
	if only != nil {
		agentQuery = agentQuery.Where("id = ?", *only)
		deviceQuery = deviceQuery.Where("agent_id = ?", *only)
		capabilityQuery = capabilityQuery.Where("agent_id = ?", *only)
	}

	var agents []agentRow
	if err := agentQuery.Order("hostname, id").Find(&agents).Error; err != nil {
		return nil, err
	}
	var devices []deviceRow
	if err := deviceQuery.Order("device_id").Find(&devices).Error; err != nil {
		return nil, err
	}
	var capabilities []capabilityRow
	if err := capabilityQuery.Order("capability").Find(&capabilities).Error; err != nil {
		return nil, err
	}

	byDevice := map[SlotKey][]api.Capability{}
	for _, c := range capabilities {
		key := SlotKey{Agent: domain.AgentID(c.AgentID), Device: c.DeviceID}
		byDevice[key] = append(byDevice[key], api.Capability{Name: c.Capability, OK: c.OK, Error: c.Error, TestedAt: c.TestedAt})
	}
	byAgent := map[domain.AgentID][]api.Device{}
	for _, d := range devices {
		agent := domain.AgentID(d.AgentID)
		byAgent[agent] = append(byAgent[agent], api.Device{
			ID:           d.DeviceID,
			Vendor:       d.Vendor,
			Name:         d.Name,
			Driver:       d.Driver,
			MaxSessions:  d.MaxSessions,
			Healthy:      d.Healthy,
			Capabilities: byDevice[SlotKey{Agent: agent, Device: d.DeviceID}],
		})
	}

	result := make([]api.Agent, 0, len(agents))
	for _, a := range agents {
		mode := a.ReportedMode
		if a.ModeOverride != nil {
			mode = *a.ModeOverride
		}
		result = append(result, api.Agent{
			ID:               domain.AgentID(a.ID),
			Hostname:         a.Hostname,
			Version:          a.Version,
			FFmpegVersion:    a.FFmpegVersion,
			Mode:             api.TransferMode(mode),
			SlotsOverride:    a.SlotsOverride,
			Online:           a.Online,
			Draining:         a.Draining,
			LastSeenAt:       a.LastSeenAt,
			ScratchFreeBytes: a.ScratchFreeBytes,
			ThroughputBps:    a.ThroughputBps,
			Devices:          byAgent[domain.AgentID(a.ID)],
		})
	}
	return result, nil
}
