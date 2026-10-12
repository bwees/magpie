package acquisition

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"magpie/internal/acquisition/api"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/db"
	"magpie/internal/platform/jobs"
	"magpie/internal/platform/parse"
	"magpie/internal/platform/settings"

	"github.com/mikestefanello/backlite"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

var (
	ErrSearchNotFound  = errors.New("acquisition: search not found")
	ErrTitleNotFound   = errors.New("acquisition: title not found")
	ErrInvalidSearch   = errors.New("acquisition: invalid search")
	ErrReleaseNotFound = errors.New("acquisition: release not found in the search")
	ErrReleaseRejected = errors.New("acquisition: the release was rejected; grab it with override")
	ErrAlreadyGrabbed  = errors.New("acquisition: the release was already grabbed from this search")
)

// Messages shown for an indexer that FAILED or was SKIPPED.
const (
	messageSkipped   = "No ID search, and the title search found nothing"
	messageCancelled = "Search cancelled"
	messageTimeout   = "No answer in %s"
)

// SearchRequest starts a search for a title, or for some of a series' episodes.
type SearchRequest struct {
	Title    domain.TitleID
	Episodes []domain.EpisodeID
	Trigger  api.SearchTrigger
}

type SearchDeps struct {
	fx.In

	DB          *db.DB
	Repo        *Repository
	Quality     *QualityRepository
	Catalog     catalogapi.Reader
	Library     libraryapi.Reader
	Prowlarr    Prowlarr
	QBittorrent QBittorrent
	Store       *settings.Store
	Bus         *bus.Bus
	Searches    *SearchRegistry
	Jobs        searchJobs
}

// searchJobs queues the jobs searching starts: the automatic grab, more searches, some of them for later.
type searchJobs interface {
	jobs.Enqueuer
	EnqueueTxAt(tx *gorm.DB, task backlite.Task, at time.Time) error
}

// grabsOnItsOwn are the triggers whose searches grab without the user.
var grabsOnItsOwn = triggerSet{api.TriggerRSS, api.TriggerAuto, api.TriggerUserAuto, api.TriggerRegrab}

// SearchService fans a search out to every indexer, decides each release as its indexer answers, and
// keeps the results in memory for the result-cache time.
type SearchService struct {
	deps SearchDeps
	now  func() time.Time

	life    context.Context
	stop    context.CancelFunc
	running sync.WaitGroup

	mu      sync.Mutex
	results map[string]cachedResult
}

type cachedResult struct {
	items   []TorznabItem
	expires time.Time
}

// indexerAnswer is what an indexer's query tiers came back with.
type indexerAnswer struct {
	items   []TorznabItem
	status  api.IndexerRunStatus
	message string
	err     error
	// asked is false when every tier was answered from the cache.
	asked bool
}

func NewSearchService(deps SearchDeps) *SearchService {
	life, stop := context.WithCancel(context.Background())
	return &SearchService{
		deps:    deps,
		now:     time.Now,
		life:    life,
		stop:    stop,
		results: map[string]cachedResult{},
	}
}

// Close cancels every search and waits for its indexer queries to return.
func (s *SearchService) Close() {
	s.stop()
	for _, st := range s.deps.Searches.all() {
		st.stopTimers()
	}
	s.running.Wait()
}

func (s *SearchService) newSearch(req SearchRequest, in *decisionInput, options IndexerOptions) *search {
	st := &search{
		id:          domain.NewID[domain.SearchID](),
		trigger:     req.Trigger,
		startedAt:   s.now().UTC(),
		input:       in,
		cacheFor:    options.ResultCache.Std(),
		keys:        map[releaseKey]bool{},
		tried:       map[domain.ReleaseID]bool{},
		triedHashes: map[string]bool{},
	}
	if grabsOnItsOwn.has(req.Trigger) {
		st.settle = settleWaiting
	}
	st.ctx, st.cancel = context.WithCancel(s.life)
	st.emit = func(e api.SearchResultsReceived) { s.deps.Bus.Emit(bus.EventOf(e)) }
	return st
}

// keep caches a search's results until they expire.
func (s *SearchService) keep(st *search, options IndexerOptions) {
	st.mu.Lock()
	st.expiry = time.AfterFunc(searchRetention(options), func() { s.expire(st) })
	st.mu.Unlock()
	s.deps.Searches.add(st)
}

