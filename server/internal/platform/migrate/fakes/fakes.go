// Package fakes holds in-memory fakes of the migration's ports: Sonarr/Radarr, TMDB and the disk.
package fakes

import (
	"context"
	"slices"
	"strings"
	"sync"

	"magpie/internal/domain"
	"magpie/internal/platform/migrate"
	"magpie/internal/platform/scripted"
)

const (
	ArrStatus      scripted.Method = "Arr.Status"
	ArrRootFolders scripted.Method = "Arr.RootFolders"
	ArrLibrary     scripted.Method = "Arr.Library"

	TMDBSeries        scripted.Method = "TMDB.Series"
	TMDBSeason        scripted.Method = "TMDB.Season"
	TMDBEpisodeGroups scripted.Method = "TMDB.EpisodeGroups"
	TMDBEpisodeGroup  scripted.Method = "TMDB.EpisodeGroup"
	TMDBFind          scripted.Method = "TMDB.Find"
	TMDBSearch        scripted.Method = "TMDB.Search"
	TMDBMovie         scripted.Method = "TMDB.Movie"
)

// Arr serves one library per app. An app without a library is unreachable.
type Arr struct {
	scripted.Script

	mu        sync.Mutex
	libraries map[migrate.App]migrate.ArrLibrary
}

var _ migrate.Arr = (*Arr)(nil)

func NewArr() *Arr {
	return &Arr{libraries: map[migrate.App]migrate.ArrLibrary{}}
}

func (a *Arr) Put(lib migrate.ArrLibrary) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.libraries[lib.App] = lib
}

func (a *Arr) library(app migrate.App) (migrate.ArrLibrary, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	lib, ok := a.libraries[app]
	if !ok {
		return migrate.ArrLibrary{}, migrate.ErrArrUnreachable
	}
	return lib, nil
}

func (a *Arr) Status(ctx context.Context, app migrate.App, _ migrate.Connection) (migrate.ArrStatus, error) {
	if err := a.Enter(ctx, ArrStatus); err != nil {
		return migrate.ArrStatus{}, err
	}
	lib, err := a.library(app)
	return migrate.ArrStatus{AppName: string(app), Version: lib.Version}, err
}

func (a *Arr) RootFolders(ctx context.Context, app migrate.App, _ migrate.Connection) ([]migrate.ArrRootFolder, error) {
	if err := a.Enter(ctx, ArrRootFolders); err != nil {
		return nil, err
	}
	lib, err := a.library(app)
	return lib.RootFolders, err
}

func (a *Arr) Library(ctx context.Context, app migrate.App, _ migrate.Connection, progress func(done, total int)) (migrate.ArrLibrary, error) {
	if err := a.Enter(ctx, ArrLibrary); err != nil {
		return migrate.ArrLibrary{}, err
	}
	lib, err := a.library(app)
	if err == nil && progress != nil {
		total := len(lib.Series) + len(lib.Movies)
		progress(total, total)
	}
	return lib, err
}

type findKey struct {
	source, id string
}

// TMDB serves what tests put in it; anything else is ErrTmdbNotFound.
type TMDB struct {
	scripted.Script

	mu      sync.Mutex
	series  map[domain.TmdbID]migrate.TmdbSeries
	seasons map[domain.TmdbID]map[int]migrate.TmdbSeason
	groups  map[domain.TmdbID][]migrate.TmdbGroup
	finds   map[findKey]migrate.TmdbFound
	movies  map[domain.TmdbID]migrate.TmdbCandidate
	search  []migrate.TmdbCandidate
}

var _ migrate.TMDB = (*TMDB)(nil)

func NewTMDB() *TMDB {
	return &TMDB{
		series: map[domain.TmdbID]migrate.TmdbSeries{}, seasons: map[domain.TmdbID]map[int]migrate.TmdbSeason{},
		groups: map[domain.TmdbID][]migrate.TmdbGroup{}, finds: map[findKey]migrate.TmdbFound{}, movies: map[domain.TmdbID]migrate.TmdbCandidate{},
	}
}

// PutSeries stores a series and its seasons; the series lists exactly those seasons.
func (t *TMDB) PutSeries(series migrate.TmdbSeries, seasons ...migrate.TmdbSeason) {
	t.mu.Lock()
	defer t.mu.Unlock()
	series.Seasons = nil
	t.seasons[series.Tmdb] = map[int]migrate.TmdbSeason{}
	for _, s := range seasons {
		series.Seasons = append(series.Seasons, s.Number)
		t.seasons[series.Tmdb][s.Number] = s
	}
	t.series[series.Tmdb] = series
}

