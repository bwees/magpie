package catalog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"time"

	"magpie/internal/catalog/api"
	"magpie/internal/domain"
	requestsapi "magpie/internal/requests/api"

	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

var (
	ErrNotASeries          = errors.New("catalog: title is not a series")
	ErrUnknownEpisodeGroup = errors.New("catalog: TMDB lists no such episode group for this series")
	ErrSeasonNotFound      = fmt.Errorf("%w: no such season", api.ErrNotFound)
	ErrEpisodeNotFound     = fmt.Errorf("%w: no such episode", api.ErrNotFound)
)

// seasonPolicy says whether a season Catalog has no row for yet starts monitored.
type seasonPolicy func(number int) bool

func regularSeason(number int) bool { return number != specialsSeason }

func noSeason(int) bool { return false }

// requestedSeasons is what a request monitors: every season but specials, or the chosen ones.
func requestedSeasons(req requestsapi.RequestApproved) seasonPolicy {
	if req.Scope == requestsapi.ScopeAllSeasons {
		return regularSeason
	}
	return func(number int) bool { return slices.Contains(req.Seasons, number) }
}

// futureSeasons is how seasons that appear on a refresh start.
func futureSeasons(row titleRow) seasonPolicy {
	if row.MonitorFutureSeasons {
		return regularSeason
	}
	return noSeason
}

// seriesSnapshot is what TMDB says about a series, fetched before the write transaction.
type seriesSnapshot struct {
	series  TmdbSeries
	seasons []TmdbSeason
	// groupID is the episode group in use; "" when none is selected or TMDB no longer has it.
	groupID string
	group   *TmdbEpisodeGroup
}

func (s *Service) fetchSeries(ctx context.Context, id domain.TmdbID, groupID, lang string) (seriesSnapshot, error) {
	series, err := s.tmdb.Series(ctx, id, lang)
	if err != nil {
		return seriesSnapshot{}, err
	}
	snap := seriesSnapshot{series: series}
	for _, summary := range series.Seasons {
		season, err := s.tmdb.Season(ctx, id, summary.Number, lang)
		if err != nil {
			return seriesSnapshot{}, fmt.Errorf("season %d: %w", summary.Number, err)
		}
		snap.seasons = append(snap.seasons, season)
	}
	if groupID == "" {
		return snap, nil
	}
	group, err := s.tmdb.EpisodeGroup(ctx, groupID)
	if errors.Is(err, ErrTmdbNotFound) {
		slog.WarnContext(ctx, "TMDB dropped the selected episode group; using aired order", "series", id, "group", groupID)
		return snap, nil
	}
	if err != nil {
		return seriesSnapshot{}, err
	}
	snap.groupID, snap.group = groupID, &group
	return snap, nil
}

// seriesUpdate says how applySeries treats episodes and seasons it hasn't seen before.
type seriesUpdate struct {
	newSeason seasonPolicy
	// releaseAired marks new episodes that have already aired as released; otherwise the release clock does.
	releaseAired bool
}

// seriesChanges is what applySeries changed.
type seriesChanges struct {
	// episodes are all the series' episodes in the current numbering.
	episodes []episodeRow
	added    []episodeRow
	moves    []api.EpisodeMove
}

func (c seriesChanges) monitored() []domain.EpisodeID {
	ids := []domain.EpisodeID{}
	for _, e := range c.episodes {
		if e.Monitored {
			ids = append(ids, domain.EpisodeID(e.ID))
		}
	}
	return ids
}

func (c seriesChanges) anyReleased() bool {
	return slices.ContainsFunc(c.episodes, func(e episodeRow) bool { return e.Released })
}

func (c seriesChanges) announced() []domain.EpisodeRef {
	refs := make([]domain.EpisodeRef, 0, len(c.added))
	for _, e := range c.added {
		refs = append(refs, domain.EpisodeRef{Episode: domain.EpisodeID(e.ID), Season: e.SeasonNumber, Number: e.EpisodeNumber})
	}
	return refs
}

