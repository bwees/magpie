package transcode

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"magpie/internal/domain"
	"magpie/internal/transcode/api"
	"magpie/internal/transcode/api/agentproto"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const (
	testSecret      = "test-secret-0123456789abcdef-0123456789"
	testHostname    = "gpu-box"
	testVersion     = "1.2.3"
	testFFmpeg      = "7.1.2-Jellyfin"
	testScratchFree = 500 << 30
	frameTimeout    = 5 * time.Second
	sessionError    = "OpenEncodeSessionEx failed: out of memory"
	// testHeartbeatEvery shortens the offline window for the tests that wait it out on the wall clock.
	testHeartbeatEvery = 50 * time.Millisecond
)

var testStart = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func (f *fixture) dial(id domain.AgentID, secret string) (*websocket.Conn, *http.Response, error) {
	return dialAgent(f.server.URL+agentproto.ControlPath, id, secret)
}

func dialAgent(socketURL string, id domain.AgentID, secret string) (*websocket.Conn, *http.Response, error) {
	header := http.Header{}
	header.Set(agentproto.HeaderAuthorization, agentproto.BearerPrefix+secret)
	header.Set(agentproto.HeaderAgentID, string(id))
	return websocket.Dial(context.Background(), socketURL, &websocket.DialOptions{HTTPHeader: header})
}

// eventuallyAgent waits for the agent to satisfy ok, since the socket handles frames on its own goroutine.
func (f *fixture) eventuallyAgent(id domain.AgentID, what string, ok func(api.Agent) bool) {
	f.t.Helper()
	deadline := time.Now().Add(eventuallyWithin)
	for !ok(f.agent(id)) {
		if time.Now().After(deadline) {
			f.t.Fatalf("agent %s never got %s: %+v", id, what, f.agent(id))
		}
		time.Sleep(eventuallyEvery)
	}
}

func (f *fixture) agentRows() int64 {
	f.t.Helper()
	var n int64
	if err := f.db.Read(context.Background()).Table(agentsTable).Count(&n).Error; err != nil {
		f.t.Fatal(err)
	}
	return n
}

func (f *fixture) published(eventType string) []string {
	f.t.Helper()
	var payloads []string
	err := f.db.Read(context.Background()).Table("platform_outbox").Where("type = ?", eventType).Order("seq").Pluck("payload", &payloads).Error
	if err != nil {
		f.t.Fatal(err)
	}
	return payloads
}

func (f *fixture) agent(id domain.AgentID) api.Agent {
	f.t.Helper()
	agents, err := f.service.Agents(context.Background())
	if err != nil {
		f.t.Fatal(err)
	}
	for _, a := range agents {
		if a.ID == id {
			return a
		}
	}
	f.t.Fatalf("agent %s not found", id)
	return api.Agent{}
}

type testAgent struct {
	t    *testing.T
	id   domain.AgentID
	conn *websocket.Conn
}

func testHello(id domain.AgentID) agentproto.Hello {
	return agentproto.Hello{
		AgentID:       id,
		Hostname:      testHostname,
		Version:       testVersion,
		FFmpegVersion: testFFmpeg,
		Mode:          agentproto.ModeDownloadFirst,
		ScratchFree:   testScratchFree,
		Devices: []agentproto.DeviceReport{{
			DeviceID:    testDevice,
			Vendor:      agentproto.VendorNVIDIA,
			Name:        "NVIDIA GeForce RTX 4070",
			Driver:      "560.35.03",
			MaxSessions: geForceSessions,
			Capabilities: []agentproto.CapabilityResult{
				{Capability: agentproto.CapEncHEVC10, OK: true},
				{Capability: agentproto.CapEncAV110, OK: false, Error: sessionError},
			},
		}},
	}
}

// connect dials, sends hello and returns the agent with the server's Welcome.
func (f *fixture) connect(hello agentproto.Hello) (*testAgent, agentproto.Welcome) {
	f.t.Helper()
	conn, _, err := f.dial(hello.AgentID, testSecret)
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() { _ = conn.CloseNow() })
	a := &testAgent{t: f.t, id: hello.AgentID, conn: conn}
	a.send(agentproto.TypeHello, hello)
	welcome := decodeBody[agentproto.Welcome](f.t, a.expect(agentproto.TypeWelcome))
	return a, welcome
}

func (h *AgentHub) attachedWith(hello agentproto.Hello) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, connected := h.conns[hello.AgentID]
	return connected && slices.Equal(h.resumable[hello.AgentID], hello.ResumableJobs)
}

