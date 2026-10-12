package acquisition

import (
	"context"
	"errors"
	"slices"

	"magpie/internal/acquisition/api"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	"magpie/internal/platform/db"

	"gorm.io/gorm"
)

// searchedOnAdd are the origins whose titles are searched as soon as they are added. Scanned and migrated
// titles wait for the backlog sweeps.
var searchedOnAdd = []domain.Origin{domain.OriginRequest, domain.OriginManual}

// SearchAdded queues an AUTO search for what a newly requested or added title wants, when the setting is on.
func (s *SearchService) SearchAdded(ctx context.Context, tx *gorm.DB, e catalogapi.TitleAdded) error {
	if !slices.Contains(searchedOnAdd, e.Origin) {
		return nil
	}
	on, err := SearchOnAddKey.GetTx(tx)
	if err != nil || !on {
		return err
	}
	return s.queueWanted(ctx, tx, e.Title, nil)
}

// SearchReleased queues an AUTO search for items that just reached their release date.
func (s *SearchService) SearchReleased(ctx context.Context, tx *gorm.DB, e catalogapi.WantedReleased) error {
	return s.queueWanted(ctx, tx, e.Title, e.Episodes)
}

// SearchMonitored searches newly monitored released items and drops the held releases of unmonitored ones.
func (s *SearchService) SearchMonitored(ctx context.Context, tx *gorm.DB, e catalogapi.MonitoringChanged) error {
	movieUnmonitored := e.Movie != nil && !*e.Movie
	if len(e.Removed) > 0 || movieUnmonitored {
		unmonitored := itemSet{}
		unmonitored.add(e.Removed)
		if err := dropPending(tx, e.Title, unmonitored); err != nil {
			return err
		}
	}
	switch {
	case e.Movie != nil && *e.Movie:
		return s.queueWanted(ctx, tx, e.Title, nil)
	case len(e.Added) > 0:
		return s.queueWanted(ctx, tx, e.Title, e.Added)
	}
	return nil
}

// ForgetTitle stops everything for a removed title: its searches, its held releases and its downloads.
func (s *SearchService) ForgetTitle(_ context.Context, tx *gorm.DB, e catalogapi.TitleRemoved) error {
	if err := dropPending(tx, e.Title, nil); err != nil {
		return err
	}
	cancelSearches := func() { s.running.Go(func() { s.cancelTitle(e.Title) }) }
	if err := db.AfterCommit(tx, cancelSearches); err != nil {
		return err
	}
	return s.deps.Jobs.EnqueueTx(tx, removeTitleTask{Title: e.Title, DeleteFiles: e.DeleteFiles})
}

func (s *SearchService) cancelTitle(title domain.TitleID) {
	for _, st := range s.deps.Searches.all() {
		if st.input.Title.ID == title {
			s.cancel(st)
		}
	}
}

// queueWanted queues one AUTO search for the title's monitored, released items, narrowed to only when it is
// set. A movie is searched when it is wanted.
func (s *SearchService) queueWanted(ctx context.Context, tx *gorm.DB, title domain.TitleID, only []domain.EpisodeID) error {
	episodes, wanted, err := s.deps.Catalog.Wanted(ctx, title)
	if errors.Is(err, catalogapi.ErrNotFound) || (err == nil && !wanted) {
		return nil
	}
	if err != nil {
		return err
	}
	task := searchTask{Title: title, Episodes: []domain.EpisodeID{}, Trigger: api.TriggerAuto}
	for _, ep := range episodes {
		if only == nil || slices.Contains(only, ep.ID) {
			task.Episodes = append(task.Episodes, ep.ID)
		}
	}
	if len(episodes) > 0 && len(task.Episodes) == 0 {
		return nil
	}
	return queueSearch(tx, s.deps.Jobs, task, s.now().UTC())
}
