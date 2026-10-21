package acquisition

import (
	"cmp"
	"context"
	"slices"
	"strings"
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

// settlePhase is where a search that grabs on its own stands with that grab.
type settlePhase string

const (
	// settleNever: an interactive search, where the user grabs.
	settleNever   settlePhase = ""
	settleWaiting settlePhase = "WAITING"
	// settleQueued: the automatic grab is queued and the search completes once it ran.
	settleQueued settlePhase = "QUEUED"
	settleDone   settlePhase = "DONE"
)

// heldReasons are the rejections that send a release to the pending queue.
var heldReasons = []api.RejectionReason{api.RejectDelayed, api.RejectPending}

// searchStep is what a change to a search asks of the service: queue its automatic grab, publish its completion.
type searchStep struct {
	settle    bool
	completed bool
	done      api.SearchCompleted
}

// search is one search and its cached results. Every change emits under mu, so the events of one
// search reach the live gateway in the order they were decided.
type search struct {
	id        domain.SearchID
	trigger   api.SearchTrigger
	startedAt time.Time
	input     *decisionInput
	// quiet searches (RSS and the pending queue) publish no search events; only their grabs show.
	quiet bool
	// cacheFor is how long an indexer's answer to one query is reused.
	cacheFor time.Duration
	ctx      context.Context
	cancel   context.CancelFunc
	emit     func(api.SearchResultsReceived)

	mu        sync.Mutex
	expiry    *time.Timer
	window    *time.Timer
	runs      []IndexerRun
	releases  []decidedRelease
	keys      map[releaseKey]bool
	completed bool
	cancelled bool
	settle    settlePhase
	// windowClosed is set once the settle window passed.
	windowClosed bool
	// held is set when the automatic grab left releases in the pending queue.
	held bool
	// tried holds releases already grabbed or rejected, so the fallback never picks them again.
	tried map[domain.ReleaseID]bool
	// triedHashes catches the same torrent listed by another indexer.
	triedHashes map[string]bool
	grabbed     *domain.DownloadID
}

// step reports what the search's latest change asks for. Caller holds mu.
func (st *search) step() searchStep {
	s := searchStep{settle: st.claimSettle()}
	s.done, s.completed = st.completion()
	return s
}

// claimSettle moves a waiting search to its automatic grab once every indexer answered, a release nothing
// can beat arrived, or the settle window passed with something accepted. Caller holds mu.
func (st *search) claimSettle() bool {
	if st.settle != settleWaiting || st.cancelled {
		return false
	}
	early := slices.ContainsFunc(st.releases, func(d decidedRelease) bool { return d.accepted() && st.input.grabsEarly(d.Candidate) })
	windowPassed := st.windowClosed && slices.ContainsFunc(st.releases, decidedRelease.accepted)
	if !st.allReported() && !early && !windowPassed {
		return false
	}
	st.settle = settleQueued
	return true
}

func (st *search) allReported() bool {
	return !slices.ContainsFunc(st.runs, func(r IndexerRun) bool { return r.Status == IndexerPending })
}

func (st *search) closeWindow() searchStep {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.windowClosed = true
	return st.step()
}

// finishSettle records that the automatic grab ran; held says it left releases pending.
func (st *search) finishSettle(held bool) searchStep {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.settle == settleQueued {
		st.settle = settleDone
		st.held = held
	}
	return st.step()
}

func (st *search) isCancelled() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.cancelled
}

func (st *search) stopTimers() {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.expiry.Stop()
	if st.window != nil {
		st.window.Stop()
	}
}

// heldReleases are the releases waiting only on the delay or the pending queue, among those keep allows.
func (st *search) heldReleases(keep func(decidedRelease) bool) []decidedRelease {
	st.mu.Lock()
	defer st.mu.Unlock()
	return slices.DeleteFunc(slices.Clone(st.releases), func(d decidedRelease) bool {
		onlyHeld := !d.accepted() && !slices.ContainsFunc(d.Rejections, func(r api.Rejection) bool {
			return !r.Temporary || !slices.Contains(heldReasons, r.Reason)
		})
		return !onlyHeld || !keep(d)
	})
}

// try claims a release for a grab. It is false when the release was tried before.
func (st *search) try(id domain.ReleaseID) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.tried[id] {
		return false
	}
	st.tried[id] = true
	return true
}

// untry gives a release back after a grab that failed before reaching qBittorrent.
func (st *search) untry(id domain.ReleaseID) {
	st.mu.Lock()
	defer st.mu.Unlock()
	delete(st.tried, id)
}

func (st *search) noteHash(hash string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.triedHashes[strings.ToLower(hash)] = true
}

