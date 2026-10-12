package acquisition

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"magpie/internal/acquisition/api"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/db"
	"magpie/internal/platform/jobs"
	"magpie/internal/platform/settings"

	"go.uber.org/fx"
	"gorm.io/gorm"
)

var ErrReleaseUnavailable = errors.New("acquisition: the release couldn't be fetched from its indexer")

// GrabStatus is where a grab stands when the grab request answers.
type GrabStatus string

const (
	// GrabChecking: a magnet waits for its file list before any content downloads.
	GrabChecking    GrabStatus = "CHECKING"
	GrabDownloading GrabStatus = "DOWNLOADING"
	// GrabRejected: the release held a Blocked file; Next is the release grabbed instead, if any.
	GrabRejected GrabStatus = "REJECTED"
)

type GrabResult struct {
	Status      GrabStatus
	Download    *domain.DownloadID // set unless rejected
	BlockedFile *string            // set when rejected
	Next        *domain.DownloadID // the fallback grab after a rejection
}

type DownloadDeps struct {
	fx.In

	DB          *db.DB
	Repo        *Repository
	Catalog     catalogapi.Reader
	Prowlarr    Prowlarr
	QBittorrent QBittorrent
	Store       *settings.Store
	Bus         *bus.Bus
	Jobs        jobs.Enqueuer
	Searches    *SearchRegistry
}

// DownloadService grabs releases into qBittorrent, checks their files, follows their progress and
// falls back to the next cached release when one is rejected.
type DownloadService struct {
	deps DownloadDeps
	now  func() time.Time

	syncMu sync.Mutex
	rid    int64

	mu sync.Mutex
	// live is the last progress seen per download, for change detection and the downloads list.
	live  map[domain.DownloadID]api.DownloadProgressed
	marks map[domain.DownloadID]progressMark
	// checking holds downloads with a check job queued, so the poll doesn't queue it twice.
	checking map[domain.DownloadID]bool
}

// progressMark is when a download last moved forward, for the stalled limit.
type progressMark struct {
	progress float64
	at       time.Time
}

func NewDownloadService(deps DownloadDeps) *DownloadService {
	return &DownloadService{
		deps:     deps,
		now:      time.Now,
		live:     map[domain.DownloadID]api.DownloadProgressed{},
		marks:    map[domain.DownloadID]progressMark{},
		checking: map[domain.DownloadID]bool{},
	}
}

// rejection is a release dropped by the blocked-file check, a timeout or a failure.
type rejection struct {
	Title       domain.TitleID
	Episodes    []domain.EpisodeID
	Release     string
	InfoHash    string
	Indexer     string
	Reason      api.RejectionReason
	BlockedFile *string
	Temporary   bool
	// Download is set when the release was in qBittorrent.
	Download *download
	// Failure is set for a failed download, which also publishes DownloadFailed.
	Failure string
	// State is where the dropped download ends; FAILED when unset.
	State api.DownloadState
}

// grabOutcome is everything one grab, check or fallback decided, committed in one transaction.
type grabOutcome struct {
	search     *search
	added      *download
	rejections []rejection
}

func (o grabOutcome) next() *domain.DownloadID {
	if o.added == nil {
		return nil
	}
	id := o.added.ID
	return &id
}

type grabSettings struct {
	options DownloadOptions
	blocked blockedFiles
	seeding api.SeedingMode
}

func (s *DownloadService) settings(ctx context.Context) (grabSettings, error) {
	options, err := DownloadOptionsKey.Get(ctx, s.deps.Store)
	if err != nil {
		return grabSettings{}, err
	}
	blocked, err := options.blockedFiles()
	if err != nil {
		return grabSettings{}, err
	}
	seeding, err := SeedingModeKey.Get(ctx, s.deps.Store)
	return grabSettings{options: options, blocked: blocked, seeding: seeding}, err
}

// CheckClient asks qBittorrent for its WebAPI version. Below 2.15 it returns ErrUnsupportedVersion.
func (s *DownloadService) CheckClient(ctx context.Context) error {
	_, err := s.deps.QBittorrent.Version(ctx)
	return err
}

