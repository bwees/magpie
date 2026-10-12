package migrate

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	acquisitionapi "magpie/internal/acquisition/api"
	activityapi "magpie/internal/activity/api"
	"magpie/internal/domain"
	"magpie/internal/platform/parse"
)

// prowlarrSuffix is what Prowlarr adds to the indexer names it syncs into Sonarr and Radarr.
const prowlarrSuffix = " (Prowlarr)"

// historyKinds is the timeline event each kept history type becomes; other types are dropped.
var historyKinds = map[string]activityapi.ImportKind{
	HistoryGrabbed:                activityapi.ImportGrabbed,
	HistoryDownloadFolderImported: activityapi.ImportImported,
	HistorySeriesFolderImported:   activityapi.ImportImported,
	HistoryMovieFolderImported:    activityapi.ImportImported,
	HistoryDownloadFailed:         activityapi.ImportDownloadFailed,
	HistoryEpisodeFileDeleted:     activityapi.ImportFileDeleted,
	HistoryMovieFileDeleted:       activityapi.ImportFileDeleted,
}

// PathRewrite maps a folder as an app sees it onto the same folder as the server sees it.
type PathRewrite struct {
	App  App    `json:"app"`
	From string `json:"from"`
	To   string `json:"to"`
}

// rewrite applies the longest matching prefix of the app's rewrites.
func rewrite(rewrites []PathRewrite, app App, path string) string {
	best := -1
	for i, r := range rewrites {
		if r.App != app || !hasPathPrefix(path, r.From) {
			continue
		}
		if best < 0 || len(r.From) > len(rewrites[best].From) {
			best = i
		}
	}
	if best < 0 || rewrites[best].To == "" {
		return path
	}
	r := rewrites[best]
	return filepath.Join(r.To, strings.TrimPrefix(path, strings.TrimSuffix(r.From, "/")))
}

