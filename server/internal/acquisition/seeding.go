package acquisition

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"magpie/internal/acquisition/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/jobs"
	"magpie/internal/platform/owner"
	"magpie/internal/platform/settings"

	"github.com/mikestefanello/backlite"
	"gorm.io/gorm"
)

// Defaults and ranges from 12-settings › Downloads and › Schedules.
const (
	DefaultSeedRatio = 2.0
	DefaultSeedDays  = 14

	SeedingGoalCheckInterval = 5 * time.Minute

	hoursPerDay = 24
)

var ErrInvalidSeedingOptions = errors.New("acquisition: invalid seeding options")

// SeedingGoal is when a seeding download has seeded enough: either the ratio or the days, whichever first.
type SeedingGoal struct {
	Ratio float64 `json:"ratio"`
	Days  int     `json:"days"`
}

// SeedingGoalKey is the global goal; an indexer's own ratio or days win over it.
var SeedingGoalKey = settings.NewKey(owner.Acquisition, "seedingGoal", SeedingGoal{Ratio: DefaultSeedRatio, Days: DefaultSeedDays})

func (g SeedingGoal) Validate() error {
	var errs []error
	if g.Ratio < 0 || g.Ratio > MaxSeedRatio {
		errs = append(errs, fmt.Errorf("seed ratio must be 0-%d", MaxSeedRatio))
	}
	if g.Days < 0 || g.Days > MaxSeedDays {
		errs = append(errs, fmt.Errorf("seed days must be 0-%d", MaxSeedDays))
	}
	if len(errs) > 0 {
		return fmt.Errorf("%w: %w", ErrInvalidSeedingOptions, errors.Join(errs...))
	}
	return nil
}

// forIndexer takes the indexer's own ratio and days where it has them.
func (g SeedingGoal) forIndexer(s IndexerSettings) SeedingGoal {
	if s.SeedRatio != nil {
		g.Ratio = *s.SeedRatio
	}
	if s.SeedDays != nil {
		g.Days = *s.SeedDays
	}
	return g
}

func (g SeedingGoal) metBy(t Torrent) bool {
	return t.Ratio >= g.Ratio || t.SeedingTime >= time.Duration(g.Days)*hoursPerDay*time.Hour
}

// SeedingOptions are the global "after download" mode and seeding goal.
type SeedingOptions struct {
	Mode api.SeedingMode
	Goal SeedingGoal
}

func (o SeedingOptions) Validate() error {
	if o.Mode != api.SeedUntilGoals && o.Mode != api.DontSeed {
		return fmt.Errorf("%w: unknown seeding mode %q", ErrInvalidSeedingOptions, o.Mode)
	}
	return o.Goal.Validate()
}

func (s *DownloadService) SeedingOptions(ctx context.Context) (SeedingOptions, error) {
	mode, err := SeedingModeKey.Get(ctx, s.deps.Store)
	if err != nil {
		return SeedingOptions{}, err
	}
	goal, err := SeedingGoalKey.Get(ctx, s.deps.Store)
	return SeedingOptions{Mode: mode, Goal: goal}, err
}

// SetDownloadSettings changes Settings → Downloads and the seeding options together.
func (s *DownloadService) SetDownloadSettings(ctx context.Context, o DownloadOptions, seeding SeedingOptions) (DownloadOptions, SeedingOptions, error) {
	if err := errors.Join(o.Validate(), seeding.Validate()); err != nil {
		return DownloadOptions{}, SeedingOptions{}, err
	}
	err := s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		return errors.Join(DownloadOptionsKey.SetTx(tx, o), SeedingModeKey.SetTx(tx, seeding.Mode), SeedingGoalKey.SetTx(tx, seeding.Goal))
	})
	if err != nil {
		return DownloadOptions{}, SeedingOptions{}, err
	}
	saved, err := s.Options(ctx)
	if err != nil {
		return DownloadOptions{}, SeedingOptions{}, err
	}
	savedSeeding, err := s.SeedingOptions(ctx)
	return saved, savedSeeding, err
}

