package transcode

import (
	"context"
	"slices"
	"time"

	"magpie/internal/domain"
	"magpie/internal/platform/jobs"
	"magpie/internal/transcode/api"
	"magpie/internal/transcode/api/agentproto"

	"github.com/mikestefanello/backlite"
	"gorm.io/gorm"
)

// TaskQueue enqueues Transcode's backlite tasks. EnqueueTxAt spaces the backlog sweep's batches.
type TaskQueue interface {
	jobs.Enqueuer
	EnqueueTxAt(tx *gorm.DB, task backlite.Task, at time.Time) error
}

const (
	sweepQueueName     = "transcode.backlog-sweep"
	sweepQueueAttempts = 3
	sweepQueueBackoff  = time.Minute
	// A sweep evaluates this many files per task, then pauses before the next batch so a large library
	// doesn't hold the database.
	sweepBatch = 50
	sweepPause = 2 * time.Second
)

// sweepTask evaluates one batch of eligible files, in media file order, then queues the next batch.
type sweepTask struct {
	// Reason, when set, evaluates only files skipped for that reason.
	Reason api.SkipReason     `json:"reason"`
	After  domain.MediaFileID `json:"after"`
}

func (sweepTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: sweepQueueName, MaxAttempts: sweepQueueAttempts, Backoff: sweepQueueBackoff}
}

func newSweepQueue(s *JobService) backlite.Queue {
	return backlite.NewQueue(s.SweepBacklog)
}

// RequestSweep queues a re-evaluation of every eligible file against the current policy.
func (s *JobService) RequestSweep(ctx context.Context) error {
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error { return s.deps.Queue.EnqueueTx(tx, sweepTask{}) })
}

// SweepBacklog evaluates one batch of the sweep. A shutdown stops it between files; backlite runs the
// task again later and the files already evaluated are no-ops.
func (s *JobService) SweepBacklog(ctx context.Context, task sweepTask) error {
	policy, err := policyKey.Get(ctx, s.deps.Store)
	if err != nil {
		return err
	}
	q := s.deps.DB.Read(ctx).Where("media_file_id > ?", task.After).Order("media_file_id").Limit(sweepBatch)
	if task.Reason != "" {
		q = q.Where("media_file_id IN (?)", s.deps.DB.Read(ctx).Model(&skipRow{}).Select("media_file_id").Where("reason = ?", task.Reason))
	}
	var batch []eligibleFileRow
	if err := q.Find(&batch).Error; err != nil {
		return err
	}
	for _, eligible := range batch {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.deps.DB.Write(ctx, func(tx *gorm.DB) error { return s.evaluate(ctx, tx, eligible, policy) }); err != nil {
			return err
		}
	}
	if len(batch) < sweepBatch {
		return nil
	}
	next := sweepTask{Reason: task.Reason, After: batch[len(batch)-1].MediaFileID}
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error { return s.deps.Queue.EnqueueTxAt(tx, next, s.now().Add(sweepPause)) })
}

// liftsNoCapableAgent reports whether a newly passed capability is one a transcode target needs.
func liftsNoCapableAgent(passed []agentproto.Capability) bool {
	return slices.ContainsFunc(passed, func(c agentproto.Capability) bool {
		for _, target := range targetVideos {
			if target.needs == c {
				return true
			}
		}
		return false
	})
}

// Policy is the current transcode policy.
func (s *JobService) Policy(ctx context.Context) (Policy, error) {
	return policyKey.Get(ctx, s.deps.Store)
}

// UpdatePolicy stores the policy. A change that decides files differently, or turning transcoding back
// on, sweeps the backlog.
func (s *JobService) UpdatePolicy(ctx context.Context, updated Policy) error {
	if err := updated.validate(); err != nil {
		return err
	}
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		current, err := policyKey.GetTx(tx)
		if err != nil {
			return err
		}
		if err := policyKey.SetTx(tx, updated); err != nil {
			return err
		}
		if err := s.kickAfterCommit(tx); err != nil {
			return err
		}
		if current.Hash() == updated.Hash() && (current.Enabled || !updated.Enabled) {
			return nil
		}
		return s.deps.Queue.EnqueueTx(tx, sweepTask{})
	})
}

// BacklogSummary is the transcode backlog: queued jobs of scanned or migrated files, the eligible files
// and how many were skipped, per reason.
type BacklogSummary struct {
	Backlog  int
	Eligible int
	Skipped  map[api.SkipReason]int
}

func (s *JobService) Backlog(ctx context.Context) (BacklogSummary, error) {
	conn := s.deps.DB.Read(ctx)
	var backlog, eligible int64
	err := conn.Model(&jobRow{}).Where("state = ? AND kind = ? AND origin <> ?", api.StateQueued, api.JobKindTranscode, domain.OriginDownload).
		Count(&backlog).Error
	if err != nil {
		return BacklogSummary{}, err
	}
	if err := conn.Model(&eligibleFileRow{}).Count(&eligible).Error; err != nil {
		return BacklogSummary{}, err
	}
	skipped, err := skipCounts(conn)
	return BacklogSummary{Backlog: int(backlog), Eligible: int(eligible), Skipped: skipped}, err
}
