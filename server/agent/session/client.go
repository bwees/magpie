// Package session holds the agent's control WebSocket to the server: Hello, heartbeats, retests and
// overrides, reconnecting with backoff whenever the connection drops.
package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"sync"
	"time"

	"magpie/internal/domain"
	"magpie/internal/transcode/api/agentproto"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const (
	InitialBackoff = time.Second
	MaxBackoff     = time.Minute
	backoffFactor  = 2
	// Each wait is spread over ±jitterFraction so restarted agents don't reconnect in lockstep.
	jitterFraction = 0.2

	// DefaultHeartbeatEvery applies until a Welcome names the interval.
	DefaultHeartbeatEvery = 10 * time.Second

	welcomeTimeout    = 10 * time.Second
	frameWriteTimeout = 10 * time.Second
)

var ErrNoWelcome = errors.New("session: server did not answer Hello with Welcome")

// Prober runs the capability tests. An empty deviceID tests every device.
type Prober interface {
	FFmpegVersion(ctx context.Context) string
	Probe(ctx context.Context, deviceID string) []agentproto.DeviceReport
}

type Options struct {
	ServerURL *url.URL
	Secret    string
	AgentID   domain.AgentID
	Hostname  string
	Version   string
	Mode      agentproto.Mode
	Prober    Prober
	// ScratchFree reports the free bytes in the scratch dir.
	ScratchFree func() (int64, error)
}

// Client keeps one control channel open to the server.
type Client struct {
	opts   Options
	http   *http.Client
	random func() float64

	mu            sync.Mutex
	ffmpegVersion string
	devices       []agentproto.DeviceReport
	welcome       agentproto.Welcome
	draining      bool

	// retestMu serialises retests so two never fight over the GPU.
	retestMu sync.Mutex
}

func New(opts Options) *Client {
	return &Client{opts: opts, http: NewHTTPClient(opts.Secret, opts.AgentID), random: rand.Float64}
}

// NewHTTPClient adds the agent's auth headers to every request it sends.
func NewHTTPClient(secret string, agent domain.AgentID) *http.Client {
	return &http.Client{Transport: authTransport{base: http.DefaultTransport, secret: secret, agent: agent}}
}

type authTransport struct {
	base   http.RoundTripper
	secret string
	agent  domain.AgentID
}

func (t authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set(agentproto.HeaderAuthorization, agentproto.BearerPrefix+t.secret)
	req.Header.Set(agentproto.HeaderAgentID, string(t.agent))
	return t.base.RoundTrip(req)
}

// Run tests the GPUs once, then connects and reconnects until ctx ends.
func (c *Client) Run(ctx context.Context) error {
	c.retest(ctx, "")
	delay := InitialBackoff
	for {
		welcomed, err := c.session(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if welcomed {
			delay = InitialBackoff
		}
		wait := c.jitter(delay)
		slog.WarnContext(ctx, "agent: disconnected from server", "error", err, "retry_in", wait)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(wait):
		}
		delay = min(delay*backoffFactor, MaxBackoff)
	}
}

func (c *Client) jitter(d time.Duration) time.Duration {
	spread := (c.random()*2 - 1) * jitterFraction
	return time.Duration(float64(d) * (1 + spread))
}

// session runs one connection. welcomed reports whether the server accepted the agent.
func (c *Client) session(ctx context.Context) (welcomed bool, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	controlURL := c.opts.ServerURL.JoinPath(agentproto.ControlPath).String()
	conn, resp, err := websocket.Dial(ctx, controlURL, &websocket.DialOptions{HTTPClient: c.http})
	if err != nil {
		if resp != nil {
			return false, fmt.Errorf("dialling %s: %s: %w", controlURL, resp.Status, err)
		}
		return false, fmt.Errorf("dialling %s: %w", controlURL, err)
	}
	defer func() { _ = conn.CloseNow() }()

	if err := c.send(ctx, conn, agentproto.TypeHello, c.hello()); err != nil {
		return false, err
	}
	welcome, err := c.readWelcome(ctx, conn)
	if err != nil {
		return false, err
	}
	c.applyWelcome(welcome)
	slog.InfoContext(ctx, "agent: connected", "server", c.opts.ServerURL.String(), "mode", welcome.Mode, "slots", welcome.Slots)

	go c.heartbeats(ctx, conn)
	return true, c.readLoop(ctx, conn)
}

func (c *Client) hello() agentproto.Hello {
	c.mu.Lock()
	defer c.mu.Unlock()
	return agentproto.Hello{
		AgentID:       c.opts.AgentID,
		Hostname:      c.opts.Hostname,
		Version:       c.opts.Version,
		FFmpegVersion: c.ffmpegVersion,
		Mode:          c.opts.Mode,
		ScratchFree:   c.scratchFree(),
		Devices:       c.devices,
	}
}