// applySeries merges TMDB's episodes into the stored ones and renumbers them for the snapshot's group. New
// episodes take their season's monitoring. When numbering moved, seasons are rebuilt and a season is
// monitored when any of its episodes is.
func (s *Service) applySeries(tx *gorm.DB, row titleRow, snap seriesSnapshot, update seriesUpdate) (seriesChanges, error) {
	id := domain.TitleID(row.ID)
	episodes, err := s.repo.titleEpisodes(tx, id)
	if err != nil {
		return seriesChanges{}, err
	}
	before := map[string]episodeNumber{}
	index := map[int64]int{}
	for i, e := range episodes {
		before[e.ID] = e.number()
		index[e.TmdbEpisodeID] = i
	}
	now := s.now()
	for _, season := range snap.seasons {
		for _, tmdb := range season.Episodes {
			if i, ok := index[int64(tmdb.ID)]; ok {
				applyEpisode(&episodes[i], tmdb, now)
				continue
			}
			fresh := episodeRow{ID: string(domain.NewID[domain.EpisodeID]()), TitleID: row.ID, TmdbEpisodeID: int64(tmdb.ID)}
			applyEpisode(&fresh, tmdb, now)
			index[fresh.TmdbEpisodeID] = len(episodes)
			episodes = append(episodes, fresh)
		}
	}
	renumber(episodes, snap.group)
	slices.SortFunc(episodes, byCurrentOrder)

	seasons, err := s.repo.seasonRows(tx, id)
	if err != nil {
		return seriesChanges{}, err
	}
	stored := map[int]seasonRow{}
	for _, season := range seasons {
		stored[season.Number] = season
	}
	seasonMonitored := func(number int) bool {
		if season, ok := stored[number]; ok {
			return season.Monitored
		}
		return update.newSeason(number)
	}

	changes := seriesChanges{moves: []api.EpisodeMove{}}
	for i := range episodes {
		e := &episodes[i]
		from, existed := before[e.ID]
		switch {
		case !existed:
			e.Monitored = seasonMonitored(e.SeasonNumber)
			if update.releaseAired && airedBy(e.AirDate, now) {
				e.Released, e.ReleasedAt = true, ptr(now.UTC())
			}
			changes.added = append(changes.added, *e)
		case from != e.number():
			changes.moves = append(changes.moves, api.EpisodeMove{
				Episode:    domain.EpisodeID(e.ID),
				FromSeason: from.Season, FromNo: from.Number,
				ToSeason: e.SeasonNumber, ToNo: e.EpisodeNumber,
			})
		}
	}
	if err := s.repo.saveEpisodes(tx, episodes); err != nil {
		return seriesChanges{}, err
	}
	changes.episodes = episodes

	rebuilt := len(changes.moves) > 0
	monitoredIn := map[int]bool{}
	for _, e := range episodes {
		monitoredIn[e.SeasonNumber] = monitoredIn[e.SeasonNumber] || e.Monitored
	}
	numbers := slices.Collect(maps.Keys(monitoredIn))
	if snap.group == nil {
		for _, summary := range snap.series.Seasons {
			numbers = append(numbers, summary.Number)
		}
	}
	if !rebuilt {
		numbers = slices.AppendSeq(numbers, maps.Keys(stored))
	}
	slices.Sort(numbers)
	rows := make([]seasonRow, 0, len(numbers))
	for _, number := range slices.Compact(numbers) {
		season, ok := stored[number]
		if !ok {
			season = seasonRow{ID: newSeasonID(), TitleID: row.ID, Number: number, Monitored: seasonMonitored(number)}
		}
		if anyMonitored, hasEpisodes := monitoredIn[number]; rebuilt && hasEpisodes {
			season.Monitored = anyMonitored
		}
		rows = append(rows, season)
	}
	return changes, s.repo.replaceSeasons(tx, id, rows)
}

func newSeasonID() string {
	return ulid.Make().String()
}

// applyEpisode copies TMDB's metadata. A released episode whose air date moves into the future goes back
// to unreleased, so the release clock releases it again on the new date.
func applyEpisode(row *episodeRow, e TmdbEpisode, now time.Time) {
	row.AiredSeason, row.AiredNumber = e.Season, e.Number
	row.Name = e.Name
	row.AirDate = utc(e.AirDate)
	row.RuntimeMinutes = e.RuntimeMinutes
	if row.Released && row.AirDate != nil && !airedBy(row.AirDate, now) {
		row.Released, row.ReleasedAt = false, nil
	}
}

