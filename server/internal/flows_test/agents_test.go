package flows_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"magpie/internal/domain"
	"magpie/internal/transcode"
	"magpie/internal/transcode/api/agentproto"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"go.uber.org/fx"
)

const (
	flowAgentSecret = "flow-agent-secret-0123456789abcdef-0123"
	agentsPath      = "/api/transcode/agents"
)

func withAgentSecret() fx.Option {
	return fx.Decorate(func(transcode.AgentSecret) transcode.AgentSecret { return flowAgentSecret })
}

// The agent socket checks the shared secret itself, so the session gate lets it through even before
// setup, while the admin agent endpoints still need a session.
func TestAgentRegistersWithoutASessionAndAdminsSeeIt(t *testing.T) {
	h := boot(t, withAgentSecret())
	server := httptest.NewServer(h.server.Mux)
	t.Cleanup(server.Close)
	id := domain.NewID[domain.AgentID]()

	header := http.Header{}
	header.Set(agentproto.HeaderAuthorization, agentproto.BearerPrefix+flowAgentSecret)
	header.Set(agentproto.HeaderAgentID, string(id))
	ctx := context.Background()
	conn, _, err := websocket.Dial(ctx, server.URL+agentproto.ControlPath, &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatalf("agent dial before setup: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })

	hello, err := json.Marshal(agentproto.Hello{AgentID: id, Hostname: "flow-agent", Mode: agentproto.ModeDownloadFirst})
	if err != nil {
		t.Fatal(err)
	}
	if err := wsjson.Write(ctx, conn, agentproto.Envelope{Type: agentproto.TypeHello, ID: string(id), Body: hello}); err != nil {
		t.Fatal(err)
	}
	var welcome agentproto.Envelope
	if err := wsjson.Read(ctx, conn, &welcome); err != nil || welcome.Type != agentproto.TypeWelcome {
		t.Fatalf("got %+v, %v; want WELCOME", welcome, err)
	}

	h.signInAsAdmin()
	rec := h.get(agentsPath)
	var agents []transcode.AgentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &agents); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("list agents = %d: %s", rec.Code, rec.Body)
	}
	if len(agents) != 1 || agents[0].ID != string(id) || !agents[0].Online {
		t.Fatalf("agents = %+v, want the connected agent online", agents)
	}
}

func TestAgentEndpointsNextToTheSocketStillNeedASession(t *testing.T) {
	h := boot(t, withAgentSecret())
	h.signInAsAdmin()
	h.session = nil

	if rec := h.get(agentsPath); rec.Code != http.StatusUnauthorized {
		t.Errorf("GET %s without a session = %d, want 401", agentsPath, rec.Code)
	}
	// An agent ID that starts like the socket path must not slip past the gate.
	lookalike := agentproto.ControlPath + "x"
	if rec := h.do(http.MethodPatch, lookalike, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("PATCH %s without a session = %d, want 401", lookalike, rec.Code)
	}
}
