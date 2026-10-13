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
	closeReasonStopping  = "server stopping"

	TypeServerStopping problem.Type = "urn:magpie:problem:server-stopping"
)

var (
	errSecretNotConfigured = problem.New(problem.TypeUnauthorized, http.StatusUnauthorized, "agents are refused until MAGPIE_AGENT_SECRET is set on the server")
	errWrongSecret         = problem.New(problem.TypeUnauthorized, http.StatusUnauthorized, "wrong agent secret")
	errBadAgentID          = problem.New(problem.TypeBadRequest, http.StatusBadRequest, agentproto.HeaderAgentID+" must be a ULID")
	errServerStopping      = problem.New(TypeServerStopping, http.StatusServiceUnavailable, "the server is stopping; reconnect later")
)

// AgentSocket serves the control WebSocket agents dial.
type AgentSocket struct {
	service  *AgentService
	jobs     *JobService
	hub      *AgentHub
	wake     *Wake
	lifetime *Lifetime
	secret   AgentSecret
	timings  Timings
}

func NewAgentSocket(service *AgentService, jobs *JobService, hub *AgentHub, wake *Wake, lifetime *Lifetime, secret AgentSecret, timings Timings) *AgentSocket {
	return &AgentSocket{service: service, jobs: jobs, hub: hub, wake: wake, lifetime: lifetime, secret: secret, timings: timings}
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
	if !s.lifetime.Serve(r.Context(), func(ctx context.Context) { s.serve(ctx, w, r, id) }) {
		problem.Write(w, r, errServerStopping)
	}
}

// serve runs one agent's socket until the agent leaves or ctx ends. Frames are read on a context that
// outlives ctx, because coder/websocket drops the connection without a close frame when a read's context
// ends; closeOnStop sends the close instead.
func (s *AgentSocket) serve(ctx context.Context, w http.ResponseWriter, r *http.Request, id domain.AgentID) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		slog.WarnContext(ctx, "transcode: agent upgrade failed", "agent", id, "error", err)
		return
	}
	defer func() { _ = conn.CloseNow() }()
	defer closeOnStop(ctx, conn)()

	hello, err := s.readHello(ctx, conn, id)
	if ctx.Err() != nil {
		return
	}
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

	env, err := newEnvelope(agentproto.TypeWelcome, welcome)
	if err != nil {
		slog.ErrorContext(ctx, "transcode: greeting agent", "agent", id, "error", err)
		_ = conn.Close(websocket.StatusInternalError, closeReasonServerErr)
		return
	}
	channel := newControlChannel(conn)
	defer s.disconnect(ctx, id, channel)
	if s.hub.join(ctx, hello, channel, env) != nil {
		return
	}
	s.wake.Kick()
	slog.InfoContext(ctx, "transcode: agent connected", "agent", id, "hostname", hello.Hostname,
		"devices", len(hello.Devices), "resumable", len(hello.ResumableJobs))

	s.readLoop(ctx, id, channel)
}

func (s *AgentSocket) readHello(ctx context.Context, conn *websocket.Conn, id domain.AgentID) (agentproto.Hello, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.timings.HelloTimeout)
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

// disconnect forgets the socket and takes the agent offline, unless a newer socket replaced it. A stopping
// server leaves its agents online: they reconnect to the next one, and the sweep catches any that don't.
func (s *AgentSocket) disconnect(ctx context.Context, id domain.AgentID, channel *controlChannel) {
	if !s.hub.detach(id, channel) || ctx.Err() != nil {
		return
	}
	if err := s.service.Disconnected(ctx, id); err != nil && ctx.Err() == nil {
		slog.ErrorContext(ctx, "transcode: taking a disconnected agent offline", "agent", id, "error", err)
	}
}

// readLoop drops a socket that sends no frame for the offline window: a peer that vanished without a
// close frame otherwise holds it open until TCP gives up, many minutes later.
func (s *AgentSocket) readLoop(ctx context.Context, id domain.AgentID, channel *controlChannel) {
	frames := context.WithoutCancel(ctx)
	for {
		var env agentproto.Envelope
		if err := s.readFrame(frames, channel.conn, &env); err != nil {
			slog.DebugContext(ctx, "transcode: agent socket closed", "agent", id, "error", err)
			return
		}
		if ctx.Err() != nil {
			return
		}
		err := s.handle(ctx, id, env)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
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

func (s *AgentSocket) readFrame(ctx context.Context, conn *websocket.Conn, env *agentproto.Envelope) error {
	ctx, cancel := context.WithTimeout(ctx, offlineAfter(s.timings))
	defer cancel()
	return wsjson.Read(ctx, conn, env)
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
	conn    *websocket.Conn
	greeted chan struct{}
}

func newControlChannel(conn *websocket.Conn) *controlChannel {
	return &controlChannel{conn: conn, greeted: make(chan struct{})}
}

func (c *controlChannel) greet(ctx context.Context, welcome agentproto.Envelope) error {
	defer close(c.greeted)
	return c.write(ctx, welcome)
}

func (c *controlChannel) send(ctx context.Context, env agentproto.Envelope) error {
	select {
	case <-c.greeted:
	case <-ctx.Done():
		return ctx.Err()
	}
	return c.write(ctx, env)
}

// write ignores ctx's cancellation: the websocket library drops the whole connection when a write's
// context ends, even after the frame went out. closeOnStop ends the socket cleanly instead.
func (c *controlChannel) write(ctx context.Context, env agentproto.Envelope) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), frameWriteTimeout)
	defer cancel()
	return wsjson.Write(ctx, c.conn, env)
}

// closeOnStop closes the socket with a normal close frame once ctx ends. The returned func stops that,
// or waits for a close already under way.
func closeOnStop(ctx context.Context, conn *websocket.Conn) func() {
	closed := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		defer close(closed)
		_ = conn.Close(websocket.StatusNormalClosure, closeReasonStopping)
	})
	return func() {
		if !stop() {
			<-closed
		}
	}
}

// replaced closes in the background: the close handshake waits on a peer that may already be gone.
func (c *controlChannel) replaced() {
	go func() { _ = c.conn.Close(websocket.StatusPolicyViolation, closeReasonReplaced) }()
}
