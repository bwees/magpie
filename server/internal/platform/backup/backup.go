// Package backup takes the daily database snapshot and lists and restores backups for admins.
package backup

import (
	"context"
	"errors"
	"fmt"
	"time"

	"magpie/internal/platform/config"
	"magpie/internal/platform/db"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/jobs"
	"magpie/internal/platform/owner"
	"magpie/internal/platform/restart"
	"magpie/internal/platform/settings"

	"github.com/mikestefanello/backlite"
	"go.uber.org/fx"
)

const (
	queueName       = "platform.backup"
	taskMaxAttempts = 3
	taskBackoff     = 15 * time.Minute
	timeOfDayLayout = "15:04"

	DefaultTime = "03:00"
	DefaultKeep = 7
	MinKeep     = 1
	MaxKeep     = 90
)

var ErrInvalidSettings = errors.New("invalid backup settings")

type Settings struct {
	Enabled bool   `json:"enabled"`
	Time    string `json:"time"`
	Keep    int    `json:"keep"`
}

var SettingsKey = settings.NewKey(owner.Platform, "backups", Settings{Enabled: true, Time: DefaultTime, Keep: DefaultKeep})

// Validate checks the keep range and parses Time as HH:MM.
func (s Settings) Validate() (jobs.DailyAt, error) {
	if s.Keep < MinKeep || s.Keep > MaxKeep {
		return jobs.DailyAt{}, fmt.Errorf("%w: keep must be %d-%d", ErrInvalidSettings, MinKeep, MaxKeep)
	}
	at, err := time.Parse(timeOfDayLayout, s.Time)
	if err != nil {
		return jobs.DailyAt{}, fmt.Errorf("%w: time must be HH:MM", ErrInvalidSettings)
	}
	return jobs.DailyAt{Hour: at.Hour(), Minute: at.Minute()}, nil
}

type restarter interface {
	Restart() error
}

type Service struct {
	db        *db.DB
	store     *settings.Store
	restarter restarter
	backupDir string
	now       func() time.Time
}

var Module = fx.Module("backup",
	fx.Provide(
		func(d *db.DB, store *settings.Store, r *restart.Restarter, env config.Env) *Service {
			return NewService(d, store, r, db.BackupDir(env.DataDir))
		},
		httpserver.AsController(NewController),
		jobs.AsQueue(newQueue),
		jobs.AsPeriodic(newPeriodic),
	),
)

func NewService(d *db.DB, store *settings.Store, r restarter, backupDir string) *Service {
	return &Service{db: d, store: store, restarter: r, backupDir: backupDir, now: time.Now}
}

// Daily snapshots today's backup and deletes the oldest daily backups beyond the kept count.
func (s *Service) Daily(ctx context.Context) (db.Backup, error) {
	cfg, err := SettingsKey.Get(ctx, s.store)
	if err != nil {
		return db.Backup{}, err
	}
	if _, err := cfg.Validate(); err != nil {
		return db.Backup{}, err
	}
	backup, err := s.db.Snapshot(ctx, s.backupDir, db.BackupKindDaily, s.now())
	if err != nil {
		return db.Backup{}, err
	}
	return backup, db.PruneBackups(s.backupDir, db.BackupKindDaily, cfg.Keep)
}

// Manual snapshots the database now. The newest manual backups are kept, like pre-migration ones.
func (s *Service) Manual(ctx context.Context) (db.Backup, error) {
	return s.db.Snapshot(ctx, s.backupDir, db.BackupKindManual, s.now())
}

func (s *Service) List() ([]db.Backup, error) {
	return db.ListBackups(s.backupDir)
}

// Restore stages the named backup and restarts the server, which swaps it in before migrating.
func (s *Service) Restore(name string) error {
	if err := db.StageRestore(s.db.Path(), s.backupDir, name); err != nil {
		return err
	}
	return s.restarter.Restart()
}

type task struct{}

func (task) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: queueName, MaxAttempts: taskMaxAttempts, Backoff: taskBackoff}
}

func newQueue(s *Service) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, _ task) error {
		_, err := s.Daily(ctx)
		return err
	})
}

func newPeriodic(s *Service) jobs.Periodic {
	return jobs.Periodic{
		Name: queueName,
		Schedule: func(ctx context.Context) (jobs.Schedule, error) {
			cfg, err := SettingsKey.Get(ctx, s.store)
			if err != nil || !cfg.Enabled {
				return nil, err
			}
			at, err := cfg.Validate()
			if err != nil {
				return nil, err
			}
			return at, nil
		},
		Task: func() backlite.Task { return task{} },
	}
}