func (a *testAgent) send(t agentproto.MessageType, body any) string {
	a.t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		a.t.Fatal(err)
	}
	env := agentproto.Envelope{Type: t, ID: string(domain.NewID[envelopeID]()), Body: raw}
	ctx, cancel := context.WithTimeout(context.Background(), frameTimeout)
	defer cancel()
	if err := wsjson.Write(ctx, a.conn, env); err != nil {
		a.t.Fatal(err)
	}
	return env.ID
}

func (a *testAgent) expect(t agentproto.MessageType) agentproto.Envelope {
	a.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), frameTimeout)
	defer cancel()
	var env agentproto.Envelope
	if err := wsjson.Read(ctx, a.conn, &env); err != nil {
		a.t.Fatalf("waiting for %s: %v", t, err)
	}
	if env.Type != t {
		a.t.Fatalf("got %s frame, want %s", env.Type, t)
	}
	return env
}

// answersClose keeps reading in the background, as a real agent does, so it answers the server's close
// at once instead of letting the close wait out its timeout. The agent can't expect frames after this.
func (a *testAgent) answersClose() {
	a.conn.CloseRead(context.Background())
}

func (a *testAgent) expectAck(of string) {
	a.t.Helper()
	if ack := decodeBody[agentproto.Ack](a.t, a.expect(agentproto.TypeAck)); ack.Of != of {
		a.t.Fatalf("ack of %q, want %q", ack.Of, of)
	}
}

// changeCapabilities reports test results, which the server answers with a Welcome sized for them and an Ack.
func (a *testAgent) changeCapabilities(devices ...agentproto.DeviceReport) agentproto.Welcome {
	a.t.Helper()
	sent := a.send(agentproto.TypeCapabilitiesChanged, agentproto.CapabilitiesChanged{Devices: devices})
	welcome := decodeBody[agentproto.Welcome](a.t, a.expect(agentproto.TypeWelcome))
	a.expectAck(sent)
	return welcome
}

func decodeBody[T any](t *testing.T, env agentproto.Envelope) T {
	t.Helper()
	var body T
	if err := json.Unmarshal(env.Body, &body); err != nil {
		t.Fatal(err)
	}
	return body
}

// Seed 28.
func TestWrongSecretIsRejectedWithoutCreatingAnAgent(t *testing.T) {
	f := newFixture(t)

	_, resp, err := f.dial(domain.NewID[domain.AgentID](), "wrong-secret")

	if err == nil {
		t.Fatal("upgrade succeeded with a wrong secret")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("response = %v, want 401", resp)
	}
	if n := f.agentRows(); n != 0 {
		t.Fatalf("agent rows = %d, want 0", n)
	}
}

func TestUnsetSecretRefusesEveryAgent(t *testing.T) {
	f := newFixture(t)
	server := httptest.NewServer(NewAgentSocket(f.service, f.jobs, NewAgentHub(), NewWake(), f.lifetime, "", DefaultTimings))
	t.Cleanup(server.Close)

	header := http.Header{}
	header.Set(agentproto.HeaderAuthorization, agentproto.BearerPrefix)
	header.Set(agentproto.HeaderAgentID, string(domain.NewID[domain.AgentID]()))
	_, resp, err := websocket.Dial(context.Background(), server.URL, &websocket.DialOptions{HTTPHeader: header})

	if err == nil || resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("dial error %v, response %v; want 401", err, resp)
	}
}

func TestMalformedAgentIDIsRejected(t *testing.T) {
	f := newFixture(t)

	_, resp, err := f.dial("not-a-ulid", testSecret)

	if err == nil || resp == nil || resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("dial error %v, response %v; want 400", err, resp)
	}
}