// Start records the search, publishes SearchStarted and asks every indexer whose caps cover the title's
// kind in parallel. Results stream out as SearchResultsReceived. A search that grabs on its own does so
// once every indexer answered, the settle window passed or a release nothing can beat arrived.
func (s *SearchService) Start(ctx context.Context, req SearchRequest) (SearchSnapshot, error) {
	in, target, err := s.decisionInput(ctx, req)
	if err != nil {
		return SearchSnapshot{}, err
	}
	options, err := IndexerOptionsKey.Get(ctx, s.deps.Store)
	if err != nil {
		return SearchSnapshot{}, err
	}
	window, err := SettleWindowKey.Get(ctx, s.deps.Store)
	if err != nil {
		return SearchSnapshot{}, err
	}
	indexers, err := s.deps.Repo.Indexers(ctx, false)
	if err != nil {
		return SearchSnapshot{}, err
	}

	st := s.newSearch(req, in, options)
	names := []string{}
	for _, ix := range indexers {
		if ix.Caps.covers(target.Title.Kind) {
			st.runs = append(st.runs, IndexerRun{Indexer: ix, Status: IndexerPending})
			names = append(names, ix.Name)
		}
	}

	started := api.SearchStarted{
		Search:      st.id,
		Title:       target.Title.ID,
		Episodes:    nonNil(req.Episodes),
		Trigger:     req.Trigger,
		Interactive: req.Trigger == api.TriggerInteractive,
		Indexers:    names,
	}
	err = s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		if err := insertSearch(tx, started, st.startedAt); err != nil {
			return err
		}
		return s.deps.Bus.Publish(tx, bus.EventOf(started))
	})
	if err != nil {
		st.cancel()
		return SearchSnapshot{}, err
	}

	s.keep(st, options)
	if st.settle == settleWaiting {
		st.mu.Lock()
		st.window = time.AfterFunc(window.Std(), func() { s.follow(st, st.closeWindow()) })
		st.mu.Unlock()
	}

	timeout := options.SearchTimeout.Std()
	for _, run := range st.runs {
		plan, hasIDTier := queryPlan(run.Indexer.Caps, target)
		s.running.Add(1)
		go s.ask(st, run.Indexer, plan, hasIDTier, timeout)
	}
	s.follow(st, st.stepNow())
	return st.snapshot(), nil
}

// searchRetention is how long a search's results stay cached. Turning the per-query cache off doesn't
// drop a search while its dialog may still be open.
func searchRetention(o IndexerOptions) time.Duration {
	return cmp.Or(o.ResultCache.Std(), DefaultResultCache)
}

func (st *search) stepNow() searchStep {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.step()
}

// follow does what a change to a search asked for.
func (s *SearchService) follow(st *search, step searchStep) {
	if step.settle {
		if err := s.deps.Jobs.Enqueue(context.WithoutCancel(s.life), settleTask{Search: st.id}); err != nil {
			slog.Error("queueing the automatic grab; the search completes without one", "search", st.id, "error", err)
			s.follow(st, st.finishSettle(false))
		}
	}
	if step.completed {
		s.publishCompleted(st, step.done)
	}
}

func (s *SearchService) decisionInput(ctx context.Context, req SearchRequest) (*decisionInput, searchTarget, error) {
	title, err := s.deps.Catalog.Title(ctx, req.Title)
	if errors.Is(err, catalogapi.ErrNotFound) {
		return nil, searchTarget{}, ErrTitleNotFound
	}
	if err != nil {
		return nil, searchTarget{}, err
	}
	target := searchTarget{Title: title}
	in := &decisionInput{Trigger: req.Trigger, Now: s.now(), Title: title}

	if title.Kind == domain.MediaKindSeries {
		if in.Episodes, err = s.deps.Catalog.Episodes(ctx, title.ID); err != nil {
			return nil, searchTarget{}, err
		}
	}
	if target.Episodes, err = pickEpisodes(in.Episodes, req.Episodes); err != nil {
		return nil, searchTarget{}, err
	}
	in.Searched = target.Episodes

	if in.Profile, err = s.profileFor(ctx, title); err != nil {
		return nil, searchTarget{}, err
	}
	files, err := s.deps.Library.FilesForTitle(ctx, title.ID)
	if err != nil {
		return nil, searchTarget{}, err
	}
	in.Files = scoreFiles(files, title, in.Profile)
	if in.Blocklist, err = s.deps.Repo.Blocklist(ctx, title.ID); err != nil {
		return nil, searchTarget{}, err
	}
	downloads, err := s.deps.Repo.titleDownloads(ctx, title.ID)
	if err != nil {
		return nil, searchTarget{}, err
	}
	in.Queue, in.History = grabsOf(downloads, title, in.Profile, in.Now)
	held, err := s.deps.Repo.PendingReleases(ctx, title.ID)
	if err != nil {
		return nil, searchTarget{}, err
	}
	in.Pending = pendingGrabs(held, title, in.Profile)
	in.MaxScore = in.Profile.MaxAttainableScore()
	delay, err := DefaultDelayKey.Get(ctx, s.deps.Store)
	if err != nil {
		return nil, searchTarget{}, err
	}
	in.Delay = delay.Std()
	if in.MaximumSize, err = MaximumSizeKey.Get(ctx, s.deps.Store); err != nil {
		return nil, searchTarget{}, err
	}
	in.FreeSpace = sync.OnceValues(s.freeSpace)
	return in, target, nil
}

