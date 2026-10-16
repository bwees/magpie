package acquisition

import (
	"context"
	"errors"
	"fmt"
	"time"

	"magpie/internal/acquisition/api"
	"magpie/internal/domain"
)

// Prowlarr is implemented by internal/adapters/prowlarr.
// Errors: ErrUnauthorized, *ErrIndexerFailed (Torznab <error>), ErrTimeout, ErrUnavailable.
type Prowlarr interface {
	// Indexers lists enabled torrent indexers with Torznab caps and Prowlarr's view of their health.
	// Called hourly and on demand. One indexer failing its caps call doesn't fail the list.
	Indexers(ctx context.Context) ([]ProwlarrIndexer, error)
	// WebURL is Prowlarr's own UI, where indexers are configured. "" when no connection is set up.
	WebURL(ctx context.Context) (string, error)
	// Search queries ONE indexer's Torznab endpoint (/{id}/api). Never the aggregate search.
	// The query carries ID params the indexer supports, else a title query. ctx lives as long as the
	// search: the caller reports FAILED after the search timeout but still takes a later answer.
	Search(ctx context.Context, indexerID int, q TorznabQuery) ([]TorznabItem, error)
	// RSS fetches an indexer's recent feed (Torznab search with no query).
	RSS(ctx context.Context, indexerID int, categories []int) ([]TorznabItem, error)
	// Download fetches a .torrent; returns MagnetRedirect when the URL 30x-redirects to a magnet.
	Download(ctx context.Context, link string) (TorrentPayload, error)
}

type ProwlarrIndexer struct {
	ID       int
	Name     string
	Privacy  IndexerPrivacy
	Priority int // Prowlarr's priority; Acquisition layers its own on top
	// Caps is nil when Prowlarr couldn't answer t=caps for this indexer; Health.Error says why.
	Caps   *TorznabCaps
	Health ProwlarrHealth
}

type IndexerPrivacy string

const (
	PrivacyPublic      IndexerPrivacy = "PUBLIC"
	PrivacySemiPrivate IndexerPrivacy = "SEMI_PRIVATE"
	PrivacyPrivate     IndexerPrivacy = "PRIVATE"
)

// ProwlarrHealth is Prowlarr's view of one indexer.
type ProwlarrHealth struct {
	// Error is set while the indexer is failing: Prowlarr backed it off after repeated failures,
	// or its caps call returned an error.
	Error           string
	FailingSince    *time.Time
	AverageResponse time.Duration // 0 until Prowlarr has stats for it
}

// TorznabParam is a search parameter an indexer may support.
type TorznabParam string

const (
	ParamQuery   TorznabParam = "QUERY"
	ParamIMDb    TorznabParam = "IMDB_ID"
	ParamTMDB    TorznabParam = "TMDB_ID"
	ParamTVDB    TorznabParam = "TVDB_ID"
	ParamSeason  TorznabParam = "SEASON"
	ParamEpisode TorznabParam = "EPISODE"
	ParamYear    TorznabParam = "YEAR"
)

type TorznabCaps struct {
	MovieSearch bool
	MovieParams []TorznabParam
	TVSearch    bool
	TVParams    []TorznabParam
	Categories  []int
}

// TorznabQuery is one search. Kind picks movie-search or tv-search; empty fields are left out.
type TorznabQuery struct {
	Kind       domain.MediaKind
	Query      string
	Imdb       string
	Tmdb       domain.TmdbID
	Tvdb       string
	Season     *int
	Episode    *int
	Year       *int
	Categories []int
}

type TorznabItem struct {
	GUID                 string
	Title                string
	DownloadURL          string // "" when only a magnet is offered
	MagnetURI            string
	InfoHash             string // "" when the indexer doesn't publish it
	SizeBytes            int64
	Seeders              int
	Peers                int
	PublishedAt          time.Time
	Categories           []int
	DownloadVolumeFactor float64 // 0 = freeleech
	UploadVolumeFactor   float64
}

