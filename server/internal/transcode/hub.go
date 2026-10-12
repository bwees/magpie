package transcode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"magpie/internal/domain"
	"magpie/internal/transcode/api/agentproto"
)

var ErrAgentNotConnected = errors.New("transcode: agent is not connected")

type envelopeID string

// frameSender is one agent's open control channel.
type frameSender interface {
	send(ctx context.Context, env agentproto.Envelope) error
	// replaced closes a channel that a newer connection from the same agent took over.
	replaced()
}

// AgentHub tracks the connected agents so the server can push frames to them, and the jobs whose scratch
// each one kept across a restart.
type AgentHub struct {
	mu        sync.Mutex
	conns     map[domain.AgentID]frameSender
	resumable map[domain.AgentID][]agentproto.ResumableJob
}

func NewAgentHub() *AgentHub {
	return &AgentHub{conns: map[domain.AgentID]frameSender{}, resumable: map[domain.AgentID][]agentproto.ResumableJob{}}
}

// SetResumable records the jobs an agent kept scratch for, from its latest Hello.
func (h *AgentHub) SetResumable(id domain.AgentID, jobs []agentproto.ResumableJob) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.resumable[id] = jobs
}

// resumableHold is an agent that can resume a job from its scratch, if the source still has etag.
type resumableHold struct {
	agent domain.AgentID
	etag  string
}

// Resumable lists the jobs connected agents can resume.
func (h *AgentHub) Resumable() map[domain.JobID]resumableHold {
	h.mu.Lock()
	defer h.mu.Unlock()
	holds := map[domain.JobID]resumableHold{}
	for agent, jobs := range h.resumable {
		if _, connected := h.conns[agent]; !connected {
			continue
		}
		for _, job := range jobs {
			holds[job.Job] = resumableHold{agent: agent, etag: job.SourceETag}
		}
	}
	return holds
}

func (h *AgentHub) attach(id domain.AgentID, conn frameSender) {
	h.mu.Lock()
	previous := h.conns[id]
	h.conns[id] = conn
	h.mu.Unlock()
	if previous != nil {
		previous.replaced()
	}
}

// detach forgets conn unless a newer connection already replaced it.
func (h *AgentHub) detach(id domain.AgentID, conn frameSender) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conns[id] == conn {
		delete(h.conns, id)
	}
}

// Connected lists the agents with an open control channel.
func (h *AgentHub) Connected() []domain.AgentID {
	h.mu.Lock()
	defer h.mu.Unlock()
	ids := make([]domain.AgentID, 0, len(h.conns))
	for id := range h.conns {
		ids = append(ids, id)
	}
	return ids
}

// Send writes one frame to the agent, or returns ErrAgentNotConnected.
func (h *AgentHub) Send(ctx context.Context, id domain.AgentID, messageType agentproto.MessageType, body any) error {
	h.mu.Lock()
	conn, ok := h.conns[id]
	h.mu.Unlock()
	if !ok {
		return ErrAgentNotConnected
	}
	env, err := newEnvelope(messageType, body)
	if err != nil {
		return err
	}
	return conn.send(ctx, env)
}

func newEnvelope(messageType agentproto.MessageType, body any) (agentproto.Envelope, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return agentproto.Envelope{}, fmt.Errorf("encoding %s: %w", messageType, err)
	}
	return agentproto.Envelope{Type: messageType, ID: string(domain.NewID[envelopeID]()), Body: raw}, nil
}