const (
	handlerFileImported  = "acquisition.download-on-file-imported"
	handlerImportFailed  = "acquisition.download-on-import-failed"
	handlerImportRemoved = "acquisition.download-on-import-removed"

	releaseImportedQueue = "acquisition.release-imported"
	importFailedQueue    = "acquisition.import-failed"
	seedingGoalQueue     = "acquisition.seeding-goals"

	downloadJobAttempts = 3
	downloadJobBackoff  = time.Minute
)

// releaseImportedTask removes a don't-seed torrent from qBittorrent once Library moved its files.
type releaseImportedTask struct {
	Download domain.DownloadID `json:"download"`
}

func (releaseImportedTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: releaseImportedQueue, MaxAttempts: downloadJobAttempts, Backoff: downloadJobBackoff}
}

// importFailedTask drops a download Library couldn't import, or whose import the user removed, and grabs
// the next release.
type importFailedTask struct {
	Download domain.DownloadID `json:"download"`
}

func (importFailedTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: importFailedQueue, MaxAttempts: downloadJobAttempts, Backoff: downloadJobBackoff}
}

type seedingGoalTask struct{}

func (seedingGoalTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: seedingGoalQueue, MaxAttempts: downloadJobAttempts, Backoff: downloadJobBackoff}
}

// newFileImportedHandler marks a download imported. Handlers can't call qBittorrent inside their
// transaction, so removing a don't-seed torrent runs as a job.
func newFileImportedHandler(s *DownloadService) bus.Handler {
	return bus.Handler{
		Name:       handlerFileImported,
		EventTypes: []string{libraryapi.TypeFileImported},
		Handle: func(_ context.Context, tx *gorm.DB, ev bus.Event) error {
			imported, err := bus.Decode[libraryapi.FileImported](ev)
			if err != nil || imported.Download == nil {
				return err
			}
			return s.MarkImported(tx, *imported.Download)
		},
	}
}

// newImportFailedHandler keeps a download whose import waits for the user, and queues dropping one
// Library gave up on.
func newImportFailedHandler(s *DownloadService) bus.Handler {
	return bus.Handler{
		Name:       handlerImportFailed,
		EventTypes: []string{libraryapi.TypeImportFailed},
		Handle: func(_ context.Context, tx *gorm.DB, ev bus.Event) error {
			failed, err := bus.Decode[libraryapi.ImportFailed](ev)
			switch {
			case err != nil || failed.Retryable:
				return err
			case failed.Manual:
				return s.AwaitUser(tx, failed.Download)
			}
			return s.deps.Jobs.EnqueueTx(tx, importFailedTask{Download: failed.Download})
		},
	}
}

// newImportRemovedHandler queues dropping a download whose import the user removed.
func newImportRemovedHandler(s *DownloadService) bus.Handler {
	return bus.Handler{
		Name:       handlerImportRemoved,
		EventTypes: []string{libraryapi.TypeImportRemoved},
		Handle: func(_ context.Context, tx *gorm.DB, ev bus.Event) error {
			removed, err := bus.Decode[libraryapi.ImportRemoved](ev)
			if err != nil {
				return err
			}
			return s.deps.Jobs.EnqueueTx(tx, importFailedTask{Download: removed.Download})
		},
	}
}

func newReleaseImportedQueue(s *DownloadService) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, t releaseImportedTask) error {
		return s.ReleaseImported(ctx, t.Download)
	})
}

func newImportFailedQueue(s *DownloadService) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, t importFailedTask) error {
		return s.DropFailedImport(ctx, t.Download)
	})
}

func newSeedingGoalQueue(s *DownloadService) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, _ seedingGoalTask) error {
		return s.CheckSeedingGoals(ctx)
	})
}

func newSeedingGoalPeriodic() jobs.Periodic {
	return jobs.Periodic{
		Name:     seedingGoalQueue,
		Schedule: func(context.Context) (jobs.Schedule, error) { return jobs.Every(SeedingGoalCheckInterval), nil },
		Task:     func() backlite.Task { return seedingGoalTask{} },
	}
}

