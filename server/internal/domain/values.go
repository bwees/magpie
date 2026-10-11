package domain

type MediaKind string

const (
	MediaKindMovie  MediaKind = "MOVIE"
	MediaKindSeries MediaKind = "SERIES"
)

type ExternalIDs struct {
	Tmdb TmdbID  `json:"tmdb"`
	Tvdb *string `json:"tvdb,omitempty"`
	Imdb *string `json:"imdb,omitempty"`
}

type EpisodeRef struct {
	Episode EpisodeID `json:"episode"`
	Season  int       `json:"season"`
	Number  int       `json:"number"`
}

// Quality uses Radarr's quality IDs so migrated profiles keep their meaning.
type Quality struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Source     string `json:"source"`
	Resolution int    `json:"resolution"`
	Revision   int    `json:"revision"`
}

// Language is an ISO 639-1 code.
type Language string

// Origin says how a title or file entered Magpie.
// Titles use REQUEST, MANUAL, LIBRARY_SCAN or MIGRATION; files use DOWNLOAD, LIBRARY_SCAN or MIGRATION.
type Origin string

const (
	OriginRequest     Origin = "REQUEST"
	OriginManual      Origin = "MANUAL"
	OriginLibraryScan Origin = "LIBRARY_SCAN"
	OriginMigration   Origin = "MIGRATION"
	OriginDownload    Origin = "DOWNLOAD"
)
