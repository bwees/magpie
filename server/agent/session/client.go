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
	"slices"
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
	// dialTimeout bounds the upgrade, so a server that vanished can't hold a reconnect for minutes.
	dialTimeout = 10 * time.Second
	// A ping unanswered for this many heartbeat intervals means the server is gone. Heartbeat writes alone
	// can't tell: they fill the socket's buffer until TCP gives up, many minutes later.
	pingTimeoutHeartbeats = 2
)

var ErrNoWelcome = errors.New("session: server did not answer Hello with Welcome")

// Prober runs the capability tests. An empty deviceID tests every device.
type Prober interface {
	FFmpegVersion(ctx context.Context) string
	Probe(ctx context.Context, deviceID string) []agentproto.DeviceReport
}

// Jobs runs the jobs the server offers.
type Jobs interface {
	Configure(welcome agentproto.Welcome)
	Drain()
	SetDevices(devices []agentproto.DeviceReport)
	Offer(offer agentproto.JobOffer) (bool, agentproto.RejectReason)
	Start(job domain.JobID)
	Cancel(job domain.JobID)
	Heartbeats() []agentproto.JobHeartbeat
	Resumable() []agentproto.ResumableJob
	// Deliver sends results the server hasn't received yet.
	Deliver(ctx context.Context)
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
	// Jobs may be set after New, before Run, since it sends through the client.
	Jobs Jobs
}

var ErrNotConnected = errors.New("session: not connected to the server")

// Client keeps one control channel open to the server.
type Client struct {
	opts        Options
	http        *http.Client
	random      func() float64
	dialTimeout time.Duration
	// probed closes once the startup tests finish.
	probed chan struct{}

	mu            sync.Mutex
	ffmpegVersion string
	devices       []agentproto.DeviceReport
	welcome       agentproto.Welcome
	draining      bool
	conn          *websocket.Conn

	// retestMu serialises retests so two never fight over the GPU.
	retestMu sync.Mutex
}

func New(opts Options) *Client {
	if opts.Jobs == nil {
		opts.Jobs = noJobs{}
	}
	return &Client{
		opts:        opts,
		http:        NewHTTPClient(opts.Secret, opts.AgentID),
		random:      rand.Float64,
		dialTimeout: dialTimeout,
		probed:      make(chan struct{}),
	}
}

// SetJobs gives the client the job runner, which sends its results through the client.
func (c *Client) SetJobs(jobs Jobs) {
	c.opts.Jobs = jobs
}

// HTTP is the client that adds the agent's auth headers, for the job byte endpoints.
func (c *Client) HTTP() *http.Client {
	return c.http
}

// Send writes a frame on the current connection, or fails with ErrNotConnected.
func (c *Client) Send(ctx context.Context, messageType agentproto.MessageType, body any) error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return ErrNotConnected
	}
	return c.send(ctx, conn, messageType, body)
}

// noJobs takes no jobs, for an agent that only reports its devices.
type noJobs struct{}

func (noJobs) Configure(agentproto.Welcome)         {}
func (noJobs) Drain()                               {}
func (noJobs) SetDevices([]agentproto.DeviceReport) {}
func (noJobs) Offer(agentproto.JobOffer) (bool, agentproto.RejectReason) {
	return false, agentproto.RejectCapability
}
func (noJobs) Start(domain.JobID)                    {}
func (noJobs) Cancel(domain.JobID)                   {}
func (noJobs) Heartbeats() []agentproto.JobHeartbeat { return []agentproto.JobHeartbeat{} }
func (noJobs) Resumable() []agentproto.ResumableJob  { return []agentproto.ResumableJob{} }
func (noJobs) Deliver(context.Context)               {}

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

// Run connects and reconnects until ctx ends. The GPU tests run alongside: a Hello sent before they
// finish carries no devices, and the results follow as CapabilitiesChanged.
func (c *Client) Run(ctx context.Context) error {
	version := c.opts.Prober.FFmpegVersion(ctx)
	c.mu.Lock()
	c.ffmpegVersion = version
	c.mu.Unlock()
	var probing sync.WaitGroup
	defer probing.Wait()
	probing.Go(func() {
		defer close(c.probed)
		c.retest(ctx, "")
	})

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

	conn, err := c.dial(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = conn.CloseNow() }()

	reported := c.isProbed()
	if err := c.send(ctx, conn, agentproto.TypeHello, c.hello()); err != nil {
		return false, err
	}
	welcome, err := c.readWelcome(ctx, conn)
	if err != nil {
		return false, err
	}
	c.applyWelcome(welcome)
	slog.InfoContext(ctx, "agent: connected", "server", c.opts.ServerURL.String(), "mode", welcome.Mode, "slots", welcome.Slots)
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.conn = nil
		c.mu.Unlock()
	}()
	c.opts.Jobs.Deliver(ctx)
	if !reported {
		go c.reportProbe(ctx, conn)
	}

	every := c.heartbeatEvery()
	go c.heartbeats(ctx, conn, every)
	go c.pings(ctx, conn, every)
	return true, c.readLoop(ctx, conn)
}

