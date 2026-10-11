// Package api is Catalog's public contract: the Title Reader, the events Catalog publishes and the migration Importer.
// Spec: docs/spec/contexts/catalog.md.
package api

import (
	"context"
	"errors"
	"time"

	"magpie/internal/domain"
)

var ErrNotFound = errors.New("catalog: not found")

// Title.SeriesStatus values; movies have "".
const (
	SeriesStatusContinuing = "CONTINUING"
	SeriesStatusEnded      = "ENDED"
)

// ImagePath kinds.
const (
	ImageKindPoster   = "POSTER"
	ImageKindBackdrop = "BACKDROP"
	ImageKindLogo     = "LOGO"
)

type Title struct {
	ID       domain.TitleID
	Kind     domain.MediaKind
	External domain.ExternalIDs // Tmdb always set; Tvdb/Imdb may be nil
	Name     string
	// OriginalName is the title in its original language.
	OriginalName string
	// Aliases are alternative titles from TMDB, for release matching.
	Aliases []string
	Year    int
	// RuntimeMinutes is the movie runtime, or the typical episode runtime; 0 if unknown.
	RuntimeMinutes       int
	OriginalLanguage     domain.Language
	SeriesStatus         string // SeriesStatusContinuing | SeriesStatusEnded; "" for movies
	Profile              domain.ProfileID
	Monitored            bool
	MonitorFutureSeasons bool
	EpisodeGroupID       string // "" = TMDB aired order
	Dates                ReleaseDates
	// Released follows the global "released" rule for movies; for series, any episode is released.
	Released    bool
	ReleasedAt  *time.Time
	Origin      domain.Origin // REQUEST | MANUAL | LIBRARY_SCAN | MIGRATION
	RequestedBy *domain.UserID
}

// ReleaseDates are the earliest dates in the release region, else in any region.
type ReleaseDates struct {
	Theatrical, Digital, Physical *time.Time
}

type WantedLanguage struct {
	Language domain.Language `json:"language"`
	// Forced also wants a forced track.
	Forced bool `json:"forced"`
	// HearingImpaired accepts the HI variant.
	HearingImpaired bool `json:"hearingImpaired"`
}

type Episode struct {
	ID    domain.EpisodeID
	Title domain.TitleID
	// Season and Number are in the current numbering (the episode group if one is set).
	Season, Number int
	// AiredSeason and AiredNumber are TMDB aired order, for release-name matching.
	AiredSeason, AiredNumber int
	Name                     string
	AirDate                  *time.Time
	RuntimeMinutes           int
	Monitored                bool
	Released                 bool
}

// TitleFilter fields that are nil or empty don't filter.
type TitleFilter struct {
	Kind      *domain.MediaKind
	Monitored *bool
	Released  *bool
	IDs       []domain.TitleID
}

// Reader is read-only and backed by Catalog's repository. It never calls TMDB; it returns what is stored.
type Reader interface {
	// Title returns one Title. ErrNotFound if the ID is unknown or removed.
	Title(ctx context.Context, id domain.TitleID) (Title, error)
	// TitleByTmdb looks up by TMDB ID and kind. ErrNotFound if not tracked.
	TitleByTmdb(ctx context.Context, kind domain.MediaKind, id domain.TmdbID) (Title, error)
	// Titles lists Titles matching the filter, ordered by name then year. Empty filter = all.
	Titles(ctx context.Context, f TitleFilter) ([]Title, error)
	// Episodes lists a series' episodes ordered by (Season, Number) in the current numbering.
	// Movies return an empty slice. ErrNotFound if the Title is unknown.
	Episodes(ctx context.Context, title domain.TitleID) ([]Episode, error)
	// Episode returns one episode. ErrNotFound if unknown.
	Episode(ctx context.Context, id domain.EpisodeID) (Episode, error)
	// EpisodeByNumber resolves a season/number in the current numbering. ErrNotFound if no match.
	EpisodeByNumber(ctx context.Context, title domain.TitleID, season, number int) (Episode, error)
	// Wanted returns what is wanted for a Title. Series: its monitored, released episodes ordered by
	// (Season, Number), with ok = at least one; an unmonitored series wants nothing. Movies: an empty slice,
	// with ok = the movie is monitored and released. Callers decide whether a file is missing.
	// ErrNotFound if the Title is unknown.
	Wanted(ctx context.Context, title domain.TitleID) (episodes []Episode, ok bool, err error)
	// ImagePath returns the local cached image path for an ImageKind*, or ErrNotFound if not cached yet.
	ImagePath(ctx context.Context, title domain.TitleID, kind string) (string, error)
	// WantedLanguages returns the effective ordered list for a Title: its override if set,
	// else the global list. ErrNotFound if the Title is unknown.
	WantedLanguages(ctx context.Context, title domain.TitleID) ([]WantedLanguage, error)
	// GlobalLanguages returns the global ordered list (never empty after setup).
	GlobalLanguages(ctx context.Context) ([]WantedLanguage, error)
}