func TestFirstHelloRegistersTheAgent(t *testing.T) {
	f := newFixture(t)
	id := domain.NewID[domain.AgentID]()

	_, welcome := f.connect(testHello(id))

	want := agentproto.Welcome{
		HeartbeatEvery: DefaultTimings.HeartbeatEvery,
		LeaseTTL:       DefaultTimings.LeaseTTL,
		Mode:           agentproto.ModeDownloadFirst,
		Slots:          map[string]int{testDevice: defaultSlotsPerGPU},
	}
	if !jsonEqual(t, welcome, want) {
		t.Fatalf("welcome = %+v, want %+v", welcome, want)
	}

	got := f.agent(id)
	if got.Hostname != testHostname || got.Version != testVersion || got.FFmpegVersion != testFFmpeg ||
		got.ScratchFreeBytes != testScratchFree || got.Mode != api.TransferDownloadFirst || !got.Online {
		t.Fatalf("agent = %+v", got)
	}
	if len(got.Devices) != 1 {
		t.Fatalf("devices = %+v, want one", got.Devices)
	}
	dev := got.Devices[0]
	if dev.ID != testDevice || dev.MaxSessions != geForceSessions || !dev.Healthy || len(dev.Capabilities) != 2 {
		t.Fatalf("device = %+v", dev)
	}
	if av1 := capabilityNamed(dev, agentproto.CapEncAV110); av1.OK || av1.Error != sessionError {
		t.Fatalf("AV1 capability = %+v, want failed with %q", av1, sessionError)
	}
	if online := f.published(api.TypeAgentOnline); len(online) != 1 {
		t.Fatalf("AgentOnline published %d times, want once", len(online))
	}
}

func TestSchedulingRightAfterWelcomeReachesTheAgent(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.cutoff(file, domain.OriginDownload)
	hello := capableHello("gpu-a")
	hello.ResumableJobs = []agentproto.ResumableJob{{Job: domain.NewID[domain.JobID](), SourceETag: "kept-scratch"}}

	a, _ := f.connect(hello)

	if !f.hub.attachedWith(hello) {
		t.Fatal("the agent got its Welcome before the hub knew it and its resumable jobs")
	}
	if offer := f.offered(a); offer.Spec.Job != f.onlyJob(file).ID {
		t.Fatalf("offered job %s, want the file's job", offer.Spec.Job)
	}
}

func TestStopClosesAgentSocketsBeforeItReturns(t *testing.T) {
	f := newFixture(t)
	a, _ := f.connect(capableHello("gpu-a"))
	closed := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), frameTimeout)
		defer cancel()
		_, _, err := a.conn.Read(ctx)
		closed <- err
	}()

	f.lifetime.Stop()

	if connected := f.hub.Connected(); len(connected) != 0 {
		t.Fatalf("socket handlers of %v still run after Stop", connected)
	}
	if err := <-closed; websocket.CloseStatus(err) != websocket.StatusNormalClosure {
		t.Fatalf("agent read error = %v, want a normal close", err)
	}
	if offline := f.published(api.TypeAgentOffline); len(offline) != 0 {
		t.Fatalf("AgentOffline = %v, want none: a restart leaves agents to reconnect", offline)
	}
}

func TestClosingTheSocketTakesTheAgentOfflineAtOnce(t *testing.T) {
	f := newFixture(t)
	id := domain.NewID[domain.AgentID]()
	a, _ := f.connect(testHello(id))

	if err := a.conn.Close(websocket.StatusNormalClosure, "agent stopping"); err != nil {
		t.Fatal(err)
	}

	f.eventuallyAgent(id, "offline", func(a api.Agent) bool { return !a.Online })
	offline := f.published(api.TypeAgentOffline)
	if len(offline) != 1 || !strings.Contains(offline[0], string(id)) {
		t.Fatalf("AgentOffline = %v, want one for %s", offline, id)
	}
}

func TestAReconnectedAgentStaysOnline(t *testing.T) {
	f := newFixture(t)
	id := domain.NewID[domain.AgentID]()
	f.connect(testHello(id))

	if err := f.service.Disconnected(context.Background(), id); err != nil {
		t.Fatal(err)
	}

	if !f.agent(id).Online || len(f.published(api.TypeAgentOffline)) != 0 {
		t.Fatal("an older socket closing took the connected agent offline")
	}
}

func TestASilentSocketIsDroppedAfterTheOfflineWindow(t *testing.T) {
	f := newFixture(t)
	timings := DefaultTimings
	timings.HeartbeatEvery = testHeartbeatEvery
	server := httptest.NewServer(NewAgentSocket(f.service, f.jobs, f.hub, NewWake(), f.lifetime, testSecret, timings))
	t.Cleanup(server.Close)
	id := domain.NewID[domain.AgentID]()
	conn, _, err := dialAgent(server.URL, id, testSecret)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	a := &testAgent{t: t, id: id, conn: conn}
	a.send(agentproto.TypeHello, testHello(id))
	a.expect(agentproto.TypeWelcome)

	for range 2 * timings.MissedHeartbeats {
		time.Sleep(timings.HeartbeatEvery)
		a.send(agentproto.TypeHeartbeat, agentproto.Heartbeat{ScratchFree: testScratchFree})
	}
	if err := f.service.Retest(context.Background(), id); err != nil {
		t.Fatalf("heartbeating agent was dropped: %v", err)
	}
	a.expect(agentproto.TypeRetest)

	ctx, cancel := context.WithTimeout(context.Background(), frameTimeout)
	defer cancel()
	if _, _, err := conn.Read(ctx); err == nil || ctx.Err() != nil {
		t.Fatalf("read error = %v, want the server to drop the silent socket", err)
	}
	f.eventuallyAgent(id, "offline", func(a api.Agent) bool { return !a.Online })
}

