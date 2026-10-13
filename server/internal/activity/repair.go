package activity

import (
	"context"
	"errors"
	"slices"
	"time"

	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/activity/api"
	"magpie/internal/domain"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/db"
	"magpie/internal/platform/jobs"
	transcodeapi "magpie/internal/transcode/api"

	"github.com/mikestefanello/backlite"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

const (
	repairQueueName   = "activity.repair-pipeline"
	repairMaxAttempts = 3
	repairBackoff     = time.Minute

	// typePipelineRepaired stands in for an event in the attention items a repair opens or clears.
	typePipelineRepaired = "activity.PipelineRepaired"
)

type PipelineRepairParams struct {
	fx.In

	Repository  *Repository
	DB          *db.DB
	Acquisition acquisitionapi.Reader
	Transcode   transcodeapi.Reader
	Service     *Service
}

// PipelineRepair frees rows waiting for an event that will never come: a download that left qBittorrent,
// or a transcode job no agent holds. Releases before the fix for both left such rows behind.
type PipelineRepair struct {
	repo        *Repository
	db          *db.DB
	acquisition acquisitionapi.Reader
	transcode   transcodeapi.Reader
	service     *Service
}

func NewPipelineRepair(p PipelineRepairParams) *PipelineRepair {
	return &PipelineRepair{repo: p.Repository, db: p.DB, acquisition: p.Acquisition, transcode: p.Transcode, service: p.Service}
}

// owners is what Acquisition and Transcode say about the downloads and jobs rows wait on.
type owners struct {
	live []domain.DownloadID
	// jobs holds nil for a job Transcode doesn't know.
	jobs map[domain.JobID]*transcodeapi.Job
}

// Run checks each waiting row against its owner. A row an event changed since it was read is left to that
// event, and last_event_id stays as it was, so events still queued apply after the repair.
func (r *PipelineRepair) Run(ctx context.Context) error {
	rows, err := r.repo.unsettledRows(ctx)
	if err != nil || len(rows) == 0 {
		return err
	}
	o, err := r.owners(ctx, rows)
	if err != nil {
		return err
	}
	return r.db.Write(ctx, func(tx *gorm.DB) error {
		var touched []domain.TitleID
		for _, read := range rows {
			repaired, err := r.repairRow(tx, read, o)
			if err != nil {
				return err
			}
			if title := domain.TitleID(read.TitleID); repaired && !slices.Contains(touched, title) {
				touched = append(touched, title)
			}
		}
		for _, title := range touched {
			if err := r.service.pipelineChanged(tx, bus.NewEvent(typePipelineRepaired, string(title), nil), title); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *PipelineRepair) owners(ctx context.Context, rows []pipelineRow) (owners, error) {
	active, err := r.acquisition.ActiveDownloads(ctx)
	if err != nil {
		return owners{}, err
	}
	o := owners{jobs: map[domain.JobID]*transcodeapi.Job{}}
	for _, d := range active {
		o.live = append(o.live, d.ID)
	}
	for _, row := range rows {
		f, err := row.facets()
		if err != nil {
			return owners{}, err
		}
		if !running(f) {
			continue
		}
		id := *f.Transcode.Job
		job, err := r.transcode.Job(ctx, id)
		switch {
		case errors.Is(err, transcodeapi.ErrNotFound):
			o.jobs[id] = nil
		case err != nil:
			return owners{}, err
		default:
			o.jobs[id] = &job
		}
	}
	return o, nil
}

func (r *PipelineRepair) repairRow(tx *gorm.DB, read pipelineRow, o owners) (bool, error) {
	current, err := r.repo.pipelineRows(tx, domain.TitleID(read.TitleID), []string{read.EpisodeID})
	if err != nil || len(current) == 0 || current[0].LastEventID != read.LastEventID {
		return false, err
	}
	row := current[0]
	f, err := row.facets()
	if err != nil {
		return false, err
	}
	f, repaired := o.settle(f)
	if !repaired {
		return false, nil
	}
	if err := row.setFacets(f); err != nil {
		return false, err
	}
	return true, r.repo.savePipelineRow(tx, &row)
}

// settle drops from f what its owners say has ended, and reports whether anything had.
func (o owners) settle(f facets) (facets, bool) {
	settled := false
	if f.Download == downloadActive && !slices.Contains(o.live, f.DownloadID) {
		f.Download, settled = downloadIdle, true
	}
	if running(f) {
		if chip, ended := chipAfter(o.jobs[*f.Transcode.Job]); ended {
			f.Transcode, settled = chip, true
		}
	}
	return f, settled
}

func running(f facets) bool {
	return f.Transcode != nil && f.Transcode.State == api.TranscodeRunning && f.Transcode.Job != nil
}

// chipAfter is the chip a RUNNING chip becomes once no agent holds its job. A job Transcode doesn't know or
// cancelled leaves no chip. Any other job keeps the chip; its own events move it.
func chipAfter(job *transcodeapi.Job) (*api.TranscodeChip, bool) {
	switch {
	case job == nil || job.State == transcodeapi.StateCancelled:
		return nil, true
	case job.State == transcodeapi.StateQueued || job.State == transcodeapi.StateOffered:
		return &api.TranscodeChip{State: api.TranscodeQueued}, true
	}
	return nil, false
}

type repairTask struct{}

func (repairTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: repairQueueName, MaxAttempts: repairMaxAttempts, Backoff: repairBackoff}
}

func newRepairQueue(r *PipelineRepair) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, _ repairTask) error { return r.Run(ctx) })
}

// repairOnStart runs the repair once per boot, so an upgrade frees the rows earlier releases left behind.
func repairOnStart(lc fx.Lifecycle, q jobs.Enqueuer) {
	lc.Append(fx.StartHook(func(ctx context.Context) error {
		return q.Enqueue(ctx, repairTask{})
	}))
}
