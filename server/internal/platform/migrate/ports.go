// Package migrate is the one-time Sonarr/Radarr import: a platform job that reads both apps read-only, plans
// the import in a dry run, and commits it through each context's api.Importer. Spec: docs/spec/11-migration.md.
package migrate

import (
	"context"
	"errors"
	"time"

	"magpie/internal/domain"
)

// App is a source the migration reads.
type App string

const (
	AppSonarr App = "SONARR"
	AppRadarr App = "RADARR"
)

// Apps lists the sources in the order the migration reads them.
var Apps = []App{AppSonarr, AppRadarr}

// Connection is where one app answers.
type Connection struct {
	URL    string `json:"url"`
	APIKey string `json:"apiKey"`
}

// Arr reads Sonarr or Radarr through its v3 API, read-only, with X-Api-Key. It is implemented by
// internal/adapters/arr.
type Arr interface {
	Status(ctx context.Context, app App, conn Connection) (ArrStatus, error)
	RootFolders(ctx context.Context, app App, conn Connection) ([]ArrRootFolder, error)
	// Library reads everything the migration maps. progress reports titles read so far out of all titles.
	Library(ctx context.Context, app App, conn Connection, progress func(done, total int)) (ArrLibrary, error)
}

var (
	ErrArrUnauthorized = errors.New("arr: the API key was rejected")
	ErrArrUnreachable  = errors.New("arr: unreachable")
	ErrArrWrongApp     = errors.New("arr: that URL is another app")
)

type ArrStatus struct {
	AppName string
	Version string
}

// ArrLibrary is everything one app holds that the migration maps. Enum values (sources, languages, flags)
// stay the app's own; the migration translates them.
type ArrLibrary struct {
	App                App
	Version            string
	Series             []ArrSeries
	Movies             []ArrMovie
	RootFolders        []ArrRootFolder
	Tags               []ArrNamed
	QualityProfiles    []ArrQualityProfile
	QualityDefinitions []ArrQualityDefinition
	CustomFormats      []ArrCustomFormat
	ReleaseProfiles    []ArrReleaseProfile
	DelayProfiles      []ArrDelayProfile
	Languages          []ArrNamed
	IndexerFlags       []ArrNamed
	Blocklist          []ArrBlocklistItem
	History            []ArrHistoryItem
}

// ArrNamed is an ID with its name: a tag, language or indexer flag.
type ArrNamed struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Sonarr series types.
const (
	SeriesTypeStandard = "standard"
	SeriesTypeDaily    = "daily"
	SeriesTypeAnime    = "anime"
)

// Sonarr monitorNewItems values.
const MonitorNewItemsAll = "all"

type ArrSeries struct {
	ID                int          `json:"id"`
	Title             string       `json:"title"`
	Year              int          `json:"year"`
	TvdbID            int          `json:"tvdbId"`
	TmdbID            int          `json:"tmdbId"`
	ImdbID            string       `json:"imdbId"`
	Path              string       `json:"path"`
	QualityProfileID  int          `json:"qualityProfileId"`
	Monitored         bool         `json:"monitored"`
	MonitorNewItems   string       `json:"monitorNewItems"`
	UseSceneNumbering bool         `json:"useSceneNumbering"`
	SeriesType        string       `json:"seriesType"`
	SeasonType        string       `json:"seasonType"` // Sonarr v5 only; "" before
	Tags              []int        `json:"tags"`
	Added             time.Time    `json:"added"`
	AlternateTitles   []string     `json:"alternateTitles"`
	Episodes          []ArrEpisode `json:"episodes"`
	Files             []ArrFile    `json:"files"`
}

type ArrEpisode struct {
	ID            int    `json:"id"`
	SeasonNumber  int    `json:"seasonNumber"`
	EpisodeNumber int    `json:"episodeNumber"`
	TvdbID        int    `json:"tvdbId"`
	Title         string `json:"title"`
	AirDate       string `json:"airDate"` // yyyy-mm-dd, "" when unknown
	Monitored     bool   `json:"monitored"`
	EpisodeFileID int    `json:"episodeFileId"`
}

type ArrMovie struct {
	ID               int       `json:"id"`
	Title            string    `json:"title"`
	Year             int       `json:"year"`
	TmdbID           int       `json:"tmdbId"`
	ImdbID           string    `json:"imdbId"`
	Path             string    `json:"path"`
	QualityProfileID int       `json:"qualityProfileId"`
	Monitored        bool      `json:"monitored"`
	Tags             []int     `json:"tags"`
	Added            time.Time `json:"added"`
	File             *ArrFile  `json:"file"`
}

