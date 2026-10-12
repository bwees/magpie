package library

import (
	"cmp"
	"slices"

	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	"magpie/internal/platform/parse"
)

// episodeIndex finds a series' episodes by ID, by number or by air date.
type episodeIndex struct {
	episodes []catalogapi.Episode
}

func (ix episodeIndex) byID(id domain.EpisodeID) (catalogapi.Episode, bool) {
	i := slices.IndexFunc(ix.episodes, func(e catalogapi.Episode) bool { return e.ID == id })
	if i < 0 {
		return catalogapi.Episode{}, false
	}
	return ix.episodes[i], true
}

// byNumbers matches numbers in the current numbering first, then in TMDB's aired order, which most
// release names follow.
func (ix episodeIndex) byNumbers(season int, numbers []int) ([]catalogapi.Episode, bool) {
	current := func(e catalogapi.Episode, n int) bool { return e.Season == season && e.Number == n }
	aired := func(e catalogapi.Episode, n int) bool { return e.AiredSeason == season && e.AiredNumber == n }
	for _, matches := range []func(catalogapi.Episode, int) bool{current, aired} {
		found := make([]catalogapi.Episode, 0, len(numbers))
		for _, n := range numbers {
			i := slices.IndexFunc(ix.episodes, func(e catalogapi.Episode) bool { return matches(e, n) })
			if i < 0 {
				break
			}
			found = append(found, ix.episodes[i])
		}
		if len(numbers) > 0 && len(found) == len(numbers) {
			return found, true
		}
	}
	return nil, false
}

func (ix episodeIndex) byAirDate(date string) ([]catalogapi.Episode, bool) {
	found := []catalogapi.Episode{}
	for _, e := range ix.episodes {
		if e.AirDate != nil && e.AirDate.Format(parse.AirDateLayout) == date {
			found = append(found, e)
		}
	}
	return found, len(found) > 0
}

// fromRelease maps a parsed name to episodes; a full-season name maps to none.
func (ix episodeIndex) fromRelease(r parse.Release) ([]catalogapi.Episode, bool) {
	if r.AirDate != "" {
		return ix.byAirDate(r.AirDate)
	}
	return ix.byNumbers(r.Season, r.Episodes)
}

// fromName parses a path, falling back to its folder name, then to the release name.
func (ix episodeIndex) fromName(path, releaseName string) ([]catalogapi.Episode, bool) {
	if r, err := parse.ParseFile(path, domain.MediaKindSeries); err == nil {
		if found, ok := ix.fromRelease(r); ok {
			return found, true
		}
	}
	if r, err := parse.ParseRelease(releaseName, domain.MediaKindSeries); err == nil {
		return ix.fromRelease(r)
	}
	return nil, false
}

// refs are the episodes as a media file records them, in (season, number) order.
func refs(episodes []catalogapi.Episode) []domain.EpisodeRef {
	out := make([]domain.EpisodeRef, 0, len(episodes))
	for _, e := range episodes {
		out = append(out, domain.EpisodeRef{Episode: e.ID, Season: e.Season, Number: e.Number})
	}
	slices.SortFunc(out, func(a, b domain.EpisodeRef) int {
		return cmp.Or(cmp.Compare(a.Season, b.Season), cmp.Compare(a.Number, b.Number))
	})
	return out
}
