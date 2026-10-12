// Package jobs runs the transcode jobs the server offers: it accepts what fits the agent's slots and
// scratch, fetches the source, encodes and checks the output, uploads it and reports the result.
// Spec: docs/spec/contexts/transcode.md#agent-side-job-run.
package jobs

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"magpie/agent/encode"
	"magpie/agent/transfer"
	"magpie/internal/domain"
	"magpie/internal/transcode/api/agentproto"
)

const (
	jobsDirName = "jobs"
	metaName    = "job.json"
	sourceName  = "source"
	outputName  = "output.mkv"

	dirPerm  = 0o750
	filePerm = 0o640

	// DefaultKeepResumable is how long scratch from before a restart waits for the server to offer its
	// job again before it is deleted.
	DefaultKeepResumable = 10 * time.Minute

	// Scratch a job needs per byte of source, as the server's scheduler counts it.
	scratchFactorDownloadFirst = 2.2
	scratchFactorDirectRead    = 1.2

	percent = 100
)

// encodeCapability is what a device must have passed to encode each target codec.
var encodeCapability = map[string]agentproto.Capability{
	"hevc": agentproto.CapEncHEVC10,
	"av1":  agentproto.CapEncAV110,
}

// Sender delivers a frame to the server on the control channel.
type Sender interface {
	Send(ctx context.Context, messageType agentproto.MessageType, body any) error
}

type Options struct {
	ScratchDir  string
	Transfers   *transfer.Client
	Encoder     *encode.Encoder
	ScratchFree func() (int64, error)
	// DirectRead returns the input ffmpeg reads the source from in direct-read mode.
	DirectRead    func(source agentproto.SourceRef) encode.Input
	KeepResumable time.Duration
}

// Manager holds the agent's jobs.
type Manager struct {
	opts   Options
	sender Sender

	// ctx is the jobs' context; stop ends it when Run's context does.
	ctx  context.Context
	stop context.CancelFunc

	mu       sync.Mutex
	running  map[domain.JobID]*job
	devices  []agentproto.DeviceReport
	slots    map[string]int
	mode     agentproto.Mode
	draining bool
	pending  []agentproto.JobResult
	wg       sync.WaitGroup
}

func NewManager(opts Options, sender Sender) *Manager {
	if opts.KeepResumable == 0 {
		opts.KeepResumable = DefaultKeepResumable
	}
	ctx, stop := context.WithCancel(context.Background())
	return &Manager{opts: opts, sender: sender, ctx: ctx, stop: stop, running: map[domain.JobID]*job{}, mode: agentproto.ModeDownloadFirst}
}

// job is one job the agent holds.
type job struct {
	spec agentproto.JobSpec
	dir  string

	cancel context.CancelFunc
	// started is closed when a prefetched job may encode.
	started chan struct{}

	mu        sync.Mutex
	prefetch  bool
	dropped   bool
	stage     agentproto.Stage
	progress  float64
	speed     float64
	eta       time.Duration
	stageFrom time.Time
}

func (j *job) set(stage agentproto.Stage, progress, speed float64, eta time.Duration) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.stage != stage {
		j.stageFrom = time.Now()
	}
	j.stage, j.progress, j.speed, j.eta = stage, progress, speed, eta
}

// transferred records a transfer's progress with its speed in bytes per second.
func (j *job) transferred(stage agentproto.Stage) transfer.Progress {
	return func(done, total int64) {
		j.mu.Lock()
		from := j.stageFrom
		j.mu.Unlock()
		elapsed := time.Since(from).Seconds()
		var speed float64
		var eta time.Duration
		if elapsed > 0 {
			speed = float64(done) / elapsed
		}
		if speed > 0 {
			eta = time.Duration(float64(total-done) / speed * float64(time.Second))
		}
		j.set(stage, float64(done)/float64(max(total, 1)), speed, eta)
	}
}

// Run deletes scratch nobody resumed after KeepResumable. When ctx ends it stops every job and waits
// for them, keeping their scratch for a resume.
func (m *Manager) Run(ctx context.Context) {
	defer m.wg.Wait()
	defer m.stop()
	timer := time.NewTimer(m.opts.KeepResumable)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
		m.dropStaleScratch()
		<-ctx.Done()
	}
}