// Grab sends a release from a search's cached results to qBittorrent. A rejected release needs
// override, which skips the decision engine but not the blocked-file check.
func (s *DownloadService) Grab(ctx context.Context, searchID domain.SearchID, releaseID domain.ReleaseID, override bool) (GrabResult, error) {
	st, err := s.deps.Searches.find(searchID)
	if err != nil {
		return GrabResult{}, err
	}
	release, ok := st.release(releaseID)
	if !ok {
		return GrabResult{}, ErrReleaseNotFound
	}
	if !release.accepted() && !override {
		return GrabResult{}, ErrReleaseRejected
	}
	if err := s.CheckClient(ctx); err != nil {
		return GrabResult{}, err
	}
	cfg, err := s.settings(ctx)
	if err != nil {
		return GrabResult{}, err
	}
	if !st.try(releaseID) {
		return GrabResult{}, ErrAlreadyGrabbed
	}
	added, rejected, err := s.attempt(ctx, st, release, override, cfg)
	if err != nil {
		st.untry(releaseID)
		return GrabResult{}, err
	}
	out := grabOutcome{search: st, added: added}
	if rejected != nil {
		out.rejections = []rejection{*rejected}
		s.fallback(ctx, &out, cfg)
	}
	if err := s.commit(ctx, out); err != nil {
		return GrabResult{}, err
	}
	switch {
	case rejected != nil:
		return GrabResult{Status: GrabRejected, BlockedFile: rejected.BlockedFile, Next: out.next()}, nil
	case added.State == api.StateAwaitingMetadata:
		return GrabResult{Status: GrabChecking, Download: out.next()}, nil
	}
	return GrabResult{Status: GrabDownloading, Download: out.next()}, nil
}

// fallback grabs the next accepted, not yet tried release of the search, skipping those that are rejected
// in turn. Without a cached search it leaves the outcome as is, and commit queues a fresh search.
func (s *DownloadService) fallback(ctx context.Context, out *grabOutcome, cfg grabSettings) {
	s.fallbackWhere(ctx, out, cfg, anyRelease)
}

func anyRelease(decidedRelease) bool { return true }

// fallbackWhere is fallback among the releases keep allows.
func (s *DownloadService) fallbackWhere(ctx context.Context, out *grabOutcome, cfg grabSettings, keep func(decidedRelease) bool) {
	if out.search == nil {
		return
	}
	for {
		next, ok := out.search.nextUntried(keep)
		if !ok {
			return
		}
		added, rejected, err := s.attempt(ctx, out.search, next, false, cfg)
		switch {
		case err != nil:
			slog.WarnContext(ctx, "fallback grab failed; trying the next release", "release", next.Candidate.Item.Title, "error", err)
		case rejected != nil:
			out.rejections = append(out.rejections, *rejected)
		default:
			out.added = added
			return
		}
	}
}

// torrentSource is a release's .torrent, or the magnet to add instead.
type torrentSource struct {
	torrent []byte
	magnet  string
}

func (s *DownloadService) fetch(ctx context.Context, item TorznabItem) (torrentSource, error) {
	if item.DownloadURL != "" {
		payload, err := s.deps.Prowlarr.Download(ctx, item.DownloadURL)
		switch {
		case err == nil && payload.MagnetRedirect != "":
			return torrentSource{magnet: payload.MagnetRedirect}, nil
		case err == nil:
			return torrentSource{torrent: payload.Torrent}, nil
		case item.MagnetURI == "":
			return torrentSource{}, fmt.Errorf("%w: %w", ErrReleaseUnavailable, err)
		}
	}
	if item.MagnetURI == "" {
		return torrentSource{}, fmt.Errorf("%w: it has no download link", ErrReleaseUnavailable)
	}
	return torrentSource{magnet: item.MagnetURI}, nil
}

// attempt adds one release to qBittorrent. A .torrent is checked here and never added when it holds a
// Blocked file; a magnet is added to stop once its metadata arrives, and the poll checks it then.
func (s *DownloadService) attempt(ctx context.Context, st *search, d decidedRelease, override bool, cfg grabSettings) (*download, *rejection, error) {
	source, err := s.fetch(ctx, d.Candidate.Item)
	if err != nil {
		return nil, nil, err
	}
	if source.torrent == nil {
		hash, err := s.deps.QBittorrent.AddTorrent(ctx, AddRequest{Magnet: source.magnet, StopCondition: StopMetadataReceived})
		if err != nil {
			return nil, nil, err
		}
		st.noteHash(hash)
		return s.newDownload(st, d, override, hash, hash, api.StateAwaitingMetadata, cfg), nil, nil
	}

	meta, err := ParseTorrent(source.torrent)
	if err != nil {
		return nil, nil, err
	}
	st.noteHash(meta.InfoHash)
	if file, blocked := cfg.blocked.firstMatch(meta.Files); blocked {
		r := releaseRejection(st, d, meta.InfoHash)
		r.Reason, r.BlockedFile = api.RejectBlockedFile, &file
		return nil, &r, nil
	}
	hash, err := s.deps.QBittorrent.AddTorrent(ctx, AddRequest{Torrent: source.torrent, Stopped: true})
	if err != nil {
		return nil, nil, err
	}
	if err := s.deps.QBittorrent.Start(ctx, hash); err != nil {
		return nil, nil, errors.Join(err, s.deps.QBittorrent.Delete(ctx, true, hash))
	}
	return s.newDownload(st, d, override, hash, meta.InfoHash, api.StateDownloading, cfg), nil, nil
}

func releaseRejection(st *search, d decidedRelease, infoHash string) rejection {
	return rejection{
		Title:    st.input.Title.ID,
		Episodes: episodeIDs(d.Candidate),
		Release:  d.Candidate.Item.Title,
		InfoHash: infoHash,
		Indexer:  d.Candidate.Indexer.Name,
	}
}

