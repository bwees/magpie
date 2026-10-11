// Package fakes holds in-memory fakes of Catalog's ports, for tests and for booting without real adapters.
package fakes

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"time"

	"magpie/internal/catalog"
	"magpie/internal/domain"
	"magpie/internal/platform/scripted"
)

const (
	TMDBMovie         scripted.Method = "TMDB.Movie"
	TMDBSeries        scripted.Method = "TMDB.Series"
	TMDBSeason        scripted.Method = "TMDB.Season"
	TMDBEpisodeGroups scripted.Method = "TMDB.EpisodeGroups"
	TMDBEpisodeGroup  scripted.Method = "TMDB.EpisodeGroup"
	TMDBFind          scripted.Method = "TMDB.Find"
	TMDBSearch        scripted.Method = "TMDB.Search"
	TMDBTrending      scripted.Method = "TMDB.Trending"
	TMDBChanges       scripted.Method = "TMDB.Changes"
	TMDBImage         scripted.Method = "TMDB.Image"

	// SearchPageSize matches TMDB's page size.
	SearchPageSize = 20
	firstPage      = 1
)

type seasonKey struct {
	series domain.TmdbID
	number int
}

type findKey struct {
	source, externalID string
}

type change struct {
	id domain.TmdbID
	at time.Time
}

// TMDB serves movies, series and images from memory. Languages and image sizes are ignored.
type TMDB struct {
	scripted.Script

	mu         sync.Mutex
	movies     map[domain.TmdbID]catalog.TmdbMovie
	series     map[domain.TmdbID]catalog.TmdbSeries
	seasons    map[seasonKey]catalog.TmdbSeason
	groupsOf   map[domain.TmdbID][]catalog.TmdbEpisodeGroupSummary
	groups     map[string]catalog.TmdbEpisodeGroup
	found      map[findKey]catalog.TmdbFindResult
	searchable []catalog.TmdbSearchItem
	trending   []catalog.TmdbSearchItem
	changes    map[domain.MediaKind][]change
	images     map[string][]byte
}

var _ catalog.TMDB = (*TMDB)(nil)

func NewTMDB() *TMDB {
	return &TMDB{
		movies:   map[domain.TmdbID]catalog.TmdbMovie{},
		series:   map[domain.TmdbID]catalog.TmdbSeries{},
		seasons:  map[seasonKey]catalog.TmdbSeason{},
		groupsOf: map[domain.TmdbID][]catalog.TmdbEpisodeGroupSummary{},
		groups:   map[string]catalog.TmdbEpisodeGroup{},
		found:    map[findKey]catalog.TmdbFindResult{},
		changes:  map[domain.MediaKind][]change{},
		images:   map[string][]byte{},
	}
}

// PutMovie adds a movie and makes it searchable.
func (f *TMDB) PutMovie(m catalog.TmdbMovie) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.movies[m.ID] = m
	item := catalog.TmdbSearchItem{ID: m.ID, Kind: domain.MediaKindMovie, Name: m.Title, OriginalName: m.OriginalTitle, Overview: m.Overview, PosterPath: m.Images.Poster}
	if m.ReleaseDate != nil {
		item.Year = m.ReleaseDate.Year()
	}
	f.searchable = append(f.searchable, item)
}

// PutSeries adds a series and makes it searchable.
func (f *TMDB) PutSeries(s catalog.TmdbSeries) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.series[s.ID] = s
	item := catalog.TmdbSearchItem{ID: s.ID, Kind: domain.MediaKindSeries, Name: s.Name, OriginalName: s.OriginalName, Overview: s.Overview, PosterPath: s.Images.Poster}
	if s.FirstAirDate != nil {
		item.Year = s.FirstAirDate.Year()
	}
	f.searchable = append(f.searchable, item)
}

func (f *TMDB) PutSeason(s catalog.TmdbSeason) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seasons[seasonKey{s.Series, s.Number}] = s
}

// PutEpisodeGroup adds a group and lists it under the series.
func (f *TMDB) PutEpisodeGroup(series domain.TmdbID, g catalog.TmdbEpisodeGroup) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.groups[g.ID] = g
	summary := catalog.TmdbEpisodeGroupSummary{ID: g.ID, Name: g.Name, Type: g.Type, GroupCount: len(g.Groups)}
	for _, season := range g.Groups {
		summary.EpisodeCount += len(season.Episodes)
	}
	f.groupsOf[series] = append(f.groupsOf[series], summary)
}

