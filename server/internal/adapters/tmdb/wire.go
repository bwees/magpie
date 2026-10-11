package tmdb

import (
	"strconv"
	"strings"
	"time"

	"magpie/internal/catalog"
	"magpie/internal/domain"
)

// TMDB's JSON shapes, decoded loosely: fields TMDB may send as null are pointers.

const (
	mediaTypeMovie = "movie"
	mediaTypeTV    = "tv"

	statusEnded    = "Ended"
	statusCanceled = "Canceled"

	vectorImageSuffix = ".svg"
)

type wireAltTitle struct {
	Title string `json:"title"`
}

type wireExternalIDs struct {
	Imdb *string `json:"imdb_id"`
	Tvdb *int64  `json:"tvdb_id"`
}

type wireImage struct {
	FilePath string  `json:"file_path"`
	Language *string `json:"iso_639_1"`
}

type wireImages struct {
	Logos []wireImage `json:"logos"`
}

type wireReleaseDate struct {
	ReleaseDate time.Time `json:"release_date"`
	Type        int       `json:"type"`
}

type wireMovie struct {
	ID                int64   `json:"id"`
	Title             string  `json:"title"`
	OriginalTitle     string  `json:"original_title"`
	OriginalLanguage  string  `json:"original_language"`
	Overview          string  `json:"overview"`
	ReleaseDate       string  `json:"release_date"`
	Runtime           *int    `json:"runtime"`
	PosterPath        *string `json:"poster_path"`
	BackdropPath      *string `json:"backdrop_path"`
	AlternativeTitles struct {
		Titles []wireAltTitle `json:"titles"`
	} `json:"alternative_titles"`
	ExternalIDs  wireExternalIDs `json:"external_ids"`
	Images       wireImages      `json:"images"`
	ReleaseDates struct {
		Results []struct {
			Region       string            `json:"iso_3166_1"`
			ReleaseDates []wireReleaseDate `json:"release_dates"`
		} `json:"results"`
	} `json:"release_dates"`
}

type wireSeasonSummary struct {
	SeasonNumber int     `json:"season_number"`
	EpisodeCount int     `json:"episode_count"`
	AirDate      *string `json:"air_date"`
}

type wireSeries struct {
	ID                int64               `json:"id"`
	Name              string              `json:"name"`
	OriginalName      string              `json:"original_name"`
	OriginalLanguage  string              `json:"original_language"`
	Overview          string              `json:"overview"`
	FirstAirDate      string              `json:"first_air_date"`
	Status            string              `json:"status"`
	EpisodeRunTime    []int               `json:"episode_run_time"`
	Seasons           []wireSeasonSummary `json:"seasons"`
	PosterPath        *string             `json:"poster_path"`
	BackdropPath      *string             `json:"backdrop_path"`
	AlternativeTitles struct {
		Results []wireAltTitle `json:"results"`
	} `json:"alternative_titles"`
	ExternalIDs wireExternalIDs `json:"external_ids"`
	Images      wireImages      `json:"images"`
}

type wireEpisode struct {
	ID            int64   `json:"id"`
	SeasonNumber  int     `json:"season_number"`
	EpisodeNumber int     `json:"episode_number"`
	Name          string  `json:"name"`
	AirDate       *string `json:"air_date"`
	Runtime       *int    `json:"runtime"`
}

type wireSeason struct {
	SeasonNumber int           `json:"season_number"`
	Episodes     []wireEpisode `json:"episodes"`
}

type wireEpisodeGroupSummary struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         int    `json:"type"`
	EpisodeCount int    `json:"episode_count"`
	GroupCount   int    `json:"group_count"`
}

type wireEpisodeGroup struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   int    `json:"type"`
	Groups []struct {
		Name     string `json:"name"`
		Order    int    `json:"order"`
		Episodes []struct {
			ID    int64 `json:"id"`
			Order int   `json:"order"`
		} `json:"episodes"`
	} `json:"groups"`
}

