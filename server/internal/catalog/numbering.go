package catalog

import (
	"cmp"
	"slices"
	"time"
)

// specialsSeason holds TMDB's specials, and episodes an episode group doesn't place.
const specialsSeason = 0

type episodeNumber struct {
	Season, Number int
}

func (row episodeRow) number() episodeNumber {
	return episodeNumber{Season: row.SeasonNumber, Number: row.EpisodeNumber}
}

func byAiredOrder(a, b *episodeRow) int {
	return cmp.Or(cmp.Compare(a.AiredSeason, b.AiredSeason), cmp.Compare(a.AiredNumber, b.AiredNumber))
}

func byCurrentOrder(a, b episodeRow) int {
	return cmp.Or(cmp.Compare(a.SeasonNumber, b.SeasonNumber), cmp.Compare(a.EpisodeNumber, b.EpisodeNumber))
}

// groupPlacement maps each episode a group places to its numbering there. Like Jellyfin, the season is the
// group's order and the number is the entry's order plus one.
func groupPlacement(group TmdbEpisodeGroup) map[TmdbEpisodeID]episodeNumber {
	placed := map[TmdbEpisodeID]episodeNumber{}
	for _, season := range group.Groups {
		for _, entry := range season.Episodes {
			if _, seen := placed[entry.Episode]; !seen {
				placed[entry.Episode] = episodeNumber{Season: season.Order, Number: entry.Order + 1}
			}
		}
	}
	return placed
}

// renumber sets each episode's current numbering: aired order without a group, else the group's. Episodes
// the group doesn't place go to season 0 in aired order, after the group's own season-0 episodes.
func renumber(episodes []episodeRow, group *TmdbEpisodeGroup) {
	if group == nil {
		for i := range episodes {
			episodes[i].SeasonNumber, episodes[i].EpisodeNumber = episodes[i].AiredSeason, episodes[i].AiredNumber
		}
		return
	}
	placed := groupPlacement(*group)
	var unplaced []*episodeRow
	lastSpecial := 0
	for i := range episodes {
		e := &episodes[i]
		at, ok := placed[TmdbEpisodeID(e.TmdbEpisodeID)]
		if !ok {
			unplaced = append(unplaced, e)
			continue
		}
		e.SeasonNumber, e.EpisodeNumber = at.Season, at.Number
		if at.Season == specialsSeason {
			lastSpecial = max(lastSpecial, at.Number)
		}
	}
	slices.SortFunc(unplaced, byAiredOrder)
	for i, e := range unplaced {
		e.SeasonNumber, e.EpisodeNumber = specialsSeason, lastSpecial+i+1
	}
}

// airedBy reports whether an episode has aired at now. The air date is a calendar date, compared with
// now's date in now's time zone.
func airedBy(airDate *time.Time, now time.Time) bool {
	if airDate == nil {
		return false
	}
	year, month, day := airDate.UTC().Date()
	aired := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return !aired.After(today)
}