func (c *Client) readWelcome(ctx context.Context, conn *websocket.Conn) (agentproto.Welcome, error) {
	ctx, cancel := context.WithTimeout(ctx, welcomeTimeout)
	defer cancel()
	var env agentproto.Envelope
	if err := wsjson.Read(ctx, conn, &env); err != nil {
		return agentproto.Welcome{}, fmt.Errorf("%w: %w", ErrNoWelcome, err)
	}
	if env.Type != agentproto.TypeWelcome {
		return agentproto.Welcome{}, fmt.Errorf("%w: got %s", ErrNoWelcome, env.Type)
	}
	var welcome agentproto.Welcome
	return welcome, json.Unmarshal(env.Body, &welcome)
}

func (c *Client) applyWelcome(welcome agentproto.Welcome) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.welcome = welcome
	c.draining = welcome.Draining
}

func (c *Client) heartbeats(ctx context.Context, conn *websocket.Conn) {
	c.mu.Lock()
	every := c.welcome.HeartbeatEvery
	c.mu.Unlock()
	if every <= 0 {
		every = DefaultHeartbeatEvery
	}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			heartbeat := agentproto.Heartbeat{ScratchFree: c.scratchFree(), Jobs: []agentproto.JobHeartbeat{}}
			if err := c.send(ctx, conn, agentproto.TypeHeartbeat, heartbeat); err != nil {
				return
			}
		}
	}
}

func (c *Client) readLoop(ctx context.Context, conn *websocket.Conn) error {
	for {
		var env agentproto.Envelope
		if err := wsjson.Read(ctx, conn, &env); err != nil {
			return err
		}
		if err := c.handle(ctx, conn, env); err != nil {
			slog.ErrorContext(ctx, "agent: handling server frame", "type", env.Type, "error", err)
		}
	}
}

func (c *Client) handle(ctx context.Context, conn *websocket.Conn, env agentproto.Envelope) error {
	switch env.Type {
	case agentproto.TypeWelcome:
		var welcome agentproto.Welcome
		if err := json.Unmarshal(env.Body, &welcome); err != nil {
			return err
		}
		c.applyWelcome(welcome)
	case agentproto.TypeRetest:
		var retest agentproto.Retest
		if err := json.Unmarshal(env.Body, &retest); err != nil {
			return err
		}
		go func() {
			devices := c.retest(ctx, retest.DeviceID)
			err := c.send(ctx, conn, agentproto.TypeCapabilitiesChanged, agentproto.CapabilitiesChanged{Devices: devices})
			if err != nil {
				slog.WarnContext(ctx, "agent: sending retest results", "error", err)
			}
		}()
	case agentproto.TypeDrain:
		c.mu.Lock()
		c.draining = true
		c.mu.Unlock()
		slog.InfoContext(ctx, "agent: draining; accepting no new jobs")
	case agentproto.TypeAck:
	default:
		slog.DebugContext(ctx, "agent: ignoring server frame", "type", env.Type)
	}
	return nil
}

// retest probes deviceID (every device when empty), keeps the results for the next Hello and returns them.
func (c *Client) retest(ctx context.Context, deviceID string) []agentproto.DeviceReport {
	c.retestMu.Lock()
	defer c.retestMu.Unlock()
	version := c.opts.Prober.FFmpegVersion(ctx)
	devices := c.opts.Prober.Probe(ctx, deviceID)
	for _, device := range devices {
		var passed []agentproto.Capability
		for _, result := range device.Capabilities {
			if result.OK {
				passed = append(passed, result.Capability)
			}
		}
		slog.InfoContext(ctx, "agent: tested device", "device", device.DeviceID, "name", device.Name, "passed", passed)
	}
	if len(devices) == 0 {
		slog.WarnContext(ctx, "agent: no usable GPU found", "device", deviceID)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.ffmpegVersion = version
	if deviceID == "" {
		c.devices = devices
		return devices
	}
	for _, device := range devices {
		for i := range c.devices {
			if c.devices[i].DeviceID == device.DeviceID {
				c.devices[i] = device
			}
		}
	}
	return devices
}

// Draining reports whether the server asked the agent to take no new jobs.
func (c *Client) Draining() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.draining
}

func (c *Client) scratchFree() int64 {
	free, err := c.opts.ScratchFree()
	if err != nil {
		slog.Warn("agent: reading scratch free space", "error", err)
		return 0
	}
	return free
}

func (c *Client) send(ctx context.Context, conn *websocket.Conn, messageType agentproto.MessageType, body any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	env := agentproto.Envelope{Type: messageType, ID: string(domain.NewID[envelopeID]()), Body: raw}
	ctx, cancel := context.WithTimeout(ctx, frameWriteTimeout)
	defer cancel()
	return wsjson.Write(ctx, conn, env)
}

type envelopeID string
