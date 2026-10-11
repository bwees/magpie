package catalog

import (
	"context"
	"errors"
	"io"
	"time"

	"magpie/internal/domain"
)

// TMDB is implemented by internal/adapters/tmdb. The adapter rate-limits to 40 requests/s and retries
// 429s; after that it returns ErrTmdbUnavailable.
type TMDB interface {
	// Movie returns details, release_dates, external_ids and images.
	Movie(ctx context.Context, id domain.TmdbID, lang string) (TmdbMovie, error)
	// Series returns details, a seasons summary, external_ids and images.
	Series(ctx context.Context, id domain.TmdbID, lang string) (TmdbSeries, error)
	Season(ctx context.Context, id domain.TmdbID, season int, lang string) (TmdbSeason, error)
	EpisodeGroups(ctx context.Context, id domain.TmdbID) ([]TmdbEpisodeGroupSummary, error)
	EpisodeGroup(ctx context.Context, groupID string) (TmdbEpisodeGroup, error)
	// Find looks up by an external ID; source is TmdbFindTvdb or TmdbFindImdb.
	Find(ctx context.Context, source string, externalID string) (TmdbFindResult, error)
	// Search finds movies and series. Pages start at 1.
	Search(ctx context.Context, query string, page int) (TmdbSearchPage, error)
	// Trending lists movies and series trending in window (TmdbTrendingWeek).
	Trending(ctx context.Context, window string) ([]TmdbSearchItem, error)
	// Changes lists IDs of that kind changed since the given time.
	Changes(ctx context.Context, kind domain.MediaKind, since time.Time) ([]domain.TmdbID, error)
	// Image downloads an image by its TMDB path at a TMDB size (TmdbImageOriginal or e.g. "w500").
	Image(ctx context.Context, path string, size string) (io.ReadCloser, error)
}

var (
	ErrTmdbNotFound     = errors.New("tmdb: not found")
	ErrTmdbUnauthorized = errors.New("tmdb: invalid api key")
	ErrTmdbUnavailable  = errors.New("tmdb: unavailable") // after retries
)

const (
	TmdbFindTvdb      = "tvdb_id"
	TmdbFindImdb      = "imdb_id"
	TmdbTrendingWeek  = "week"
	TmdbImageOriginal = "original"
)

// TmdbReleaseType is TMDB's release_dates type.
type TmdbReleaseType int

const (
	TmdbReleasePremiere TmdbReleaseType = iota + 1
	TmdbReleaseTheatricalLimited
	TmdbReleaseTheatrical
	TmdbReleaseDigital
	TmdbReleasePhysical
	TmdbReleaseTV
)

// TmdbEpisodeGroupType is TMDB's episode group type.
type TmdbEpisodeGroupType int

const (
	TmdbGroupOriginalAirDate TmdbEpisodeGroupType = iota + 1
	TmdbGroupAbsolute
	TmdbGroupDVD
	TmdbGroupDigital
	TmdbGroupStoryArc
	TmdbGroupProduction
	TmdbGroupTV
)

// TmdbEpisodeID is TMDB's ID for one episode. Catalog's EpisodeID is tied to it.
type TmdbEpisodeID int64

type TmdbReleaseDate struct {
	Region string // ISO 3166-1
	Type   TmdbReleaseType
	Date   time.Time
}

// TmdbImages are TMDB image paths; "" when TMDB has none.
type TmdbImages struct {
	Poster, Backdrop, Logo string
}

type TmdbExternalIDs struct {
	Tvdb, Imdb *string
}

type TmdbMovie struct {
	ID                domain.TmdbID
	Title             string
	OriginalTitle     string
	OriginalLanguage  domain.Language
	Overview          string
	ReleaseDate       *time.Time // TMDB's primary date, for the year
	RuntimeMinutes    int        // 0 if unknown
	AlternativeTitles []string
	ReleaseDates      []TmdbReleaseDate
	External          TmdbExternalIDs
	Images            TmdbImages
}

type TmdbSeries struct {
	ID                    domain.TmdbID
	Name                  string
	OriginalName          string
	OriginalLanguage      domain.Language
	Overview              string
	FirstAirDate          *time.Time
	Ended                 bool // TMDB status Ended or Canceled
	EpisodeRuntimeMinutes int  // typical episode runtime; 0 if unknown
	AlternativeTitles     []string
	Seasons               []TmdbSeasonSummary
	External              TmdbExternalIDs
	Images                TmdbImages
}

type TmdbSeasonSummary struct {
	Number       int
	EpisodeCount int
	AirDate      *time.Time
}

type TmdbSeason struct {
	Series   domain.TmdbID
	Number   int
	Episodes []TmdbEpisode
}

type TmdbEpisode struct {
	ID             TmdbEpisodeID
	Season, Number int // aired order
	Name           string
	AirDate        *time.Time
	RuntimeMinutes int
}

type TmdbEpisodeGroupSummary struct {
	ID           string
	Name         string
	Type         TmdbEpisodeGroupType
	EpisodeCount int
	GroupCount   int
}

// TmdbEpisodeGroup orders a series' episodes into groups. Each group becomes a season.
type TmdbEpisodeGroup struct {
	ID     string
	Name   string
	Type   TmdbEpisodeGroupType
	Groups []TmdbEpisodeGroupSeason
}

type TmdbEpisodeGroupSeason struct {
	Name     string
	Order    int // season position in the group
	Episodes []TmdbEpisodeGroupEntry
}

type TmdbEpisodeGroupEntry struct {
	Episode TmdbEpisodeID
	Order   int // 0-based position inside its group
}

type TmdbFindResult struct {
	Movies   []TmdbSearchItem
	Series   []TmdbSearchItem
	Episodes []TmdbFoundEpisode
}

type TmdbFoundEpisode struct {
	ID             TmdbEpisodeID
	Series         domain.TmdbID
	Season, Number int
}

type TmdbSearchPage struct {
	Page, TotalPages, TotalResults int
	Items                          []TmdbSearchItem
}

type TmdbSearchItem struct {
	ID           domain.TmdbID
	Kind         domain.MediaKind
	Name         string
	OriginalName string
	Year         int // 0 if unknown
	Overview     string
	PosterPath   string
}