// TorrentPayload is a downloaded .torrent, or the magnet its link redirected to.
type TorrentPayload struct {
	Torrent        []byte // bencoded .torrent; nil when MagnetRedirect is set
	MagnetRedirect string
}

// QBittorrent is implemented by internal/adapters/qbittorrent. Requires qBittorrent ≥ 5.2 (WebAPI ≥ 2.15).
// Auth: API key (Authorization: Bearer). Errors: ErrUnauthorized, ErrTorrentNotFound (hash gone), ErrUnavailable.
type QBittorrent interface {
	// Version is the startup check; below MinWebAPIVersion it returns the version and ErrUnsupportedVersion.
	Version(ctx context.Context) (WebAPIVersion, error)
	// AddTorrent adds a parsed .torrent or magnet in the configured category, started, whatever qBittorrent's
	// own defaults say. Magnets stop at StopMetadataReceived for their file check. Returns the info hash.
	AddTorrent(ctx context.Context, t AddRequest) (InfoHash string, err error)
	Start(ctx context.Context, hashes ...string) error
	Delete(ctx context.Context, deleteFiles bool, hashes ...string) error
	// Files returns the file list; empty until has_metadata.
	Files(ctx context.Context, hash string) ([]TorrentFile, error)
	// Sync returns incremental state since rid (sync/maindata), merged by the adapter into the full
	// state of Magpie's category. Pass 0 for a full update, then the returned RID.
	Sync(ctx context.Context, rid int64) (SyncState, error)
	// SetShareLimits always sends the full parameter set (ratio, seeding time, inactive time, action, mode).
	SetShareLimits(ctx context.Context, hash string, l ShareLimits) error
	// FreeSpace is the free bytes on qBittorrent's download disk (sync/maindata server_state.free_space_on_disk).
	FreeSpace(ctx context.Context) (int64, error)
}

type WebAPIVersion struct{ Major, Minor, Patch int }

// MinWebAPIVersion is qBittorrent 5.2's WebAPI version.
var MinWebAPIVersion = WebAPIVersion{Major: 2, Minor: 15}

type StopCondition string

const (
	StopNone             StopCondition = "NONE"
	StopMetadataReceived StopCondition = "METADATA_RECEIVED"
)

// AddRequest carries either Torrent or Magnet.
type AddRequest struct {
	Torrent       []byte
	Magnet        string
	Category      string
	SavePath      string // "" = qBittorrent's default for the category
	StopCondition StopCondition
}

type TorrentFile struct {
	Index    int
	Path     string // relative to the torrent's content root
	Size     int64
	Progress float64
	Priority int // 0 = not downloaded
}

// TorrentStatus is qBittorrent's torrent state.
type TorrentStatus string

const (
	TorrentError         TorrentStatus = "ERROR"
	TorrentMissingFiles  TorrentStatus = "MISSING_FILES"
	TorrentUploading     TorrentStatus = "UPLOADING"
	TorrentStalledUp     TorrentStatus = "STALLED_UP"
	TorrentStoppedUp     TorrentStatus = "STOPPED_UP"
	TorrentQueuedUp      TorrentStatus = "QUEUED_UP"
	TorrentForcedUp      TorrentStatus = "FORCED_UP"
	TorrentCheckingUp    TorrentStatus = "CHECKING_UP"
	TorrentAllocating    TorrentStatus = "ALLOCATING"
	TorrentDownloading   TorrentStatus = "DOWNLOADING"
	TorrentMetaDL        TorrentStatus = "META_DL"
	TorrentStoppedDL     TorrentStatus = "STOPPED_DL"
	TorrentQueuedDL      TorrentStatus = "QUEUED_DL"
	TorrentStalledDL     TorrentStatus = "STALLED_DL"
	TorrentForcedDL      TorrentStatus = "FORCED_DL"
	TorrentCheckingDL    TorrentStatus = "CHECKING_DL"
	TorrentCheckingData  TorrentStatus = "CHECKING_RESUME_DATA"
	TorrentMoving        TorrentStatus = "MOVING"
	TorrentUnknownStatus TorrentStatus = "UNKNOWN"
)