type wireSearchItem struct {
	ID            int64   `json:"id"`
	MediaType     string  `json:"media_type"`
	Title         string  `json:"title"`
	Name          string  `json:"name"`
	OriginalTitle string  `json:"original_title"`
	OriginalName  string  `json:"original_name"`
	ReleaseDate   string  `json:"release_date"`
	FirstAirDate  string  `json:"first_air_date"`
	Overview      string  `json:"overview"`
	PosterPath    *string `json:"poster_path"`
}

type wirePage struct {
	Page         int              `json:"page"`
	TotalPages   int              `json:"total_pages"`
	TotalResults int              `json:"total_results"`
	Results      []wireSearchItem `json:"results"`
}

type wireFoundEpisode struct {
	ID            int64 `json:"id"`
	ShowID        int64 `json:"show_id"`
	SeasonNumber  int   `json:"season_number"`
	EpisodeNumber int   `json:"episode_number"`
}

type wireFind struct {
	Movies   []wireSearchItem   `json:"movie_results"`
	Series   []wireSearchItem   `json:"tv_results"`
	Episodes []wireFoundEpisode `json:"tv_episode_results"`
}

type wireChanges struct {
	Page       int `json:"page"`
	TotalPages int `json:"total_pages"`
	Results    []struct {
		ID int64 `json:"id"`
	} `json:"results"`
}

func (m wireMovie) toPort(lang string) catalog.TmdbMovie {
	movie := catalog.TmdbMovie{
		ID:                domain.TmdbID(m.ID),
		Title:             m.Title,
		OriginalTitle:     m.OriginalTitle,
		OriginalLanguage:  domain.Language(m.OriginalLanguage),
		Overview:          m.Overview,
		ReleaseDate:       parseDate(m.ReleaseDate),
		RuntimeMinutes:    deref(m.Runtime),
		AlternativeTitles: altTitles(m.AlternativeTitles.Titles),
		External:          m.ExternalIDs.toPort(),
		Images: catalog.TmdbImages{
			Poster:   deref(m.PosterPath),
			Backdrop: deref(m.BackdropPath),
			Logo:     pickLogo(m.Images.Logos, lang),
		},
	}
	for _, region := range m.ReleaseDates.Results {
		for _, d := range region.ReleaseDates {
			movie.ReleaseDates = append(movie.ReleaseDates, catalog.TmdbReleaseDate{
				Region: region.Region,
				Type:   catalog.TmdbReleaseType(d.Type),
				Date:   d.ReleaseDate.UTC(),
			})
		}
	}
	return movie
}

func (s wireSeries) toPort(lang string) catalog.TmdbSeries {
	series := catalog.TmdbSeries{
		ID:                domain.TmdbID(s.ID),
		Name:              s.Name,
		OriginalName:      s.OriginalName,
		OriginalLanguage:  domain.Language(s.OriginalLanguage),
		Overview:          s.Overview,
		FirstAirDate:      parseDate(s.FirstAirDate),
		Ended:             s.Status == statusEnded || s.Status == statusCanceled,
		AlternativeTitles: altTitles(s.AlternativeTitles.Results),
		External:          s.ExternalIDs.toPort(),
		Images: catalog.TmdbImages{
			Poster:   deref(s.PosterPath),
			Backdrop: deref(s.BackdropPath),
			Logo:     pickLogo(s.Images.Logos, lang),
		},
	}
	if len(s.EpisodeRunTime) > 0 {
		series.EpisodeRuntimeMinutes = s.EpisodeRunTime[0]
	}
	for _, season := range s.Seasons {
		series.Seasons = append(series.Seasons, catalog.TmdbSeasonSummary{
			Number:       season.SeasonNumber,
			EpisodeCount: season.EpisodeCount,
			AirDate:      parseDate(deref(season.AirDate)),
		})
	}
	return series
}

func (s wireSeason) toPort(series domain.TmdbID) catalog.TmdbSeason {
	season := catalog.TmdbSeason{Series: series, Number: s.SeasonNumber}
	for _, e := range s.Episodes {
		season.Episodes = append(season.Episodes, catalog.TmdbEpisode{
			ID:             catalog.TmdbEpisodeID(e.ID),
			Season:         e.SeasonNumber,
			Number:         e.EpisodeNumber,
			Name:           e.Name,
			AirDate:        parseDate(deref(e.AirDate)),
			RuntimeMinutes: deref(e.Runtime),
		})
	}
	return season
}

