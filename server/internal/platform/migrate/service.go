package migrate

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"magpie/internal/domain"
	platformapi "magpie/internal/platform/api"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/db"
	"magpie/internal/platform/jobs"

	"go.uber.org/fx"
	"gorm.io/gorm"
)

// Phase is where the migration is.
type Phase string

const (
	PhaseSetup        Phase = "SETUP"
	PhaseDryRunning   Phase = "DRY_RUN_RUNNING"
	PhaseDryRunFailed Phase = "DRY_RUN_FAILED"
	PhaseReview       Phase = "REVIEW"
	PhaseCommitting   Phase = "COMMIT_RUNNING"
	PhaseCommitFailed Phase = "COMMIT_FAILED"
	PhaseDone         Phase = "DONE"
)

// minSonarrMajor is the oldest Sonarr with custom formats and tmdbId.
const minSonarrMajor = 4

// Steps of a dry run, for its progress.
const (
	StepReading  = "READING"
	StepMatching = "MATCHING"
)

const interruptedError = "Magpie stopped while this was running. Start it again; finished work is kept."

var (
	ErrUnsupportedVersion = errors.New("migrate: unsupported version")
	ErrBusy               = errors.New("migrate: the dry run or commit is running")
	ErrNotReady           = errors.New("migrate: not ready")
	ErrNoSource           = errors.New("migrate: connect Sonarr or Radarr first")
)

// Source is one app the user connected, or skipped.
type Source struct {
	App        App        `json:"app"`
	Connection Connection `json:"connection"`
	Version    string     `json:"version"`
	Skipped    bool       `json:"skipped"`
	// RootFolders are the app's root folders as it sees them, for the path step.
	RootFolders []string `json:"rootFolders"`
}

// Progress is the running job's position.
type Progress struct {
	Step  string `json:"step"`
	Done  int    `json:"done"`
	Total int    `json:"total"`
}

// Session is the whole migration, stored as one document.
type Session struct {
	Sources          []Source      `json:"sources"`
	Paths            []PathRewrite `json:"paths"`
	CutoverConfirmed bool          `json:"cutoverConfirmed"`
	Options          Options       `json:"options"`
	Phase            Phase         `json:"phase"`
	RunID            string        `json:"runId"`
	Progress         Progress      `json:"progress"`
	Error            string        `json:"error"`
	FailedStep       CommitStep    `json:"failedStep"`
	Plan             *Plan         `json:"plan,omitempty"`
	Summary          *Summary      `json:"summary,omitempty"`
}

func newSession() Session {
	return Session{Sources: []Source{}, Paths: []PathRewrite{}, Phase: PhaseSetup}
}

func (s Session) running() bool {
	return s.Phase == PhaseDryRunning || s.Phase == PhaseCommitting
}

// CanCommit reports whether Commit may start: the plan is built, every review queue is settled and the
// cutover checklist is confirmed.
func (s Session) CanCommit() bool {
	planned := s.Phase == PhaseReview || s.Phase == PhaseCommitFailed
	return planned && s.Plan != nil && s.Plan.Reviews.Cleared() && s.CutoverConfirmed
}

type Params struct {
	fx.In

	DB        *db.DB
	Bus       *bus.Bus
	Jobs      jobs.Enqueuer
	Arr       Arr
	TMDB      TMDB
	Disk      Disk
	Importers Importers
}

// Service runs the migration: it keeps the session, starts the dry run and commit jobs, and applies the
// user's review decisions.
type Service struct {
	p      Params
	mu     sync.Mutex
	active atomic.Bool
	live   atomic.Pointer[Progress]
	now    func() time.Time
}

func NewService(p Params) *Service {
	return &Service{p: p, now: time.Now}
}

// State is the stored session with the running job's latest progress.
func (s *Service) State(ctx context.Context) (Session, error) {
	session, err := loadSession(s.p.DB.Read(ctx))
	if live := s.live.Load(); err == nil && session.running() && live != nil {
		session.Progress = *live
	}
	return session, err
}

// update changes the session in one transaction; refused while a job runs unless allowRunning.
func (s *Service) update(ctx context.Context, allowRunning bool, change func(tx *gorm.DB, session *Session) error) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out Session
	err := s.p.DB.Write(ctx, func(tx *gorm.DB) error {
		session, err := loadSession(tx)
		if err != nil {
			return err
		}
		if session.running() && !allowRunning {
			return ErrBusy
		}
		if err := change(tx, &session); err != nil {
			return err
		}
		out = session
		return saveSession(tx, session, s.now().UTC())
	})
	return out, err
}

// MarkInterrupted fails a job a crash or restart left running, so the user can start it again.
func (s *Service) MarkInterrupted(ctx context.Context) error {
	if s.active.Load() {
		return nil
	}
	_, err := s.update(ctx, true, func(_ *gorm.DB, session *Session) error {
		switch session.Phase {
		case PhaseDryRunning:
			session.Phase, session.Error = PhaseDryRunFailed, interruptedError
		case PhaseCommitting:
			session.Phase, session.Error = PhaseCommitFailed, interruptedError
		}
		return nil
	})
	return err
}

