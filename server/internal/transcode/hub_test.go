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

func TestDetachingAReplacedChannelKeepsTheAgentAttached(t *testing.T) {
	hub := NewAgentHub()
	id := domain.NewID[domain.AgentID]()
	old, current := &welcomeWitness{hub: hub}, &welcomeWitness{hub: hub}
	hub.attach(id, old)
	hub.attach(id, current)

	if hub.detach(id, old) {
		t.Fatal("detaching the replaced channel reported the agent gone")
	}
	if !hub.connected(id) {
		t.Fatal("the agent lost its newer channel")
	}
	if !hub.detach(id, current) || hub.connected(id) {
		t.Fatal("detaching the current channel kept the agent attached")
	}
}

func TestReconnectingClearsBusyDevices(t *testing.T) {
	hub := NewAgentHub()
	id := domain.NewID[domain.AgentID]()
	hub.markBusy(SlotKey{Agent: id, Device: testDevice})

	hub.attach(id, &welcomeWitness{hub: hub})

	if busy := hub.Busy(); len(busy) != 0 {
		t.Fatalf("busy = %v, want none after a new Hello", busy)
	}
}

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
