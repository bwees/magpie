package jobs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"magpie/agent/encode"
	"magpie/agent/transfer"
	"magpie/internal/domain"
	"magpie/internal/transcode/api/agentproto"
)

const (
	testDevice = "cuda:0"
	testETag   = `"7-64-1700000000"`
	waitFor    = 5 * time.Second
	pollEvery  = 5 * time.Millisecond

	sourceProbe = `{"format":{"duration":"60.0"},"streams":[
		{"index":0,"codec_type":"video","codec_name":"h264","pix_fmt":"yuv420p"},
		{"index":1,"codec_type":"audio","codec_name":"aac"}]}`
	outputProbe = `{"format":{"duration":"60.2"},"streams":[
		{"index":0,"codec_type":"video","codec_name":"hevc","pix_fmt":"p010le"},
		{"index":1,"codec_type":"audio","codec_name":"aac"}]}`
)

var sourceContent = bytes.Repeat([]byte("s"), 64)

// server stands in for the server's byte endpoints of one job.
type server struct {
	mu       sync.Mutex
	etag     string
	output   []byte
	ranges   []string
	hold     chan struct{}
	received chan struct{}
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasSuffix(r.URL.Path, "/source"):
		s.mu.Lock()
		s.ranges = append(s.ranges, r.Header.Get("Range"))
		etag, hold := s.etag, s.hold
		s.mu.Unlock()
		w.Header().Set("ETag", etag)
		if hold != nil && r.Header.Get("Range") == "" {
			w.Header().Set("Content-Length", strconv.Itoa(len(sourceContent)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(sourceContent[:10])
			w.(http.Flusher).Flush()
			close(s.received)
			<-hold
			return
		}
		http.ServeContent(w, r, "source", time.Time{}, bytes.NewReader(sourceContent))
	case r.Method == http.MethodHead:
		s.mu.Lock()
		w.Header().Set(agentproto.HeaderUploadOffset, strconv.Itoa(len(s.output)))
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		body, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.output = append(s.output, body...)
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *server) uploaded() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.output
}

// runner answers ffprobe and ffmpeg like a clean encode that writes outputSize bytes.
type runner struct {
	outputSize   int
	sourceHashes string
	outputHashes string
}

func (r *runner) Run(_ context.Context, c encode.Command) (string, string, error) {
	line := c.String()
	switch {
	case c.Name == encode.DefaultFFprobe && strings.Contains(line, "output.mkv"):
		return outputProbe, "", nil
	case c.Name == encode.DefaultFFprobe:
		return sourceProbe, "", nil
	case strings.Contains(line, "streamhash") && strings.Contains(line, "output.mkv"):
		return r.outputHashes, "", nil
	case strings.Contains(line, "streamhash"):
		return r.sourceHashes, "", nil
	case strings.Contains(line, "-progress"):
		return "", "", os.WriteFile(c.Args[len(c.Args)-1], bytes.Repeat([]byte("o"), r.outputSize), filePerm)
	}
	return "", "", nil
}

type sender struct {
	mu      sync.Mutex
	results []agentproto.JobResult
}

func (s *sender) Send(_ context.Context, _ agentproto.MessageType, body any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results = append(s.results, body.(agentproto.JobResult))
	return nil
}

func (s *sender) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.results)
}

func (s *sender) result(t *testing.T) agentproto.JobResult {
	t.Helper()
	deadline := time.Now().Add(waitFor)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		if len(s.results) > 0 {
			defer s.mu.Unlock()
			return s.results[0]
		}
		s.mu.Unlock()
		time.Sleep(pollEvery)
	}
	t.Fatal("no result reported")
	return agentproto.JobResult{}
}

type fixture struct {
	server  *server
	runner  *runner
	sender  *sender
	manager *Manager
	scratch string
	free    int64
	cancel  context.CancelFunc
	done    chan struct{}
}

func newFixture(t *testing.T, scratch string) *fixture {
	t.Helper()
	f := &fixture{
		server:  &server{etag: testETag},
		runner:  &runner{outputSize: 32, sourceHashes: "0,a,SHA256=aa\n", outputHashes: "0,a,SHA256=aa\n"},
		sender:  &sender{},
		scratch: scratch,
		free:    1 << 30,
	}
	httpServer := httptest.NewServer(f.server)
	t.Cleanup(httpServer.Close)
	serverURL, _ := url.Parse(httpServer.URL)
	transfers := transfer.New(httpServer.Client(), serverURL)
	transfers.RetryWait = time.Millisecond
	f.manager = NewManager(Options{
		ScratchDir:  scratch,
		Transfers:   transfers,
		Encoder:     &encode.Encoder{Runner: f.runner, FFmpeg: encode.DefaultFFmpeg, FFprobe: encode.DefaultFFprobe},
		ScratchFree: func() (int64, error) { return f.free, nil },
		DirectRead:  func(s agentproto.SourceRef) encode.Input { return encode.Input{Path: transfers.SourceURL(s.URL)} },
	}, f.sender)
	f.manager.SetDevices([]agentproto.DeviceReport{{
		DeviceID: testDevice, Vendor: agentproto.VendorNVIDIA,
		Capabilities: []agentproto.CapabilityResult{{Capability: agentproto.CapEncHEVC10, OK: true}},
	}})
	f.manager.Configure(agentproto.Welcome{Mode: agentproto.ModeDownloadFirst, Slots: map[string]int{testDevice: 1}})
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel, f.done = cancel, make(chan struct{})
	go func() {
		defer close(f.done)
		f.manager.Run(ctx)
	}()
	t.Cleanup(f.stop)
	return f
}

