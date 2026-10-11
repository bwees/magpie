package transcode

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"magpie/internal/domain"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/db"
	"magpie/internal/platform/db/dbtest"
	"magpie/internal/platform/settings"
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

type fixture struct {
	t       *testing.T
	db      *db.DB
	service *AgentService
	clock   *testClock
	server  *httptest.Server
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	d := dbtest.Open(t, Migrations())
	clock := &testClock{now: testStart}
	hub := NewAgentHub()
	service := NewAgentService(d, bus.New(), NewAgentRepository(d), hub, settings.NewStore(d), DefaultTimings)
	service.now = clock.Now
	server := httptest.NewServer(NewAgentSocket(service, hub, AgentSecret(testSecret), DefaultTimings))
	t.Cleanup(server.Close)
	return &fixture{t: t, db: d, service: service, clock: clock, server: server}
}

func (f *fixture) dial(id domain.AgentID, secret string) (*websocket.Conn, *http.Response, error) {
	header := http.Header{}
	header.Set(agentproto.HeaderAuthorization, agentproto.BearerPrefix+secret)
	header.Set(agentproto.HeaderAgentID, string(id))
	return websocket.Dial(context.Background(), f.server.URL, &websocket.DialOptions{HTTPHeader: header})
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
	return a, decodeBody[agentproto.Welcome](f.t, a.expect(agentproto.TypeWelcome))
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

func (a *testAgent) expectAck(of string) {
	a.t.Helper()
	if ack := decodeBody[agentproto.Ack](a.t, a.expect(agentproto.TypeAck)); ack.Of != of {
		a.t.Fatalf("ack of %q, want %q", ack.Of, of)
	}
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
	hub := NewAgentHub()
	server := httptest.NewServer(NewAgentSocket(f.service, hub, "", DefaultTimings))
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
	a.expectAck(a.send(agentproto.TypeCapabilitiesChanged, agentproto.CapabilitiesChanged{Devices: []agentproto.DeviceReport{report}}))

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
	a.expectAck(a.send(agentproto.TypeCapabilitiesChanged, agentproto.CapabilitiesChanged{Devices: []agentproto.DeviceReport{report}}))

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