// applySeriesMeta copies TMDB's series metadata onto a row.
func applySeriesMeta(row *titleRow, series TmdbSeries, now time.Time) error {
	aliases, err := encodeAliases(series.AlternativeTitles)
	if err != nil {
		return err
	}
	row.Aliases = aliases
	row.Name = series.Name
	row.OriginalName = series.OriginalName
	row.Overview = series.Overview
	row.OriginalLanguage = string(series.OriginalLanguage)
	row.RuntimeMinutes = series.EpisodeRuntimeMinutes
	if series.FirstAirDate != nil {
		row.Year = series.FirstAirDate.Year()
	}
	row.SeriesStatus = api.SeriesStatusContinuing
	if series.Ended {
		row.SeriesStatus = api.SeriesStatusEnded
	}
	if series.External.Imdb != nil || series.External.Tvdb != nil {
		row.ImdbID, row.TvdbID, row.ExternalIDSource = series.External.Imdb, series.External.Tvdb, ExternalIDSourceTMDB
	}
	row.MetadataRefreshedAt = now
	return nil
}

// addRequestedSeries fetches a requested series from TMDB, stores it with the request's profile and
// monitoring, and publishes TitleAdded.
func (s *Service) addRequestedSeries(ctx context.Context, req requestsapi.RequestApproved) error {
	meta, err := metadataSetting.Get(ctx, s.settings)
	if err != nil {
		return err
	}
	snap, err := s.fetchSeries(ctx, req.Tmdb, "", meta.Language)
	if err != nil {
		return err
	}

	var added *domain.TitleID
	err = s.db.Write(ctx, func(tx *gorm.DB) error {
		existing, err := s.repo.titleByTmdb(tx, domain.MediaKindSeries, req.Tmdb)
		if err == nil {
			return s.monitorForRequest(tx, existing, req)
		}
		if !errors.Is(err, api.ErrNotFound) {
			return err
		}

		row := s.newSeriesRow(req.Tmdb)
		row.ProfileID = string(req.Profile)
		row.Monitored = true
		row.MonitorFutureSeasons = req.Scope == requestsapi.ScopeAllSeasons
		row.Origin = string(domain.OriginRequest)
		row.RequestedBy = ptr(string(req.User))
		row.RequestID = ptr(string(req.Request))
		row, changes, err := s.createSeries(tx, row, snap, requestedSeasons(req))
		if err != nil {
			return err
		}
		added = ptr(domain.TitleID(row.ID))
		return s.publish(tx, api.TitleAdded{
			Title:       domain.TitleID(row.ID),
			Kind:        domain.MediaKindSeries,
			External:    domain.ExternalIDs{Tmdb: req.Tmdb, Tvdb: row.TvdbID, Imdb: row.ImdbID},
			Profile:     req.Profile,
			Monitored:   changes.monitored(),
			Released:    row.Released,
			RequestedBy: ptr(req.User),
			RequestID:   ptr(req.Request),
			Origin:      domain.OriginRequest,
		})
	})
	if err != nil || added == nil {
		return err
	}
	if err := s.cacheImages(ctx, *added, snap.series.Images); err != nil {
		slog.WarnContext(ctx, "caching title images failed; the refresh job retries", "title", *added, "error", err)
	}
	return nil
}

// newSeriesRow is a series row not yet stored; createSeries fills in TMDB's metadata.
func (s *Service) newSeriesRow(tmdb domain.TmdbID) titleRow {
	return titleRow{
		ID:               string(domain.NewID[domain.TitleID]()),
		Kind:             string(domain.MediaKindSeries),
		TmdbID:           int64(tmdb),
		ExternalIDSource: ExternalIDSourceTMDB,
		AddedAt:          s.now().UTC(),
	}
}

