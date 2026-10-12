package migrate

import (
	"cmp"
	"context"
	"errors"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"magpie/internal/domain"
)

// specials is season 0: TMDB's specials, and episodes an episode group doesn't place.
const specials = 0

// Sonarr v5 seasonType values that mean TVDB's aired order.
var officialSeasonTypes = []string{"", "official", "aired", "default"}

// seasonTypeGroups prefers a TMDB group type for a Sonarr v5 seasonType.
var seasonTypeGroups = map[string]int{"dvd": GroupDVD, "absolute": GroupAbsolute, "alternate": GroupDigital}

// groupPreference breaks ties between equally scored groups.
var groupPreference = []int{GroupDVD, GroupDigital, GroupTV, GroupOriginalAirDate, GroupAbsolute}

type numbering struct {
	Season, Number int
}

// tmdbShow is what TMDB says about a series, enough to map Sonarr's episodes onto it.
type tmdbShow struct {
	series   TmdbSeries
	episodes []TmdbEpisode
}

func fetchShow(ctx context.Context, tmdb TMDB, id domain.TmdbID) (tmdbShow, error) {
	series, err := tmdb.Series(ctx, id)
	if err != nil {
		return tmdbShow{}, err
	}
	show := tmdbShow{series: series}
	for _, number := range series.Seasons {
		season, err := tmdb.Season(ctx, id, number)
		if errors.Is(err, ErrTmdbNotFound) {
			continue
		}
		if err != nil {
			return tmdbShow{}, err
		}
		show.episodes = append(show.episodes, season.Episodes...)
	}
	return show, nil
}

// seasonCounts counts episodes per regular season.
func seasonCounts[T any](items []T, season func(T) int) map[int]int {
	counts := map[int]int{}
	for _, item := range items {
		if s := season(item); s != specials {
			counts[s]++
		}
	}
	return counts
}

func sonarrCounts(s ArrSeries) map[int]int {
	return seasonCounts(s.Episodes, func(e ArrEpisode) int { return e.SeasonNumber })
}

func (show tmdbShow) airedCounts() map[int]int {
	return seasonCounts(show.episodes, func(e TmdbEpisode) int { return e.Season })
}

func groupCounts(g TmdbGroup) map[int]int {
	counts := map[int]int{}
	for _, season := range g.Seasons {
		if season.Order != specials {
			counts[season.Order] = len(season.Episodes)
		}
	}
	return counts
}

// structureScore is the share of seasons whose episode counts agree; 1 when every season does.
func structureScore(sonarr, other map[int]int) float64 {
	seasons := max(len(sonarr), len(other))
	if seasons == 0 {
		return 0
	}
	matching := 0
	for season, count := range sonarr {
		if other[season] == count {
			matching++
		}
	}
	return float64(matching) / float64(seasons)
}

// alignsWithAiredOrder reports whether Sonarr's numbering can map to TMDB's aired order directly.
func alignsWithAiredOrder(s ArrSeries, show tmdbShow) bool {
	return !s.UseSceneNumbering && slices.Contains(officialSeasonTypes, strings.ToLower(s.SeasonType)) &&
		maps.Equal(sonarrCounts(s), show.airedCounts())
}