func hasPathPrefix(path, prefix string) bool {
	prefix = strings.TrimSuffix(prefix, "/")
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

// planner builds a Plan from what both apps hold. It reads TMDB and the disk but writes nothing.
type planner struct {
	tmdb     TMDB
	disk     Disk
	rewrites []PathRewrite
	progress func(done, total int)
	now      func() time.Time
	shows    map[domain.TmdbID]tmdbShow
	plan     *Plan
	done     int
	total    int
}

func newPlanner(tmdb TMDB, disk Disk, rewrites []PathRewrite, progress func(done, total int), now func() time.Time) *planner {
	return &planner{tmdb: tmdb, disk: disk, rewrites: rewrites, progress: progress, now: now, shows: map[domain.TmdbID]tmdbShow{}}
}

func (p *planner) build(ctx context.Context, libs []ArrLibrary) (*Plan, error) {
	p.plan = &Plan{BuiltAt: p.now().UTC(), Warnings: []string{}}
	for _, lib := range libs {
		p.total += len(lib.Series) + len(lib.Movies)
	}
	for _, lib := range libs {
		p.qualityModel(lib)
	}
	p.clashes()
	for _, lib := range libs {
		if err := p.titles(ctx, lib); err != nil {
			return nil, err
		}
	}
	for _, lib := range libs {
		p.roots(lib)
		p.files(lib)
		p.history(lib)
	}
	return p.plan, nil
}

func (p *planner) step() {
	p.done++
	if p.progress != nil {
		p.progress(p.done, p.total)
	}
}

func (p *planner) warn(format string, args ...any) {
	p.plan.Warnings = append(p.plan.Warnings, fmt.Sprintf(format, args...))
}

// qualityModel translates an app's custom formats, release profiles, quality profiles and delay profiles.
func (p *planner) qualityModel(lib ArrLibrary) {
	t := newTranslator(lib)
	formats := t.formats()
	releaseFormats := t.releaseProfileFormats()
	profiles, languageFormats := profiles(lib, releaseFormats)
	p.plan.Formats = append(p.plan.Formats, formats...)
	p.plan.Formats = append(p.plan.Formats, releaseFormats...)
	p.plan.Formats = append(p.plan.Formats, languageFormats...)
	p.plan.Profiles = append(p.plan.Profiles, profiles...)
	p.plan.Reviews.Specs = append(p.plan.Reviews.Specs, t.reviews...)
	p.plan.Warnings = append(p.plan.Warnings, t.warnings...)

	tags := map[int]string{}
	for _, tag := range lib.Tags {
		tags[tag.ID] = tag.Name
	}
	for _, d := range lib.DelayProfiles {
		if !d.EnableTorrent {
			continue
		}
		delay := acquisitionapi.ImportDelayProfile{
			Source: recordKey(lib.App, recordDelayProfile, d.ID), Delay: time.Duration(d.TorrentDelay) * time.Minute,
			BypassIfHighestQuality: d.BypassIfHighestQuality,
		}
		if d.BypassIfAboveCustomFormatScore {
			delay.BypassIfAboveFormatScore = &d.MinimumCustomFormatScore
		}
		for _, id := range d.Tags {
			delay.Tags = append(delay.Tags, tags[id])
		}
		if len(delay.Tags) > 0 {
			p.warn("%s delay profile for tags %s isn't imported: Magpie has one delay for everything", appName(lib.App), strings.Join(delay.Tags, ", "))
		}
		p.plan.Delays = append(p.plan.Delays, delay)
	}
}

func appName(app App) string {
	if app == AppSonarr {
		return "Sonarr"
	}
	return "Radarr"
}

// clashes lists the formats and profiles both apps have under one name with different contents.
func (p *planner) clashes() {
	byName := func(name string) string { return strings.ToLower(strings.TrimSpace(name)) }
	formatNames := map[domain.SourceKey]string{}
	formats := map[string]map[App]PlannedFormat{}
	for _, f := range p.plan.Formats {
		formatNames[f.Source] = f.Name
		if formats[byName(f.Name)] == nil {
			formats[byName(f.Name)] = map[App]PlannedFormat{}
		}
		formats[byName(f.Name)][f.App] = f
	}
	for key, apps := range formats {
		sonarr, inSonarr := apps[AppSonarr]
		radarr, inRadarr := apps[AppRadarr]
		if !inSonarr || !inRadarr {
			continue
		}
		left, right := formatLines(sonarr), formatLines(radarr)
		if !slices.Equal(left, right) {
			p.plan.Reviews.Clashes = append(p.plan.Reviews.Clashes, Clash{ID: string(ClashFormat) + keySeparator + key, Kind: ClashFormat, Name: sonarr.Name, Sonarr: left, Radarr: right, Choice: ClashPending})
		}
	}
	profiles := map[string]map[App]PlannedProfile{}
	for _, pr := range p.plan.Profiles {
		if profiles[byName(pr.Name)] == nil {
			profiles[byName(pr.Name)] = map[App]PlannedProfile{}
		}
		profiles[byName(pr.Name)][pr.App] = pr
	}
	for key, apps := range profiles {
		sonarr, inSonarr := apps[AppSonarr]
		radarr, inRadarr := apps[AppRadarr]
		if !inSonarr || !inRadarr {
			continue
		}
		left, right := profileLines(sonarr, formatNames), profileLines(radarr, formatNames)
		if !slices.Equal(left, right) {
			p.plan.Reviews.Clashes = append(p.plan.Reviews.Clashes, Clash{ID: string(ClashProfile) + keySeparator + key, Kind: ClashProfile, Name: sonarr.Name, Sonarr: left, Radarr: right, Choice: ClashPending})
		}
	}
	slices.SortFunc(p.plan.Reviews.Clashes, func(a, b Clash) int { return strings.Compare(a.ID, b.ID) })
}

func formatLines(f PlannedFormat) []string {
	lines := specLines(f.Specs)
	for _, pending := range f.Pending {
		lines = append(lines, "UNTRANSLATED "+pending.Spec.Name)
	}
	return lines
}

func (p *planner) titles(ctx context.Context, lib ArrLibrary) error {
	for _, m := range lib.Movies {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := p.movie(ctx, lib, m); err != nil {
			return fmt.Errorf("movie %q: %w", m.Title, err)
		}
		p.step()
	}
	for _, s := range lib.Series {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := p.series(ctx, lib, s); err != nil {
			return fmt.Errorf("series %q: %w", s.Title, err)
		}
		p.step()
	}
	return nil
}

func (p *planner) movie(ctx context.Context, lib ArrLibrary, m ArrMovie) error {
	title := PlannedTitle{
		Source: titleKey(lib.App, m.ID), App: lib.App, ArrID: m.ID, Kind: domain.MediaKindMovie, Name: m.Title, Year: m.Year,
		External: externalIDs(0, m.ImdbID), Profile: recordKey(lib.App, recordQualityProfile, m.QualityProfileID),
		Monitored: m.Monitored, AddedAt: m.Added, Path: rewrite(p.rewrites, lib.App, m.Path), Episodes: []PlannedEpisode{},
		Status: TitleMatched, Tmdb: domain.TmdbID(m.TmdbID), Aliases: []string{},
	}
	if title.Tmdb == 0 {
		found, err := p.find(ctx, FindImdb, m.ImdbID, func(f TmdbFound) []TmdbCandidate { return f.Movies })
		if err != nil {
			return err
		}
		if len(found) == 1 {
			title.Tmdb = found[0].Tmdb
		} else if err := p.review(ctx, &title, found); err != nil {
			return err
		}
	}
	title.External.Tmdb = title.Tmdb
	p.plan.Titles = append(p.plan.Titles, title)
	return nil
}

func externalIDs(tvdb int, imdb string) domain.ExternalIDs {
	ids := domain.ExternalIDs{}
	if tvdb > 0 {
		v := strconv.Itoa(tvdb)
		ids.Tvdb = &v
	}
	if imdb != "" {
		ids.Imdb = &imdb
	}
	return ids
}

// find looks an external ID up on TMDB; an empty ID finds nothing.
func (p *planner) find(ctx context.Context, source, id string, results func(TmdbFound) []TmdbCandidate) ([]TmdbCandidate, error) {
	if id == "" || id == "0" {
		return nil, nil
	}
	found, err := p.tmdb.Find(ctx, source, id)
	if errors.Is(err, ErrTmdbNotFound) {
		return nil, nil
	}
	return results(found), err
}

// review puts a title in the review queue with TMDB's search results by name, or the given candidates.
func (p *planner) review(ctx context.Context, title *PlannedTitle, candidates []TmdbCandidate) error {
	if len(candidates) == 0 {
		found, err := p.tmdb.Search(ctx, title.Name)
		if err != nil && !errors.Is(err, ErrTmdbNotFound) {
			return err
		}
		for _, c := range found {
			if c.Kind == title.Kind {
				candidates = append(candidates, c)
			}
		}
	}
	title.Status = TitleInReview
	p.plan.Reviews.Titles = append(p.plan.Reviews.Titles, TitleReview{
		ID: string(title.Source), App: title.App, Kind: title.Kind, Name: title.Name, Year: title.Year,
		Candidates: candidates, Decision: TitlePending, Tmdb: []domain.TmdbID{},
	})
	return nil
}

func (p *planner) series(ctx context.Context, lib ArrLibrary, s ArrSeries) error {
	series := s
	title := PlannedTitle{
		Source: titleKey(lib.App, s.ID), App: lib.App, ArrID: s.ID, Kind: domain.MediaKindSeries, Name: s.Title, Year: s.Year,
		External: externalIDs(s.TvdbID, s.ImdbID), Profile: recordKey(lib.App, recordQualityProfile, s.QualityProfileID),
		Monitored: s.Monitored, MonitorNewSeasons: s.MonitorNewItems == MonitorNewItemsAll, AddedAt: s.Added,
		Path: rewrite(p.rewrites, lib.App, s.Path), Status: TitleMatched, Series: &series, Aliases: s.AlternateTitles,
	}
	title.Episodes = unmappedEpisodes(s)
	if title.Aliases == nil {
		title.Aliases = []string{}
	}
	show, ok, err := p.matchSeries(ctx, s)
	if err != nil {
		return err
	}
	if !ok {
		if err := p.review(ctx, &title, nil); err != nil {
			return err
		}
		p.plan.Titles = append(p.plan.Titles, title)
		return nil
	}
	title.Tmdb = show.series.Tmdb
	title.External.Tmdb = title.Tmdb
	review, err := planNumbering(ctx, p.tmdb, &title, show)
	if err != nil {
		return err
	}
	if review != nil {
		p.plan.Reviews.Numbering = append(p.plan.Reviews.Numbering, *review)
	}
	if s.SeriesType == SeriesTypeDaily || s.SeriesType == SeriesTypeAnime {
		p.plan.Reviews.SeriesTypes = append(p.plan.Reviews.SeriesTypes, SeriesTypeReview{ID: string(title.Source), Name: s.Title, Type: s.SeriesType, Decision: SeriesTypePending})
	}
	p.plan.Titles = append(p.plan.Titles, title)
	return nil
}

func unmappedEpisodes(s ArrSeries) []PlannedEpisode {
	out := make([]PlannedEpisode, 0, len(s.Episodes))
	for _, e := range s.Episodes {
		out = append(out, PlannedEpisode{Source: recordKey(AppSonarr, recordEpisode, e.ID), ArrSeason: e.SeasonNumber, ArrNumber: e.EpisodeNumber, Monitored: e.Monitored})
	}
	return out
}

// matchSeries finds a series on TMDB: Sonarr's tmdbId when TMDB agrees on the TVDB ID, else TMDB /find by
// TVDB ID, then by IMDb ID.
func (p *planner) matchSeries(ctx context.Context, s ArrSeries) (tmdbShow, bool, error) {
	if s.TmdbID > 0 {
		show, err := p.show(ctx, domain.TmdbID(s.TmdbID))
		switch {
		case errors.Is(err, ErrTmdbNotFound):
		case err != nil:
			return tmdbShow{}, false, err
		case show.series.Tvdb == nil || s.TvdbID == 0 || *show.series.Tvdb == strconv.Itoa(s.TvdbID):
			return show, true, nil
		}
	}
	for _, lookup := range []struct{ source, id string }{{FindTvdb, strconv.Itoa(s.TvdbID)}, {FindImdb, s.ImdbID}} {
		found, err := p.find(ctx, lookup.source, lookup.id, func(f TmdbFound) []TmdbCandidate { return f.Series })
		if err != nil {
			return tmdbShow{}, false, err
		}
		if len(found) == 1 {
			show, err := p.show(ctx, found[0].Tmdb)
			return show, err == nil, err
		}
	}
	return tmdbShow{}, false, nil
}

func (p *planner) show(ctx context.Context, id domain.TmdbID) (tmdbShow, error) {
	if show, ok := p.shows[id]; ok {
		return show, nil
	}
	show, err := fetchShow(ctx, p.tmdb, id)
	if err == nil {
		p.shows[id] = show
	}
	return show, err
}

// planNumbering maps a matched series' episodes. When its seasons don't match TMDB's aired order, the best
// scoring episode group is preselected and the series goes to the numbering review.
func planNumbering(ctx context.Context, tmdb TMDB, title *PlannedTitle, show tmdbShow) (*NumberingReview, error) {
	mapper := episodeMapper{tmdb: tmdb, series: *title.Series, show: show}
	if alignsWithAiredOrder(*title.Series, show) {
		episodes, err := mapper.mapEpisodes(ctx, nil)
		title.Episodes, title.EpisodeGroup = episodes, ""
		return nil, err
	}
	options, groups, err := scoreGroups(ctx, tmdb, *title.Series, show)
	if err != nil {
		return nil, err
	}
	review := &NumberingReview{
		ID: string(title.Source), Name: title.Name, SonarrSeasons: sonarrCounts(*title.Series), TmdbSeasons: show.airedCounts(),
		Groups: options, Decision: NumberingPending,
	}
	var group *TmdbGroup
	if len(options) > 0 && options[0].Score > 0 {
		review.Suggested = options[0].ID
		g := groups[review.Suggested]
		group = &g
	}
	episodes, err := mapper.mapEpisodes(ctx, group)
	if err != nil {
		return nil, err
	}
	title.Episodes = episodes
	title.EpisodeGroup = review.Suggested
	if len(options) == 0 {
		return nil, nil
	}
	return review, nil
}

func (p *planner) roots(lib ArrLibrary) {
	kind := domain.MediaKindMovie
	if lib.App == AppSonarr {
		kind = domain.MediaKindSeries
	}
	for _, r := range lib.RootFolders {
		p.plan.Roots = append(p.plan.Roots, PlannedRoot{
			Source: recordKey(lib.App, recordRootFolder, r.ID), App: lib.App, Kind: kind, From: r.Path, Path: rewrite(p.rewrites, lib.App, r.Path),
		})
	}
}

func (p *planner) files(lib ArrLibrary) {
	for _, m := range lib.Movies {
		if m.File != nil {
			p.plan.Files = append(p.plan.Files, p.file(lib.App, recordMovieFile, titleKey(lib.App, m.ID), *m.File, nil))
		}
	}
	for _, s := range lib.Series {
		for _, f := range s.Files {
			var episodes []domain.SourceKey
			for _, e := range s.Episodes {
				if e.EpisodeFileID == f.ID {
					episodes = append(episodes, recordKey(lib.App, recordEpisode, e.ID))
				}
			}
			p.plan.Files = append(p.plan.Files, p.file(lib.App, recordEpisodeFile, titleKey(lib.App, s.ID), f, episodes))
		}
	}
}

func (p *planner) file(app App, record string, title domain.SourceKey, f ArrFile, episodes []domain.SourceKey) PlannedFile {
	path := rewrite(p.rewrites, app, f.Path)
	quality, ok := magpieQuality(app, f.QualityID, f.Revision)
	if !ok {
		def, _ := parse.QualityByID(parse.QualityUnknown)
		quality = def.Quality(f.Revision)
	}
	planned := PlannedFile{
		Source: recordKey(app, record, f.ID), App: app, Title: title, Episodes: episodes, Path: path, Size: f.Size,
		Quality: quality, ReleaseName: f.SceneName, ReleaseGroup: f.ReleaseGroup, AddedAt: f.DateAdded,
		Languages: []domain.Language{}, Missing: !p.disk.Exists(path),
	}
	if planned.Episodes == nil {
		planned.Episodes = []domain.SourceKey{}
	}
	for _, name := range f.Languages {
		if code, ok := languageByName(name); ok && code != languageOrigin {
			planned.Languages = append(planned.Languages, domain.Language(code))
		}
	}
	return planned
}

// grabKey matches a blocklist row to the grab of the same release for the same title.
type grabKey struct {
	title       domain.SourceKey
	sourceTitle string
}

func (p *planner) history(lib ArrLibrary) {
	known := map[domain.SourceKey]bool{}
	for _, t := range p.plan.Titles {
		known[t.Source] = true
	}
	titleOf := func(seriesID, movieID int) domain.SourceKey {
		if lib.App == AppSonarr {
			return titleKey(lib.App, seriesID)
		}
		return titleKey(lib.App, movieID)
	}
	hashes := map[grabKey]string{}
	for _, h := range lib.History {
		title := titleOf(h.SeriesID, h.MovieID)
		if !known[title] {
			continue
		}
		kind, kept := historyKinds[h.EventType]
		if h.EventType == HistoryGrabbed {
			if hash := grabHash(h); hash != "" {
				hashes[grabKey{title: title, sourceTitle: h.SourceTitle}] = hash
			}
		}
		if !kept || strings.EqualFold(h.Data[HistoryDataProtocol], ProtocolUsenet) {
			continue
		}
		row := PlannedHistory{
			Source: recordKey(lib.App, recordHistory, h.ID), Title: title, Episodes: []domain.SourceKey{}, Kind: kind,
			ReleaseName: h.SourceTitle, At: h.Date,
		}
		if h.EpisodeID > 0 {
			row.Episodes = append(row.Episodes, recordKey(lib.App, recordEpisode, h.EpisodeID))
		}
		if kind == activityapi.ImportDownloadFailed || kind == activityapi.ImportFileDeleted {
			row.Reason = cmp.Or(h.Data[HistoryDataReason], h.Data[HistoryDataMessage])
		}
		p.plan.History = append(p.plan.History, row)
	}
	for _, b := range lib.Blocklist {
		title := titleOf(b.SeriesID, b.MovieID)
		if !known[title] || strings.EqualFold(b.Protocol, ProtocolUsenet) {
			continue
		}
		row := PlannedBlock{
			Source: recordKey(lib.App, recordBlocklist, b.ID), Title: title, Episodes: []domain.SourceKey{},
			ReleaseName: b.SourceTitle, Indexer: strings.TrimSuffix(b.Indexer, prowlarrSuffix), At: b.Date,
			InfoHash: hashes[grabKey{title: title, sourceTitle: b.SourceTitle}],
		}
		for _, id := range b.EpisodeIDs {
			row.Episodes = append(row.Episodes, recordKey(lib.App, recordEpisode, id))
		}
		p.plan.Blocklist = append(p.plan.Blocklist, row)
	}
}

// grabHash is a grab's torrent hash: torrentInfoHash, else the download ID qBittorrent gave it.
func grabHash(h ArrHistoryItem) string {
	if strings.EqualFold(h.Data[HistoryDataProtocol], ProtocolUsenet) {
		return ""
	}
	return strings.ToLower(cmp.Or(h.Data[HistoryDataTorrentInfoHash], h.DownloadID))
}