// Configure applies the server's Welcome.
func (m *Manager) Configure(welcome agentproto.Welcome) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.slots, m.mode, m.draining = welcome.Slots, welcome.Mode, welcome.Draining
}

// Drain stops the agent from accepting jobs.
func (m *Manager) Drain() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.draining = true
}

// SetDevices records the devices the capability tests found.
func (m *Manager) SetDevices(devices []agentproto.DeviceReport) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.devices = devices
}

// Offer accepts a job that fits and starts it, or says why not.
func (m *Manager) Offer(offer agentproto.JobOffer) (bool, agentproto.RejectReason) {
	m.mu.Lock()
	defer m.mu.Unlock()
	spec := offer.Spec
	if _, held := m.running[spec.Job]; held {
		return true, ""
	}
	if m.draining {
		return false, agentproto.RejectDraining
	}
	device, ok := m.device(spec)
	if !ok {
		return false, agentproto.RejectCapability
	}
	if !m.slotFree(spec.DeviceID, offer.Prefetch) {
		return false, agentproto.RejectDeviceBusy
	}
	dir := filepath.Join(m.opts.ScratchDir, jobsDirName, string(spec.Job))
	if !m.scratchFits(spec, dir) {
		return false, agentproto.RejectNoScratch
	}

	ctx, cancel := context.WithCancel(m.ctx)
	j := &job{spec: spec, dir: dir, cancel: cancel, started: make(chan struct{}), prefetch: offer.Prefetch, stage: agentproto.StageFetching, stageFrom: time.Now()}
	if !offer.Prefetch {
		close(j.started)
	}
	m.running[spec.Job] = j
	mode := m.mode
	m.wg.Go(func() { m.run(ctx, j, device, mode) })
	return true, ""
}

func (m *Manager) device(spec agentproto.JobSpec) (agentproto.DeviceReport, bool) {
	if spec.Kind != agentproto.KindTranscode || spec.Transcode == nil {
		return agentproto.DeviceReport{}, false
	}
	needs := encodeCapability[spec.Transcode.Codec]
	for _, d := range m.devices {
		if d.DeviceID == spec.DeviceID && slices.ContainsFunc(d.Capabilities, func(r agentproto.CapabilityResult) bool { return r.Capability == needs && r.OK }) {
			return d, true
		}
	}
	return agentproto.DeviceReport{}, false
}

// slotFree reports whether the device can take the job: a free slot, or for a prefetch, no other
// prefetched job.
func (m *Manager) slotFree(device string, prefetch bool) bool {
	var encoding, prefetched []domain.JobID
	for id, j := range m.running {
		if j.spec.DeviceID != device {
			continue
		}
		if j.waiting() {
			prefetched = append(prefetched, id)
		} else {
			encoding = append(encoding, id)
		}
	}
	if prefetch {
		return len(prefetched) == 0
	}
	return len(encoding) < m.slots[device]
}

func (j *job) waiting() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.prefetch
}

func (m *Manager) scratchFits(spec agentproto.JobSpec, dir string) bool {
	free, err := m.opts.ScratchFree()
	if err != nil {
		slog.Warn("agent: reading scratch free space", "error", err)
		return false
	}
	factor := scratchFactorDownloadFirst
	if m.mode == agentproto.ModeDirectRead {
		factor = scratchFactorDirectRead
	}
	have, _ := sizeOf(filepath.Join(dir, sourceName))
	return free+have >= int64(factor*float64(spec.Source.Size))
}