// pickEpisodes resolves the requested episode IDs; a movie takes none.
func pickEpisodes(all []catalogapi.Episode, ids []domain.EpisodeID) ([]catalogapi.Episode, error) {
	picked := make([]catalogapi.Episode, 0, len(ids))
	for _, id := range ids {
		i := slices.IndexFunc(all, func(ep catalogapi.Episode) bool { return ep.ID == id })
		if i < 0 {
			return nil, fmt.Errorf("%w: episode %s isn't part of the title", ErrInvalidSearch, id)
		}
		picked = append(picked, all[i])
	}
	return picked, nil
}

// scoreFiles evaluates the title's library files by their original release name, else their path.
func scoreFiles(files []libraryapi.MediaFile, title catalogapi.Title, p ScoringProfile) []existingFile {
	scored := make([]existingFile, 0, len(files))
	for _, f := range files {
		scored = append(scored, scoreFile(f, title, p))
	}
	return scored
}

func scoreFile(f libraryapi.MediaFile, title catalogapi.Title, p ScoringProfile) existingFile {
	release := fileRelease(f, title.Kind)
	facts := ReleaseFacts{Release: release, SizeBytes: f.Size, OriginalLanguage: title.OriginalLanguage}
	return existingFile{File: f, Eval: Evaluate(facts, p), Group: release.Group}
}

func fileRelease(f libraryapi.MediaFile, kind domain.MediaKind) parse.Release {
	var release parse.Release
	var err error
	if f.ReleaseName != "" {
		release, err = parse.ParseRelease(f.ReleaseName, kind)
	} else {
		release, err = parse.ParseFile(f.Path, kind)
	}
	if err != nil {
		release = parse.Release{Name: cmp.Or(f.ReleaseName, f.Path), Kind: kind}
	}
	if f.Quality.ID != int(parse.QualityUnknown) {
		release.Quality = f.Quality
	}
	return release
}

func (s *SearchService) freeSpace() (int64, bool) {
	free, err := s.deps.QBittorrent.FreeSpace(s.life)
	if err != nil {
		slog.Warn("download disk free space unknown; skipping the free-space check", "error", err)
		return 0, false
	}
	return free, true
}

// ask runs one indexer's query tiers. After the timeout it reports FAILED but keeps waiting, so a late
// answer still streams into the open search.
func (s *SearchService) ask(st *search, ix Indexer, plan []TorznabQuery, hasIDTier bool, timeout time.Duration) {
	defer s.running.Done()
	result := make(chan indexerAnswer, 1)
	s.running.Add(1)
	go func() {
		defer s.running.Done()
		result <- s.query(st, ix, plan, hasIDTier)
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case answer := <-result:
		s.answered(st, ix, answer)
		return
	case <-timer.C:
		s.recordHealth(ix, fmt.Sprintf(messageTimeout, timeout))
		s.follow(st, st.fail(ix, fmt.Sprintf(messageTimeout, timeout)))
	}
	if answer := <-result; answer.err == nil {
		s.answered(st, ix, answer)
	}
}

func (s *SearchService) answered(st *search, ix Indexer, answer indexerAnswer) {
	if answer.err != nil {
		if st.ctx.Err() != nil {
			return
		}
		message := indexerErrorMessage(answer.err)
		s.recordHealth(ix, message)
		s.follow(st, st.fail(ix, message))
		return
	}
	if answer.asked {
		s.recordHealth(ix, "")
	}
	s.follow(st, st.answer(ix, answer.status, answer.message, st.input.decideAll(answer.items, ix)))
}

