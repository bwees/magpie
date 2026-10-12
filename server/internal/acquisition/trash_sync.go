package acquisition

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"magpie/internal/acquisition/api"
	"magpie/internal/domain"
	"magpie/internal/platform/db"
	"magpie/internal/platform/jobs"
	"magpie/internal/platform/settings"

	"github.com/mikestefanello/backlite"
	"gorm.io/gorm"
)

const (
	trashSyncQueue       = "acquisition.trash-sync"
	trashSyncInterval    = 24 * time.Hour
	trashSyncMaxAttempts = 3
	trashSyncBackoff     = 10 * time.Minute
)

// trashApps are synced in this order. When both guides have a format of the same name, the first
// app's definition is kept, since movies and TV share one pool of formats.
var trashApps = []TrashApp{TrashRadarr, TrashSonarr}

// Sonarr names its remux qualities "Bluray-1080p Remux"; Radarr and Magpie say "Remux-1080p".
var trashNameAliases = strings.NewReplacer("bluray-1080p remux", "remux-1080p", "bluray-2160p remux", "remux-2160p")

func trashNameKey(name string) string {
	return trashNameAliases.Replace(strings.ToLower(strings.TrimSpace(name)))
}

// TrashSyncService pulls TRaSH Guides' formats and guide profiles into the shared pool.
type TrashSyncService struct {
	db     *db.DB
	repo   *QualityRepository
	store  *settings.Store
	guides TrashGuides
	jobs   jobs.Enqueuer
	now    func() time.Time
}

func NewTrashSyncService(d *db.DB, repo *QualityRepository, store *settings.Store, guides TrashGuides, q jobs.Enqueuer) *TrashSyncService {
	return &TrashSyncService{db: d, repo: repo, store: store, guides: guides, jobs: q, now: time.Now}
}

func (s *TrashSyncService) Status(ctx context.Context) (TrashSyncState, error) {
	return trashSyncStateKey.Get(ctx, s.store)
}

// RequestTrashSync queues a sync. It runs as a job because fetching the guides outlasts a request.
func RequestTrashSync(ctx context.Context, q jobs.Enqueuer) error {
	return q.Enqueue(ctx, trashSyncTask{})
}

func (s *TrashSyncService) GuideProfiles(ctx context.Context) ([]TrashGuideProfile, error) {
	return s.repo.TrashProfiles(ctx)
}

type trashGuide struct {
	app      TrashApp
	formats  []TrashCustomFormat
	profiles []TrashQualityProfile
}

// Sync fetches both guides, then applies them in one transaction, so a failed fetch changes nothing.
func (s *TrashSyncService) Sync(ctx context.Context) error {
	attempt := s.now()
	guides, err := s.fetch(ctx)
	if err == nil {
		err = s.db.Write(ctx, func(tx *gorm.DB) error {
			if err := applyTrashGuides(tx, guides, attempt); err != nil {
				return err
			}
			return s.jobs.EnqueueTx(tx, rescoreEveryFile)
		})
	}
	return s.record(ctx, attempt, err)
}

func (s *TrashSyncService) fetch(ctx context.Context) ([]trashGuide, error) {
	guides := make([]trashGuide, 0, len(trashApps))
	for _, app := range trashApps {
		formats, err := s.guides.CustomFormats(ctx, app)
		if err != nil {
			return nil, fmt.Errorf("TRaSH %s custom formats: %w", app, err)
		}
		profiles, err := s.guides.QualityProfiles(ctx, app)
		if err != nil {
			return nil, fmt.Errorf("TRaSH %s quality profiles: %w", app, err)
		}
		guides = append(guides, trashGuide{app: app, formats: formats, profiles: profiles})
	}
	return guides, nil
}

func (s *TrashSyncService) record(ctx context.Context, attempt time.Time, syncErr error) error {
	err := s.db.Write(ctx, func(tx *gorm.DB) error {
		state, err := trashSyncStateKey.GetTx(tx)
		if err != nil {
			return err
		}
		state.LastAttemptAt = &attempt
		state.LastError = ""
		if syncErr != nil {
			state.LastError = syncErr.Error()
		} else {
			state.LastSyncedAt = &attempt
		}
		return trashSyncStateKey.SetTx(tx, state)
	})
	return errors.Join(syncErr, err)
}

// formatPool indexes the stored formats while a sync adds to it.
type formatPool struct {
	byTrashID map[string]CustomFormat
	byName    map[string]CustomFormat
	// resolved maps every guide trash_id to the stored format that stands for it, including a
	// same-named format from the other guide.
	resolved map[string]api.CustomFormatID
}

func newFormatPool(formats []CustomFormat) *formatPool {
	pool := &formatPool{byTrashID: map[string]CustomFormat{}, byName: map[string]CustomFormat{}, resolved: map[string]api.CustomFormatID{}}
	for _, f := range formats {
		pool.add(f)
	}
	return pool
}

func (p *formatPool) add(f CustomFormat) {
	p.byName[trashNameKey(f.Name)] = f
	if f.TrashID != "" {
		p.byTrashID[f.TrashID] = f
		p.resolved[f.TrashID] = f.ID
	}
}

