package acquisition

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"

	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
)

// Newznab category ranges.
const (
	movieCategoryFirst = 2000
	movieCategoryLast  = 2999
	tvCategoryFirst    = 5000
	tvCategoryLast     = 5999
)

// searchTarget is what one search looks for.
type searchTarget struct {
	Title catalogapi.Title
	// Episodes are the searched episodes; empty searches the whole title.
	Episodes []catalogapi.Episode
}

// covers reports whether the indexer's caps can search this kind of title.
func (c TorznabCaps) covers(kind domain.MediaKind) bool {
	if kind == domain.MediaKindSeries {
		return c.TVSearch
	}
	return c.MovieSearch
}

func (c TorznabCaps) params(kind domain.MediaKind) []TorznabParam {
	if kind == domain.MediaKindSeries {
		return c.TVParams
	}
	return c.MovieParams
}

// queryPlan lists an indexer's query tiers in order: an ID query with whatever IDs its caps support,
// then a title query. hasIDTier is false when the caps support none of the title's IDs.
func queryPlan(caps TorznabCaps, target searchTarget) (plan []TorznabQuery, hasIDTier bool) {
	kind := target.Title.Kind
	params := caps.params(kind)
	supports := func(p TorznabParam) bool { return slices.Contains(params, p) }
	season, episode := episodeScope(target.Episodes)

	base := TorznabQuery{Kind: kind, Categories: kindCategories(caps.Categories, kind)}
	numbered := supports(ParamSeason) && (episode == nil || supports(ParamEpisode))
	if numbered {
		base.Season, base.Episode = season, episode
	}

	byID := base
	external := target.Title.External
	if supports(ParamTMDB) && external.Tmdb != 0 {
		byID.Tmdb, hasIDTier = external.Tmdb, true
	}
	if supports(ParamIMDb) && external.Imdb != nil && *external.Imdb != "" {
		byID.Imdb, hasIDTier = *external.Imdb, true
	}
	if kind == domain.MediaKindSeries && supports(ParamTVDB) && external.Tvdb != nil && *external.Tvdb != "" {
		byID.Tvdb, hasIDTier = *external.Tvdb, true
	}
	if hasIDTier {
		plan = append(plan, byID)
	}

	byTitle := base
	terms := []string{searchName(target.Title.Name)}
	switch {
	case kind == domain.MediaKindMovie && target.Title.Year != 0 && supports(ParamYear):
		year := target.Title.Year
		byTitle.Year = &year
	case kind == domain.MediaKindMovie && target.Title.Year != 0:
		terms = append(terms, strconv.Itoa(target.Title.Year))
	case kind == domain.MediaKindSeries && season != nil && !numbered:
		terms = append(terms, episodeTerm(*season, episode))
	}
	byTitle.Query = strings.Join(terms, " ")
	return append(plan, byTitle), hasIDTier
}

// episodeScope narrows a series query: the season when every searched episode is in one, plus the
// episode when exactly one was searched.
func episodeScope(episodes []catalogapi.Episode) (season, episode *int) {
	if len(episodes) == 0 {
		return nil, nil
	}
	first := episodes[0]
	if slices.ContainsFunc(episodes, func(ep catalogapi.Episode) bool { return ep.Season != first.Season }) {
		return nil, nil
	}
	s := first.Season
	if len(episodes) == 1 {
		n := first.Number
		return &s, &n
	}
	return &s, nil
}

func episodeTerm(season int, episode *int) string {
	if episode == nil {
		return fmt.Sprintf("S%02d", season)
	}
	return fmt.Sprintf("S%02dE%02d", season, *episode)
}

func kindCategories(categories []int, kind domain.MediaKind) []int {
	first, last := movieCategoryFirst, movieCategoryLast
	if kind == domain.MediaKindSeries {
		first, last = tvCategoryFirst, tvCategoryLast
	}
	matching := []int{}
	for _, c := range categories {
		if c >= first && c <= last {
			matching = append(matching, c)
		}
	}
	if len(matching) == 0 {
		return nil
	}
	return matching
}

// searchName keeps letters, digits and spaces, which indexers search best on.
func searchName(name string) string {
	return strings.Join(strings.FieldsFunc(name, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '\''
	}), " ")
}

// cacheKey identifies a query in the per-indexer result cache.
func (q TorznabQuery) cacheKey() string {
	number := func(n *int) string {
		if n == nil {
			return ""
		}
		return strconv.Itoa(*n)
	}
	return strings.Join([]string{
		string(q.Kind), q.Query, q.Imdb, strconv.FormatInt(int64(q.Tmdb), 10), q.Tvdb,
		number(q.Season), number(q.Episode), number(q.Year), fmt.Sprint(q.Categories),
	}, "|")
}