func TestNewCapabilitiesResendTheWelcomeWithTheirSlots(t *testing.T) {
	f := newFixture(t)
	hello := testHello(domain.NewID[domain.AgentID]())
	devices := hello.Devices
	hello.Devices = nil
	a, welcome := f.connect(hello)
	if len(welcome.Slots) != 0 {
		t.Fatalf("welcome slots = %v, want none before the GPU tests", welcome.Slots)
	}

	if resent := a.changeCapabilities(devices...); resent.Slots[testDevice] != defaultSlotsPerGPU {
		t.Fatalf("welcome slots = %v, want %d on %s", resent.Slots, defaultSlotsPerGPU, testDevice)
	}
}

func TestAgentSocketIsRefusedOnceStopped(t *testing.T) {
	f := newFixture(t)
	f.lifetime.Stop()

	_, resp, err := f.dial(domain.NewID[domain.AgentID](), testSecret)

	if err == nil || resp == nil || resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("dial error %v, response %v; want 503", err, resp)
	}
}

func TestHelloMustNameTheConnectingAgent(t *testing.T) {
	f := newFixture(t)
	conn, _, err := f.dial(domain.NewID[domain.AgentID](), testSecret)
	if err != nil {
		t.Fatal(err)
	}
	a := &testAgent{t: t, conn: conn}
	a.send(agentproto.TypeHello, testHello(domain.NewID[domain.AgentID]()))

	ctx, cancel := context.WithTimeout(context.Background(), frameTimeout)
	defer cancel()
	_, _, err = conn.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Fatalf("read error = %v, want a policy-violation close", err)
	}
	if n := f.agentRows(); n != 0 {
		t.Fatalf("agent rows = %d, want 0", n)
	}
}

