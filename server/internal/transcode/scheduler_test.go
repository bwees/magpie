package transcode

import (
	"reflect"
	"testing"

	"magpie/internal/domain"
	"magpie/internal/transcode/api"
	"magpie/internal/transcode/api/agentproto"
)

const (
	testDevice      = "cuda:0"
	secondDevice    = "/dev/dri/renderD128"
	testDefault     = 1
	geForceSessions = 12
)

type deviceOption func(*api.Device)

func passed(c agentproto.Capability) deviceOption {
	return func(d *api.Device) {
		d.Capabilities = append(d.Capabilities, api.Capability{Name: string(c), OK: true})
	}
}

func failed(c agentproto.Capability) deviceOption {
	return func(d *api.Device) {
		d.Capabilities = append(d.Capabilities, api.Capability{Name: string(c), Error: "OpenEncodeSessionEx failed"})
	}
}

func maxSessions(n int) deviceOption {
	return func(d *api.Device) { d.MaxSessions = n }
}

func unhealthy() deviceOption {
	return func(d *api.Device) { d.Healthy = false }
}

func device(id string, options ...deviceOption) api.Device {
	d := api.Device{ID: id, Vendor: string(agentproto.VendorNVIDIA), Healthy: true}
	for _, option := range options {
		option(&d)
	}
	return d
}

func onlineAgent(devices ...api.Device) api.Agent {
	return api.Agent{ID: domain.NewID[domain.AgentID](), Online: true, Devices: devices}
}

func queuedJob(needs agentproto.Capability) QueuedJob {
	return QueuedJob{ID: domain.NewID[domain.JobID](), Needs: needs}
}

// Seed 8: a stubbed queued job stands in for ticket 24's jobs table.
func TestPlanOffersOnlyToADevicePassingTheTargetCapability(t *testing.T) {
	agentA := onlineAgent(device(testDevice, passed(agentproto.CapEncHEVC10), failed(agentproto.CapEncAV110)))
	agentB := onlineAgent(device(testDevice, passed(agentproto.CapEncAV110)))
	first := queuedJob(agentproto.CapEncAV110)
	second := queuedJob(agentproto.CapEncAV110)

	offers := Plan([]QueuedJob{first, second}, []api.Agent{agentA, agentB}, nil, testDefault)

	want := []Offer{{Job: first.ID, Slot: SlotKey{Agent: agentB.ID, Device: testDevice}}}
	if !reflect.DeepEqual(offers, want) {
		t.Fatalf("offers = %+v, want %+v (agent A has no AV1 encode, B has one slot)", offers, want)
	}
}

// Seed 10.
func TestPlanSkipsADeviceAtItsSessionLimit(t *testing.T) {
	slots := geForceSessions + 1
	agent := onlineAgent(device(testDevice, passed(agentproto.CapEncHEVC10), maxSessions(geForceSessions)))
	agent.SlotsOverride = &slots
	running := map[SlotKey]int{{Agent: agent.ID, Device: testDevice}: geForceSessions}

	if offers := Plan([]QueuedJob{queuedJob(agentproto.CapEncHEVC10)}, []api.Agent{agent}, running, testDefault); len(offers) != 0 {
		t.Fatalf("offers = %+v, want none: the device runs %d of %d sessions", offers, geForceSessions, geForceSessions)
	}
}

func TestPlanSkipsExcludedAgents(t *testing.T) {
	agent := onlineAgent(device(testDevice, passed(agentproto.CapEncHEVC10)))
	job := queuedJob(agentproto.CapEncHEVC10)
	job.ExcludedAgents = []domain.AgentID{agent.ID}

	if offers := Plan([]QueuedJob{job}, []api.Agent{agent}, nil, testDefault); len(offers) != 0 {
		t.Fatalf("offers = %+v, want none", offers)
	}
}

func TestPlanPrefersTheDeviceRunningFewestJobs(t *testing.T) {
	busy := onlineAgent(device(testDevice, passed(agentproto.CapEncHEVC10)))
	idle := onlineAgent(device(testDevice, passed(agentproto.CapEncHEVC10)))
	slots := 2
	busy.SlotsOverride = &slots
	idle.SlotsOverride = &slots
	running := map[SlotKey]int{{Agent: busy.ID, Device: testDevice}: 1}
	job := queuedJob(agentproto.CapEncHEVC10)

	offers := Plan([]QueuedJob{job}, []api.Agent{busy, idle}, running, testDefault)

	want := []Offer{{Job: job.ID, Slot: SlotKey{Agent: idle.ID, Device: testDevice}}}
	if !reflect.DeepEqual(offers, want) {
		t.Fatalf("offers = %+v, want %+v", offers, want)
	}
}

func TestFreeSlots(t *testing.T) {
	two, zero := 2, 0
	withOverride := func(n *int, a api.Agent) api.Agent { a.SlotsOverride = n; return a }
	offline := onlineAgent(device(testDevice))
	offline.Online = false
	draining := onlineAgent(device(testDevice))
	draining.Draining = true

	tests := []struct {
		name    string
		agent   api.Agent
		running int
		want    int
	}{
		{"default slots", onlineAgent(device(testDevice)), 0, testDefault},
		{"default slots used up", onlineAgent(device(testDevice)), testDefault, 0},
		{"override raises slots", withOverride(&two, onlineAgent(device(testDevice))), 1, 1},
		{"override of zero drains", withOverride(&zero, onlineAgent(device(testDevice))), 0, 0},
		{"session limit caps the override", withOverride(&two, onlineAgent(device(testDevice, maxSessions(1)))), 0, 1},
		{"unhealthy device", onlineAgent(device(testDevice, unhealthy())), 0, 0},
		{"offline agent", offline, 0, 0},
		{"draining agent", draining, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := SlotKey{Agent: tt.agent.ID, Device: testDevice}
			free := FreeSlots([]api.Agent{tt.agent}, map[SlotKey]int{key: tt.running}, testDefault)
			if free[key] != tt.want {
				t.Fatalf("free slots = %d, want %d", free[key], tt.want)
			}
		})
	}
}

func TestFreeSlotsCountsEachDevice(t *testing.T) {
	agent := onlineAgent(device(testDevice), device(secondDevice))
	running := map[SlotKey]int{{Agent: agent.ID, Device: testDevice}: testDefault}

	free := FreeSlots([]api.Agent{agent}, running, testDefault)

	want := map[SlotKey]int{{Agent: agent.ID, Device: secondDevice}: testDefault}
	if !reflect.DeepEqual(free, want) {
		t.Fatalf("free = %v, want %v", free, want)
	}
}
