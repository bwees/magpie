package transcode

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	acquisitionapi "magpie/internal/acquisition/api"
	catalogapi "magpie/internal/catalog/api"
	catalogtest "magpie/internal/catalog/api/contracttest"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	librarytest "magpie/internal/library/api/contracttest"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/config"
	"magpie/internal/platform/db"
	"magpie/internal/platform/db/dbtest"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/settings"
	"magpie/internal/transcode/api"
	"magpie/internal/transcode/api/agentproto"

	"github.com/mikestefanello/backlite"
	"gorm.io/gorm"
)

const (
	eventuallyWithin = 5 * time.Second
	eventuallyEvery  = 5 * time.Millisecond

	movieMbps   = 12
	movieWidth  = 1920
	movieHeight = 1080
	movieLength = 2 * time.Hour
)

type fixture struct {
	t         *testing.T
	db        *db.DB
	clock     *testClock
	store     *settings.Store
	service   *AgentService
	jobs      *JobService
	hub       *AgentHub
	socket    *AgentSocket
	transfers *Transfers
	gate      *TransferGate
	lifetime  *Lifetime
	library   *librarytest.Fake
	catalog   *catalogtest.Fake
	prober    *testProber
	queue     *testQueue
	root      string
	server    *httptest.Server
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	d := dbtest.Open(t, Migrations())
	clock := &testClock{now: testStart}
	store := settings.NewStore(d)
	b := bus.New()
	agents := NewAgentRepository(d)
	hub := NewAgentHub()
	service := NewAgentService(d, b, agents, hub, store, DefaultTimings)
	service.now = clock.Now
	gate := NewTransferGate()
	gate.now = clock.Now
	lifetime := NewLifetime()
	t.Cleanup(lifetime.Stop)
	wake := NewWake()
	f := &fixture{
		t: t, db: d, clock: clock, store: store, service: service, hub: hub, gate: gate, lifetime: lifetime,
		library: librarytest.NewFake(librarytest.Fixture{}), catalog: catalogtest.NewFake(catalogtest.Fixture{}),
		prober: &testProber{infos: map[string]libraryapi.MediaInfo{}}, queue: &testQueue{}, root: t.TempDir(),
	}
	f.jobs = NewJobService(JobDeps{
		DB: d, Bus: b, Jobs: NewJobRepository(d, agents, store), Agents: agents, Hub: hub, Store: store,
		Library: f.library, Catalog: f.catalog, Files: osFileStat{}, Prober: f.prober, Queue: f.queue, Gate: gate,
		Lifetime: lifetime, Wake: wake, Timings: DefaultTimings,
	})
	f.jobs.now = clock.Now
	f.socket = NewAgentSocket(service, f.jobs, hub, wake, AgentSecret(testSecret), DefaultTimings)
	f.transfers = NewTransfers(f.jobs, gate, AgentSecret(testSecret), lifetime)
	controller := NewController(service, f.jobs, f.socket, f.transfers, testSecret)
	f.server = httptest.NewServer(httpserver.Build(config.Env{}, httpserver.ModeServe, []httpserver.Controller{controller}).Mux)
	t.Cleanup(f.server.Close)
	return f
}

// osFileStat is the disk adapter's FileStat, which the transcode package can't import.
type osFileStat struct{}

func (osFileStat) Stat(path string) (FileInfo, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return FileInfo{}, err
	}
	info := FileInfo{Size: fi.Size(), ModTime: fi.ModTime()}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		info.Device, info.Inode = uint64(st.Dev), st.Ino
	}
	return info, nil
}

type testProber struct {
	mu    sync.Mutex
	infos map[string]libraryapi.MediaInfo
}

func (p *testProber) set(path string, info libraryapi.MediaInfo) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.infos[path] = info
}

func (p *testProber) Probe(_ context.Context, path string) (libraryapi.MediaInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	info, ok := p.infos[path]
	if !ok {
		return libraryapi.MediaInfo{}, fmt.Errorf("probe %s: %w", path, fs.ErrNotExist)
	}
	return info, nil
}

type testQueue struct {
	mu    sync.Mutex
	tasks []backlite.Task
}

func (q *testQueue) Enqueue(_ context.Context, task backlite.Task) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.tasks = append(q.tasks, task)
	return nil
}

func (q *testQueue) EnqueueTx(_ *gorm.DB, task backlite.Task) error {
	return q.Enqueue(context.Background(), task)
}

func (q *testQueue) drain() []backlite.Task {
	q.mu.Lock()
	defer q.mu.Unlock()
	tasks := q.tasks
	q.tasks = nil
	return tasks
}

