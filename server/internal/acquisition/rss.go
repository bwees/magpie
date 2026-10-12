package acquisition

import (
	"context"
	"errors"
	"slices"

	"magpie/internal/acquisition/api"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	"magpie/internal/platform/parse"
)

// QueueFeeds queues one RSS read per indexer, so a slow or failing indexer holds up no other.
func (s *SearchService) QueueFeeds(ctx context.Context) error {
	indexers, err := s.deps.Repo.Indexers(ctx, false)
	if err != nil {
		return err
	}
	for _, ix := range indexers {
		if err := s.deps.Jobs.Enqueue(ctx, rssFeedTask{Indexer: ix.ProwlarrID}); err != nil {
			return err
		}
	}
	return nil
}

// SyncFeed reads one indexer's RSS feed, matches its items to wanted titles and decides them like an
// automatic search would, with the RSS-only specs.
func (s *SearchService) SyncFeed(ctx context.Context, prowlarrID int) error {
	ix, err := s.deps.Repo.Indexer(ctx, prowlarrID)
	if errors.Is(err, api.ErrNotFound) {
		return nil
	}
	if err != nil || ix.Status == IndexerGone {
		return err
	}
	items, err := s.deps.Prowlarr.RSS(ctx, prowlarrID, feedCategories(ix.Caps))
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		s.recordHealth(ix, indexerErrorMessage(err))
		return err
	}
	s.recordHealth(ix, "")
	matches, err := s.matchFeed(ctx, items)
	if err != nil {
		return err
	}
	var errs []error
	for _, m := range matches {
		req := SearchRequest{Title: m.title.ID, Trigger: api.TriggerRSS}
		in, _, err := s.decisionInput(ctx, req)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		offers := make([]offer, 0, len(m.items))
		for _, item := range m.items {
			offers = append(offers, offer{Item: item, Indexer: ix})
		}
		errs = append(errs, s.decideBatch(ctx, req, in, offers))
	}
	return errors.Join(errs...)
}

// feedCategories asks a feed for the indexer's movie and TV categories only.
func feedCategories(caps TorznabCaps) []int {
	return append(kindCategories(caps.Categories, domain.MediaKindMovie), kindCategories(caps.Categories, domain.MediaKindSeries)...)
}

type feedMatch struct {
	title catalogapi.Title
	items []TorznabItem
}

// wantedNames indexes the titles that want something by kind and cleaned name, original name and aliases.
type wantedNames map[domain.MediaKind]map[string][]catalogapi.Title

func (s *SearchService) wantedNames(ctx context.Context) (wantedNames, error) {
	monitored := true
	titles, err := s.deps.Catalog.Titles(ctx, catalogapi.TitleFilter{Monitored: &monitored})
	if err != nil {
		return nil, err
	}
	names := wantedNames{domain.MediaKindMovie: {}, domain.MediaKindSeries: {}}
	for _, t := range titles {
		_, wanted, err := s.deps.Catalog.Wanted(ctx, t.ID)
		if err != nil {
			return nil, err
		}
		if !wanted {
			continue
		}
		for _, name := range append([]string{t.Name, t.OriginalName}, t.Aliases...) {
			if clean := cleanTitle(name); clean != "" && !slices.ContainsFunc(names[t.Kind][clean], sameTitle(t)) {
				names[t.Kind][clean] = append(names[t.Kind][clean], t)
			}
		}
	}
	return names, nil
}

func sameTitle(t catalogapi.Title) func(catalogapi.Title) bool {
	return func(other catalogapi.Title) bool { return other.ID == t.ID }
}

// matchFeed groups feed items by the wanted title their parsed name and year match. The decision engine
// then checks the episodes, monitoring and release dates.
func (s *SearchService) matchFeed(ctx context.Context, items []TorznabItem) ([]feedMatch, error) {
	names, err := s.wantedNames(ctx)
	if err != nil {
		return nil, err
	}
	matches := []feedMatch{}
	for _, item := range items {
		for _, t := range names.match(item) {
			i := slices.IndexFunc(matches, func(m feedMatch) bool { return m.title.ID == t.ID })
			if i < 0 {
				matches = append(matches, feedMatch{title: t})
				i = len(matches) - 1
			}
			matches[i].items = append(matches[i].items, item)
		}
	}
	return matches, nil
}

func (n wantedNames) match(item TorznabItem) []catalogapi.Title {
	found := []catalogapi.Title{}
	for _, kind := range feedKinds(item.Categories) {
		r, err := parse.ParseRelease(item.Title, kind)
		if err != nil {
			continue
		}
		for _, name := range append([]string{r.Title}, r.Aliases...) {
			for _, t := range n[kind][cleanTitle(name)] {
				yearOff := kind == domain.MediaKindMovie && r.Year != 0 && t.Year != 0 && abs(r.Year-t.Year) > yearTolerance
				if !yearOff && !slices.ContainsFunc(found, sameTitle(t)) {
					found = append(found, t)
				}
			}
		}
	}
	return found
}

// feedKinds reads an item's kind from its Newznab categories; an item without either kind may be both.
func feedKinds(categories []int) []domain.MediaKind {
	kinds := []domain.MediaKind{}
	if len(kindCategories(categories, domain.MediaKindMovie)) > 0 {
		kinds = append(kinds, domain.MediaKindMovie)
	}
	if len(kindCategories(categories, domain.MediaKindSeries)) > 0 {
		kinds = append(kinds, domain.MediaKindSeries)
	}
	if len(kinds) == 0 {
		return []domain.MediaKind{domain.MediaKindMovie, domain.MediaKindSeries}
	}
	return kinds
}
