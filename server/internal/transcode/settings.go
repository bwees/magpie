package transcode

import (
	"strings"
	"time"

	"magpie/internal/platform/config"
	"magpie/internal/platform/owner"
	"magpie/internal/platform/settings"
)

const (
	defaultTransferCap = 2
	MinTransferCap     = 1
	MaxTransferCap     = 16

	defaultSlotsPerGPU = 1
	MinSlotsPerGPU     = 1
	MaxSlotsPerGPU     = 4

	// A per-agent override of 0 drains the agent through its slots.
	MinSlotsOverride = 0
	MaxSlotsOverride = 8
)

// AgentSettings are the server-wide agent settings an admin edits under Settings → Agents.
type AgentSettings struct {
	TransferCap        int `json:"transferCap"`
	DefaultSlotsPerGPU int `json:"defaultSlotsPerGpu"`
}

var agentSettingsKey = settings.NewKey(owner.Transcode, "agents", AgentSettings{
	TransferCap:        defaultTransferCap,
	DefaultSlotsPerGPU: defaultSlotsPerGPU,
})

// Timings are the fixed protocol intervals from the spec. Tests move a clock instead of shortening them,
// except for the socket's offline window, which runs on the wall clock.
type Timings struct {
	HeartbeatEvery time.Duration
	LeaseTTL       time.Duration
	// MissedHeartbeats in a row take an agent offline.
	MissedHeartbeats int
	// RetestDelay is how long an unhealthy device waits before the server asks for a retest.
	RetestDelay time.Duration
	// HelloTimeout is how long a new socket has to send its Hello.
	HelloTimeout time.Duration
	// SweepEvery is how often the server looks for silent agents and due retests, and schedules jobs.
	SweepEvery time.Duration
	// OfferTimeout returns an offer nobody accepted to the queue.
	OfferTimeout time.Duration
	// AvoidAgent keeps a job away from an agent that lost its lease or rejected it.
	AvoidAgent time.Duration
}

var DefaultTimings = Timings{
	HeartbeatEvery:   10 * time.Second,
	LeaseTTL:         60 * time.Second,
	MissedHeartbeats: 3,
	RetestDelay:      10 * time.Minute,
	HelloTimeout:     10 * time.Second,
	SweepEvery:       5 * time.Second,
	OfferTimeout:     10 * time.Second,
	AvoidAgent:       2 * time.Minute,
}

func offlineAfter(t Timings) time.Duration {
	return t.HeartbeatEvery * time.Duration(t.MissedHeartbeats)
}

// AgentSecret is the shared secret from MAGPIE_AGENT_SECRET. Empty refuses every agent.
type AgentSecret string

func newAgentSecret(env config.Env) AgentSecret {
	return AgentSecret(env.AgentSecret)
}

const (
	maskRune   = "•"
	maskLength = 12
	// Only secrets longer than twice this show their tail, so a short one is never half revealed.
	maskVisibleTail = 4
)

// Masked hides all but the last few characters, for display next to the Copy button.
func (s AgentSecret) Masked() string {
	if s == "" {
		return ""
	}
	mask := strings.Repeat(maskRune, maskLength)
	runes := []rune(string(s))
	if len(runes) <= 2*maskVisibleTail {
		return mask
	}
	return mask + string(runes[len(runes)-maskVisibleTail:])
}
