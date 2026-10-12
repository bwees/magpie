package flows_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"magpie/internal/domain"
	"magpie/internal/transcode/api/agentproto"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const (
	agentDevice   = "cuda:0"
	agentHostname = "flow-agent"
	// agentScratch is the free scratch space the agent reports, more than any test file needs.
	agentScratch int64 = 1 << 40
	firstOffset        = "0"
)

// fakeAgent speaks the agent protocol over the real control socket and byte endpoints, as the agent binary
// does, with one NVIDIA device that passed one capability.
type fakeAgent struct {
	t      *testing.T
	id     domain.AgentID
	server string
	conn   *websocket.Conn
}

func connectAgent(h *harness, capability agentproto.Capability) *fakeAgent {
	h.t.Helper()
	server := httptest.NewServer(h.server.Mux)
	h.t.Cleanup(server.Close)
	a := &fakeAgent{t: h.t, id: domain.NewID[domain.AgentID](), server: server.URL}
	conn, _, err := websocket.Dial(context.Background(), server.URL+agentproto.ControlPath, &websocket.DialOptions{HTTPHeader: a.headers()})
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { _ = conn.CloseNow() })
	a.conn = conn
	a.send(agentproto.TypeHello, agentproto.Hello{
		AgentID: a.id, Hostname: agentHostname, Mode: agentproto.ModeDownloadFirst, ScratchFree: agentScratch,
		Devices: []agentproto.DeviceReport{{
			DeviceID: agentDevice, Vendor: agentproto.VendorNVIDIA,
			Capabilities: []agentproto.CapabilityResult{{Capability: capability, OK: true}},
		}},
	})
	return a
}

func (a *fakeAgent) headers() http.Header {
	header := http.Header{}
	header.Set(agentproto.HeaderAuthorization, agentproto.BearerPrefix+flowAgentSecret)
	header.Set(agentproto.HeaderAgentID, string(a.id))
	return header
}

func (a *fakeAgent) send(t agentproto.MessageType, body any) {
	a.t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		a.t.Fatal(err)
	}
	if err := wsjson.Write(context.Background(), a.conn, agentproto.Envelope{Type: t, ID: string(domain.NewID[domain.JobID]()), Body: raw}); err != nil {
		a.t.Fatal(err)
	}
}

// offer reads frames until the server offers a job.
func (a *fakeAgent) offer() agentproto.JobOffer {
	a.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), flowWithin)
	defer cancel()
	for {
		var env agentproto.Envelope
		if err := wsjson.Read(ctx, a.conn, &env); err != nil {
			a.t.Fatalf("waiting for a job offer: %v", err)
		}
		if env.Type != agentproto.TypeJobOffer {
			continue
		}
		var offer agentproto.JobOffer
		if err := json.Unmarshal(env.Body, &offer); err != nil {
			a.t.Fatal(err)
		}
		return offer
	}
}

// request calls a byte endpoint as this agent and returns the status and body.
func (a *fakeAgent) request(method, path string, header http.Header, body []byte) (int, []byte) {
	a.t.Helper()
	req, err := http.NewRequest(method, a.server+path, bytes.NewReader(body))
	if err != nil {
		a.t.Fatal(err)
	}
	for name, values := range a.headers() {
		header[name] = values
	}
	req.Header = header
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		a.t.Fatal(err)
	}
	return resp.StatusCode, got
}

func (a *fakeAgent) accept(spec agentproto.JobSpec) {
	a.t.Helper()
	a.send(agentproto.TypeJobAccept, agentproto.JobAccept{Job: spec.Job})
}

// fetchSource downloads the job's source, as a DOWNLOAD_FIRST agent does before it encodes.
func (a *fakeAgent) fetchSource(spec agentproto.JobSpec) []byte {
	a.t.Helper()
	status, source := a.request(http.MethodGet, spec.Source.URL, http.Header{}, nil)
	if status != http.StatusOK {
		a.t.Fatalf("source = %d: %s", status, source)
	}
	return source
}

// deliver uploads the whole output in one chunk and reports it as passing every check.
func (a *fakeAgent) deliver(spec agentproto.JobSpec, output []byte) {
	a.t.Helper()
	sum := sha256.Sum256(output)
	header := http.Header{}
	header.Set(agentproto.HeaderUploadOffset, firstOffset)
	header.Set(agentproto.HeaderUploadLength, strconv.Itoa(len(output)))
	header.Set(agentproto.HeaderUploadComplete, agentproto.UploadCompleteTrue)
	header.Set(agentproto.HeaderContentDigest, agentproto.DigestSHA256Prefix+base64.StdEncoding.EncodeToString(sum[:])+agentproto.DigestSuffix)
	if status, body := a.request(http.MethodPut, spec.OutputURL, header, output); status != http.StatusNoContent {
		a.t.Fatalf("upload = %d: %s", status, body)
	}
	a.send(agentproto.TypeJobResult, agentproto.JobResult{
		Job: spec.Job, Outcome: agentproto.OutcomeUploaded,
		Verify: agentproto.VerifyReport{
			DecodeOK: true, StreamMapOK: true, CopiedStreamsOK: true,
			OutputSize: int64(len(output)), SHA256: hex.EncodeToString(sum[:]),
		},
	})
}