type ArrFile struct {
	ID           int       `json:"id"`
	Path         string    `json:"path"`
	Size         int64     `json:"size"`
	QualityID    int       `json:"qualityId"` // the app's own quality ID
	Revision     int       `json:"revision"`
	SceneName    string    `json:"sceneName"`
	ReleaseGroup string    `json:"releaseGroup"`
	Languages    []string  `json:"languages"` // names
	DateAdded    time.Time `json:"dateAdded"`
}

type ArrRootFolder struct {
	ID   int    `json:"id"`
	Path string `json:"path"`
}

type ArrQualityProfile struct {
	ID                    int              `json:"id"`
	Name                  string           `json:"name"`
	UpgradeAllowed        bool             `json:"upgradeAllowed"`
	Cutoff                int              `json:"cutoff"` // a quality ID, or a group ID ≥ 1000
	Items                 []ArrProfileItem `json:"items"`  // least preferred first, as the API lists them
	MinFormatScore        int              `json:"minFormatScore"`
	CutoffFormatScore     int              `json:"cutoffFormatScore"`
	MinUpgradeFormatScore int              `json:"minUpgradeFormatScore"`
	FormatItems           []ArrFormatScore `json:"formatItems"`
	Language              *ArrNamed        `json:"language"` // Radarr only
}

// ArrProfileItem is a quality (QualityID) or a group (GroupID ≥ 1000 with Items).
type ArrProfileItem struct {
	GroupID   int              `json:"groupId"`
	Name      string           `json:"name"`
	QualityID int              `json:"qualityId"`
	Items     []ArrProfileItem `json:"items"`
	Allowed   bool             `json:"allowed"`
	// Per-profile sizes, Sonarr v5 only.
	MinSize       *float64 `json:"minSize"`
	MaxSize       *float64 `json:"maxSize"`
	PreferredSize *float64 `json:"preferredSize"`
}

type ArrFormatScore struct {
	Format int `json:"format"`
	Score  int `json:"score"`
}

// ArrQualityDefinition is the app's global size table row for one quality, in MB per minute.
type ArrQualityDefinition struct {
	QualityID     int      `json:"qualityId"`
	MinSize       float64  `json:"minSize"`
	MaxSize       *float64 `json:"maxSize"`
	PreferredSize *float64 `json:"preferredSize"`
}

type ArrCustomFormat struct {
	ID                  int       `json:"id"`
	Name                string    `json:"name"`
	IncludeWhenRenaming bool      `json:"includeWhenRenaming"`
	Specs               []ArrSpec `json:"specs"`
}

// Custom format spec implementations.
const (
	ImplReleaseTitle    = "ReleaseTitleSpecification"
	ImplReleaseGroup    = "ReleaseGroupSpecification"
	ImplEdition         = "EditionSpecification"
	ImplSize            = "SizeSpecification"
	ImplResolution      = "ResolutionSpecification"
	ImplYear            = "YearSpecification"
	ImplReleaseType     = "ReleaseTypeSpecification"
	ImplQualityModifier = "QualityModifierSpecification"
	ImplSource          = "SourceSpecification"
	ImplLanguage        = "LanguageSpecification"
	ImplIndexerFlag     = "IndexerFlagSpecification"
)

// ArrSpec is one custom-format condition. Value is the "value" field as text; Min and Max the "min"/"max" fields.
type ArrSpec struct {
	Name           string  `json:"name"`
	Implementation string  `json:"implementation"`
	Negate         bool    `json:"negate"`
	Required       bool    `json:"required"`
	Value          string  `json:"value"`
	Min            float64 `json:"min"`
	Max            float64 `json:"max"`
}

type ArrReleaseProfile struct {
	ID        int      `json:"id"`
	Name      string   `json:"name"`
	Enabled   bool     `json:"enabled"`
	Required  []string `json:"required"`
	Ignored   []string `json:"ignored"`
	IndexerID int      `json:"indexerId"`
	Tags      []int    `json:"tags"`
}

type ArrDelayProfile struct {
	ID                             int   `json:"id"`
	EnableTorrent                  bool  `json:"enableTorrent"`
	TorrentDelay                   int   `json:"torrentDelay"` // minutes
	BypassIfHighestQuality         bool  `json:"bypassIfHighestQuality"`
	BypassIfAboveCustomFormatScore bool  `json:"bypassIfAboveCustomFormatScore"`
	MinimumCustomFormatScore       int   `json:"minimumCustomFormatScore"`
	Tags                           []int `json:"tags"`
}

// Download protocols.
const (
	ProtocolTorrent = "torrent"
	ProtocolUsenet  = "usenet"
)