// runVerify runs the verify tasks queued so far.
func (f *fixture) runVerify() {
	f.t.Helper()
	for _, task := range f.queue.drain() {
		if v, ok := task.(verifyTask); ok {
			if err := f.jobs.Verify(context.Background(), v.Job); err != nil {
				f.t.Fatal(err)
			}
		}
	}
}

// sourceFile is a library file on disk in the fixture's root folder.
type sourceFile struct {
	libraryapi.MediaFile
	content []byte
}

func h264Movie(size int64) libraryapi.MediaInfo {
	return libraryapi.MediaInfo{
		Container:   "matroska,webm",
		Duration:    movieLength,
		BitrateKbps: movieMbps * kbpsPerMbps,
		Width:       movieWidth,
		Height:      movieHeight,
		HDR:         libraryapi.HDRNone,
		Streams: []libraryapi.Stream{
			{Index: 0, Kind: libraryapi.StreamVideo, Codec: "h264", BitDepth: 8},
			{Index: 1, Kind: libraryapi.StreamAudio, Codec: "eac3", Language: "en"},
			{Index: 2, Kind: libraryapi.StreamAudio, Codec: "aac", Language: "ja"},
			{Index: 3, Kind: libraryapi.StreamSubtitle, Codec: "subrip", Language: "en"},
		},
	}
}

// addFile writes a movie to disk and puts it in the fake library.
func (f *fixture) addFile(info libraryapi.MediaInfo) sourceFile {
	f.t.Helper()
	id := domain.NewID[domain.MediaFileID]()
	content := bytes.Repeat([]byte("source "+string(id)+"\n"), 64)
	path := filepath.Join(f.root, "Movie (2021)", string(id)+".mkv")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		f.t.Fatal(err)
	}
	title := catalogapi.Title{ID: domain.NewID[domain.TitleID](), Kind: domain.MediaKindMovie, Name: "Movie", Year: 2021}
	f.catalog.PutTitle(title)
	file := libraryapi.MediaFile{
		ID: id, Title: title.ID, Episodes: []domain.EpisodeRef{}, RootFolder: f.root, Path: path,
		Size: int64(len(content)), MediaInfo: info, Origin: domain.OriginDownload, ImportedAt: testStart,
	}
	f.library.Put(librarytest.StoredFile{File: file, ETag: "library-" + string(id)})
	return sourceFile{MediaFile: file, content: content}
}

// replaceSource writes new bytes over the file, as an upgrade or a tag edit would.
func (f *fixture) replaceSource(file sourceFile) {
	f.t.Helper()
	if err := os.WriteFile(file.Path, append(file.content, []byte("changed\n")...), 0o644); err != nil {
		f.t.Fatal(err)
	}
	stored := librarytest.StoredFile{File: file.MediaFile, ETag: "library-changed-" + string(file.ID)}
	f.library.Put(stored)
}

func (f *fixture) cutoff(file sourceFile, origin domain.Origin) {
	f.t.Helper()
	met := acquisitionapi.CutoffMet{File: file.ID, Title: file.Title, Origin: origin}
	f.handle(newCutoffMetHandler(f.jobs), met)
}

// handle runs a bus handler on one message in its own transaction, as the dispatcher would.
func (f *fixture) handle(h bus.Handler, m domain.Message) {
	f.t.Helper()
	err := f.db.Write(context.Background(), func(tx *gorm.DB) error { return h.Handle(context.Background(), tx, bus.EventOf(m)) })
	if err != nil {
		f.t.Fatal(err)
	}
	f.lifetime.Wait()
}

func (f *fixture) onlyJob(file sourceFile) jobRow {
	f.t.Helper()
	var rows []jobRow
	if err := f.db.Read(context.Background()).Where("media_file_id = ?", file.ID).Find(&rows).Error; err != nil {
		f.t.Fatal(err)
	}
	if len(rows) != 1 {
		f.t.Fatalf("file %s has %d jobs, want 1", file.ID, len(rows))
	}
	return rows[0]
}

func (f *fixture) job(id domain.JobID) jobRow {
	f.t.Helper()
	row, err := jobIn(f.db.Read(context.Background()), id)
	if err != nil {
		f.t.Fatal(err)
	}
	return row
}

// eventually waits for the job to satisfy ok, since the socket handles frames on its own goroutine.
func (f *fixture) eventually(id domain.JobID, what string, ok func(jobRow) bool) jobRow {
	f.t.Helper()
	deadline := time.Now().Add(eventuallyWithin)
	for {
		row := f.job(id)
		if ok(row) {
			return row
		}
		if time.Now().After(deadline) {
			f.t.Fatalf("job %s never got %s: %+v", id, what, row)
		}
		time.Sleep(eventuallyEvery)
	}
}

func (f *fixture) inState(id domain.JobID, state api.JobState) jobRow {
	f.t.Helper()
	return f.eventually(id, string(state), func(r jobRow) bool { return r.State == state })
}