func (f *fixture) stop() {
	f.cancel()
	<-f.done
}

func offer(id domain.JobID) agentproto.JobOffer {
	return agentproto.JobOffer{Spec: agentproto.JobSpec{
		Job: id, Kind: agentproto.KindTranscode, DeviceID: testDevice,
		Source: agentproto.SourceRef{URL: agentproto.SourcePath(id), Size: int64(len(sourceContent)), ETag: testETag},
		Transcode: &agentproto.TranscodeParams{
			Codec: "hevc", BitDepth: 10, Encoder: "hevc_nvenc", KeepAudio: []int{1}, KeepSubs: []int{}, MinSavingPct: 15,
		},
		OutputURL: agentproto.OutputPath(id), OutputName: string(id) + ".mkv",
	}}
}

func (f *fixture) accept(t *testing.T, o agentproto.JobOffer) {
	t.Helper()
	if ok, reason := f.manager.Offer(o); !ok {
		t.Fatalf("offer rejected: %s", reason)
	}
}

func TestAJobRunsFromDownloadToUpload(t *testing.T) {
	f := newFixture(t, t.TempDir())
	id := domain.NewID[domain.JobID]()

	f.accept(t, offer(id))
	result := f.sender.result(t)

	output := f.server.uploaded()
	sum := sha256.Sum256(output)
	if result.Outcome != agentproto.OutcomeUploaded || result.Verify.SHA256 != hex.EncodeToString(sum[:]) || len(output) != f.runner.outputSize {
		t.Fatalf("result = %+v with %d bytes uploaded", result, len(output))
	}
	if !result.Verify.DecodeOK || !result.Verify.StreamMapOK || !result.Verify.CopiedStreamsOK || result.Verify.OutputSize != int64(f.runner.outputSize) {
		t.Fatalf("report = %+v", result.Verify)
	}
	if _, err := os.Stat(filepath.Join(f.scratch, jobsDirName, string(id))); !os.IsNotExist(err) {
		t.Fatal("job scratch left behind")
	}
	if beats := f.manager.Heartbeats(); len(beats) != 0 {
		t.Fatalf("heartbeats = %+v, want the finished job gone", beats)
	}
}

func TestASmallSavingIsNotWorthUploading(t *testing.T) {
	f := newFixture(t, t.TempDir())
	f.runner.outputSize = len(sourceContent) * 91 / 100

	f.accept(t, offer(domain.NewID[domain.JobID]()))

	if result := f.sender.result(t); result.Outcome != agentproto.OutcomeNotWorthIt {
		t.Fatalf("result = %+v, want NOT_WORTH_IT", result)
	}
	if n := len(f.server.uploaded()); n != 0 {
		t.Fatalf("%d bytes uploaded, want none", n)
	}
}

// Seed 18, the agent's part.
func TestAnOutputFailingItsChecksIsNotUploaded(t *testing.T) {
	f := newFixture(t, t.TempDir())
	f.runner.outputHashes = "0,a,SHA256=00\n"

	f.accept(t, offer(domain.NewID[domain.JobID]()))

	result := f.sender.result(t)
	if result.Outcome != agentproto.OutcomeFailed || result.Error != agentproto.ErrorVerifyFailed || result.Verify.CopiedStreamsOK {
		t.Fatalf("result = %+v, want FAILED VERIFY_FAILED", result)
	}
	if n := len(f.server.uploaded()); n != 0 {
		t.Fatalf("%d bytes uploaded, want none", n)
	}
}

// Seed 14, the agent's part.
func TestAChangedSourceFailsTheJob(t *testing.T) {
	f := newFixture(t, t.TempDir())
	f.server.etag = `"8-65-1700000001"`

	f.accept(t, offer(domain.NewID[domain.JobID]()))

	if result := f.sender.result(t); result.Outcome != agentproto.OutcomeFailed || result.Error != agentproto.ErrorSourceChanged {
		t.Fatalf("result = %+v, want FAILED SOURCE_CHANGED", result)
	}
}