// createSeries stores a new series with TMDB's metadata and episodes. Seasons the policy picks start
// monitored, and episodes that have aired are released at once.
func (s *Service) createSeries(tx *gorm.DB, row titleRow, snap seriesSnapshot, policy seasonPolicy) (titleRow, seriesChanges, error) {
	now := s.now().UTC()
	if err := applySeriesMeta(&row, snap.series, now); err != nil {
		return titleRow{}, seriesChanges{}, err
	}
	if err := s.repo.createTitle(tx, row); err != nil {
		return titleRow{}, seriesChanges{}, err
	}
	changes, err := s.applySeries(tx, row, snap, seriesUpdate{newSeason: policy, releaseAired: true})
	if err != nil {
		return titleRow{}, seriesChanges{}, err
	}
	if changes.anyReleased() {
		row.Released, row.ReleasedAt = true, &now
		if err := s.repo.saveTitle(tx, row); err != nil {
			return titleRow{}, seriesChanges{}, err
		}
	}
	return row, changes, nil
}

// monitorSeriesForRequest monitors the requested seasons of a tracked series and their episodes.
func (s *Service) monitorSeriesForRequest(tx *gorm.DB, row titleRow, req requestsapi.RequestApproved) error {
	id := domain.TitleID(row.ID)
	wanted := requestedSeasons(req)
	seasons, err := s.repo.seasonRows(tx, id)
	if err != nil {
		return err
	}
	for _, season := range seasons {
		if wanted(season.Number) && !season.Monitored {
			season.Monitored = true
			if err := s.repo.saveSeason(tx, season); err != nil {
				return err
			}
		}
	}
	episodes, err := s.repo.titleEpisodes(tx, id)
	if err != nil {
		return err
	}
	added := []domain.EpisodeID{}
	for i := range episodes {
		e := &episodes[i]
		newlyMonitored := wanted(e.SeasonNumber) && !e.Monitored
		e.Monitored = e.Monitored || newlyMonitored
		if newlyMonitored || (e.Monitored && !row.Monitored) {
			added = append(added, domain.EpisodeID(e.ID))
		}
	}
	if err := s.repo.saveEpisodes(tx, episodes); err != nil {
		return err
	}
	row.Monitored = true
	row.MonitorFutureSeasons = row.MonitorFutureSeasons || req.Scope == requestsapi.ScopeAllSeasons
	if err := s.repo.saveTitle(tx, row); err != nil {
		return err
	}
	return s.publish(tx, api.MonitoringChanged{
		Title:     id,
		Added:     added,
		Removed:   []domain.EpisodeID{},
		RequestID: ptr(req.Request),
	})
}

// refreshSeries re-reads a series, its seasons and its episode group from TMDB. New episodes publish
// EpisodesAnnounced; episodes whose numbering moved publish EpisodeGroupChanged.
func (s *Service) refreshSeries(ctx context.Context, row titleRow) error {
	meta, err := metadataSetting.Get(ctx, s.settings)
	if err != nil {
		return err
	}
	snap, err := s.fetchSeries(ctx, domain.TmdbID(row.TmdbID), row.EpisodeGroupID, meta.Language)
	if err != nil {
		return err
	}
	id := domain.TitleID(row.ID)
	err = s.db.Write(ctx, func(tx *gorm.DB) error {
		current, err := s.repo.title(tx, id)
		if err != nil {
			return err
		}
		if err := applySeriesMeta(&current, snap.series, s.now().UTC()); err != nil {
			return err
		}
		groupDropped := current.EpisodeGroupID != snap.groupID
		current.EpisodeGroupID = snap.groupID
		if err := s.repo.saveTitle(tx, current); err != nil {
			return err
		}
		changes, err := s.applySeries(tx, current, snap, seriesUpdate{newSeason: futureSeasons(current)})
		if err != nil {
			return err
		}
		if len(changes.moves) > 0 || groupDropped {
			if err := s.publish(tx, api.EpisodeGroupChanged{Title: id, GroupID: snap.groupID, Moves: changes.moves}); err != nil {
				return err
			}
		}
		if len(changes.added) == 0 {
			return nil
		}
		return s.publish(tx, api.EpisodesAnnounced{Title: id, Episodes: changes.announced()})
	})
	if err != nil {
		return err
	}
	return s.cacheImages(ctx, id, snap.series.Images)
}