// ConnectRequest connects or skips one app.
type ConnectRequest struct {
	App        App
	Connection Connection
	Skip       bool
}

// Connect checks an app's version and reads its root folders, or records that the user skipped it.
// Sonarr before 4.0 is refused.
func (s *Service) Connect(ctx context.Context, req ConnectRequest) (Session, error) {
	source := Source{App: req.App, Skipped: req.Skip, RootFolders: []string{}}
	if !req.Skip {
		req.Connection.URL = strings.TrimRight(strings.TrimSpace(req.Connection.URL), "/")
		status, err := s.p.Arr.Status(ctx, req.App, req.Connection)
		if err != nil {
			return Session{}, err
		}
		if req.App == AppSonarr && majorVersion(status.Version) < minSonarrMajor {
			return Session{}, fmt.Errorf("%w: Sonarr %s; Magpie imports from Sonarr 4.0 or later", ErrUnsupportedVersion, status.Version)
		}
		roots, err := s.p.Arr.RootFolders(ctx, req.App, req.Connection)
		if err != nil {
			return Session{}, err
		}
		for _, r := range roots {
			source.RootFolders = append(source.RootFolders, r.Path)
		}
		source.Connection, source.Version = req.Connection, status.Version
	}
	return s.update(ctx, false, func(_ *gorm.DB, session *Session) error {
		session.Sources = slices.DeleteFunc(session.Sources, func(src Source) bool { return src.App == req.App })
		session.Sources = append(session.Sources, source)
		slices.SortFunc(session.Sources, func(a, b Source) int { return slices.Index(Apps, a.App) - slices.Index(Apps, b.App) })
		session.Paths = slices.DeleteFunc(session.Paths, func(p PathRewrite) bool { return p.App == req.App })
		for _, root := range source.RootFolders {
			session.Paths = append(session.Paths, PathRewrite{App: req.App, From: root, To: root})
		}
		return nil
	})
}

func majorVersion(version string) int {
	major, _, _ := strings.Cut(version, ".")
	n, err := strconv.Atoi(major)
	if err != nil {
		return 0
	}
	return n
}

// CheckPath says what the server can do with a folder.
func (s *Service) CheckPath(path string) PathAccess {
	return s.p.Disk.CheckFolder(path)
}

// SetPaths stores the rewrite of each root folder the apps listed.
func (s *Service) SetPaths(ctx context.Context, paths []PathRewrite) (Session, error) {
	return s.update(ctx, false, func(_ *gorm.DB, session *Session) error {
		for i := range session.Paths {
			for _, p := range paths {
				if p.App == session.Paths[i].App && p.From == session.Paths[i].From {
					session.Paths[i].To = strings.TrimSpace(p.To)
				}
			}
		}
		return nil
	})
}

func (s *Service) SetCutover(ctx context.Context, confirmed bool) (Session, error) {
	return s.update(ctx, false, func(_ *gorm.DB, session *Session) error {
		session.CutoverConfirmed = confirmed
		return nil
	})
}

func (s *Service) SetOptions(ctx context.Context, options Options) (Session, error) {
	return s.update(ctx, false, func(_ *gorm.DB, session *Session) error {
		session.Options = options
		return nil
	})
}

// migrationTask runs the dry run or the commit of one run.
type migrationTask struct {
	RunID string `json:"runId"`
	Phase Phase  `json:"phase"`
}

// StartDryRun queues a dry run. It may run again until the commit starts; a new one replaces the plan and
// the decisions made on it.
func (s *Service) StartDryRun(ctx context.Context) (Session, error) {
	return s.update(ctx, false, func(tx *gorm.DB, session *Session) error {
		if session.Phase == PhaseDone || session.Phase == PhaseCommitFailed {
			return fmt.Errorf("%w: the commit has started", ErrNotReady)
		}
		connected := slices.ContainsFunc(session.Sources, func(src Source) bool { return !src.Skipped })
		if !connected {
			return ErrNoSource
		}
		return s.queue(tx, session, PhaseDryRunning)
	})
}

// StartCommit queues the commit, or resumes a failed one.
func (s *Service) StartCommit(ctx context.Context) (Session, error) {
	return s.update(ctx, false, func(tx *gorm.DB, session *Session) error {
		if !session.CanCommit() {
			return fmt.Errorf("%w: run the dry run, settle every review queue and confirm the cutover first", ErrNotReady)
		}
		return s.queue(tx, session, PhaseCommitting)
	})
}

func (s *Service) queue(tx *gorm.DB, session *Session, phase Phase) error {
	session.Phase, session.RunID, session.Error, session.FailedStep = phase, domain.NewID[string](), "", ""
	session.Progress = Progress{}
	return s.p.Jobs.EnqueueTx(tx, migrationTask{RunID: session.RunID, Phase: phase})
}

// Reset forgets the migration so it can start over. Whatever was committed stays.
func (s *Service) Reset(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.p.DB.Write(ctx, func(tx *gorm.DB) error {
		session, err := loadSession(tx)
		if err != nil {
			return err
		}
		if session.running() {
			return ErrBusy
		}
		return deleteSession(tx)
	})
}

