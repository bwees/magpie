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
	"magpie/internal/platform/parse"
	"magpie/internal/platform/settings"

	"go.uber.org/fx"
	"gorm.io/gorm"
)

var (
	ErrSearchNotFound  = errors.New("acquisition: search not found")
	ErrTitleNotFound   = errors.New("acquisition: title not found")
	ErrInvalidSearch   = errors.New("acquisition: invalid search")
	ErrReleaseNotFound = errors.New("acquisition: release not found in the search")
	ErrReleaseRejected = errors.New("acquisition: the release was rejected; grab it with override")
	// ErrGrabUnavailable is returned for a valid grab until grabbing into qBittorrent exists.
	ErrGrabUnavailable = errors.New("acquisition: grabbing isn't available yet")
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
}

// SearchService fans a search out to every indexer, decides each release as its indexer answers, and
// keeps the results in memory for the result-cache time.
type SearchService struct {
	deps SearchDeps
	now  func() time.Time

	life    context.Context
	stop    context.CancelFunc
	running sync.WaitGroup

	mu       sync.Mutex
	searches map[domain.SearchID]*search
	results  map[string]cachedResult
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
		deps:     deps,
		now:      time.Now,
		life:     life,
		stop:     stop,
		searches: map[domain.SearchID]*search{},
		results:  map[string]cachedResult{},
	}
}

// Close cancels every search and waits for its indexer queries to return.
func (s *SearchService) Close() {
	s.stop()
	s.mu.Lock()
	for _, st := range s.searches {
		st.mu.Lock()
		st.expiry.Stop()
		st.mu.Unlock()
	}
	s.mu.Unlock()
	s.running.Wait()
}

// Start records the search, publishes SearchStarted and asks every indexer whose caps cover the title's
// kind in parallel. Results stream out as SearchResultsReceived.
func (s *SearchService) Start(ctx context.Context, req SearchRequest) (SearchSnapshot, error) {
	in, target, err := s.decisionInput(ctx, req)
	if err != nil {
		return SearchSnapshot{}, err
	}
	options, err := IndexerOptionsKey.Get(ctx, s.deps.Store)
	if err != nil {
		return SearchSnapshot{}, err
	}
	indexers, err := s.deps.Repo.Indexers(ctx, false)
	if err != nil {
		return SearchSnapshot{}, err
	}

	st := &search{
		id:        domain.NewID[domain.SearchID](),
		trigger:   req.Trigger,
		startedAt: s.now().UTC(),
		input:     in,
		cacheFor:  options.ResultCache.Std(),
		keys:      map[releaseKey]bool{},
	}
	st.ctx, st.cancel = context.WithCancel(s.life)
	st.emit = func(e api.SearchResultsReceived) { s.deps.Bus.Emit(bus.EventOf(e)) }
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

	st.mu.Lock()
	st.expiry = time.AfterFunc(searchRetention(options), func() { s.expire(st) })
	st.mu.Unlock()
	s.mu.Lock()
	s.searches[st.id] = st
	s.mu.Unlock()

	timeout := options.SearchTimeout.Std()
	for _, run := range st.runs {
		plan, hasIDTier := queryPlan(run.Indexer.Caps, target)
		s.running.Add(1)
		go s.ask(st, run.Indexer, plan, hasIDTier, timeout)
	}
	if done, completed := st.completionNow(); completed {
		s.publishCompleted(done)
	}
	return st.snapshot(), nil
}

// searchRetention is how long a search's results stay cached. Turning the per-query cache off doesn't
// drop a search while its dialog may still be open.
func searchRetention(o IndexerOptions) time.Duration {
	return cmp.Or(o.ResultCache.Std(), DefaultResultCache)
}

func (st *search) completionNow() (api.SearchCompleted, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.completion()
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

	profileID := title.Profile
	if profileID == "" {
		if profileID, err = DefaultProfileKey.Get(ctx, s.deps.Store); err != nil {
			return nil, searchTarget{}, err
		}
	}
	if in.Profile, err = s.deps.Quality.ScoringProfile(ctx, profileID); err != nil {
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
		release := fileRelease(f, title.Kind)
		facts := ReleaseFacts{Release: release, SizeBytes: f.Size, OriginalLanguage: title.OriginalLanguage}
		scored = append(scored, existingFile{File: f, Eval: Evaluate(facts, p), Group: release.Group})
	}
	return scored
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
		if done, completed := st.fail(ix, fmt.Sprintf(messageTimeout, timeout)); completed {
			s.publishCompleted(done)
		}
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
		if done, completed := st.fail(ix, message); completed {
			s.publishCompleted(done)
		}
		return
	}
	if answer.asked {
		s.recordHealth(ix, "")
	}
	decided := make([]decidedRelease, 0, len(answer.items))
	for _, item := range answer.items {
		c := st.input.newCandidate(item, ix)
		decided = append(decided, decidedRelease{Candidate: c, Rejections: decide(c, st.input)})
	}
	if done, completed := st.answer(ix, answer.status, answer.message, decided); completed {
		s.publishCompleted(done)
	}
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

func (s *SearchService) publishCompleted(done api.SearchCompleted) {
	err := s.deps.DB.Write(context.WithoutCancel(s.life), func(tx *gorm.DB) error {
		if err := completeSearch(tx, done, s.now().UTC()); err != nil {
			return err
		}
		return s.deps.Bus.Publish(tx, bus.EventOf(done))
	})
	if err != nil {
		slog.Error("publishing SearchCompleted", "search", done.Search, "error", err)
	}
}

func (s *SearchService) find(id domain.SearchID) (*search, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.searches[id]
	if !ok {
		return nil, ErrSearchNotFound
	}
	return st, nil
}

// Search returns a search's cached state. ErrSearchNotFound once it expired.
func (s *SearchService) Search(id domain.SearchID) (SearchSnapshot, error) {
	st, err := s.find(id)
	if err != nil {
		return SearchSnapshot{}, err
	}
	return st.snapshot(), nil
}

// Cancel stops a search's in-flight queries; indexers that hadn't answered report FAILED. Its results stay cached.
func (s *SearchService) Cancel(id domain.SearchID) error {
	st, err := s.find(id)
	if err != nil {
		return err
	}
	s.cancel(st)
	return nil
}

func (s *SearchService) cancel(st *search) {
	st.cancel()
	if done, completed := st.cancelPending(messageCancelled); completed {
		s.publishCompleted(done)
	}
}

func (s *SearchService) expire(st *search) {
	s.cancel(st)
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.searches, st.id)
	for key, cached := range s.results {
		if !s.now().Before(cached.expires) {
			delete(s.results, key)
		}
	}
}

// Grab checks a release can be grabbed: it is in the search's cache, and accepted unless override is set.
// Grabbing itself comes with downloads; until then a valid grab returns ErrGrabUnavailable.
func (s *SearchService) Grab(id domain.SearchID, release domain.ReleaseID, override bool) error {
	st, err := s.find(id)
	if err != nil {
		return err
	}
	d, ok := st.release(release)
	if !ok {
		return ErrReleaseNotFound
	}
	if !d.accepted() && !override {
		return ErrReleaseRejected
	}
	return ErrGrabUnavailable
}