// EpisodeGroups lists the episode groups TMDB has for a series.
func (s *Service) EpisodeGroups(ctx context.Context, id domain.TitleID) ([]TmdbEpisodeGroupSummary, error) {
	row, err := s.seriesRow(s.db.Read(ctx), id)
	if err != nil {
		return nil, err
	}
	return s.tmdb.EpisodeGroups(ctx, domain.TmdbID(row.TmdbID))
}

// SelectEpisodeGroup renumbers a series by a TMDB episode group, or by aired order for "", and publishes
// EpisodeGroupChanged with every episode that moved. Monitoring stays with each episode.
func (s *Service) SelectEpisodeGroup(ctx context.Context, id domain.TitleID, groupID string) (TitleView, error) {
	row, err := s.seriesRow(s.db.Read(ctx), id)
	if err != nil {
		return TitleView{}, err
	}
	snap := seriesSnapshot{groupID: groupID}
	if groupID != "" {
		group, err := s.listedGroup(ctx, domain.TmdbID(row.TmdbID), groupID)
		if err != nil {
			return TitleView{}, err
		}
		snap.group = &group
	}
	var view TitleView
	err = s.db.Write(ctx, func(tx *gorm.DB) error {
		current, err := s.repo.title(tx, id)
		if err != nil {
			return err
		}
		if current.EpisodeGroupID != groupID {
			current.EpisodeGroupID = groupID
			if err := s.repo.saveTitle(tx, current); err != nil {
				return err
			}
			changes, err := s.applySeries(tx, current, snap, seriesUpdate{newSeason: futureSeasons(current)})
			if err != nil {
				return err
			}
			if err := s.publish(tx, api.EpisodeGroupChanged{Title: id, GroupID: groupID, Moves: changes.moves}); err != nil {
				return err
			}
		}
		view, err = s.view(tx, id)
		return err
	})
	return view, err
}

func (s *Service) listedGroup(ctx context.Context, series domain.TmdbID, groupID string) (TmdbEpisodeGroup, error) {
	listed, err := s.tmdb.EpisodeGroups(ctx, series)
	if err != nil {
		return TmdbEpisodeGroup{}, err
	}
	if !slices.ContainsFunc(listed, func(g TmdbEpisodeGroupSummary) bool { return g.ID == groupID }) {
		return TmdbEpisodeGroup{}, fmt.Errorf("%w: %s", ErrUnknownEpisodeGroup, groupID)
	}
	return s.tmdb.EpisodeGroup(ctx, groupID)
}

func (s *Service) seriesRow(conn *gorm.DB, id domain.TitleID) (titleRow, error) {
	row, err := s.repo.title(conn, id)
	if err != nil {
		return titleRow{}, err
	}
	if domain.MediaKind(row.Kind) != domain.MediaKindSeries {
		return titleRow{}, fmt.Errorf("%w: %s", ErrNotASeries, id)
	}
	return row, nil
}

// SetSeasonMonitored sets a season's flag and every episode in it, publishing the episodes that changed.
func (s *Service) SetSeasonMonitored(ctx context.Context, id domain.TitleID, number int, monitored bool) (TitleView, error) {
	return s.changeMonitoring(ctx, id, func(tx *gorm.DB) (func(episodeRow) bool, error) {
		season, err := s.repo.season(tx, id, number)
		if errors.Is(err, api.ErrNotFound) {
			return nil, fmt.Errorf("%w: season %d of %s", ErrSeasonNotFound, number, id)
		}
		if err != nil {
			return nil, err
		}
		season.Monitored = monitored
		if err := s.repo.saveSeason(tx, season); err != nil {
			return nil, err
		}
		return func(e episodeRow) bool { return e.SeasonNumber == number }, nil
	}, monitored)
}

