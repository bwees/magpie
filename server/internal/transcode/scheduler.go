package transcode

import (
	"cmp"
	"slices"

	"magpie/internal/domain"
	"magpie/internal/transcode/api"
	"magpie/internal/transcode/api/agentproto"
)

// SlotKey names one device on one agent.
type SlotKey struct {
	Agent  domain.AgentID
	Device string
}

// QueuedJob is what the scheduler needs to place a job.
type QueuedJob struct {
	ID domain.JobID
	// Needs is the capability the device must have passed.
	Needs          agentproto.Capability
	ExcludedAgents []domain.AgentID
	SourceSize     int64
	// LastAgent ran the previous attempt; other agents are preferred.
	LastAgent *domain.AgentID
	// Avoid may not get the job, unless it is ResumableOn.
	Avoid *domain.AgentID
	// ResumableOn still holds the job's scratch from before a restart.
	ResumableOn *domain.AgentID
	// Backlog jobs take only slots no other waiting job could use.
	Backlog bool
}

// Fleet is the agents the scheduler can offer to and the jobs they already run.
type Fleet struct {
	Agents []api.Agent
	// Running counts the jobs each device runs or was offered, prefetched jobs aside.
	Running map[SlotKey]int
	// Prefetching are the agents that already hold a prefetched job.
	Prefetching  map[domain.AgentID]bool
	DefaultSlots int
}

type Offer struct {
	Job  domain.JobID
	Slot SlotKey
	// Prefetch offers a busy agent its next job to download while the current one encodes.
	Prefetch bool
}

// Scratch an agent needs per byte of source: the source and the output in download-first mode, the
// output alone when ffmpeg reads the source over HTTP.
const (
	scratchFactorDownloadFirst = 2.2
	scratchFactorDirectRead    = 1.2
)

func scratchNeeded(mode api.TransferMode, sourceSize int64) int64 {
	factor := scratchFactorDownloadFirst
	if mode == api.TransferDirectRead {
		factor = scratchFactorDirectRead
	}
	return int64(factor * float64(sourceSize))
}

// SlotLimit is how many jobs a device may run at once: the agent's slots override or the default,
// capped by the device's session limit when it has one.
func SlotLimit(agent api.Agent, device api.Device, defaultSlots int) int {
	limit := defaultSlots
	if agent.SlotsOverride != nil {
		limit = *agent.SlotsOverride
	}
	if device.MaxSessions > 0 {
		limit = min(limit, device.MaxSessions)
	}
	return limit
}

// FreeSlots is scheduler step 1. It returns every healthy device of an online, non-draining agent that
// can take more jobs, with how many more it can take.
func FreeSlots(agents []api.Agent, running map[SlotKey]int, defaultSlots int) map[SlotKey]int {
	free := map[SlotKey]int{}
	for _, agent := range schedulable(agents) {
		for _, device := range agent.Devices {
			key := SlotKey{Agent: agent.ID, Device: device.ID}
			if left := SlotLimit(agent, device, defaultSlots) - running[key]; device.Healthy && left > 0 {
				free[key] = left
			}
		}
	}
	return free
}

func schedulable(agents []api.Agent) []api.Agent {
	return slices.DeleteFunc(slices.Clone(agents), func(a api.Agent) bool { return !a.Online || a.Draining })
}

// planner places queued jobs on a fleet, one offer at a time.
type planner struct {
	fleet    Fleet
	free     map[SlotKey]int
	load     map[SlotKey]int
	scratch  map[domain.AgentID]int64
	agents   map[domain.AgentID]api.Agent
	passedBy map[SlotKey]map[agentproto.Capability]bool
}

// Plan offers each job, in the order given, to the best free slot (scheduler step 3), then gives each
// busy download-first agent one job to prefetch. A backlog job never takes a slot, or a prefetch, that a
// waiting job ahead of the backlog could use, even one kept from that agent for a moment (step 4).
func Plan(jobs []QueuedJob, fleet Fleet) []Offer {
	p := planner{
		fleet:    fleet,
		free:     FreeSlots(fleet.Agents, fleet.Running, fleet.DefaultSlots),
		load:     map[SlotKey]int{},
		scratch:  map[domain.AgentID]int64{},
		agents:   map[domain.AgentID]api.Agent{},
		passedBy: passedCapabilities(fleet.Agents),
	}
	for _, agent := range schedulable(fleet.Agents) {
		p.agents[agent.ID] = agent
		p.scratch[agent.ID] = agent.ScratchFreeBytes
		for _, device := range agent.Devices {
			key := SlotKey{Agent: agent.ID, Device: device.ID}
			p.load[key] = fleet.Running[key]
		}
	}

	var offers []Offer
	var waiting, waitingAhead []QueuedJob
	for _, job := range jobs {
		slots := p.freeSlots()
		if job.Backlog {
			slots = p.spare(slots, waitingAhead)
		}
		slot, ok := p.best(job, slots)
		if !ok {
			waiting = append(waiting, job)
			if !job.Backlog {
				waitingAhead = append(waitingAhead, job)
			}
			continue
		}
		offers = append(offers, p.take(job, slot, false))
	}
	prefetching := map[domain.AgentID]bool{}
	for agent, holds := range fleet.Prefetching {
		prefetching[agent] = holds
	}
	for _, job := range waiting {
		slots := p.prefetchSlots(prefetching)
		if job.Backlog {
			slots = p.spare(slots, waitingAhead)
		}
		slot, ok := p.best(job, slots)
		if !ok {
			continue
		}
		prefetching[slot.Agent] = true
		offers = append(offers, p.take(job, slot, true))
	}
	return offers
}