// Decide applies a review decision.
func (s *Service) Decide(ctx context.Context, d Decision) (Session, error) {
	return s.reviewing(ctx, func(session *Session) error { return decide(ctx, s.p.TMDB, session.Plan, d) })
}

// SkipQueue settles every open item of a queue with its default.
func (s *Service) SkipQueue(ctx context.Context, queue ReviewQueue) (Session, error) {
	return s.reviewing(ctx, func(session *Session) error { return skipQueue(session.Plan, queue) })
}

func (s *Service) reviewing(ctx context.Context, change func(*Session) error) (Session, error) {
	return s.update(ctx, false, func(_ *gorm.DB, session *Session) error {
		if session.Phase != PhaseReview || session.Plan == nil {
			return fmt.Errorf("%w: there is no dry run to review", ErrNotReady)
		}
		return change(session)
	})
}

// Search looks a title up on TMDB for the unmatched-titles queue.
func (s *Service) Search(ctx context.Context, kind domain.MediaKind, query string) ([]TmdbCandidate, error) {
	found, err := s.p.TMDB.Search(ctx, strings.TrimSpace(query))
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(found, func(c TmdbCandidate) bool { return c.Kind != kind }), nil
}

// Run runs a queued dry run or commit. A task whose run was replaced, or that MarkInterrupted failed,
// does nothing.
func (s *Service) Run(ctx context.Context, task migrationTask) error {
	s.active.Store(true)
	s.live.Store(nil)
	defer s.active.Store(false)
	session, err := s.State(ctx)
	if err != nil || session.RunID != task.RunID || session.Phase != task.Phase {
		return err
	}
	if task.Phase == PhaseDryRunning {
		return s.dryRun(ctx, session)
	}
	return s.commit(ctx, session)
}

func (s *Service) dryRun(ctx context.Context, session Session) error {
	var libs []ArrLibrary
	for _, src := range session.Sources {
		if src.Skipped {
			continue
		}
		lib, err := s.p.Arr.Library(ctx, src.App, src.Connection, func(done, total int) {
			s.progress(session.Phase, StepReading+"_"+string(src.App), done, total)
		})
		if err != nil {
			return s.fail(ctx, session.RunID, PhaseDryRunFailed, "", fmt.Errorf("reading %s: %w", appName(src.App), err))
		}
		lib.App = src.App
		libs = append(libs, lib)
	}
	planner := newPlanner(s.p.TMDB, s.p.Disk, session.Paths, func(done, total int) {
		s.progress(session.Phase, StepMatching, done, total)
	}, s.now)
	plan, err := planner.build(ctx, libs)
	if err != nil {
		return s.fail(ctx, session.RunID, PhaseDryRunFailed, "", err)
	}
	_, err = s.update(context.WithoutCancel(ctx), true, func(_ *gorm.DB, current *Session) error {
		if current.RunID != session.RunID {
			return nil
		}
		current.Plan, current.Phase, current.Summary = plan, PhaseReview, nil
		return nil
	})
	s.emit(PhaseReview, "", 0, 0)
	return err
}

func (s *Service) commit(ctx context.Context, session Session) error {
	c := newCommitter(s.p.Importers, session.Plan, session.Options, func(step CommitStep, done, total int) {
		s.progress(session.Phase, string(step), done, total)
	})
	summary, err := c.run(ctx)
	if err != nil {
		var stepErr *StepError
		step := CommitStep("")
		if errors.As(err, &stepErr) {
			step = stepErr.Step
		}
		return s.fail(ctx, session.RunID, PhaseCommitFailed, step, err)
	}
	_, err = s.update(context.WithoutCancel(ctx), true, func(_ *gorm.DB, current *Session) error {
		current.Phase, current.Summary = PhaseDone, &summary
		return nil
	})
	s.emit(PhaseDone, "", 0, 0)
	return err
}

// fail records why a run stopped, also when a shutdown stopped it, so the phase never stays running.
func (s *Service) fail(ctx context.Context, runID string, phase Phase, step CommitStep, cause error) error {
	message := cause.Error()
	if ctx.Err() != nil {
		message = interruptedError
	}
	slog.WarnContext(ctx, "migration stopped", "phase", phase, "step", step, "error", cause)
	_, err := s.update(context.WithoutCancel(ctx), true, func(_ *gorm.DB, current *Session) error {
		if current.RunID != runID {
			return nil
		}
		current.Phase, current.Error, current.FailedStep = phase, message, step
		return nil
	})
	s.emit(phase, string(step), 0, 0)
	return errors.Join(cause, err)
}

// progress keeps the job's position in memory, since storing the session for every batch would rewrite
// the whole plan, and tells the wizard.
func (s *Service) progress(phase Phase, step string, done, total int) {
	s.live.Store(&Progress{Step: step, Done: done, Total: total})
	s.emit(phase, step, done, total)
}

func (s *Service) emit(phase Phase, step string, done, total int) {
	s.p.Bus.Emit(bus.EventOf(platformapi.MigrationProgressed{Phase: string(phase), Step: step, Done: done, Total: total}))
}