// SetEpisodeMonitored sets one episode's flag. Its season's flag stays as it is.
func (s *Service) SetEpisodeMonitored(ctx context.Context, id domain.TitleID, episode domain.EpisodeID, monitored bool) (TitleView, error) {
	return s.changeMonitoring(ctx, id, func(tx *gorm.DB) (func(episodeRow) bool, error) {
		if _, err := s.repo.episode(tx, id, episode); errors.Is(err, api.ErrNotFound) {
			return nil, fmt.Errorf("%w: episode %s of %s", ErrEpisodeNotFound, episode, id)
		} else if err != nil {
			return nil, err
		}
		return func(e episodeRow) bool { return e.ID == string(episode) }, nil
	}, monitored)
}

// changeMonitoring sets monitored on the episodes pick selects and publishes MonitoringChanged for the
// ones whose flag changed.
func (s *Service) changeMonitoring(ctx context.Context, id domain.TitleID, pick func(tx *gorm.DB) (func(episodeRow) bool, error), monitored bool) (TitleView, error) {
	var view TitleView
	err := s.db.Write(ctx, func(tx *gorm.DB) error {
		if _, err := s.seriesRow(tx, id); err != nil {
			return err
		}
		selected, err := pick(tx)
		if err != nil {
			return err
		}
		episodes, err := s.repo.titleEpisodes(tx, id)
		if err != nil {
			return err
		}
		var changed []episodeRow
		changedIDs := []domain.EpisodeID{}
		for _, e := range episodes {
			if selected(e) && e.Monitored != monitored {
				e.Monitored = monitored
				changed = append(changed, e)
				changedIDs = append(changedIDs, domain.EpisodeID(e.ID))
			}
		}
		if err := s.repo.saveEpisodes(tx, changed); err != nil {
			return err
		}
		if len(changed) > 0 {
			event := api.MonitoringChanged{Title: id, Added: []domain.EpisodeID{}, Removed: []domain.EpisodeID{}}
			if monitored {
				event.Added = changedIDs
			} else {
				event.Removed = changedIDs
			}
			if err := s.publish(tx, event); err != nil {
				return err
			}
		}
		view, err = s.view(tx, id)
		return err
	})
	return view, err
}

// seriesMonitoringChanged is the event for turning a whole series on or off: its monitored episodes start
// or stop being wanted.
func (s *Service) seriesMonitoringChanged(tx *gorm.DB, row titleRow) (api.MonitoringChanged, error) {
	episodes, err := s.repo.titleEpisodes(tx, domain.TitleID(row.ID))
	if err != nil {
		return api.MonitoringChanged{}, err
	}
	ids := seriesChanges{episodes: episodes}.monitored()
	event := api.MonitoringChanged{Title: domain.TitleID(row.ID), Added: []domain.EpisodeID{}, Removed: []domain.EpisodeID{}}
	if row.Monitored {
		event.Added = ids
	} else {
		event.Removed = ids
	}
	return event, nil
}

// releaseEpisodes marks episodes that have aired as released and publishes one WantedReleased per
// monitored series for its monitored ones. A series is released once any episode is.
func (s *Service) releaseEpisodes(tx *gorm.DB, now time.Time) error {
	rows, err := s.repo.airedUnreleased(tx)
	if err != nil {
		return err
	}
	var titles []domain.TitleID
	released := map[domain.TitleID][]domain.EpisodeID{}
	for _, row := range rows {
		if !airedBy(row.AirDate, now) {
			continue
		}
		row.Released, row.ReleasedAt = true, ptr(now.UTC())
		if err := s.repo.saveEpisodes(tx, []episodeRow{row}); err != nil {
			return err
		}
		title := domain.TitleID(row.TitleID)
		if _, seen := released[title]; !seen {
			titles = append(titles, title)
			released[title] = []domain.EpisodeID{}
		}
		if row.Monitored {
			released[title] = append(released[title], domain.EpisodeID(row.ID))
		}
	}
	for _, id := range titles {
		row, err := s.repo.title(tx, id)
		if err != nil {
			return err
		}
		if !row.Released {
			row.Released, row.ReleasedAt = true, ptr(now.UTC())
			if err := s.repo.saveTitle(tx, row); err != nil {
				return err
			}
		}
		if !row.Monitored || len(released[id]) == 0 {
			continue
		}
		if err := s.publish(tx, api.WantedReleased{Title: id, Episodes: released[id]}); err != nil {
			return err
		}
	}
	return nil
}
