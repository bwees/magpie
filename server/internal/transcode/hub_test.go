package transcode

import (
	"context"
	"testing"

	"magpie/internal/domain"
	"magpie/internal/transcode/api/agentproto"
)

// welcomeWitness records whether the hub knew the agent when its Welcome went out.
type welcomeWitness struct {
	hub               *AgentHub
	hello             agentproto.Hello
	attachedAtWelcome bool
}

func (w *welcomeWitness) greet(context.Context, agentproto.Envelope) error {
	w.attachedAtWelcome = w.hub.attachedWith(w.hello)
	return nil
}

func (w *welcomeWitness) send(context.Context, agentproto.Envelope) error { return nil }

func (w *welcomeWitness) replaced() {}

func TestAgentIsAttachedBeforeItsWelcome(t *testing.T) {
	hub := NewAgentHub()
	hello := capableHello("gpu-a")
	hello.ResumableJobs = []agentproto.ResumableJob{{Job: domain.NewID[domain.JobID](), SourceETag: "kept-scratch"}}
	witness := &welcomeWitness{hub: hub, hello: hello}

	if err := hub.join(context.Background(), hello, witness, agentproto.Envelope{Type: agentproto.TypeWelcome}); err != nil {
		t.Fatal(err)
	}

	if !witness.attachedAtWelcome {
		t.Fatal("the Welcome went out before the hub knew the agent and its resumable jobs")
	}
}