func (p *planner) freeSlots() []SlotKey {
	var slots []SlotKey
	for key, left := range p.free {
		if left > 0 {
			slots = append(slots, key)
		}
	}
	return slots
}

// prefetchSlots are the full devices of download-first agents that hold no prefetched job yet.
func (p *planner) prefetchSlots(prefetching map[domain.AgentID]bool) []SlotKey {
	var slots []SlotKey
	for _, agent := range p.agents {
		if agent.Mode != api.TransferDownloadFirst || prefetching[agent.ID] {
			continue
		}
		for _, device := range agent.Devices {
			key := SlotKey{Agent: agent.ID, Device: device.ID}
			if device.Healthy && p.free[key] == 0 && p.load[key] > 0 {
				slots = append(slots, key)
			}
		}
	}
	return slots
}

// best picks among the slots that can take the job: a resumable match first, then an agent other than
// the last attempt's, then the fastest link, then the least busy device.
func (p *planner) best(job QueuedJob, slots []SlotKey) (SlotKey, bool) {
	candidates := slices.DeleteFunc(slots, func(key SlotKey) bool { return !p.fits(job, key) })
	if len(candidates) == 0 {
		return SlotKey{}, false
	}
	is := func(agent *domain.AgentID, key SlotKey) bool { return agent != nil && *agent == key.Agent }
	rank := func(match bool) int {
		if match {
			return 0
		}
		return 1
	}
	return slices.MinFunc(candidates, func(a, b SlotKey) int {
		return cmp.Or(
			cmp.Compare(rank(is(job.ResumableOn, a)), rank(is(job.ResumableOn, b))),
			cmp.Compare(rank(!is(job.LastAgent, a)), rank(!is(job.LastAgent, b))),
			cmp.Compare(p.agents[b.Agent].ThroughputBps, p.agents[a.Agent].ThroughputBps),
			cmp.Compare(p.load[a], p.load[b]),
			cmp.Compare(a.Agent, b.Agent),
			cmp.Compare(a.Device, b.Device),
		)
	}), true
}

func (p *planner) fits(job QueuedJob, key SlotKey) bool {
	resumable := job.ResumableOn != nil && *job.ResumableOn == key.Agent
	avoided := job.Avoid != nil && *job.Avoid == key.Agent && !resumable
	return p.couldUse(job, key) && !avoided
}

// couldUse is fits without the short avoid window.
func (p *planner) couldUse(job QueuedJob, key SlotKey) bool {
	agent := p.agents[key.Agent]
	return p.passedBy[key][job.Needs] &&
		!slices.Contains(job.ExcludedAgents, key.Agent) &&
		p.scratch[key.Agent] >= scratchNeeded(agent.Mode, job.SourceSize)
}

// spare drops the slots a waiting job ahead of the backlog could use.
func (p *planner) spare(slots []SlotKey, ahead []QueuedJob) []SlotKey {
	return slices.DeleteFunc(slots, func(key SlotKey) bool {
		return slices.ContainsFunc(ahead, func(job QueuedJob) bool { return p.couldUse(job, key) })
	})
}

func (p *planner) take(job QueuedJob, slot SlotKey, prefetch bool) Offer {
	p.scratch[slot.Agent] -= scratchNeeded(p.agents[slot.Agent].Mode, job.SourceSize)
	if !prefetch {
		p.free[slot]--
		p.load[slot]++
	}
	return Offer{Job: job.ID, Slot: slot, Prefetch: prefetch}
}

func passedCapabilities(agents []api.Agent) map[SlotKey]map[agentproto.Capability]bool {
	passed := map[SlotKey]map[agentproto.Capability]bool{}
	for _, agent := range agents {
		for _, device := range agent.Devices {
			set := map[agentproto.Capability]bool{}
			for _, c := range device.Capabilities {
				if c.OK {
					set[agentproto.Capability(c.Name)] = true
				}
			}
			passed[SlotKey{Agent: agent.ID, Device: device.ID}] = set
		}
	}
	return passed
}
