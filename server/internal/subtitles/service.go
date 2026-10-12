package subtitles

import (
	"context"
	"errors"
	"sync"
	"time"

	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/db"
	"magpie/internal/platform/jobs"
	"magpie/internal/platform/settings"
	"magpie/internal/subtitles/api"

	"github.com/mikestefanello/backlite"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

var (
	ErrEmbedded          = errors.New("subtitles: an embedded track can't be removed")
	ErrCandidateNotFound = errors.New("subtitles: OpenSubtitles no longer offers that subtitle")
)

// TaskQueue queues Subtitles' jobs, some of them for later. *jobs.Queue implements it.
type TaskQueue interface {
	jobs.Enqueuer
	EnqueueTxAt(tx *gorm.DB, task backlite.Task, at time.Time) error
}

type Deps struct {
	fx.In

	DB            *db.DB
	Repo          *Repository
	Store         *settings.Store
	Catalog       catalogapi.Reader
	Library       libraryapi.Reader
	OpenSubtitles OpenSubtitles
	Syncer        Syncer
	Sidecars      SidecarFiles
	Bus           *bus.Bus
	Jobs          TaskQueue
}

// Service keeps every media file's subtitles in line with its wanted languages.
type Service struct {
	deps Deps
	now  func() time.Time

	// fileLocks serialise the searches of one file, so two passes never download the same track.
	fileLocks sync.Map
	// drainLock lets one pass at a time spend quota on waiting downloads.
	drainLock sync.Mutex
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps, now: time.Now}
}

func (s *Service) lockFile(file domain.MediaFileID) func() {
	value, _ := s.fileLocks.LoadOrStore(file, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func (s *Service) Settings(ctx context.Context) (Settings, error) {
	return SettingsKey.Get(ctx, s.deps.Store)
}

func (s *Service) SetSettings(ctx context.Context, next Settings) (Settings, error) {
	if err := next.Validate(); err != nil {
		return Settings{}, err
	}
	return next, SettingsKey.Set(ctx, s.deps.Store, next)
}

// QuotaView is the OpenSubtitles quota badge. Remaining is nil until OpenSubtitles first reports it.
type QuotaView struct {
	Remaining   *int
	ResetAt     *time.Time
	VIP         bool
	PausedUntil *time.Time
	Problem     string
}

func (s *Service) Quota(ctx context.Context) (QuotaView, error) {
	sess, err := session(s.deps.DB.Read(ctx))
	if err != nil {
		return QuotaView{}, err
	}
	view := QuotaView{Remaining: sess.DownloadsRemaining, ResetAt: sess.ResetAt, VIP: sess.VIP}
	if paused, until := sess.paused(s.now()); paused {
		view.PausedUntil, view.Problem = &until, sess.Problem
	}
	return view, nil
}

// FileView is a file's subtitles and wanted tracks for the Files tab.
type FileView struct {
	Subtitles []subtitleRow
	Wanted    []wantedRow
}

// File returns what Subtitles knows about a media file; an unknown file has no subtitles and no coverage.
func (s *Service) File(ctx context.Context, file domain.MediaFileID) (FileView, error) {
	st, _, err := s.deps.Repo.state(ctx, file)
	if err != nil {
		return FileView{}, err
	}
	return FileView{Subtitles: nonNil(st.Subtitles), Wanted: nonNil(st.Wanted)}, nil
}

func nonNil[T any](list []T) []T {
	if list == nil {
		return []T{}
	}
	return list
}

func (s *Service) publish(tx *gorm.DB, m domain.Message) error {
	return s.deps.Bus.Publish(tx, bus.EventOf(m))
}

// settle records whether a file still has work in progress and publishes CoverageSettled when the last
// in-progress track finishes.
func (s *Service) settle(tx *gorm.DB, file domain.MediaFileID) error {
	st, ok, err := loadState(tx, file)
	if err != nil || !ok {
		return err
	}
	inProgress := false
	for _, row := range st.Wanted {
		inProgress = inProgress || row.Status.InProgress()
	}
	if inProgress == !st.File.Settled {
		return nil
	}
	st.File.Settled = !inProgress
	if err := saveFile(tx, st.File); err != nil {
		return err
	}
	if inProgress {
		return nil
	}
	satisfied, missing := settledLanguages(st.Wanted)
	return s.publish(tx, api.CoverageSettled{MediaFile: file, Title: st.File.TitleID, Satisfied: satisfied, Missing: missing})
}

// queueSearch books a search of the file's open tracks, now or at a later time.
func (s *Service) queueSearch(tx *gorm.DB, file domain.MediaFileID, priority Priority, at time.Time) error {
	return s.deps.Jobs.EnqueueTxAt(tx, searchTask{MediaFile: file, Priority: priority}, at)
}

// hasOpenTrack reports whether any row still needs a search.
func hasOpenTrack(rows []wantedRow) bool {
	for _, row := range rows {
		if row.Status == StatusPending && row.Candidate == nil {
			return true
		}
	}
	return false
}