type ArrBlocklistItem struct {
	ID          int       `json:"id"`
	SeriesID    int       `json:"seriesId"`
	EpisodeIDs  []int     `json:"episodeIds"`
	MovieID     int       `json:"movieId"`
	SourceTitle string    `json:"sourceTitle"`
	Protocol    string    `json:"protocol"`
	Indexer     string    `json:"indexer"`
	Message     string    `json:"message"`
	Date        time.Time `json:"date"`
}

// History event types.
const (
	HistoryGrabbed                = "grabbed"
	HistoryDownloadFolderImported = "downloadFolderImported"
	HistorySeriesFolderImported   = "seriesFolderImported"
	HistoryMovieFolderImported    = "movieFolderImported"
	HistoryDownloadFailed         = "downloadFailed"
	HistoryEpisodeFileDeleted     = "episodeFileDeleted"
	HistoryMovieFileDeleted       = "movieFileDeleted"
)

// History data keys the migration reads.
const (
	HistoryDataTorrentInfoHash = "torrentInfoHash"
	HistoryDataIndexer         = "indexer"
	HistoryDataMessage         = "message"
	HistoryDataReason          = "reason"
	HistoryDataProtocol        = "protocol"
)

type ArrHistoryItem struct {
	ID          int               `json:"id"`
	SeriesID    int               `json:"seriesId"`
	EpisodeID   int               `json:"episodeId"`
	MovieID     int               `json:"movieId"`
	SourceTitle string            `json:"sourceTitle"`
	EventType   string            `json:"eventType"`
	Date        time.Time         `json:"date"`
	DownloadID  string            `json:"downloadId"`
	Data        map[string]string `json:"data"` // keys lower-camel, as the API sends them
}

// TMDB is the slice of Catalog's TMDB port the migration matches with. internal/app adapts the Catalog
// adapter to it, so both share one rate limit.
type TMDB interface {
	Series(ctx context.Context, id domain.TmdbID) (TmdbSeries, error)
	Season(ctx context.Context, id domain.TmdbID, season int) (TmdbSeason, error)
	EpisodeGroups(ctx context.Context, id domain.TmdbID) ([]TmdbGroupSummary, error)
	EpisodeGroup(ctx context.Context, groupID string) (TmdbGroup, error)
	// Find looks up by an external ID; source is FindTvdb or FindImdb.
	Find(ctx context.Context, source, externalID string) (TmdbFound, error)
	Search(ctx context.Context, query string) ([]TmdbCandidate, error)
	Movie(ctx context.Context, id domain.TmdbID) (TmdbCandidate, error)
}

const (
	FindTvdb = "tvdb_id"
	FindImdb = "imdb_id"
)

var ErrTmdbNotFound = errors.New("tmdb: not found")

// TmdbCandidate is a TMDB movie or series offered as a match.
type TmdbCandidate struct {
	Tmdb domain.TmdbID    `json:"tmdb"`
	Kind domain.MediaKind `json:"kind"`
	Name string           `json:"name"`
	Year int              `json:"year"`
}

type TmdbSeries struct {
	TmdbCandidate
	Tvdb    *string
	Seasons []int
}

type TmdbSeason struct {
	Number   int
	Episodes []TmdbEpisode
}

type TmdbEpisode struct {
	ID             int64
	Season, Number int
	Name           string
	AirDate        string // yyyy-mm-dd
}

// TMDB episode group types the migration prefers.
const (
	GroupOriginalAirDate = 1
	GroupAbsolute        = 2
	GroupDVD             = 3
	GroupDigital         = 4
	GroupTV              = 7
)

type TmdbGroupSummary struct {
	ID   string
	Name string
	Type int
}

type TmdbGroup struct {
	ID      string
	Name    string
	Type    int
	Seasons []TmdbGroupSeason
}

type TmdbGroupSeason struct {
	Order    int
	Episodes []TmdbGroupEntry
}

type TmdbGroupEntry struct {
	Episode int64
	Order   int // 0-based place in its group
}

type TmdbFound struct {
	Movies   []TmdbCandidate
	Series   []TmdbCandidate
	Episodes []TmdbFoundEpisode
}

type TmdbFoundEpisode struct {
	ID             int64
	Series         domain.TmdbID
	Season, Number int
}

// Disk checks paths as the server sees them.
type Disk interface {
	CheckFolder(path string) PathAccess
	Exists(path string) bool
}

// PathAccess is what the server can do with a folder.
type PathAccess string

const (
	PathOK          PathAccess = "OK"
	PathMissing     PathAccess = "MISSING"
	PathNotFolder   PathAccess = "NOT_A_FOLDER"
	PathNotWritable PathAccess = "NOT_WRITABLE"
	PathNotAbsolute PathAccess = "NOT_ABSOLUTE"
)
