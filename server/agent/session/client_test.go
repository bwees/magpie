package session

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"magpie/internal/domain"
	"magpie/internal/transcode/api/agentproto"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const (
	testSecret      = "0123456789abcdef0123456789abcdef"
	testHostname    = "gpu-box"
	testScratchFree = 100 << 30
	testHeartbeat   = 20 * time.Millisecond
	testDevice      = "cuda:0"
	testFFmpeg      = "7.1.2-Jellyfin"
	frameTimeout    = 5 * time.Second
)

type fakeProber struct {
	mu     sync.Mutex
	probed []string
}

func (p *fakeProber) FFmpegVersion(context.Context) string { return testFFmpeg }

func (p *fakeProber) Probe(_ context.Context, deviceID string) []agentproto.DeviceReport {
	p.mu.Lock()
	p.probed = append(p.probed, deviceID)
	p.mu.Unlock()
	return []agentproto.DeviceReport{{
		DeviceID:     testDevice,
		Vendor:       agentproto.VendorNVIDIA,
		Capabilities: []agentproto.CapabilityResult{{Capability: agentproto.CapEncHEVC10, OK: true}},
	}}
}

// fakeServer accepts agent sockets and hands each one to the test.
type fakeServer struct {
	*httptest.Server
	conns   chan *websocket.Conn
	headers chan http.Header
}

func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	s := &fakeServer{conns: make(chan *websocket.Conn, 1), headers: make(chan http.Header, 1)}
	mux := http.NewServeMux()
	mux.HandleFunc(agentproto.ControlPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(agentproto.HeaderAuthorization) != agentproto.BearerPrefix+testSecret {
			http.Error(w, "wrong secret", http.StatusUnauthorized)
			return
		}
		s.headers <- r.Header.Clone()
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		s.conns <- conn
		<-r.Context().Done()
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func (s *fakeServer) accept(t *testing.T) *websocket.Conn {
	t.Helper()
	select {
	case conn := <-s.conns:
		t.Cleanup(func() { _ = conn.CloseNow() })
		return conn
	case <-time.After(frameTimeout):
		t.Fatal("agent never connected")
		return nil
	}
}

func newClient(t *testing.T, server *fakeServer, secret string) (*Client, *fakeProber, domain.AgentID) {
	t.Helper()
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	prober := &fakeProber{}
	id := domain.NewID[domain.AgentID]()
	client := New(Options{
		ServerURL:   serverURL,
		Secret:      secret,
		AgentID:     id,
		Hostname:    testHostname,
		Mode:        agentproto.ModeDownloadFirst,
		Prober:      prober,
		ScratchFree: func() (int64, error) { return testScratchFree, nil },
	})
	return client, prober, id
}

func run(t *testing.T, client *Client) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = client.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
}

func read(t *testing.T, conn *websocket.Conn, want agentproto.MessageType) agentproto.Envelope {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), frameTimeout)
	defer cancel()
	for {
		var env agentproto.Envelope
		if err := wsjson.Read(ctx, conn, &env); err != nil {
			t.Fatalf("waiting for %s: %v", want, err)
		}
		if env.Type == want {
			return env
		}
		if env.Type != agentproto.TypeHeartbeat {
			t.Fatalf("got %s, want %s", env.Type, want)
		}
	}
}

func write(t *testing.T, conn *websocket.Conn, messageType agentproto.MessageType, body any) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), frameTimeout)
	defer cancel()
	if err := wsjson.Write(ctx, conn, agentproto.Envelope{Type: messageType, ID: string(domain.NewID[envelopeID]()), Body: raw}); err != nil {
		t.Fatal(err)
	}
}

