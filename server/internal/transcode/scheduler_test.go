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

	offers := Plan([]QueuedJob{first, second}, fleetOf(nil, agentA, agentB))

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

	if offers := Plan([]QueuedJob{queuedJob(agentproto.CapEncHEVC10)}, fleetOf(running, agent)); len(offers) != 0 {
		t.Fatalf("offers = %+v, want none: the device runs %d of %d sessions", offers, geForceSessions, geForceSessions)
	}
}

func TestPlanSkipsExcludedAgents(t *testing.T) {
	agent := onlineAgent(device(testDevice, passed(agentproto.CapEncHEVC10)))
	job := queuedJob(agentproto.CapEncHEVC10)
	job.ExcludedAgents = []domain.AgentID{agent.ID}

	if offers := Plan([]QueuedJob{job}, fleetOf(nil, agent)); len(offers) != 0 {
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

	offers := Plan([]QueuedJob{job}, fleetOf(running, busy, idle))

	want := []Offer{{Job: job.ID, Slot: SlotKey{Agent: idle.ID, Device: testDevice}}}
	if !reflect.DeepEqual(offers, want) {
		t.Fatalf("offers = %+v, want %+v", offers, want)
	}
}

func fleetOf(running map[SlotKey]int, agents ...api.Agent) Fleet {
	return Fleet{Agents: agents, Running: running, DefaultSlots: testDefault}
}

func slotOn(a api.Agent) SlotKey { return SlotKey{Agent: a.ID, Device: testDevice} }

// Seed 33, the scheduler's part: a retry prefers an agent other than the one that failed it.
func TestPlanPrefersAnAgentOtherThanTheLastAttempts(t *testing.T) {
	agentA := onlineAgent(device(testDevice, passed(agentproto.CapEncHEVC10)))
	agentB := onlineAgent(device(testDevice, passed(agentproto.CapEncHEVC10)))
	agentA.ThroughputBps = 1 << 30
	job := queuedJob(agentproto.CapEncHEVC10)
	job.LastAgent = &agentA.ID

	offers := Plan([]QueuedJob{job}, fleetOf(nil, agentA, agentB))

	if want := []Offer{{Job: job.ID, Slot: slotOn(agentB)}}; !reflect.DeepEqual(offers, want) {
		t.Fatalf("offers = %+v, want B", offers)
	}
}

// Seed 13, the scheduler's part: an agent back with the job's scratch gets it, even inside its
// avoid window and ahead of a faster agent.
func TestPlanGivesAResumableJobBackToTheAgentHoldingItsScratch(t *testing.T) {
	restarted := onlineAgent(device(testDevice, passed(agentproto.CapEncHEVC10)))
	faster := onlineAgent(device(testDevice, passed(agentproto.CapEncHEVC10)))
	faster.ThroughputBps = 1 << 30
	job := queuedJob(agentproto.CapEncHEVC10)
	job.Avoid, job.LastAgent, job.ResumableOn = &restarted.ID, &restarted.ID, &restarted.ID

	offers := Plan([]QueuedJob{job}, fleetOf(nil, restarted, faster))

	if want := []Offer{{Job: job.ID, Slot: slotOn(restarted)}}; !reflect.DeepEqual(offers, want) {
		t.Fatalf("offers = %+v, want the restarted agent", offers)
	}
}

// Seed 11, the scheduler's part: an avoided agent isn't offered the job.
func TestPlanSkipsAnAvoidedAgent(t *testing.T) {
	agent := onlineAgent(device(testDevice, passed(agentproto.CapEncHEVC10)))
	job := queuedJob(agentproto.CapEncHEVC10)
	job.Avoid = &agent.ID

	if offers := Plan([]QueuedJob{job}, fleetOf(nil, agent)); len(offers) != 0 {
		t.Fatalf("offers = %+v, want none", offers)
	}
}

func TestPlanNeedsScratchForTheSourceAndTheOutput(t *testing.T) {
	const source = 10 << 30
	tests := []struct {
		name    string
		mode    api.TransferMode
		scratch int64
		offered bool
	}{
		{"download-first needs 2.2x", api.TransferDownloadFirst, 22 << 30, true},
		{"download-first refuses less", api.TransferDownloadFirst, 21 << 30, false},
		{"direct read needs 1.2x", api.TransferDirectRead, 12 << 30, true},
		{"direct read refuses less", api.TransferDirectRead, 11 << 30, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := onlineAgent(device(testDevice, passed(agentproto.CapEncHEVC10)))
			agent.Mode, agent.ScratchFreeBytes = tt.mode, tt.scratch
			job := queuedJob(agentproto.CapEncHEVC10)
			job.SourceSize = source

			if offers := Plan([]QueuedJob{job}, fleetOf(nil, agent)); (len(offers) == 1) != tt.offered {
				t.Fatalf("offers = %+v, want offered %v", offers, tt.offered)
			}
		})
	}
}

func TestPlanGivesABusyDownloadFirstAgentOnePrefetch(t *testing.T) {
	agent := onlineAgent(device(testDevice, passed(agentproto.CapEncHEVC10)))
	agent.Mode = api.TransferDownloadFirst
	running := map[SlotKey]int{slotOn(agent): testDefault}
	next, after := queuedJob(agentproto.CapEncHEVC10), queuedJob(agentproto.CapEncHEVC10)

	offers := Plan([]QueuedJob{next, after}, fleetOf(running, agent))

	if want := []Offer{{Job: next.ID, Slot: slotOn(agent), Prefetch: true}}; !reflect.DeepEqual(offers, want) {
		t.Fatalf("offers = %+v, want one prefetch", offers)
	}

	fleet := fleetOf(running, agent)
	fleet.Prefetching = map[domain.AgentID]bool{agent.ID: true}
	if offers := Plan([]QueuedJob{after}, fleet); len(offers) != 0 {
		t.Fatalf("offers = %+v, want none: the agent already prefetches", offers)
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
