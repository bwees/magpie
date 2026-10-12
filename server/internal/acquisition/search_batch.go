package acquisition

import (
	"context"
	"errors"
	"slices"
	"time"

	"magpie/internal/acquisition/api"
	"magpie/internal/domain"

	"gorm.io/gorm"
)

// offer is a release listed outside a search: an RSS item or a held release.
type offer struct {
	Item    TorznabItem
	Indexer Indexer
}

// decideBatch decides releases that arrived without a search, such as an RSS feed's items for one title, and
// grabs the best like an automatic search. It publishes no search events; only its grabs show.
func (s *SearchService) decideBatch(ctx context.Context, req SearchRequest, in *decisionInput, offers []offer) error {
	options, err := IndexerOptionsKey.Get(ctx, s.deps.Store)
	if err != nil {
		return err
	}
	st := s.newSearch(req, in, options)
	st.quiet = true
	st.emit = func(api.SearchResultsReceived) {}
	names := []string{}
	st.mu.Lock()
	for _, o := range offers {
		st.add(o.Indexer, in.decideAll([]TorznabItem{o.Item}, o.Indexer))
		if !slices.Contains(names, o.Indexer.Name) {
			names = append(names, o.Indexer.Name)
		}
	}
	st.mu.Unlock()

	started := api.SearchStarted{Search: st.id, Title: req.Title, Episodes: nonNil(req.Episodes), Trigger: req.Trigger, Indexers: names}
	if err := s.deps.DB.Write(ctx, func(tx *gorm.DB) error { return insertSearch(tx, started, st.startedAt) }); err != nil {
		st.cancel()
		return err
	}
	s.keep(st, options)
	s.follow(st, st.stepNow())
	return nil
}

// ReviewPending grabs the best held release for each item whose oldest held release has waited out the delay.
func (s *SearchService) ReviewPending(ctx context.Context) error {
	held, err := s.deps.Repo.PendingReleases(ctx, "")
	if err != nil {
		return err
	}
	delay, err := DefaultDelayKey.Get(ctx, s.deps.Store)
	if err != nil {
		return err
	}
	titles := []domain.TitleID{}
	byTitle := map[domain.TitleID][]PendingRelease{}
	for _, p := range held {
		if _, seen := byTitle[p.Title]; !seen {
			titles = append(titles, p.Title)
		}
		byTitle[p.Title] = append(byTitle[p.Title], p)
	}
	var errs []error
	for _, title := range titles {
		if batch := dueBatch(byTitle[title], delay.Std(), s.now()); len(batch) > 0 {
			errs = append(errs, s.grabPending(ctx, title, batch))
		}
	}
	return errors.Join(errs...)
}

// dueBatch is every held release covering an item whose delay passed, so the best of them wins.
func dueBatch(held []PendingRelease, delay time.Duration, now time.Time) []PendingRelease {
	due := itemSet{}
	for _, p := range held {
		if !p.PublishAt.Add(delay).After(now) {
			due.add(p.Episodes)
		}
	}
	return slices.DeleteFunc(slices.Clone(held), func(p PendingRelease) bool { return !due.overlaps(p.Episodes) })
}

// grabPending takes a batch out of the pending queue and decides it again, without the delay.
func (s *SearchService) grabPending(ctx context.Context, title domain.TitleID, batch []PendingRelease) error {
	ids := make([]string, 0, len(batch))
	for _, p := range batch {
		ids = append(ids, p.ID)
	}
	if err := s.deps.DB.Write(ctx, func(tx *gorm.DB) error { return deletePending(tx, ids) }); err != nil {
		return err
	}
	req := SearchRequest{Title: title, Trigger: batch[0].Trigger}
	in, _, err := s.decisionInput(ctx, req)
	if errors.Is(err, ErrTitleNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	in.Delay, in.Pending = 0, nil
	offers := make([]offer, 0, len(batch))
	for _, p := range batch {
		ix, err := s.deps.Repo.Indexer(ctx, p.IndexerID)
		if errors.Is(err, api.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		offers = append(offers, offer{Item: p.Release, Indexer: ix})
	}
	return s.decideBatch(ctx, req, in, offers)
}