func (c *Client) dial(ctx context.Context) (*websocket.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, c.dialTimeout)
	defer cancel()
	controlURL := c.opts.ServerURL.JoinPath(agentproto.ControlPath).String()
	conn, resp, err := websocket.Dial(ctx, controlURL, &websocket.DialOptions{HTTPClient: c.http})
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("dialling %s: %s: %w", controlURL, resp.Status, err)
		}
		return nil, fmt.Errorf("dialling %s: %w", controlURL, err)
	}
	return conn, nil
}

func (c *Client) isProbed() bool {
	select {
	case <-c.probed:
		return true
	default:
		return false
	}
}

// reportProbe sends the startup test results that missed the Hello once they are in.
func (c *Client) reportProbe(ctx context.Context, conn *websocket.Conn) {
	select {
	case <-ctx.Done():
		return
	case <-c.probed:
	}
	c.mu.Lock()
	devices := slices.Clone(c.devices)
	c.mu.Unlock()
	if err := c.send(ctx, conn, agentproto.TypeCapabilitiesChanged, agentproto.CapabilitiesChanged{Devices: devices}); err != nil {
		slog.WarnContext(ctx, "agent: sending test results", "error", err)
	}
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
		ResumableJobs: c.opts.Jobs.Resumable(),
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
	c.welcome = welcome
	c.draining = welcome.Draining
	c.mu.Unlock()
	c.opts.Jobs.Configure(welcome)
}

func (c *Client) heartbeatEvery() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.welcome.HeartbeatEvery <= 0 {
		return DefaultHeartbeatEvery
	}
	return c.welcome.HeartbeatEvery
}

// pings closes the connection once the server stops answering, so the session reconnects.
func (c *Client) pings(ctx context.Context, conn *websocket.Conn, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := ping(ctx, conn, every*pingTimeoutHeartbeats); err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					slog.WarnContext(ctx, "agent: server stopped answering pings", "error", err)
				}
				_ = conn.CloseNow()
				return
			}
		}
	}
}

func ping(ctx context.Context, conn *websocket.Conn, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return conn.Ping(ctx)
}

func (c *Client) heartbeats(ctx context.Context, conn *websocket.Conn, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			heartbeat := agentproto.Heartbeat{ScratchFree: c.scratchFree(), Jobs: c.opts.Jobs.Heartbeats()}
			if err := c.send(ctx, conn, agentproto.TypeHeartbeat, heartbeat); err != nil {
				return
			}
			c.opts.Jobs.Deliver(ctx)
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
		c.opts.Jobs.Drain()
		slog.InfoContext(ctx, "agent: draining; accepting no new jobs")
	case agentproto.TypeJobOffer:
		var offer agentproto.JobOffer
		if err := json.Unmarshal(env.Body, &offer); err != nil {
			return err
		}
		return c.answerOffer(ctx, conn, offer)
	case agentproto.TypeStart:
		var start agentproto.Start
		if err := json.Unmarshal(env.Body, &start); err != nil {
			return err
		}
		c.opts.Jobs.Start(start.Job)
	case agentproto.TypeCancel:
		var cancel agentproto.Cancel
		if err := json.Unmarshal(env.Body, &cancel); err != nil {
			return err
		}
		slog.InfoContext(ctx, "agent: server cancelled a job", "job", cancel.Job, "reason", cancel.Reason)
		c.opts.Jobs.Cancel(cancel.Job)
	case agentproto.TypeAck:
	default:
		slog.DebugContext(ctx, "agent: ignoring server frame", "type", env.Type)
	}
	return nil
}

func (c *Client) answerOffer(ctx context.Context, conn *websocket.Conn, offer agentproto.JobOffer) error {
	accepted, reason := c.opts.Jobs.Offer(offer)
	if !accepted {
		slog.InfoContext(ctx, "agent: rejecting a job", "job", offer.Spec.Job, "reason", reason)
		return c.send(ctx, conn, agentproto.TypeJobReject, agentproto.JobReject{Job: offer.Spec.Job, Reason: reason})
	}
	slog.InfoContext(ctx, "agent: accepted a job", "job", offer.Spec.Job, "device", offer.Spec.DeviceID, "prefetch", offer.Prefetch)
	return c.send(ctx, conn, agentproto.TypeJobAccept, agentproto.JobAccept{Job: offer.Spec.Job})
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
	c.ffmpegVersion = version
	if deviceID == "" {
		c.devices = devices
	}
	for _, device := range devices {
		for i := range c.devices {
			if c.devices[i].DeviceID == device.DeviceID {
				c.devices[i] = device
			}
		}
	}
	all := slices.Clone(c.devices)
	c.mu.Unlock()
	c.opts.Jobs.SetDevices(all)
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