func TestMissedHeartbeatsTakeTheAgentOfflineAndReconnectingBringsItBack(t *testing.T) {
	f := newFixture(t)
	id := domain.NewID[domain.AgentID]()
	f.connect(testHello(id))
	ctx := context.Background()

	for range DefaultTimings.MissedHeartbeats {
		f.clock.Advance(DefaultTimings.HeartbeatEvery)
		if err := f.service.Heartbeat(ctx, id, agentproto.Heartbeat{ScratchFree: testScratchFree}); err != nil {
			t.Fatal(err)
		}
		if err := f.service.Sweep(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if !f.agent(id).Online || len(f.published(api.TypeAgentOffline)) != 0 {
		t.Fatal("agent went offline while heartbeating")
	}

	f.clock.Advance(offlineAfter(DefaultTimings) + time.Second)
	if err := f.service.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if f.agent(id).Online {
		t.Fatal("agent still online after 3 missed heartbeats")
	}
	offline := f.published(api.TypeAgentOffline)
	if len(offline) != 1 || !strings.Contains(offline[0], string(id)) {
		t.Fatalf("AgentOffline = %v, want one for %s", offline, id)
	}

	if err := f.service.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if n := len(f.published(api.TypeAgentOffline)); n != 1 {
		t.Fatalf("AgentOffline published %d times, want once", n)
	}

	f.connect(testHello(id))
	if !f.agent(id).Online {
		t.Fatal("reconnected agent is offline")
	}
	if n := len(f.published(api.TypeAgentOnline)); n != 2 {
		t.Fatalf("AgentOnline published %d times, want 2 (connect and reconnect)", n)
	}
}

func TestRetestUpdatesCapabilities(t *testing.T) {
	f := newFixture(t)
	id := domain.NewID[domain.AgentID]()
	a, _ := f.connect(testHello(id))

	if err := f.service.Retest(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if retest := decodeBody[agentproto.Retest](t, a.expect(agentproto.TypeRetest)); retest.DeviceID != "" {
		t.Fatalf("retest device = %q, want all devices", retest.DeviceID)
	}

	f.clock.Advance(time.Minute)
	report := testHello(id).Devices[0]
	report.Capabilities = []agentproto.CapabilityResult{
		{Capability: agentproto.CapEncHEVC10, OK: true},
		{Capability: agentproto.CapEncAV110, OK: true},
	}
	a.changeCapabilities(report)

	av1 := capabilityNamed(f.agent(id).Devices[0], agentproto.CapEncAV110)
	if !av1.OK || av1.Error != "" || !av1.TestedAt.Equal(testStart.Add(time.Minute)) {
		t.Fatalf("AV1 capability = %+v, want passed at the retest time", av1)
	}
}

func TestRetestOfAnOfflineAgentFails(t *testing.T) {
	f := newFixture(t)

	if err := f.service.Retest(context.Background(), domain.NewID[domain.AgentID]()); err == nil {
		t.Fatal("retest of an unconnected agent succeeded")
	}
}

func TestUpdateSendsOverridesAndDrain(t *testing.T) {
	f := newFixture(t)
	id := domain.NewID[domain.AgentID]()
	a, _ := f.connect(testHello(id))
	slots := 3

	updated, err := f.service.Update(context.Background(), id, AgentOverrides{SlotsOverride: &slots, Mode: api.TransferDirectRead, Draining: true})
	if err != nil {
		t.Fatal(err)
	}

	welcome := decodeBody[agentproto.Welcome](t, a.expect(agentproto.TypeWelcome))
	if !welcome.Draining || welcome.Mode != agentproto.ModeDirectRead || welcome.Slots[testDevice] != slots {
		t.Fatalf("welcome = %+v, want draining, direct read and %d slots", welcome, slots)
	}
	a.expect(agentproto.TypeDrain)
	if !updated.Draining || updated.Mode != api.TransferDirectRead || updated.SlotsOverride == nil || *updated.SlotsOverride != slots {
		t.Fatalf("updated = %+v", updated)
	}

	f.connect(testHello(id))
	if got := f.agent(id); got.Mode != api.TransferDirectRead || !got.Draining {
		t.Fatalf("after reconnect agent = %+v, want the server overrides to win", got)
	}
}

func TestUpdateOfAnUnknownAgentIsNotFound(t *testing.T) {
	f := newFixture(t)

	_, err := f.service.Update(context.Background(), domain.NewID[domain.AgentID](), AgentOverrides{Mode: api.TransferDownloadFirst})

	if err == nil {
		t.Fatal("update of an unknown agent succeeded")
	}
}

func TestUnhealthyDeviceFreesSlotsAndIsRetestedLater(t *testing.T) {
	f := newFixture(t)
	id := domain.NewID[domain.AgentID]()
	a, _ := f.connect(testHello(id))
	ctx := context.Background()
	key := SlotKey{Agent: id, Device: testDevice}

	if free := FreeSlots([]api.Agent{f.agent(id)}, nil, defaultSlotsPerGPU); free[key] != defaultSlotsPerGPU {
		t.Fatalf("free slots before = %d", free[key])
	}

	a.expectAck(a.send(agentproto.TypeDeviceUnhealthy, agentproto.DeviceUnhealthy{DeviceID: testDevice, Error: sessionError}))
	if free := FreeSlots([]api.Agent{f.agent(id)}, nil, defaultSlotsPerGPU); free[key] != 0 {
		t.Fatalf("free slots after DeviceUnhealthy = %d, want 0", free[key])
	}

	f.clock.Advance(DefaultTimings.RetestDelay - time.Second)
	keepAlive := func() {
		if err := f.service.Heartbeat(ctx, id, agentproto.Heartbeat{ScratchFree: testScratchFree}); err != nil {
			t.Fatal(err)
		}
		if err := f.service.Sweep(ctx); err != nil {
			t.Fatal(err)
		}
	}
	keepAlive()
	f.clock.Advance(time.Second)
	keepAlive()

	if retest := decodeBody[agentproto.Retest](t, a.expect(agentproto.TypeRetest)); retest.DeviceID != testDevice {
		t.Fatalf("retest device = %q, want %q", retest.DeviceID, testDevice)
	}
	report := testHello(id).Devices[0]
	a.changeCapabilities(report)

	if dev := f.agent(id).Devices[0]; !dev.Healthy {
		t.Fatalf("device = %+v, want healthy after a passing retest", dev)
	}
}

func capabilityNamed(d api.Device, c agentproto.Capability) api.Capability {
	for _, capability := range d.Capabilities {
		if capability.Name == string(c) {
			return capability
		}
	}
	return api.Capability{}
}

func jsonEqual(t *testing.T, a, b any) bool {
	t.Helper()
	left, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	right, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	return string(left) == string(right)
}