// nextUntried claims the best accepted release not tried yet among those keep allows, in the automatic
// order. A grabbing search skips a release whose agreed seeders are below MinAutomaticSeeders.
func (st *search) nextUntried(keep func(decidedRelease) bool) (decidedRelease, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	order := automaticRanking(st.releases)
	accepted := slices.DeleteFunc(slices.Clone(st.releases), func(d decidedRelease) bool {
		return !d.accepted() || st.tried[d.ID] || !keep(d) ||
			(d.Candidate.Item.InfoHash != "" && st.triedHashes[strings.ToLower(d.Candidate.Item.InfoHash)]) ||
			(grabbingSearches.has(st.trigger) && order.seeders(d) < MinAutomaticSeeders)
	})
	if len(accepted) == 0 {
		return decidedRelease{}, false
	}
	slices.SortFunc(accepted, order.compare)
	st.tried[accepted[0].ID] = true
	return accepted[0], true
}

// ranking is the order the search shows: the automatic one for a search that grabs on its own. Caller holds mu.
func (st *search) ranking() ranking {
	if st.trigger == api.TriggerInteractive {
		return interactiveRanking
	}
	return automaticRanking(st.releases)
}

// acceptedAny reports whether any release passed the decision engine.
func (st *search) acceptedAny() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return slices.ContainsFunc(st.releases, decidedRelease.accepted)
}

// grabbedAs records a grab, so a search that completes afterwards reports GRABBED.
func (st *search) grabbedAs(id domain.DownloadID) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.grabbed == nil {
		st.grabbed = &id
	}
}

func (st *search) runIndex(prowlarrID int) int {
	return slices.IndexFunc(st.runs, func(r IndexerRun) bool { return r.Indexer.ProwlarrID == prowlarrID })
}

// fail reports an indexer FAILED unless it already answered.
func (st *search) fail(ix Indexer, message string) searchStep {
	st.mu.Lock()
	defer st.mu.Unlock()
	i := st.runIndex(ix.ProwlarrID)
	if i < 0 || st.runs[i].Status != IndexerPending {
		return searchStep{}
	}
	st.runs[i].Status, st.runs[i].Message = api.IndexerFailed, message
	st.emit(api.SearchResultsReceived{Search: st.id, Title: st.input.Title.ID, Indexer: ix.Name, Status: api.IndexerFailed, Message: message, Releases: []api.ReleaseDecision{}})
	return st.step()
}

// answer adds an indexer's decided releases. A late answer replaces the FAILED its timeout reported,
// as long as the search is still open.
func (st *search) answer(ix Indexer, status api.IndexerRunStatus, message string, decided []decidedRelease) searchStep {
	st.mu.Lock()
	defer st.mu.Unlock()
	i := st.runIndex(ix.ProwlarrID)
	if i < 0 || st.cancelled || st.runs[i].Status == api.IndexerDone || st.runs[i].Status == api.IndexerSkipped {
		return searchStep{}
	}
	st.runs[i].Status, st.runs[i].Message = status, message
	added := st.add(ix, decided)

	current := ranks(st.releases, st.ranking())
	decisions := make([]api.ReleaseDecision, 0, len(added))
	for _, d := range st.releases[len(st.releases)-len(added):] {
		decisions = append(decisions, d.decision(current[d.ID]))
	}
	st.emit(api.SearchResultsReceived{Search: st.id, Title: st.input.Title.ID, Indexer: ix.Name, Status: status, Message: message, Releases: decisions})
	return st.step()
}

// add keeps the releases the search hasn't seen from that indexer yet. Caller holds mu.
func (st *search) add(ix Indexer, decided []decidedRelease) []domain.ReleaseID {
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
	return added
}

// cancelPending marks every indexer still running FAILED with message, so the search completes.
func (st *search) cancelPending(message string) searchStep {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.cancelled {
		return searchStep{}
	}
	st.cancelled = true
	for i, run := range st.runs {
		if run.Status != IndexerPending {
			continue
		}
		st.runs[i].Status, st.runs[i].Message = api.IndexerFailed, message
		st.emit(api.SearchResultsReceived{Search: st.id, Title: st.input.Title.ID, Indexer: run.Indexer.Name, Status: api.IndexerFailed, Message: message, Releases: []api.ReleaseDecision{}})
	}
	return st.step()
}

// completion returns SearchCompleted the first time every indexer has reported and any automatic grab
// has run. Caller holds mu.
func (st *search) completion() (api.SearchCompleted, bool) {
	grabPending := st.settle == settleQueued || (st.settle == settleWaiting && !st.cancelled)
	if st.completed || grabPending || !st.allReported() {
		return api.SearchCompleted{}, false
	}
	st.completed = true
	done := api.SearchCompleted{
		Search:         st.id,
		Title:          st.input.Title.ID,
		Total:          len(st.releases),
		FailedIndexers: []string{},
	}
	for _, d := range st.releases {
		if d.accepted() {
			done.Accepted++
		}
	}
	switch {
	case st.grabbed != nil:
		done.Outcome, done.Grabbed = api.OutcomeGrabbed, st.grabbed
	case st.held:
		done.Outcome = api.OutcomePendingDelay
	case st.settle == settleNever && done.Accepted > 0:
		done.Outcome = api.OutcomeReleasesFound
	case done.Total == 0:
		done.Outcome = api.OutcomeNoReleases
	default:
		done.Outcome = api.OutcomeNothingAccepted
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
	current := ranks(st.releases, st.ranking())
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