type Torrent struct {
	Hash         string
	Name         string
	Category     string
	Status       TorrentStatus
	HasMetadata  bool
	Progress     float64 // 0..1
	SizeBytes    int64
	DownloadBps  int64
	ETA          time.Duration
	Ratio        float64
	SeedingTime  time.Duration
	ContentPath  string // as qBittorrent reports it; Library applies the remote path mapping
	AddedAt      time.Time
	CompletedAt  *time.Time
	LastActivity time.Time
}

// SyncState is the merged view after one sync/maindata call.
type SyncState struct {
	RID      int64
	Torrents map[string]Torrent // by hash
}

type ShareLimitAction string

const (
	ShareStop              ShareLimitAction = "STOP"
	ShareRemove            ShareLimitAction = "REMOVE"
	ShareRemoveWithContent ShareLimitAction = "REMOVE_WITH_CONTENT"
	ShareSuperSeeding      ShareLimitAction = "ENABLE_SUPER_SEEDING"
)

type ShareLimitMode string

const (
	ShareLimitsGlobal    ShareLimitMode = "GLOBAL"
	ShareLimitsUnlimited ShareLimitMode = "UNLIMITED"
	ShareLimitsCustom    ShareLimitMode = "CUSTOM"
)

type ShareLimits struct {
	Ratio               float64
	SeedingTime         time.Duration
	InactiveSeedingTime time.Duration
	Action              ShareLimitAction
	Mode                ShareLimitMode
}

// TrashGuides is implemented by internal/adapters/trash. It reads TRaSH Guides' public JSON
// (docs/json/{radarr,sonarr}). Errors: ErrUnavailable.
type TrashGuides interface {
	CustomFormats(ctx context.Context, app TrashApp) ([]TrashCustomFormat, error)
	QualityProfiles(ctx context.Context, app TrashApp) ([]TrashQualityProfile, error)
	QualitySizes(ctx context.Context, app TrashApp) ([]TrashQualitySize, error)
}

type TrashApp string

const (
	TrashRadarr TrashApp = "RADARR"
	TrashSonarr TrashApp = "SONARR"
)

type TrashCustomFormat struct {
	TrashID             string
	Name                string
	Scores              map[string]int // trash_scores: score per profile context, e.g. "default"
	IncludeWhenRenaming bool
	Specs               []api.FormatSpec
}

type TrashProfileItem struct {
	Name      string   // quality or group name
	Qualities []string // group members; empty for a single quality
	Allowed   bool
}

type TrashQualityProfile struct {
	TrashID               string
	Name                  string
	ScoreSet              string // which trash_scores context the format scores come from; "default" when unset
	UpgradeAllowed        bool
	Cutoff                string // item name
	MinFormatScore        int
	CutoffFormatScore     int
	MinUpgradeFormatScore int
	Items                 []TrashProfileItem // most preferred first
	// Formats are the trash_ids of the formats it scores; each score is that format's Scores[ScoreSet].
	Formats []string
}

// TrashQualitySize is TRaSH's recommended MB per minute for one quality.
type TrashQualitySize struct {
	Quality string
	Sizes   api.SizeLimits
}

var (
	ErrUnauthorized       = errors.New("acquisition: unauthorized")
	ErrTimeout            = errors.New("acquisition: request timed out")
	ErrUnavailable        = errors.New("acquisition: service unavailable")
	ErrUnsupportedVersion = errors.New("qbittorrent: WebAPI older than 2.15 (qBittorrent 5.2)")
	ErrTorrentNotFound    = errors.New("qbittorrent: torrent not found")
)

// ErrIndexerFailed is a Torznab <error> answer from one indexer.
type ErrIndexerFailed struct {
	Code    int
	Message string
}

func (e *ErrIndexerFailed) Error() string {
	return fmt.Sprintf("indexer error %d: %s", e.Code, e.Message)
}
