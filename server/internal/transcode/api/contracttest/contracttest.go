// Package contracttest holds the transcode.Reader contract suite and the in-memory Fake that passes it.
// The real repository runs the same suite in its own tests.
package contracttest

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"
	"time"

	"magpie/internal/domain"
	"magpie/internal/transcode/api"
)

// Fixture is the stored state a Reader starts with.
type Fixture struct {
	Jobs     []api.Job
	Agents   []api.Agent
	Excluded []domain.TitleID
	Target   api.Target
}

// Factory returns a Reader holding exactly the fixture's state.
type Factory func(t *testing.T, f Fixture) api.Reader

const (
	whisperPriority  = 2
	downloadPriority = 1
	backlogPriority  = 0

	capUHD  = 25
	capFHD  = 10
	capHD   = 5
	capSD   = 2
	attempt = 1
)

// Run checks a Reader against the transcode contract.
func Run(t *testing.T, newReader Factory) {
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Second)
	file := domain.NewID[domain.MediaFileID]()

	older := job(file, api.StateSwapped, backlogPriority, base)
	newer := job(file, api.StateQueued, downloadPriority, base.Add(time.Minute))
	backlogOld := job(domain.NewID[domain.MediaFileID](), api.StateEncoding, backlogPriority, base.Add(2*time.Minute))
	downloadNew := job(domain.NewID[domain.MediaFileID](), api.StateUploading, downloadPriority, base.Add(4*time.Minute))
	downloadOld := job(domain.NewID[domain.MediaFileID](), api.StateOffered, downloadPriority, base.Add(3*time.Minute))
	whisper := job(domain.NewID[domain.MediaFileID](), api.StateVerifying, whisperPriority, base.Add(5*time.Minute))
	whisper.Kind = api.JobKindWhisper
	failed := job(domain.NewID[domain.MediaFileID](), api.StateFailed, whisperPriority, base.Add(6*time.Minute))

	zeta := api.Agent{ID: domain.NewID[domain.AgentID](), Hostname: "zeta", Mode: api.TransferDownloadFirst, Jobs: []domain.JobID{backlogOld.ID}}
	alpha := api.Agent{ID: domain.NewID[domain.AgentID](), Hostname: "alpha", Mode: api.TransferDirectRead}
	excluded := domain.NewID[domain.TitleID]()
	target := api.Target{
		Codec:    api.TargetAV110,
		Preset:   api.PresetBalanced,
		CapsMbps: map[api.ResolutionClass]int{api.Resolution2160: capUHD, api.Resolution1080: capFHD, api.Resolution720: capHD, api.ResolutionSD: capSD},
	}
	fixture := Fixture{
		Jobs:     []api.Job{older, newer, backlogOld, downloadNew, downloadOld, whisper, failed},
		Agents:   []api.Agent{zeta, alpha},
		Excluded: []domain.TitleID{excluded},
		Target:   target,
	}

	t.Run("Job returns ErrNotFound for an unknown ID", func(t *testing.T) {
		if _, err := newReader(t, fixture).Job(ctx, domain.NewID[domain.JobID]()); !errors.Is(err, api.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("Job returns the stored job", func(t *testing.T) {
		got, err := newReader(t, fixture).Job(ctx, whisper.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.ID != whisper.ID || got.Kind != api.JobKindWhisper || got.State != api.StateVerifying || got.MediaFile != whisper.MediaFile {
			t.Errorf("Job = %+v, want %+v", got, whisper)
		}
	})

	t.Run("JobsForMediaFile lists newest first", func(t *testing.T) {
		got, err := newReader(t, fixture).JobsForMediaFile(ctx, file)
		if err != nil {
			t.Fatal(err)
		}
		assertJobs(t, got, newer, older)
	})

	t.Run("JobsForMediaFile returns an empty slice for an unknown file", func(t *testing.T) {
		got, err := newReader(t, fixture).JobsForMediaFile(ctx, domain.NewID[domain.MediaFileID]())
		if err != nil || got == nil || len(got) != 0 {
			t.Errorf("JobsForMediaFile = %v, %v; want empty slice", got, err)
		}
	})

	t.Run("ActiveJobs lists OFFERED..VERIFYING by priority then age", func(t *testing.T) {
		got, err := newReader(t, fixture).ActiveJobs(ctx)
		if err != nil {
			t.Fatal(err)
		}
		assertJobs(t, got, whisper, downloadOld, downloadNew, backlogOld)
	})

	t.Run("QueueSummary counts jobs per state", func(t *testing.T) {
		got, err := newReader(t, fixture).QueueSummary(ctx)
		if err != nil {
			t.Fatal(err)
		}
		want := map[api.JobState]int{}
		for _, j := range fixture.Jobs {
			want[j.State]++
		}
		if !maps.Equal(got.Counts, want) {
			t.Errorf("Counts = %v, want %v", got.Counts, want)
		}
	})

	t.Run("Agents lists every agent ordered by hostname with its jobs", func(t *testing.T) {
		got, err := newReader(t, fixture).Agents(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(fixture.Agents) || got[0].ID != alpha.ID || got[1].ID != zeta.ID {
			t.Fatalf("Agents = %+v, want alpha then zeta", got)
		}
		if !slices.Equal(got[1].Jobs, zeta.Jobs) {
			t.Errorf("zeta jobs = %v, want %v", got[1].Jobs, zeta.Jobs)
		}
	})

	t.Run("IsExcluded reports excluded titles only", func(t *testing.T) {
		r := newReader(t, fixture)
		if ok, err := r.IsExcluded(ctx, excluded); err != nil || !ok {
			t.Errorf("excluded title = %v, %v", ok, err)
		}
		if ok, err := r.IsExcluded(ctx, domain.NewID[domain.TitleID]()); err != nil || ok {
			t.Errorf("unknown title = %v, %v", ok, err)
		}
	})

	t.Run("Target returns the active target", func(t *testing.T) {
		got, err := newReader(t, fixture).Target(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got.Codec != target.Codec || got.Preset != target.Preset || !maps.Equal(got.CapsMbps, target.CapsMbps) {
			t.Errorf("Target = %+v, want %+v", got, target)
		}
	})
}

func job(file domain.MediaFileID, state api.JobState, priority int, queuedAt time.Time) api.Job {
	return api.Job{
		ID:        domain.NewID[domain.JobID](),
		Kind:      api.JobKindTranscode,
		MediaFile: file,
		State:     state,
		Priority:  priority,
		Attempt:   attempt,
		QueuedAt:  queuedAt,
	}
}

func assertJobs(t *testing.T, got []api.Job, want ...api.Job) {
	t.Helper()
	ids := func(jobs []api.Job) []domain.JobID {
		out := make([]domain.JobID, 0, len(jobs))
		for _, j := range jobs {
			out = append(out, j.ID)
		}
		return out
	}
	if !slices.Equal(ids(got), ids(want)) {
		t.Errorf("jobs = %v, want %v", ids(got), ids(want))
	}
}