// scoreGroups fetches every episode group TMDB lists and scores each against Sonarr's seasons, best first.
func scoreGroups(ctx context.Context, tmdb TMDB, s ArrSeries, show tmdbShow) ([]GroupOption, map[string]TmdbGroup, error) {
	summaries, err := tmdb.EpisodeGroups(ctx, show.series.Tmdb)
	if err != nil && !errors.Is(err, ErrTmdbNotFound) {
		return nil, nil, err
	}
	counts := sonarrCounts(s)
	groups := map[string]TmdbGroup{}
	options := make([]GroupOption, 0, len(summaries))
	for _, summary := range summaries {
		group, err := tmdb.EpisodeGroup(ctx, summary.ID)
		if errors.Is(err, ErrTmdbNotFound) {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		groups[group.ID] = group
		options = append(options, GroupOption{ID: group.ID, Name: cmp.Or(group.Name, summary.Name), Type: summary.Type, Score: structureScore(counts, groupCounts(group))})
	}
	preferred := seasonTypeGroups[strings.ToLower(s.SeasonType)]
	rank := func(groupType int) int {
		if groupType == preferred {
			return -1
		}
		if i := slices.Index(groupPreference, groupType); i >= 0 {
			return i
		}
		return len(groupPreference)
	}
	slices.SortStableFunc(options, func(a, b GroupOption) int {
		return cmp.Or(cmp.Compare(b.Score, a.Score), cmp.Compare(rank(a.Type), rank(b.Type)))
	})
	return options, groups, nil
}

// placement numbers TMDB episodes the way Catalog does: aired order without a group; else the group's
// season order and entry order + 1, with episodes it leaves out appended to season 0 in aired order.
func placement(show tmdbShow, group *TmdbGroup) map[int64]numbering {
	placed := map[int64]numbering{}
	if group == nil {
		for _, e := range show.episodes {
			placed[e.ID] = numbering{Season: e.Season, Number: e.Number}
		}
		return placed
	}
	lastSpecial := 0
	for _, season := range group.Seasons {
		for _, entry := range season.Episodes {
			if _, seen := placed[entry.Episode]; seen {
				continue
			}
			placed[entry.Episode] = numbering{Season: season.Order, Number: entry.Order + 1}
			if season.Order == specials {
				lastSpecial = max(lastSpecial, entry.Order+1)
			}
		}
	}
	unplaced := slices.DeleteFunc(slices.Clone(show.episodes), func(e TmdbEpisode) bool { _, ok := placed[e.ID]; return ok })
	slices.SortFunc(unplaced, func(a, b TmdbEpisode) int {
		return cmp.Or(cmp.Compare(a.Season, b.Season), cmp.Compare(a.Number, b.Number))
	})
	for i, e := range unplaced {
		placed[e.ID] = numbering{Season: specials, Number: lastSpecial + i + 1}
	}
	return placed
}

// episodeMapper maps one Sonarr series' episodes onto Magpie's numbering.
type episodeMapper struct {
	tmdb   TMDB
	series ArrSeries
	show   tmdbShow
	// split maps only through the crosswalk: the series is one part of a Sonarr series.
	split bool
}

// mapEpisodes places every Sonarr episode. Regular episodes map S/E directly when Sonarr's seasons match
// the target numbering; otherwise, and always for specials, each goes through the crosswalk: TVDB episode
// ID, then air date, then title. An episode none of those finds stays unmapped.
func (m episodeMapper) mapEpisodes(ctx context.Context, group *TmdbGroup) ([]PlannedEpisode, error) {
	placed := placement(m.show, group)
	byNumbering := map[numbering]bool{}
	for _, n := range placed {
		byNumbering[n] = true
	}
	direct := alignsWithAiredOrder(m.series, m.show)
	if group != nil {
		direct = maps.Equal(sonarrCounts(m.series), groupCounts(*group))
	}
	direct = direct && !m.split
	out := make([]PlannedEpisode, 0, len(m.series.Episodes))
	for _, e := range m.series.Episodes {
		planned := PlannedEpisode{
			Source: recordKey(AppSonarr, recordEpisode, e.ID), ArrSeason: e.SeasonNumber, ArrNumber: e.EpisodeNumber, Monitored: e.Monitored,
		}
		target := numbering{Season: e.SeasonNumber, Number: e.EpisodeNumber}
		mapped := direct && e.SeasonNumber != specials && byNumbering[target]
		if !mapped {
			id, found, err := m.crosswalk(ctx, e)
			if err != nil {
				return nil, err
			}
			target, mapped = placed[id], found
		}
		if mapped {
			planned.Season, planned.Number, planned.Mapped = target.Season, target.Number, true
		}
		out = append(out, planned)
	}
	return out, nil
}

// crosswalk finds the TMDB episode a Sonarr episode is.
func (m episodeMapper) crosswalk(ctx context.Context, e ArrEpisode) (int64, bool, error) {
	if e.TvdbID > 0 {
		found, err := m.tmdb.Find(ctx, FindTvdb, strconv.Itoa(e.TvdbID))
		if err != nil && !errors.Is(err, ErrTmdbNotFound) {
			return 0, false, err
		}
		for _, f := range found.Episodes {
			if f.Series != m.show.series.Tmdb {
				continue
			}
			if id, ok := m.unique(func(t TmdbEpisode) bool { return t.ID == f.ID || (t.Season == f.Season && t.Number == f.Number) }); ok {
				return id, true, nil
			}
		}
	}
	if e.AirDate != "" {
		if id, ok := m.unique(func(t TmdbEpisode) bool { return t.AirDate == e.AirDate }); ok {
			return id, true, nil
		}
	}
	if title := normalisedTitle(e.Title); title != "" {
		if id, ok := m.unique(func(t TmdbEpisode) bool { return normalisedTitle(t.Name) == title }); ok {
			return id, true, nil
		}
	}
	return 0, false, nil
}

func (m episodeMapper) unique(match func(TmdbEpisode) bool) (int64, bool) {
	var found []int64
	for _, t := range m.show.episodes {
		if match(t) {
			found = append(found, t.ID)
		}
	}
	if len(found) != 1 {
		return 0, false
	}
	return found[0], true
}

func normalisedTitle(title string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, title)
}