func (t *TMDB) PutGroup(series domain.TmdbID, g migrate.TmdbGroup) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.groups[series] = append(t.groups[series], g)
}

func (t *TMDB) PutFind(source, id string, found migrate.TmdbFound) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.finds[findKey{source: source, id: id}] = found
}

func (t *TMDB) PutMovie(m migrate.TmdbCandidate) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.movies[m.Tmdb] = m
}

// SetSearch makes every search return results whose names contain the query.
func (t *TMDB) SetSearch(results ...migrate.TmdbCandidate) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.search = results
}

func (t *TMDB) Series(ctx context.Context, id domain.TmdbID) (migrate.TmdbSeries, error) {
	if err := t.Enter(ctx, TMDBSeries); err != nil {
		return migrate.TmdbSeries{}, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	s, ok := t.series[id]
	if !ok {
		return migrate.TmdbSeries{}, migrate.ErrTmdbNotFound
	}
	return s, nil
}

func (t *TMDB) Season(ctx context.Context, id domain.TmdbID, number int) (migrate.TmdbSeason, error) {
	if err := t.Enter(ctx, TMDBSeason); err != nil {
		return migrate.TmdbSeason{}, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	s, ok := t.seasons[id][number]
	if !ok {
		return migrate.TmdbSeason{}, migrate.ErrTmdbNotFound
	}
	return s, nil
}

func (t *TMDB) EpisodeGroups(ctx context.Context, id domain.TmdbID) ([]migrate.TmdbGroupSummary, error) {
	if err := t.Enter(ctx, TMDBEpisodeGroups); err != nil {
		return nil, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []migrate.TmdbGroupSummary
	for _, g := range t.groups[id] {
		out = append(out, migrate.TmdbGroupSummary{ID: g.ID, Name: g.Name, Type: g.Type})
	}
	return out, nil
}

func (t *TMDB) EpisodeGroup(ctx context.Context, groupID string) (migrate.TmdbGroup, error) {
	if err := t.Enter(ctx, TMDBEpisodeGroup); err != nil {
		return migrate.TmdbGroup{}, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, groups := range t.groups {
		if i := slices.IndexFunc(groups, func(g migrate.TmdbGroup) bool { return g.ID == groupID }); i >= 0 {
			return groups[i], nil
		}
	}
	return migrate.TmdbGroup{}, migrate.ErrTmdbNotFound
}

func (t *TMDB) Find(ctx context.Context, source, externalID string) (migrate.TmdbFound, error) {
	if err := t.Enter(ctx, TMDBFind); err != nil {
		return migrate.TmdbFound{}, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.finds[findKey{source: source, id: externalID}], nil
}

func (t *TMDB) Search(ctx context.Context, query string) ([]migrate.TmdbCandidate, error) {
	if err := t.Enter(ctx, TMDBSearch); err != nil {
		return nil, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []migrate.TmdbCandidate
	for _, c := range t.search {
		if strings.Contains(strings.ToLower(c.Name), strings.ToLower(query)) {
			out = append(out, c)
		}
	}
	return out, nil
}

func (t *TMDB) Movie(ctx context.Context, id domain.TmdbID) (migrate.TmdbCandidate, error) {
	if err := t.Enter(ctx, TMDBMovie); err != nil {
		return migrate.TmdbCandidate{}, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	m, ok := t.movies[id]
	if !ok {
		return migrate.TmdbCandidate{}, migrate.ErrTmdbNotFound
	}
	return m, nil
}

// Disk knows which folders and files exist; every known folder is writable.
type Disk struct {
	mu      sync.Mutex
	folders map[string]bool
	files   map[string]bool
}

var _ migrate.Disk = (*Disk)(nil)

func NewDisk() *Disk {
	return &Disk{folders: map[string]bool{}, files: map[string]bool{}}
}

func (d *Disk) AddFolder(path string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.folders[path] = true
}

func (d *Disk) AddFile(path string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.files[path] = true
}

func (d *Disk) CheckFolder(path string) migrate.PathAccess {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch {
	case !strings.HasPrefix(path, "/"):
		return migrate.PathNotAbsolute
	case d.files[path]:
		return migrate.PathNotFolder
	case d.folders[path]:
		return migrate.PathOK
	}
	return migrate.PathMissing
}

func (d *Disk) Exists(path string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.files[path] || d.folders[path]
}
