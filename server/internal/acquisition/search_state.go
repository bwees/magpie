package acquisition

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"time"

	"magpie/internal/acquisition/api"
	"magpie/internal/domain"
)

// IndexerPending is an indexer that hasn't answered yet. Events carry only the three final statuses.
const IndexerPending api.IndexerRunStatus = "PENDING"

// IndexerRun is one indexer's part in a search.
type IndexerRun struct {
	Indexer Indexer
	Status  api.IndexerRunStatus
	// Message says why it FAILED or was SKIPPED.
	Message string
}

// SearchSnapshot is a search's cached state, for a dialog that reconnects or refreshes.
type SearchSnapshot struct {
	ID        domain.SearchID
	Title     domain.TitleID
	Trigger   api.SearchTrigger
	StartedAt time.Time
	Completed bool
	Cancelled bool
	Profile   api.Profile
	Indexers  []IndexerRun
	// Releases are in arrival order, ranked as of now.
	Releases []api.ReleaseDecision
}

type releaseKey struct {
	indexer int
	guid    string
}

// search is one search and its cached results. Every change emits under mu, so the events of one
// search reach the live gateway in the order they were decided.
type search struct {
	id        domain.SearchID
	trigger   api.SearchTrigger
	startedAt time.Time
	input     *decisionInput
	// cacheFor is how long an indexer's answer to one query is reused.
	cacheFor time.Duration
	ctx      context.Context
	cancel   context.CancelFunc
	emit     func(api.SearchResultsReceived)

	mu        sync.Mutex
	expiry    *time.Timer
	runs      []IndexerRun
	releases  []decidedRelease
	keys      map[releaseKey]bool
	completed bool
	cancelled bool
}

func (st *search) runIndex(prowlarrID int) int {
	return slices.IndexFunc(st.runs, func(r IndexerRun) bool { return r.Indexer.ProwlarrID == prowlarrID })
}

// fail reports an indexer FAILED unless it already answered. It returns the completion when this was
// the last indexer to report.
func (st *search) fail(ix Indexer, message string) (api.SearchCompleted, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	i := st.runIndex(ix.ProwlarrID)
	if i < 0 || st.runs[i].Status != IndexerPending {
		return api.SearchCompleted{}, false
	}
	st.runs[i].Status, st.runs[i].Message = api.IndexerFailed, message
	st.emit(api.SearchResultsReceived{Search: st.id, Indexer: ix.Name, Status: api.IndexerFailed, Message: message, Releases: []api.ReleaseDecision{}})
	return st.completion()
}

// answer adds an indexer's decided releases. A late answer replaces the FAILED its timeout reported,
// as long as the search is still open.
func (st *search) answer(ix Indexer, status api.IndexerRunStatus, message string, decided []decidedRelease) (api.SearchCompleted, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	i := st.runIndex(ix.ProwlarrID)
	if i < 0 || st.cancelled || st.runs[i].Status == api.IndexerDone || st.runs[i].Status == api.IndexerSkipped {
		return api.SearchCompleted{}, false
	}
	st.runs[i].Status, st.runs[i].Message = status, message

	added := make([]domain.ReleaseID, 0, len(decided))
	for _, d := range decided {
		key := releaseKey{indexer: ix.ProwlarrID, guid: releaseGUID(d.Candidate.Item)}
		if st.keys[key] {
			continue
		}
		st.keys[key] = true
		d.ID = domain.NewID[domain.ReleaseID]()
		d.Arrival = len(st.releases)
		st.releases = append(st.releases, d)
		added = append(added, d.ID)
	}

	current := ranks(st.releases)
	decisions := make([]api.ReleaseDecision, 0, len(added))
	for _, d := range st.releases[len(st.releases)-len(added):] {
		decisions = append(decisions, d.decision(current[d.ID]))
	}
	st.emit(api.SearchResultsReceived{Search: st.id, Indexer: ix.Name, Status: status, Message: message, Releases: decisions})
	return st.completion()
}

// cancelPending marks every indexer still running FAILED with message, so the search completes.
func (st *search) cancelPending(message string) (api.SearchCompleted, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.cancelled {
		return api.SearchCompleted{}, false
	}
	st.cancelled = true
	for i, run := range st.runs {
		if run.Status != IndexerPending {
			continue
		}
		st.runs[i].Status, st.runs[i].Message = api.IndexerFailed, message
		st.emit(api.SearchResultsReceived{Search: st.id, Indexer: run.Indexer.Name, Status: api.IndexerFailed, Message: message, Releases: []api.ReleaseDecision{}})
	}
	return st.completion()
}

// completion returns SearchCompleted the first time every indexer has reported. Caller holds mu.
func (st *search) completion() (api.SearchCompleted, bool) {
	if st.completed || slices.ContainsFunc(st.runs, func(r IndexerRun) bool { return r.Status == IndexerPending }) {
		return api.SearchCompleted{}, false
	}
	st.completed = true
	done := api.SearchCompleted{
		Search:         st.id,
		Title:          st.input.Title.ID,
		Total:          len(st.releases),
		FailedIndexers: []string{},
		Outcome:        api.OutcomeNothingAccepted,
	}
	for _, d := range st.releases {
		if d.accepted() {
			done.Accepted++
		}
	}
	for _, run := range st.runs {
		if run.Status == api.IndexerFailed {
			done.FailedIndexers = append(done.FailedIndexers, run.Indexer.Name)
		}
	}
	return done, true
}

func (st *search) release(id domain.ReleaseID) (decidedRelease, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	i := slices.IndexFunc(st.releases, func(d decidedRelease) bool { return d.ID == id })
	if i < 0 {
		return decidedRelease{}, false
	}
	return st.releases[i], true
}

func (st *search) snapshot() SearchSnapshot {
	st.mu.Lock()
	defer st.mu.Unlock()
	current := ranks(st.releases)
	decisions := make([]api.ReleaseDecision, 0, len(st.releases))
	for _, d := range st.releases {
		decisions = append(decisions, d.decision(current[d.ID]))
	}
	runs := slices.Clone(st.runs)
	slices.SortFunc(runs, func(a, b IndexerRun) int { return cmp.Compare(a.Indexer.Name, b.Indexer.Name) })
	return SearchSnapshot{
		ID:        st.id,
		Title:     st.input.Title.ID,
		Trigger:   st.trigger,
		StartedAt: st.startedAt,
		Completed: st.completed,
		Cancelled: st.cancelled,
		Profile:   st.input.Profile.Profile(),
		Indexers:  runs,
		Releases:  decisions,
	}
}

// releaseGUID identifies a release within one indexer's answers.
func releaseGUID(item TorznabItem) string {
	return cmp.Or(item.GUID, item.DownloadURL, item.MagnetURI, item.Title)
}

func (d decidedRelease) decision(rank int) api.ReleaseDecision {
	c := d.Candidate
	return api.ReleaseDecision{
		Release:     d.ID,
		Name:        c.Item.Title,
		Indexer:     c.Indexer.Name,
		InfoHash:    c.Item.InfoHash,
		Quality:     c.Eval.Quality,
		Formats:     nonNil(c.Eval.Formats),
		CFScore:     c.Eval.Score,
		SizeBytes:   c.Item.SizeBytes,
		Seeders:     c.Item.Seeders,
		PublishedAt: c.Item.PublishedAt,
		Freeleech:   c.freeleech(),
		Rejections:  nonNil(d.Rejections),
		Rank:        rank,
	}
}