func (f *fixture) schedule() {
	f.t.Helper()
	if err := f.jobs.Schedule(context.Background()); err != nil {
		f.t.Fatal(err)
	}
	f.lifetime.Wait()
}

// capableHello is an agent with one NVIDIA device that passed HEVC 10-bit encode.
func capableHello(hostname string) agentproto.Hello {
	hello := testHello(domain.NewID[domain.AgentID]())
	hello.Hostname = hostname
	return hello
}

// offered schedules and returns the offer the agent receives.
func (f *fixture) offered(a *testAgent) agentproto.JobOffer {
	f.t.Helper()
	f.schedule()
	return decodeBody[agentproto.JobOffer](f.t, a.expect(agentproto.TypeJobOffer))
}

// lease offers the next job to the agent and accepts it.
func (f *fixture) lease(a *testAgent) agentproto.JobSpec {
	f.t.Helper()
	offer := f.offered(a)
	a.send(agentproto.TypeJobAccept, agentproto.JobAccept{Job: offer.Spec.Job})
	f.inState(offer.Spec.Job, api.StateLeased)
	return offer.Spec
}

// beat sends a heartbeat listing one job and waits until the server recorded its stage.
func (f *fixture) beat(a *testAgent, job domain.JobID, stage agentproto.Stage, progress float64) {
	f.t.Helper()
	a.send(agentproto.TypeHeartbeat, agentproto.Heartbeat{ScratchFree: testScratchFree, Jobs: []agentproto.JobHeartbeat{{Job: job, Stage: stage, Progress: progress}}})
	f.eventually(job, string(stage), func(r jobRow) bool { return r.State == stageStates[stage] && r.Progress == progress })
}

// request sends an agent HTTP request to the fixture's server.
func (f *fixture) request(agent domain.AgentID, method, path string, header http.Header, body []byte) *http.Response {
	f.t.Helper()
	req, err := http.NewRequest(method, f.server.URL+path, bytes.NewReader(body))
	if err != nil {
		f.t.Fatal(err)
	}
	for k, v := range header {
		req.Header[k] = v
	}
	req.Header.Set(agentproto.HeaderAuthorization, agentproto.BearerPrefix+testSecret)
	req.Header.Set(agentproto.HeaderAgentID, string(agent))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func readAll(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// put uploads a chunk of the output at offset; complete marks the last chunk with the digest of whole.
func (f *fixture) put(agent domain.AgentID, job domain.JobID, offset int64, chunk, whole []byte, complete bool) *http.Response {
	f.t.Helper()
	header := http.Header{}
	header.Set(agentproto.HeaderUploadOffset, strconv.FormatInt(offset, 10))
	header.Set(agentproto.HeaderUploadLength, strconv.Itoa(len(whole)))
	if complete {
		sum := sha256.Sum256(whole)
		header.Set(agentproto.HeaderUploadComplete, agentproto.UploadCompleteTrue)
		header.Set(agentproto.HeaderContentDigest, agentproto.DigestSHA256Prefix+base64.StdEncoding.EncodeToString(sum[:])+agentproto.DigestSuffix)
	}
	return f.request(agent, http.MethodPut, agentproto.OutputPath(job), header, chunk)
}

// passingReport is what an agent reports for an output that passed every check.
func passingReport(output []byte) agentproto.VerifyReport {
	sum := sha256.Sum256(output)
	return agentproto.VerifyReport{
		DecodeOK: true, StreamMapOK: true, CopiedStreamsOK: true, OutputSize: int64(len(output)),
		SHA256: fmt.Sprintf("%x", sum),
	}
}

// hevcOutput is how ffprobe reads a good output of the movie.
func hevcOutput() libraryapi.MediaInfo {
	info := h264Movie(0)
	info.Streams[0] = libraryapi.Stream{Index: 0, Kind: libraryapi.StreamVideo, Codec: api.VideoCodecHEVC, BitDepth: tenBit}
	return info
}

// uploadAndReport puts the whole output, then reports the result, and waits for VERIFYING.
func (f *fixture) uploadAndReport(a *testAgent, spec agentproto.JobSpec, output []byte) {
	f.t.Helper()
	f.beat(a, spec.Job, agentproto.StageUploading, 0)
	if resp := f.put(a.id, spec.Job, 0, output, output, true); resp.StatusCode != http.StatusNoContent {
		f.t.Fatalf("upload = %d: %s", resp.StatusCode, readAll(f.t, resp))
	}
	a.send(agentproto.TypeJobResult, agentproto.JobResult{Job: spec.Job, Outcome: agentproto.OutcomeUploaded, Verify: passingReport(output)})
	f.inState(spec.Job, api.StateVerifying)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return !errors.Is(err, fs.ErrNotExist)
}