// PutFind sets what Find returns for an external ID.
func (f *TMDB) PutFind(source, externalID string, r catalog.TmdbFindResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.found[findKey{source, externalID}] = r
}

func (f *TMDB) SetTrending(items ...catalog.TmdbSearchItem) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.trending = items
}

// MarkChanged records that a movie or series changed at a time, for Changes.
func (f *TMDB) MarkChanged(kind domain.MediaKind, id domain.TmdbID, at time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.changes[kind] = append(f.changes[kind], change{id: id, at: at})
}

func (f *TMDB) PutImage(path string, data []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.images[path] = data
}

func (f *TMDB) Movie(ctx context.Context, id domain.TmdbID, _ string) (catalog.TmdbMovie, error) {
	if err := f.Enter(ctx, TMDBMovie); err != nil {
		return catalog.TmdbMovie{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return lookup(f.movies, id)
}

func (f *TMDB) Series(ctx context.Context, id domain.TmdbID, _ string) (catalog.TmdbSeries, error) {
	if err := f.Enter(ctx, TMDBSeries); err != nil {
		return catalog.TmdbSeries{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return lookup(f.series, id)
}

func (f *TMDB) Season(ctx context.Context, id domain.TmdbID, season int, _ string) (catalog.TmdbSeason, error) {
	if err := f.Enter(ctx, TMDBSeason); err != nil {
		return catalog.TmdbSeason{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return lookup(f.seasons, seasonKey{id, season})
}

func (f *TMDB) EpisodeGroups(ctx context.Context, id domain.TmdbID) ([]catalog.TmdbEpisodeGroupSummary, error) {
	if err := f.Enter(ctx, TMDBEpisodeGroups); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.series[id]; !ok {
		return nil, catalog.ErrTmdbNotFound
	}
	return append([]catalog.TmdbEpisodeGroupSummary{}, f.groupsOf[id]...), nil
}

func (f *TMDB) EpisodeGroup(ctx context.Context, groupID string) (catalog.TmdbEpisodeGroup, error) {
	if err := f.Enter(ctx, TMDBEpisodeGroup); err != nil {
		return catalog.TmdbEpisodeGroup{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return lookup(f.groups, groupID)
}

// Find returns an empty result for unknown IDs, as TMDB does.
func (f *TMDB) Find(ctx context.Context, source string, externalID string) (catalog.TmdbFindResult, error) {
	if err := f.Enter(ctx, TMDBFind); err != nil {
		return catalog.TmdbFindResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.found[findKey{source, externalID}], nil
}

// Search matches query case-insensitively against names, in the order items were added.
func (f *TMDB) Search(ctx context.Context, query string, page int) (catalog.TmdbSearchPage, error) {
	if err := f.Enter(ctx, TMDBSearch); err != nil {
		return catalog.TmdbSearchPage{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	needle := strings.ToLower(query)
	matched := []catalog.TmdbSearchItem{}
	for _, item := range f.searchable {
		if strings.Contains(strings.ToLower(item.Name), needle) || strings.Contains(strings.ToLower(item.OriginalName), needle) {
			matched = append(matched, item)
		}
	}
	page = max(page, firstPage)
	start := min((page-firstPage)*SearchPageSize, len(matched))
	end := min(start+SearchPageSize, len(matched))
	return catalog.TmdbSearchPage{
		Page:         page,
		TotalPages:   (len(matched) + SearchPageSize - 1) / SearchPageSize,
		TotalResults: len(matched),
		Items:        matched[start:end],
	}, nil
}

func (f *TMDB) Trending(ctx context.Context, _ string) ([]catalog.TmdbSearchItem, error) {
	if err := f.Enter(ctx, TMDBTrending); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]catalog.TmdbSearchItem{}, f.trending...), nil
}

func (f *TMDB) Changes(ctx context.Context, kind domain.MediaKind, since time.Time) ([]domain.TmdbID, error) {
	if err := f.Enter(ctx, TMDBChanges); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	ids := []domain.TmdbID{}
	for _, c := range f.changes[kind] {
		if c.at.After(since) {
			ids = append(ids, c.id)
		}
	}
	return ids, nil
}

func (f *TMDB) Image(ctx context.Context, path string, _ string) (io.ReadCloser, error) {
	if err := f.Enter(ctx, TMDBImage); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	data, err := lookup(f.images, path)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func lookup[K comparable, V any](m map[K]V, key K) (V, error) {
	v, ok := m[key]
	if !ok {
		return v, catalog.ErrTmdbNotFound
	}
	return v, nil
}
