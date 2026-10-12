package subtitles

import (
	"context"
	"time"

	"magpie/internal/domain"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/db"
	"magpie/internal/subtitles/api"
	"magpie/internal/subtitles/api/contracttest"

	"github.com/mikestefanello/backlite"
	"gorm.io/gorm"
)

var (
	NewLibraryHandler          = newLibraryHandler
	NewIssueActionHandler      = newIssueActionHandler
	NewLanguagesChangedHandler = newLanguagesChangedHandler
)

func SetClock(s *Service, now func() time.Time) { s.now = now }

// RunTask runs a queued task as its queue would. It reports false for a task of another queue.
func RunTask(ctx context.Context, s *Service, task backlite.Task) (bool, error) {
	switch t := task.(type) {
	case searchTask:
		return true, s.searchFile(ctx, t.MediaFile, t.Priority)
	case downloadsTask:
		return true, s.drainDownloads(ctx)
	case sweepTask:
		return true, s.sweep(ctx)
	case recomputeTask:
		return true, s.recompute(ctx, t)
	case removeSidecarsTask:
		return true, s.removeSidecars(ctx, t.Paths)
	}
	return false, nil
}

func SweepTask() backlite.Task { return sweepTask{} }

func IsRecompute(task backlite.Task) bool {
	_, ok := task.(recomputeTask)
	return ok
}

// IsSearch reports whether a task searches the given file.
func IsSearch(task backlite.Task, file domain.MediaFileID) bool {
	t, ok := task.(searchTask)
	return ok && t.MediaFile == file
}

// Handle runs a bus handler on one message in its own transaction, as the dispatcher would.
func Handle(ctx context.Context, d *db.DB, h bus.Handler, m domain.Message) error {
	return d.Write(ctx, func(tx *gorm.DB) error { return h.Handle(ctx, tx, bus.EventOf(m)) })
}

// SeedQuota stores what OpenSubtitles last said about the daily quota.
func SeedQuota(ctx context.Context, d *db.DB, remaining int, resetAt time.Time) error {
	return d.Write(ctx, func(tx *gorm.DB) error {
		return saveSession(tx, sessionRow{DownloadsRemaining: &remaining, ResetAt: &resetAt})
	})
}

// SeedReader stores a contract-suite fixture. Scores are stored as percent of 100.
func SeedReader(ctx context.Context, d *db.DB, f contracttest.Fixture) error {
	return d.Write(ctx, func(tx *gorm.DB) error {
		for id, file := range f.Files {
			if err := insertFile(tx, fileRow{MediaFileID: id, TitleID: domain.NewID[domain.TitleID](), Settled: true, ImportedAt: time.Now()}); err != nil {
				return err
			}
			for _, sub := range file.Subtitles {
				row := subtitleRow{
					ID: newSubtitleID(), MediaFileID: id, Language: sub.Language, Forced: sub.Forced, SDH: sub.SDH,
					Source: sub.Source, Synced: sub.Synced, Path: sub.Path, AddedAt: time.Now(),
				}
				if sub.Score != nil {
					points, max := *sub.Score, percent
					row.Score, row.MaxScore = &points, &max
				}
				if err := saveSubtitle(tx, row); err != nil {
					return err
				}
			}
			for position, cov := range file.Coverage {
				row := wantedRow{
					MediaFileID: id, Language: cov.Language, Forced: cov.Forced, Position: position,
					Status: statusFor(cov.Status), Source: cov.Source, Priority: PriorityImport, CreatedAt: time.Now(),
				}
				if err := saveWanted(tx, []wantedRow{row}); err != nil {
					return err
				}
			}
		}
		return saveSession(tx, sessionRow{DownloadsRemaining: &f.Quota.Remaining, ResetAt: &f.Quota.ResetAt})
	})
}

func statusFor(c api.CoverageStatus) WantedStatus {
	switch c {
	case api.CoverageSatisfied:
		return StatusSatisfied
	case api.CoverageMissing:
		return StatusMissing
	default:
		return StatusPending
	}
}
