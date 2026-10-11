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

// QueuedJob is what the scheduler needs to place a job: the capability its device must have passed
// and the agents it may not go to.
type QueuedJob struct {
	ID             domain.JobID
	Needs          agentproto.Capability
	ExcludedAgents []domain.AgentID
}

type Offer struct {
	Job  domain.JobID
	Slot SlotKey
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
	for _, agent := range agents {
		if !agent.Online || agent.Draining {
			continue
		}
		for _, device := range agent.Devices {
			if !device.Healthy {
				continue
			}
			key := SlotKey{Agent: agent.ID, Device: device.ID}
			if left := SlotLimit(agent, device, defaultSlots) - running[key]; left > 0 {
				free[key] = left
			}
		}
	}
	return free
}

// Plan offers each job, in the order given, to a free slot whose device passed the capability the job
// needs. Among those it picks the device running the fewest jobs.
func Plan(jobs []QueuedJob, agents []api.Agent, running map[SlotKey]int, defaultSlots int) []Offer {
	free := FreeSlots(agents, running, defaultSlots)
	load := map[SlotKey]int{}
	for key := range free {
		load[key] = running[key]
	}
	passedBy := passedCapabilities(agents)

	var offers []Offer
	for _, job := range jobs {
		var candidates []SlotKey
		for key := range free {
			if passedBy[key][job.Needs] && !slices.Contains(job.ExcludedAgents, key.Agent) {
				candidates = append(candidates, key)
			}
		}
		if len(candidates) == 0 {
			continue
		}
		best := slices.MinFunc(candidates, func(a, b SlotKey) int {
			return cmp.Or(cmp.Compare(load[a], load[b]), cmp.Compare(a.Agent, b.Agent), cmp.Compare(a.Device, b.Device))
		})
		offers = append(offers, Offer{Job: job.ID, Slot: best})
		load[best]++
		if free[best]--; free[best] == 0 {
			delete(free, best)
		}
	}
	return offers
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