func applyTrashGuides(tx *gorm.DB, guides []trashGuide, now time.Time) error {
	formats, err := loadFormats(tx)
	if err != nil {
		return err
	}
	pool := newFormatPool(formats)
	var guideProfiles []TrashGuideProfile
	for _, guide := range guides {
		for _, tf := range guide.formats {
			if err := syncFormat(tx, pool, tf, now); err != nil {
				return err
			}
		}
		for _, p := range guide.profiles {
			guideProfiles = append(guideProfiles, TrashGuideProfile{App: guide.app, Profile: p, SyncedAt: now})
		}
	}
	if err := replaceTrashProfiles(tx, guideProfiles); err != nil {
		return err
	}

	ids, err := profileIDs(tx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := syncProfileScores(tx, id, pool, guideProfiles, now); err != nil {
			return err
		}
	}
	return nil
}

func syncFormat(tx *gorm.DB, pool *formatPool, tf TrashCustomFormat, now time.Time) error {
	f, linked := pool.byTrashID[tf.TrashID]
	if !linked {
		named, found := pool.byName[trashNameKey(tf.Name)]
		switch {
		case found && named.TrashID != "":
			pool.resolved[tf.TrashID] = named.ID
			return nil
		case found:
			f = named
		default:
			f = CustomFormat{ID: domain.NewID[api.CustomFormatID](), Name: tf.Name}
		}
	}
	f.TrashID, f.TrashScores, f.IncludeWhenRenaming, f.Specs = tf.TrashID, tf.Scores, tf.IncludeWhenRenaming, tf.Specs
	if renamed, taken := pool.byName[trashNameKey(tf.Name)]; !taken || renamed.ID == f.ID {
		f.Name = tf.Name
	}
	if _, err := CompileFormat(f); err != nil {
		slog.Warn("skipping TRaSH custom format", "trash_id", tf.TrashID, "name", tf.Name, "error", err)
		return nil
	}
	if err := saveFormat(tx, f, now); err != nil {
		return err
	}
	pool.add(f)
	return nil
}

// syncProfileScores links a profile to its guide profile (by trash_id, else by name), attaches the
// formats its guide profile or preset names, and refreshes every score the user hasn't changed.
func syncProfileScores(tx *gorm.DB, id domain.ProfileID, pool *formatPool, guideProfiles []TrashGuideProfile, now time.Time) error {
	p, err := loadProfile(tx, id)
	if err != nil {
		return err
	}
	var wanted []api.CustomFormatID
	if guide, ok := matchGuideProfile(p, guideProfiles); ok {
		p.TrashProfileID, p.TrashScoreSet = guide.TrashID, guide.ScoreSet
		for _, trashID := range guide.Formats {
			if formatID, ok := pool.resolved[trashID]; ok {
				wanted = append(wanted, formatID)
			}
		}
	}
	if p.Preset != "" {
		for _, name := range presetFormatNames(p.Preset) {
			if f, ok := pool.byName[trashNameKey(name)]; ok && f.TrashID != "" {
				wanted = append(wanted, f.ID)
			}
		}
	}

	formats := map[api.CustomFormatID]CustomFormat{}
	for _, f := range pool.byName {
		formats[f.ID] = f
	}
	scored := map[api.CustomFormatID]bool{}
	for i, score := range p.FormatScores {
		scored[score.Format] = true
		if trash, ok := trashScore(formats[score.Format], p.TrashScoreSet); ok && !score.UserOverride {
			p.FormatScores[i].Score = trash
		}
	}
	for _, formatID := range wanted {
		if scored[formatID] {
			continue
		}
		if trash, ok := trashScore(formats[formatID], p.TrashScoreSet); ok {
			p.FormatScores = append(p.FormatScores, api.FormatScore{Format: formatID, Score: trash})
			scored[formatID] = true
		}
	}
	return saveProfile(tx, p, now)
}

func matchGuideProfile(p api.Profile, guideProfiles []TrashGuideProfile) (TrashQualityProfile, bool) {
	for _, g := range guideProfiles {
		if p.TrashProfileID != "" && g.Profile.TrashID == p.TrashProfileID {
			return g.Profile, true
		}
	}
	if p.TrashProfileID != "" {
		return TrashQualityProfile{}, false
	}
	for _, g := range guideProfiles {
		if trashNameKey(g.Profile.Name) == trashNameKey(p.Name) {
			return g.Profile, true
		}
	}
	return TrashQualityProfile{}, false
}

type trashSyncTask struct{}

func (trashSyncTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: trashSyncQueue, MaxAttempts: trashSyncMaxAttempts, Backoff: trashSyncBackoff}
}

func newTrashSyncQueue(s *TrashSyncService) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, _ trashSyncTask) error { return s.Sync(ctx) })
}

// newTrashSyncPeriodic runs the sync daily while the TRaSH sync setting is on.
func newTrashSyncPeriodic(s *TrashSyncService) jobs.Periodic {
	return jobs.Periodic{
		Name: trashSyncQueue,
		Schedule: func(ctx context.Context) (jobs.Schedule, error) {
			enabled, err := TrashSyncEnabledKey.Get(ctx, s.store)
			if err != nil || !enabled {
				return nil, err
			}
			return jobs.Every(trashSyncInterval), nil
		},
		Task: func() backlite.Task { return trashSyncTask{} },
	}
}