func decode[T any](t *testing.T, env agentproto.Envelope) T {
	t.Helper()
	var body T
	if err := json.Unmarshal(env.Body, &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func welcome(t *testing.T, conn *websocket.Conn, draining bool) {
	t.Helper()
	write(t, conn, agentproto.TypeWelcome, agentproto.Welcome{HeartbeatEvery: testHeartbeat, Mode: agentproto.ModeDownloadFirst, Draining: draining})
}

func TestClientAuthenticatesHelloesAndHeartbeats(t *testing.T) {
	server := newFakeServer(t)
	client, _, id := newClient(t, server, testSecret)
	run(t, client)

	conn := server.accept(t)
	headers := <-server.headers
	if headers.Get(agentproto.HeaderAgentID) != string(id) {
		t.Errorf("%s = %q, want %q", agentproto.HeaderAgentID, headers.Get(agentproto.HeaderAgentID), id)
	}

	hello := decode[agentproto.Hello](t, read(t, conn, agentproto.TypeHello))
	if hello.AgentID != id || hello.Hostname != testHostname || hello.FFmpegVersion != testFFmpeg ||
		hello.ScratchFree != testScratchFree || len(hello.Devices) != 1 || hello.Devices[0].DeviceID != testDevice {
		t.Fatalf("hello = %+v", hello)
	}

	welcome(t, conn, false)
	heartbeat := decode[agentproto.Heartbeat](t, read(t, conn, agentproto.TypeHeartbeat))
	if heartbeat.ScratchFree != testScratchFree {
		t.Fatalf("heartbeat = %+v", heartbeat)
	}
}

func TestClientRetestsTheRequestedDevice(t *testing.T) {
	server := newFakeServer(t)
	client, prober, _ := newClient(t, server, testSecret)
	run(t, client)
	conn := server.accept(t)
	read(t, conn, agentproto.TypeHello)
	welcome(t, conn, false)

	write(t, conn, agentproto.TypeRetest, agentproto.Retest{DeviceID: testDevice})

	changed := decode[agentproto.CapabilitiesChanged](t, read(t, conn, agentproto.TypeCapabilitiesChanged))
	if len(changed.Devices) != 1 || changed.Devices[0].DeviceID != testDevice {
		t.Fatalf("capabilities changed = %+v", changed)
	}
	prober.mu.Lock()
	defer prober.mu.Unlock()
	if want := []string{"", testDevice}; strings.Join(prober.probed, ",") != strings.Join(want, ",") {
		t.Fatalf("probed %q, want startup then %q", prober.probed, testDevice)
	}
}

func TestClientFollowsDrainAndWelcome(t *testing.T) {
	server := newFakeServer(t)
	client, _, _ := newClient(t, server, testSecret)
	run(t, client)
	conn := server.accept(t)
	read(t, conn, agentproto.TypeHello)
	welcome(t, conn, false)

	write(t, conn, agentproto.TypeDrain, agentproto.Drain{})
	waitFor(t, client.Draining)

	welcome(t, conn, false)
	waitFor(t, func() bool { return !client.Draining() })
}

func TestClientReconnectsAfterTheServerDrops(t *testing.T) {
	server := newFakeServer(t)
	client, _, id := newClient(t, server, testSecret)
	client.random = func() float64 { return 0 }
	run(t, client)

	first := server.accept(t)
	read(t, first, agentproto.TypeHello)
	welcome(t, first, false)
	_ = first.Close(websocket.StatusGoingAway, "restart")
	<-server.headers

	second := server.accept(t)
	if hello := decode[agentproto.Hello](t, read(t, second, agentproto.TypeHello)); hello.AgentID != id {
		t.Fatalf("reconnected as %s, want %s", hello.AgentID, id)
	}
}

func TestSessionReportsARejectedSecret(t *testing.T) {
	server := newFakeServer(t)
	client, _, _ := newClient(t, server, "wrong")

	welcomed, err := client.session(context.Background())

	if welcomed || err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("welcomed %v, err %v; want a 401 error", welcomed, err)
	}
}

// fakeJobs accepts offers for one job and holds it encoding.
type fakeJobs struct {
	noJobs
	mu        sync.Mutex
	accept    domain.JobID
	cancelled []domain.JobID
}

func (j *fakeJobs) Offer(offer agentproto.JobOffer) (bool, agentproto.RejectReason) {
	if offer.Spec.Job != j.accept {
		return false, agentproto.RejectDeviceBusy
	}
	return true, ""
}

func (j *fakeJobs) Cancel(job domain.JobID) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.cancelled = append(j.cancelled, job)
}

func (j *fakeJobs) Heartbeats() []agentproto.JobHeartbeat {
	return []agentproto.JobHeartbeat{{Job: j.accept, Stage: agentproto.StageEncoding, Progress: 0.5}}
}

func (j *fakeJobs) Resumable() []agentproto.ResumableJob {
	return []agentproto.ResumableJob{{Job: j.accept, SourceETag: `"1-2-3"`, BytesHave: 2}}
}

func TestClientAnswersOffersAndHeartbeatsItsJobs(t *testing.T) {
	server := newFakeServer(t)
	client, _, _ := newClient(t, server, testSecret)
	jobs := &fakeJobs{accept: domain.NewID[domain.JobID]()}
	client.SetJobs(jobs)
	run(t, client)
	conn := server.accept(t)
	if hello := decode[agentproto.Hello](t, read(t, conn, agentproto.TypeHello)); len(hello.ResumableJobs) != 1 || hello.ResumableJobs[0].Job != jobs.accept {
		t.Fatalf("hello resumable = %+v", hello.ResumableJobs)
	}
	welcome(t, conn, false)

	busy := domain.NewID[domain.JobID]()
	write(t, conn, agentproto.TypeJobOffer, agentproto.JobOffer{Spec: agentproto.JobSpec{Job: busy}})
	if reject := decode[agentproto.JobReject](t, read(t, conn, agentproto.TypeJobReject)); reject.Job != busy || reject.Reason != agentproto.RejectDeviceBusy {
		t.Fatalf("reject = %+v", reject)
	}
	write(t, conn, agentproto.TypeJobOffer, agentproto.JobOffer{Spec: agentproto.JobSpec{Job: jobs.accept}})
	if accept := decode[agentproto.JobAccept](t, read(t, conn, agentproto.TypeJobAccept)); accept.Job != jobs.accept {
		t.Fatalf("accept = %+v", accept)
	}
	heartbeat := decode[agentproto.Heartbeat](t, read(t, conn, agentproto.TypeHeartbeat))
	if len(heartbeat.Jobs) != 1 || heartbeat.Jobs[0].Job != jobs.accept || heartbeat.Jobs[0].Stage != agentproto.StageEncoding {
		t.Fatalf("heartbeat jobs = %+v", heartbeat.Jobs)
	}

	write(t, conn, agentproto.TypeCancel, agentproto.Cancel{Job: jobs.accept})
	waitFor(t, func() bool {
		jobs.mu.Lock()
		defer jobs.mu.Unlock()
		return len(jobs.cancelled) == 1
	})
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(frameTimeout)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition never held")
		}
		time.Sleep(testHeartbeat)
	}
}