func TestOffersThatDontFitAreRejected(t *testing.T) {
	f := newFixture(t, t.TempDir())
	f.server.hold, f.server.received = make(chan struct{}), make(chan struct{})
	t.Cleanup(func() { close(f.server.hold) })
	f.accept(t, offer(domain.NewID[domain.JobID]()))
	<-f.server.received

	av1 := offer(domain.NewID[domain.JobID]())
	av1.Spec.Transcode.Codec = "av1"
	prefetch := offer(domain.NewID[domain.JobID]())
	prefetch.Prefetch = true
	for _, tt := range []struct {
		name  string
		offer agentproto.JobOffer
		want  agentproto.RejectReason
	}{
		{"device busy", offer(domain.NewID[domain.JobID]()), agentproto.RejectDeviceBusy},
		{"capability", av1, agentproto.RejectCapability},
	} {
		if ok, reason := f.manager.Offer(tt.offer); ok || reason != tt.want {
			t.Errorf("%s: accepted %v reason %s, want %s", tt.name, ok, reason, tt.want)
		}
	}
	if ok, _ := f.manager.Offer(prefetch); !ok {
		t.Error("prefetch rejected next to one running job")
	}

	f.free = 0
	f.manager.Configure(agentproto.Welcome{Mode: agentproto.ModeDownloadFirst, Slots: map[string]int{testDevice: 4}})
	if ok, reason := f.manager.Offer(offer(domain.NewID[domain.JobID]())); ok || reason != agentproto.RejectNoScratch {
		t.Errorf("no scratch: accepted %v reason %s", ok, reason)
	}
	f.manager.Drain()
	if ok, reason := f.manager.Offer(offer(domain.NewID[domain.JobID]())); ok || reason != agentproto.RejectDraining {
		t.Errorf("draining: accepted %v reason %s", ok, reason)
	}
}

func TestACancelledJobDropsItsScratchWithoutAResult(t *testing.T) {
	f := newFixture(t, t.TempDir())
	f.server.hold, f.server.received = make(chan struct{}), make(chan struct{})
	t.Cleanup(func() { close(f.server.hold) })
	id := domain.NewID[domain.JobID]()
	f.accept(t, offer(id))
	<-f.server.received

	f.manager.Cancel(id)

	waitUntil(t, "the job to go", func() bool { return len(f.manager.Heartbeats()) == 0 })
	if _, err := os.Stat(filepath.Join(f.scratch, jobsDirName, string(id))); !os.IsNotExist(err) {
		t.Fatal("scratch kept after a cancel")
	}
	if n := f.sender.count(); n != 0 {
		t.Fatalf("%d results, want none", n)
	}
}

// Seed 13, the agent's part: a restart keeps the partial source, Hello lists it, and the download
// resumes at the bytes it has.
func TestARestartedAgentResumesItsDownload(t *testing.T) {
	scratch := t.TempDir()
	f := newFixture(t, scratch)
	f.server.hold, f.server.received = make(chan struct{}), make(chan struct{})
	id := domain.NewID[domain.JobID]()
	f.accept(t, offer(id))
	<-f.server.received
	waitUntil(t, "the first bytes on disk", func() bool {
		beats := f.manager.Heartbeats()
		return len(beats) == 1 && beats[0].Progress > 0
	})
	f.stop()
	close(f.server.hold)

	restarted := newFixture(t, scratch)
	resumable := restarted.manager.Resumable()
	if len(resumable) != 1 || resumable[0].Job != id || resumable[0].SourceETag != testETag || resumable[0].BytesHave == 0 {
		t.Fatalf("resumable = %+v, want the job with its ETag and the bytes kept", resumable)
	}
	restarted.accept(t, offer(id))

	if result := restarted.sender.result(t); result.Outcome != agentproto.OutcomeUploaded {
		t.Fatalf("result = %+v", result)
	}
	if first := restarted.server.ranges[0]; first != "bytes="+strconv.FormatInt(resumable[0].BytesHave, 10)+"-" {
		t.Fatalf("first Range = %q, want a resume from %d", first, resumable[0].BytesHave)
	}
}

func TestAPrefetchedJobWaitsForStart(t *testing.T) {
	f := newFixture(t, t.TempDir())
	o := offer(domain.NewID[domain.JobID]())
	o.Prefetch = true

	f.accept(t, o)
	waitUntil(t, "the source to be fetched", func() bool {
		beats := f.manager.Heartbeats()
		return len(beats) == 1 && beats[0].Stage == agentproto.StageFetching && beats[0].Progress == 1
	})
	if f.sender.count() != 0 {
		t.Fatal("a prefetched job encoded before Start")
	}

	f.manager.Start(o.Spec.Job)

	if result := f.sender.result(t); result.Outcome != agentproto.OutcomeUploaded {
		t.Fatalf("result = %+v", result)
	}
}

func waitUntil(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(waitFor)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(pollEvery)
	}
}