func (in *decisionInput) decideAll(items []TorznabItem, ix Indexer) []decidedRelease {
	decided := make([]decidedRelease, 0, len(items))
	for _, item := range items {
		c := in.newCandidate(item, ix)
		decided = append(decided, decidedRelease{Candidate: c, Rejections: decide(c, in)})
	}
	return decided
}

// query tries each tier in turn until one returns releases.
func (s *SearchService) query(st *search, ix Indexer, plan []TorznabQuery, hasIDTier bool) indexerAnswer {
	answer := indexerAnswer{status: api.IndexerDone}
	for _, q := range plan {
		items, asked, err := s.cachedSearch(st, ix, q)
		answer.asked = answer.asked || asked
		if err != nil {
			answer.err = err
			return answer
		}
		if len(items) > 0 {
			answer.items = items
			return answer
		}
	}
	if !hasIDTier {
		answer.status, answer.message = api.IndexerSkipped, messageSkipped
	}
	return answer
}

func (s *SearchService) cachedSearch(st *search, ix Indexer, q TorznabQuery) (items []TorznabItem, asked bool, err error) {
	key := fmt.Sprintf("%d|%s", ix.ProwlarrID, q.cacheKey())
	now := s.now()
	s.mu.Lock()
	cached, hit := s.results[key]
	s.mu.Unlock()
	if hit && now.Before(cached.expires) {
		return cached.items, false, nil
	}
	items, err = s.deps.Prowlarr.Search(st.ctx, ix.ProwlarrID, q)
	if err != nil || st.cacheFor == 0 {
		return items, true, err
	}
	s.mu.Lock()
	s.results[key] = cachedResult{items: items, expires: now.Add(st.cacheFor)}
	s.mu.Unlock()
	return items, true, nil
}

func indexerErrorMessage(err error) string {
	var failed *ErrIndexerFailed
	switch {
	case errors.As(err, &failed):
		return failed.Message
	case errors.Is(err, ErrTimeout):
		return "Prowlarr didn't answer in time"
	case errors.Is(err, ErrUnauthorized):
		return "Prowlarr rejected the API key"
	case errors.Is(err, ErrUnavailable):
		return "Prowlarr is unreachable"
	}
	return err.Error()
}

// recordHealth writes an indexer's answer to its health; failure is "" for a good answer. Writes outlive
// Close's cancel, which waits for them.
func (s *SearchService) recordHealth(ix Indexer, failure string) {
	err := s.deps.Repo.RecordIndexerAnswer(context.WithoutCancel(s.life), ix.ProwlarrID, failure, s.now().UTC())
	if err != nil {
		slog.Error("recording indexer health", "indexer", ix.Name, "error", err)
	}
}

func (s *SearchService) publishCompleted(st *search, done api.SearchCompleted) {
	if err := recordCompletion(context.WithoutCancel(s.life), s.deps.DB, s.deps.Bus, st, done, s.now().UTC()); err != nil {
		slog.Error("publishing SearchCompleted", "search", done.Search, "error", err)
	}
}

// recordCompletion stores a search's counts and publishes SearchCompleted, unless the search is quiet.
func recordCompletion(ctx context.Context, d *db.DB, b *bus.Bus, st *search, done api.SearchCompleted, at time.Time) error {
	return d.Write(ctx, func(tx *gorm.DB) error {
		if err := completeSearch(tx, done, at); err != nil || st.quiet {
			return err
		}
		return b.Publish(tx, bus.EventOf(done))
	})
}

// Search returns a search's cached state. ErrSearchNotFound once it expired.
func (s *SearchService) Search(id domain.SearchID) (SearchSnapshot, error) {
	st, err := s.deps.Searches.find(id)
	if err != nil {
		return SearchSnapshot{}, err
	}
	return st.snapshot(), nil
}

// Cancel stops a search's in-flight queries; indexers that hadn't answered report FAILED. Its results stay cached.
func (s *SearchService) Cancel(id domain.SearchID) error {
	st, err := s.deps.Searches.find(id)
	if err != nil {
		return err
	}
	s.cancel(st)
	return nil
}

func (s *SearchService) cancel(st *search) {
	st.cancel()
	s.follow(st, st.cancelPending(messageCancelled))
}

// expire drops a search from the cache. An automatic grab that never ran no longer holds up its completion.
func (s *SearchService) expire(st *search) {
	st.stopTimers()
	s.cancel(st)
	s.follow(st, st.finishSettle(false))
	s.deps.Searches.remove(st.id)
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, cached := range s.results {
		if !s.now().Before(cached.expires) {
			delete(s.results, key)
		}
	}
}
