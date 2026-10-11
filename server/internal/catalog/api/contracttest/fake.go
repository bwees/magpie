package contracttest

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"sync"

	"magpie/internal/catalog/api"
	"magpie/internal/domain"
	"magpie/internal/platform/scripted"
)

const (
	ReaderTitle           scripted.Method = "catalog.Reader.Title"
	ReaderTitleByTmdb     scripted.Method = "catalog.Reader.TitleByTmdb"
	ReaderTitles          scripted.Method = "catalog.Reader.Titles"
	ReaderEpisodes        scripted.Method = "catalog.Reader.Episodes"
	ReaderEpisode         scripted.Method = "catalog.Reader.Episode"
	ReaderEpisodeByNumber scripted.Method = "catalog.Reader.EpisodeByNumber"
	ReaderWanted          scripted.Method = "catalog.Reader.Wanted"
	ReaderImagePath       scripted.Method = "catalog.Reader.ImagePath"
	ReaderWantedLanguages scripted.Method = "catalog.Reader.WantedLanguages"
	ReaderGlobalLanguages scripted.Method = "catalog.Reader.GlobalLanguages"
)

type imageKey struct {
	title domain.TitleID
	kind  string
}

// Fake is the in-memory catalog.Reader other contexts test against. It passes Run.
type Fake struct {
	scripted.Script

	mu        sync.RWMutex
	titles    map[domain.TitleID]api.Title
	episodes  map[domain.EpisodeID]api.Episode
	images    map[imageKey]string
	global    []api.WantedLanguage
	overrides map[domain.TitleID][]api.WantedLanguage
}

var _ api.Reader = (*Fake)(nil)

func NewFake(f Fixture) *Fake {
	fake := &Fake{
		titles:    map[domain.TitleID]api.Title{},
		episodes:  map[domain.EpisodeID]api.Episode{},
		images:    map[imageKey]string{},
		overrides: map[domain.TitleID][]api.WantedLanguage{},
	}
	for _, t := range f.Titles {
		fake.PutTitle(t)
	}
	for _, e := range f.Episodes {
		fake.PutEpisode(e)
	}
	for _, img := range f.Images {
		fake.PutImage(img)
	}
	fake.SetGlobalLanguages(f.GlobalLanguages)
	for title, langs := range f.LanguageOverrides {
		fake.SetLanguageOverride(title, langs)
	}
	return fake
}

// PutTitle adds or replaces a Title.
func (f *Fake) PutTitle(t api.Title) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.titles[t.ID] = t
}

// RemoveTitle deletes a Title and its episodes, images and language override.
func (f *Fake) RemoveTitle(id domain.TitleID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.titles, id)
	delete(f.overrides, id)
	for eid, e := range f.episodes {
		if e.Title == id {
			delete(f.episodes, eid)
		}
	}
	for key := range f.images {
		if key.title == id {
			delete(f.images, key)
		}
	}
}

// PutEpisode adds or replaces an episode.
func (f *Fake) PutEpisode(e api.Episode) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.episodes[e.ID] = e
}

// PutImage records a cached image.
func (f *Fake) PutImage(img Image) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.images[imageKey{img.Title, img.Kind}] = img.Path
}

func (f *Fake) SetGlobalLanguages(langs []api.WantedLanguage) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.global = slices.Clone(langs)
}

// SetLanguageOverride sets a Title's override; nil clears it.
func (f *Fake) SetLanguageOverride(title domain.TitleID, langs []api.WantedLanguage) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if langs == nil {
		delete(f.overrides, title)
		return
	}
	f.overrides[title] = slices.Clone(langs)
}