// Start lets a prefetched job encode.
func (m *Manager) Start(id domain.JobID) {
	m.mu.Lock()
	j, ok := m.running[id]
	m.mu.Unlock()
	if !ok {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.prefetch {
		j.prefetch = false
		close(j.started)
	}
}

// Cancel stops a job and deletes its scratch.
func (m *Manager) Cancel(id domain.JobID) {
	m.mu.Lock()
	j, ok := m.running[id]
	m.mu.Unlock()
	if !ok {
		return
	}
	j.mu.Lock()
	j.dropped = true
	j.mu.Unlock()
	j.cancel()
}

// Heartbeats lists every job the agent holds, for the heartbeat that extends their leases.
func (m *Manager) Heartbeats() []agentproto.JobHeartbeat {
	m.mu.Lock()
	defer m.mu.Unlock()
	beats := make([]agentproto.JobHeartbeat, 0, len(m.running))
	for id, j := range m.running {
		j.mu.Lock()
		beats = append(beats, agentproto.JobHeartbeat{Job: id, Stage: j.stage, Progress: j.progress, Speed: j.speed, ETA: j.eta})
		j.mu.Unlock()
	}
	slices.SortFunc(beats, func(a, b agentproto.JobHeartbeat) int { return cmp.Compare(a.Job, b.Job) })
	return beats
}

// meta is what a job's scratch dir remembers across a restart.
type meta struct {
	SourceETag string `json:"sourceETag"`
}

// Resumable lists the jobs whose scratch survived a restart, for Hello.
func (m *Manager) Resumable() []agentproto.ResumableJob {
	m.mu.Lock()
	defer m.mu.Unlock()
	resumable := []agentproto.ResumableJob{}
	for _, dir := range m.scratchDirs() {
		id := domain.JobID(filepath.Base(dir))
		if _, held := m.running[id]; held {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, metaName))
		var saved meta
		if err != nil || json.Unmarshal(raw, &saved) != nil {
			continue
		}
		have, _ := sizeOf(filepath.Join(dir, sourceName))
		resumable = append(resumable, agentproto.ResumableJob{Job: id, SourceETag: saved.SourceETag, BytesHave: have})
	}
	return resumable
}

func (m *Manager) scratchDirs() []string {
	dirs, _ := filepath.Glob(filepath.Join(m.opts.ScratchDir, jobsDirName, "*"))
	return dirs
}

func (m *Manager) dropStaleScratch() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, dir := range m.scratchDirs() {
		if _, held := m.running[domain.JobID(filepath.Base(dir))]; !held {
			if err := os.RemoveAll(dir); err != nil {
				slog.Warn("agent: removing stale scratch", "dir", dir, "error", err)
			}
		}
	}
}

// Deliver sends the results the server hasn't received yet. A result whose send fails is kept and
// sent again on the next call.
func (m *Manager) Deliver(ctx context.Context) {
	m.mu.Lock()
	pending := m.pending
	m.pending = nil
	m.mu.Unlock()
	var failed []agentproto.JobResult
	for _, result := range pending {
		if err := m.sender.Send(ctx, agentproto.TypeJobResult, result); err != nil {
			failed = append(failed, result)
		}
	}
	if len(failed) > 0 {
		m.mu.Lock()
		m.pending = append(failed, m.pending...)
		m.mu.Unlock()
	}
}

// finish forgets the job before its result goes out, so no later heartbeat lists it.
func (m *Manager) finish(ctx context.Context, j *job, result agentproto.JobResult) {
	m.mu.Lock()
	delete(m.running, j.spec.Job)
	m.pending = append(m.pending, result)
	m.mu.Unlock()
	slog.InfoContext(ctx, "agent: job finished", "job", j.spec.Job, "outcome", result.Outcome, "error", result.Error)
	if err := os.RemoveAll(j.dir); err != nil {
		slog.WarnContext(ctx, "agent: removing job scratch", "job", j.spec.Job, "error", err)
	}
	m.Deliver(context.WithoutCancel(ctx))
}

// abandon forgets a job without a result: the server cancelled it, or the agent is stopping and keeps
// its scratch to resume.
func (m *Manager) abandon(j *job, keepScratch bool) {
	m.mu.Lock()
	delete(m.running, j.spec.Job)
	m.mu.Unlock()
	if !keepScratch {
		_ = os.RemoveAll(j.dir)
	}
}

func sizeOf(path string) (int64, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func writeMeta(dir string, saved meta) error {
	raw, err := json.Marshal(saved)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, metaName), raw, filePerm)
}

// prepare makes the job's scratch dir, keeping a source from before a restart only if it is the same file.
func prepare(j *job) error {
	raw, err := os.ReadFile(filepath.Join(j.dir, metaName))
	var saved meta
	if err == nil && json.Unmarshal(raw, &saved) == nil && saved.SourceETag != j.spec.Source.ETag {
		if err := os.RemoveAll(j.dir); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(j.dir, dirPerm); err != nil {
		return fmt.Errorf("making job scratch: %w", err)
	}
	return writeMeta(j.dir, meta{SourceETag: j.spec.Source.ETag})
}