func (g wireEpisodeGroup) toPort() catalog.TmdbEpisodeGroup {
	group := catalog.TmdbEpisodeGroup{ID: g.ID, Name: g.Name, Type: catalog.TmdbEpisodeGroupType(g.Type)}
	for _, season := range g.Groups {
		entry := catalog.TmdbEpisodeGroupSeason{Name: season.Name, Order: season.Order}
		for _, e := range season.Episodes {
			entry.Episodes = append(entry.Episodes, catalog.TmdbEpisodeGroupEntry{Episode: catalog.TmdbEpisodeID(e.ID), Order: e.Order})
		}
		group.Groups = append(group.Groups, entry)
	}
	return group
}

func (w wireSearchItem) toPort(kind domain.MediaKind) catalog.TmdbSearchItem {
	item := catalog.TmdbSearchItem{
		ID:         domain.TmdbID(w.ID),
		Kind:       kind,
		Overview:   w.Overview,
		PosterPath: deref(w.PosterPath),
	}
	date := w.ReleaseDate
	item.Name, item.OriginalName = w.Title, w.OriginalTitle
	if kind == domain.MediaKindSeries {
		item.Name, item.OriginalName, date = w.Name, w.OriginalName, w.FirstAirDate
	}
	if parsed := parseDate(date); parsed != nil {
		item.Year = parsed.Year()
	}
	return item
}

// mixedItems keeps movies and tv results, dropping people.
func mixedItems(results []wireSearchItem) []catalog.TmdbSearchItem {
	items := []catalog.TmdbSearchItem{}
	for _, r := range results {
		switch r.MediaType {
		case mediaTypeMovie:
			items = append(items, r.toPort(domain.MediaKindMovie))
		case mediaTypeTV:
			items = append(items, r.toPort(domain.MediaKindSeries))
		}
	}
	return items
}

func itemsOfKind(results []wireSearchItem, kind domain.MediaKind) []catalog.TmdbSearchItem {
	items := make([]catalog.TmdbSearchItem, 0, len(results))
	for _, r := range results {
		items = append(items, r.toPort(kind))
	}
	return items
}

func (e wireExternalIDs) toPort() catalog.TmdbExternalIDs {
	var ids catalog.TmdbExternalIDs
	if e.Imdb != nil && *e.Imdb != "" {
		imdb := *e.Imdb
		ids.Imdb = &imdb
	}
	if e.Tvdb != nil {
		tvdb := strconv.FormatInt(*e.Tvdb, 10)
		ids.Tvdb = &tvdb
	}
	return ids
}

func altTitles(titles []wireAltTitle) []string {
	names := make([]string, 0, len(titles))
	for _, t := range titles {
		names = append(names, t.Title)
	}
	return names
}

// pickLogo prefers the metadata language, then English, then a language-neutral logo. SVGs are skipped
// because the image cache resizes raster images only.
func pickLogo(logos []wireImage, lang string) string {
	for _, want := range logoLanguages(lang) {
		for _, logo := range logos {
			if strings.HasSuffix(logo.FilePath, vectorImageSuffix) || deref(logo.Language) != want {
				continue
			}
			return logo.FilePath
		}
	}
	return ""
}

const (
	englishLanguage   = "en"
	languageRegionSep = "-"
	neutralLanguage   = ""
)

// logoLanguages lists ISO 639-1 codes to try in order; "" is a language-neutral image.
func logoLanguages(lang string) []string {
	primary, _, _ := strings.Cut(lang, languageRegionSep)
	if primary == "" || primary == englishLanguage {
		return []string{englishLanguage, neutralLanguage}
	}
	return []string{primary, englishLanguage, neutralLanguage}
}

func parseDate(value string) *time.Time {
	parsed, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return nil
	}
	return &parsed
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