// importableStates are the downloads Library may still import.
var importableStates = []api.DownloadState{api.StateCompleted, api.StateImportPendingUser}

// AwaitUser keeps a completed download, with its data, while its import waits for the user.
func (s *DownloadService) AwaitUser(tx *gorm.DB, id domain.DownloadID) error {
	_, err := moveDownload(tx, id, []api.DownloadState{api.StateCompleted}, map[string]any{"state": api.StateImportPendingUser})
	return err
}

// MarkImported moves a completed or waiting download to IMPORTED, once however many files a pack had,
// and queues the removal of a don't-seed torrent.
func (s *DownloadService) MarkImported(tx *gorm.DB, id domain.DownloadID) error {
	moved, err := moveDownload(tx, id, importableStates, map[string]any{"state": api.StateImported})
	if err != nil || !moved {
		return err
	}
	var mode api.SeedingMode
	if err := tx.Model(&downloadRow{}).Where("id = ?", id).Select("seeding_mode").Scan(&mode).Error; err != nil {
		return err
	}
	if mode != api.DontSeed {
		return nil
	}
	return s.deps.Jobs.EnqueueTx(tx, releaseImportedTask{Download: id})
}

// ReleaseImported removes an imported torrent from qBittorrent and keeps its data, which Library moved.
func (s *DownloadService) ReleaseImported(ctx context.Context, id domain.DownloadID) error {
	return s.retire(ctx, id, false)
}

// retire removes an imported download from qBittorrent; it stays IMPORTED.
func (s *DownloadService) retire(ctx context.Context, id domain.DownloadID, deleteData bool) error {
	d, err := s.deps.Repo.Download(ctx, id)
	if errors.Is(err, ErrDownloadNotFound) {
		return nil
	}
	if err != nil || d.RemovedAt != nil || d.State != api.StateImported {
		return err
	}
	if err := s.deps.QBittorrent.Delete(ctx, deleteData, d.QbitHash); ignoreGone(err) != nil {
		return err
	}
	err = s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		_, err := moveDownload(tx, id, []api.DownloadState{api.StateImported}, map[string]any{"removed_at": s.now().UTC()})
		return err
	})
	s.forget(id)
	return err
}

// DropFailedImport deletes a download Library couldn't import with its data, blocklists the release and
// grabs the next cached one, or queues a fresh search. An imported or removed download is left alone.
func (s *DownloadService) DropFailedImport(ctx context.Context, id domain.DownloadID) error {
	d, err := s.deps.Repo.Download(ctx, id)
	if errors.Is(err, ErrDownloadNotFound) {
		return nil
	}
	if err != nil || d.RemovedAt != nil || !slices.Contains(droppable, d.State) {
		return err
	}
	cfg, err := s.settings(ctx)
	if err != nil {
		return err
	}
	return s.drop(ctx, d, rejection{Reason: api.RejectImportFailed, State: api.StateImportFailed}, cfg)
}

// CheckSeedingGoals removes every imported, seeding download that reached its goal, with its data. The
// library keeps its hardlink or copy.
func (s *DownloadService) CheckSeedingGoals(ctx context.Context) error {
	downloads, err := s.deps.Repo.activeDownloads(ctx)
	if err != nil {
		return err
	}
	seeding := slices.DeleteFunc(downloads, func(d download) bool {
		return d.State != api.StateImported || d.SeedingMode != api.SeedUntilGoals
	})
	if len(seeding) == 0 {
		return nil
	}
	state, err := s.sync(ctx)
	if err != nil {
		return err
	}
	global, err := SeedingGoalKey.Get(ctx, s.deps.Store)
	if err != nil {
		return err
	}
	var errs []error
	for _, d := range seeding {
		t, found := state.Torrents[d.QbitHash]
		if !found {
			continue
		}
		goal := global
		if ix, err := s.deps.Repo.Indexer(ctx, d.IndexerID); err == nil {
			goal = global.forIndexer(ix.Settings)
		}
		if goal.metBy(t) {
			errs = append(errs, s.retire(ctx, d.ID, true))
		}
	}
	return errors.Join(errs...)
}