func episodeIDs(c candidate) []domain.EpisodeID {
	ids := make([]domain.EpisodeID, 0, len(c.Episodes))
	for _, ep := range c.Episodes {
		ids = append(ids, ep.ID)
	}
	return ids
}

func (s *DownloadService) newDownload(st *search, d decidedRelease, override bool, qbitHash, infoHash string, state api.DownloadState, cfg grabSettings) *download {
	c := d.Candidate
	seeding := cfg.seeding
	if c.Indexer.Settings.SeedingOverride != OverrideUseGlobal {
		seeding = api.SeedingMode(c.Indexer.Settings.SeedingOverride)
	}
	searchID := st.id
	return &download{
		Download: api.Download{
			ID:          domain.NewID[domain.DownloadID](),
			Title:       st.input.Title.ID,
			Episodes:    episodeIDs(c),
			Search:      &searchID,
			ReleaseName: c.Item.Title,
			Indexer:     c.Indexer.Name,
			InfoHash:    infoHash,
			Quality:     c.Eval.Quality,
			CFScore:     c.Eval.Score,
			State:       state,
			SeedingMode: seeding,
			GrabbedAt:   s.now().UTC(),
		},
		IndexerID: c.Indexer.ProwlarrID,
		QbitHash:  qbitHash,
		Release:   d.ID,
		SizeBytes: c.Item.SizeBytes,
		Trigger:   st.trigger,
		Override:  override,
	}
}

// droppable are the states a rejection can drop a download from.
var droppable = []api.DownloadState{api.StateAwaitingMetadata, api.StateDownloading, api.StateCompleted, api.StateImportPendingUser}

// commit stores a grab outcome and publishes its events. When nothing was grabbed after a rejection,
// it queues a fresh AUTO search for the rejected release's items.
func (s *DownloadService) commit(ctx context.Context, out grabOutcome) error {
	now := s.now().UTC()
	next := out.next()
	err := s.deps.DB.Write(context.WithoutCancel(ctx), func(tx *gorm.DB) error {
		if out.added != nil {
			if err := insertDownload(tx, *out.added); err != nil {
				return err
			}
			if err := s.deps.Bus.Publish(tx, bus.EventOf(grabbedEvent(*out.added))); err != nil {
				return err
			}
		}
		for _, r := range out.rejections {
			if err := s.storeRejection(tx, r, next, now); err != nil {
				return err
			}
		}
		if out.added != nil || len(out.rejections) == 0 {
			return nil
		}
		first := out.rejections[0]
		return queueSearch(tx, s.deps.Jobs, searchTask{Title: first.Title, Episodes: nonNil(first.Episodes), Trigger: api.TriggerAuto}, now)
	})
	if err != nil {
		return err
	}
	if out.added != nil && out.search != nil {
		out.search.grabbedAs(out.added.ID)
	}
	for _, r := range out.rejections {
		if r.Download != nil {
			s.forget(r.Download.ID)
		}
	}
	return nil
}

func (s *DownloadService) storeRejection(tx *gorm.DB, r rejection, next *domain.DownloadID, now time.Time) error {
	if r.Download != nil {
		moved, err := moveDownload(tx, r.Download.ID, droppable, map[string]any{
			"state": cmp.Or(r.State, api.StateFailed), "removed_at": now, "failure": cmp.Or(r.Failure, string(r.Reason)),
		})
		if err != nil || !moved {
			return err
		}
	}
	entry := api.BlocklistEntry{Title: r.Title, InfoHash: r.InfoHash, ReleaseName: r.Release, Indexer: r.Indexer, Reason: api.BlocklistReason(r.Reason), At: now}
	if r.Temporary {
		expires := now.Add(TemporaryBlockFor)
		entry.ExpiresAt = &expires
	}
	if _, err := insertBlocklistEntry(tx, entry); err != nil {
		return err
	}
	if r.Download != nil && r.Failure != "" {
		failed := api.DownloadFailed{Download: r.Download.ID, Title: r.Title, Reason: r.Failure}
		if err := s.deps.Bus.Publish(tx, bus.EventOf(failed)); err != nil {
			return err
		}
	}
	return s.deps.Bus.Publish(tx, bus.EventOf(api.ReleaseRejected{
		Title: r.Title, Release: r.Release, InfoHash: r.InfoHash, Indexer: r.Indexer,
		Reason: r.Reason, BlockedFile: r.BlockedFile, Next: next,
	}))
}

func grabbedEvent(d download) api.ReleaseGrabbed {
	return api.ReleaseGrabbed{
		Download: d.ID, Title: d.Title, Episodes: nonNil(d.Episodes), Release: d.ReleaseName, Indexer: d.Indexer,
		InfoHash: d.InfoHash, Quality: d.Quality, CFScore: d.CFScore, Trigger: d.Trigger, Override: d.Override,
	}
}
