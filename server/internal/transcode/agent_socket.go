package transcode

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"magpie/internal/domain"
	"magpie/internal/platform/problem"
	"magpie/internal/transcode/api/agentproto"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const (
	frameWriteTimeout = 10 * time.Second

	closeReasonNoHello   = "first frame must be a valid HELLO for the connecting agent"
	closeReasonReplaced  = "replaced by a newer connection"
	closeReasonServerErr = "server error"
)

var (
	errSecretNotConfigured = problem.New(problem.TypeUnauthorized, http.StatusUnauthorized, "agents are refused until MAGPIE_AGENT_SECRET is set on the server")
	errWrongSecret         = problem.New(problem.TypeUnauthorized, http.StatusUnauthorized, "wrong agent secret")
	errBadAgentID          = problem.New(problem.TypeBadRequest, http.StatusBadRequest, agentproto.HeaderAgentID+" must be a ULID")
)

// AgentSocket serves the control WebSocket agents dial.
type AgentSocket struct {
	service *AgentService
	jobs    *JobService
	hub     *AgentHub
	wake    *Wake
	secret  AgentSecret
	timings Timings
}

func NewAgentSocket(service *AgentService, jobs *JobService, hub *AgentHub, wake *Wake, secret AgentSecret, timings Timings) *AgentSocket {
	return &AgentSocket{service: service, jobs: jobs, hub: hub, wake: wake, secret: secret, timings: timings}
}

// authenticateAgent checks the shared secret and agent ID headers every agent request carries.
func authenticateAgent(r *http.Request, secret AgentSecret) (domain.AgentID, error) {
	if secret == "" {
		return "", errSecretNotConfigured
	}
	token, ok := strings.CutPrefix(r.Header.Get(agentproto.HeaderAuthorization), agentproto.BearerPrefix)
	if !ok || subtle.ConstantTimeCompare([]byte(token), []byte(secret)) != 1 {
		return "", errWrongSecret
	}
	id := domain.AgentID(r.Header.Get(agentproto.HeaderAgentID))
	if !domain.ValidID(id) {
		return "", errBadAgentID
	}
	return id, nil
}

func (s *AgentSocket) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, err := authenticateAgent(r, s.secret)
	if err != nil {
		problem.Write(w, r, err)
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		slog.WarnContext(r.Context(), "transcode: agent upgrade failed", "agent", id, "error", err)
		return
	}
	defer func() { _ = conn.CloseNow() }()
	ctx := r.Context()

	hello, err := s.readHello(ctx, conn, id)
	if err != nil {
		slog.WarnContext(ctx, "transcode: rejecting agent", "agent", id, "error", err)
		_ = conn.Close(websocket.StatusPolicyViolation, closeReasonNoHello)
		return
	}
	welcome, err := s.service.Connect(ctx, hello)
	if err != nil {
		slog.ErrorContext(ctx, "transcode: registering agent", "agent", id, "error", err)
		_ = conn.Close(websocket.StatusInternalError, closeReasonServerErr)
		return
	}

	channel := &controlChannel{conn: conn}
	env, err := newEnvelope(agentproto.TypeWelcome, welcome)
	if err != nil || channel.send(ctx, env) != nil {
		return
	}
	s.hub.attach(id, channel)
	defer s.hub.detach(id, channel)
	s.hub.SetResumable(id, hello.ResumableJobs)
	s.wake.Kick()
	slog.InfoContext(ctx, "transcode: agent connected", "agent", id, "hostname", hello.Hostname,
		"devices", len(hello.Devices), "resumable", len(hello.ResumableJobs))

	s.readLoop(ctx, id, channel)
}

func (s *AgentSocket) readHello(ctx context.Context, conn *websocket.Conn, id domain.AgentID) (agentproto.Hello, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timings.HelloTimeout)
	defer cancel()
	var env agentproto.Envelope
	if err := wsjson.Read(ctx, conn, &env); err != nil {
		return agentproto.Hello{}, err
	}
	if env.Type != agentproto.TypeHello {
		return agentproto.Hello{}, fmt.Errorf("first frame is %s", env.Type)
	}
	var hello agentproto.Hello
	if err := json.Unmarshal(env.Body, &hello); err != nil {
		return agentproto.Hello{}, err
	}
	if hello.AgentID != id {
		return agentproto.Hello{}, fmt.Errorf("hello names agent %s", hello.AgentID)
	}
	return hello, nil
}

func (s *AgentSocket) readLoop(ctx context.Context, id domain.AgentID, channel *controlChannel) {
	for {
		var env agentproto.Envelope
		if err := wsjson.Read(ctx, channel.conn, &env); err != nil {
			slog.DebugContext(ctx, "transcode: agent socket closed", "agent", id, "error", err)
			return
		}
		if err := s.handle(ctx, id, env); err != nil {
			slog.ErrorContext(ctx, "transcode: handling agent frame", "agent", id, "type", env.Type, "error", err)
			continue
		}
		if acknowledged[env.Type] {
			ack, err := newEnvelope(agentproto.TypeAck, agentproto.Ack{Of: env.ID})
			if err != nil || channel.send(ctx, ack) != nil {
				return
			}
		}
	}
}

// acknowledged are the frames that change server state without a reply of their own.
var acknowledged = map[agentproto.MessageType]bool{
	agentproto.TypeCapabilitiesChanged: true,
	agentproto.TypeDeviceUnhealthy:     true,
}

func (s *AgentSocket) handle(ctx context.Context, id domain.AgentID, env agentproto.Envelope) error {
	switch env.Type {
	case agentproto.TypeHeartbeat:
		return handleBody(env, func(body agentproto.Heartbeat) error {
			return errors.Join(s.service.Heartbeat(ctx, id, body), s.jobs.Heartbeat(ctx, id, body))
		})
	case agentproto.TypeCapabilitiesChanged:
		defer s.wake.Kick()
		return handleBody(env, func(body agentproto.CapabilitiesChanged) error { return s.service.CapabilitiesChanged(ctx, id, body) })
	case agentproto.TypeDeviceUnhealthy:
		return handleBody(env, func(body agentproto.DeviceUnhealthy) error { return s.service.DeviceUnhealthy(ctx, id, body) })
	case agentproto.TypeJobAccept:
		return handleBody(env, func(body agentproto.JobAccept) error { return s.jobs.Accept(ctx, id, body) })
	case agentproto.TypeJobReject:
		return handleBody(env, func(body agentproto.JobReject) error { return s.jobs.Reject(ctx, id, body) })
	case agentproto.TypeJobResult:
		return handleBody(env, func(body agentproto.JobResult) error { return s.jobs.Result(ctx, id, body) })
	default:
		slog.DebugContext(ctx, "transcode: ignoring agent frame", "agent", id, "type", env.Type)
		return nil
	}
}

func handleBody[T any](env agentproto.Envelope, fn func(T) error) error {
	var body T
	if err := json.Unmarshal(env.Body, &body); err != nil {
		return fmt.Errorf("decoding %s: %w", env.Type, err)
	}
	return fn(body)
}

// controlChannel is one agent's WebSocket.
type controlChannel struct {
	conn *websocket.Conn
}

func (c *controlChannel) send(ctx context.Context, env agentproto.Envelope) error {
	ctx, cancel := context.WithTimeout(ctx, frameWriteTimeout)
	defer cancel()
	return wsjson.Write(ctx, c.conn, env)
}

// replaced closes in the background: the close handshake waits on a peer that may already be gone.
func (c *controlChannel) replaced() {
	go func() { _ = c.conn.Close(websocket.StatusPolicyViolation, closeReasonReplaced) }()
}