func (f *Fake) Title(ctx context.Context, id domain.TitleID) (api.Title, error) {
	if err := f.Enter(ctx, ReaderTitle); err != nil {
		return api.Title{}, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	t, ok := f.titles[id]
	if !ok {
		return api.Title{}, api.ErrNotFound
	}
	return t, nil
}

func (f *Fake) TitleByTmdb(ctx context.Context, kind domain.MediaKind, id domain.TmdbID) (api.Title, error) {
	if err := f.Enter(ctx, ReaderTitleByTmdb); err != nil {
		return api.Title{}, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	for _, t := range f.titles {
		if t.Kind == kind && t.External.Tmdb == id {
			return t, nil
		}
	}
	return api.Title{}, api.ErrNotFound
}

func (f *Fake) Titles(ctx context.Context, filter api.TitleFilter) ([]api.Title, error) {
	if err := f.Enter(ctx, ReaderTitles); err != nil {
		return nil, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	titles := []api.Title{}
	for _, t := range f.titles {
		if matches(filter, t) {
			titles = append(titles, t)
		}
	}
	slices.SortFunc(titles, func(a, b api.Title) int {
		return cmp.Or(cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)), cmp.Compare(a.Year, b.Year))
	})
	return titles, nil
}

func matches(filter api.TitleFilter, t api.Title) bool {
	switch {
	case filter.Kind != nil && *filter.Kind != t.Kind:
		return false
	case filter.Monitored != nil && *filter.Monitored != t.Monitored:
		return false
	case filter.Released != nil && *filter.Released != t.Released:
		return false
	case len(filter.IDs) > 0 && !slices.Contains(filter.IDs, t.ID):
		return false
	}
	return true
}

func (f *Fake) Episodes(ctx context.Context, title domain.TitleID) ([]api.Episode, error) {
	if err := f.Enter(ctx, ReaderEpisodes); err != nil {
		return nil, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	if _, ok := f.titles[title]; !ok {
		return nil, api.ErrNotFound
	}
	return f.episodesOf(title, func(api.Episode) bool { return true }), nil
}

func (f *Fake) Episode(ctx context.Context, id domain.EpisodeID) (api.Episode, error) {
	if err := f.Enter(ctx, ReaderEpisode); err != nil {
		return api.Episode{}, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	e, ok := f.episodes[id]
	if !ok {
		return api.Episode{}, api.ErrNotFound
	}
	return e, nil
}

func (f *Fake) EpisodeByNumber(ctx context.Context, title domain.TitleID, season, number int) (api.Episode, error) {
	if err := f.Enter(ctx, ReaderEpisodeByNumber); err != nil {
		return api.Episode{}, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	for _, e := range f.episodes {
		if e.Title == title && e.Season == season && e.Number == number {
			return e, nil
		}
	}
	return api.Episode{}, api.ErrNotFound
}

func (f *Fake) Wanted(ctx context.Context, title domain.TitleID) ([]api.Episode, bool, error) {
	if err := f.Enter(ctx, ReaderWanted); err != nil {
		return nil, false, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	t, ok := f.titles[title]
	if !ok {
		return nil, false, api.ErrNotFound
	}
	if t.Kind == domain.MediaKindMovie {
		return []api.Episode{}, t.Monitored && t.Released, nil
	}
	if !t.Monitored {
		return []api.Episode{}, false, nil
	}
	wanted := f.episodesOf(title, func(e api.Episode) bool { return e.Monitored && e.Released })
	return wanted, len(wanted) > 0, nil
}

func (f *Fake) ImagePath(ctx context.Context, title domain.TitleID, kind string) (string, error) {
	if err := f.Enter(ctx, ReaderImagePath); err != nil {
		return "", err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	path, ok := f.images[imageKey{title, kind}]
	if !ok {
		return "", api.ErrNotFound
	}
	return path, nil
}

func (f *Fake) WantedLanguages(ctx context.Context, title domain.TitleID) ([]api.WantedLanguage, error) {
	if err := f.Enter(ctx, ReaderWantedLanguages); err != nil {
		return nil, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	if _, ok := f.titles[title]; !ok {
		return nil, api.ErrNotFound
	}
	if override, ok := f.overrides[title]; ok {
		return slices.Clone(override), nil
	}
	return slices.Clone(f.global), nil
}

func (f *Fake) GlobalLanguages(ctx context.Context) ([]api.WantedLanguage, error) {
	if err := f.Enter(ctx, ReaderGlobalLanguages); err != nil {
		return nil, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	return slices.Clone(f.global), nil
}

func (f *Fake) episodesOf(title domain.TitleID, keep func(api.Episode) bool) []api.Episode {
	episodes := []api.Episode{}
	for _, e := range f.episodes {
		if e.Title == title && keep(e) {
			episodes = append(episodes, e)
		}
	}
	slices.SortFunc(episodes, func(a, b api.Episode) int {
		return cmp.Or(cmp.Compare(a.Season, b.Season), cmp.Compare(a.Number, b.Number))
	})
	return episodes
}
