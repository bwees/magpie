package api

import (
	"time"

	"magpie/internal/domain"
)

const (
	TypeSearchStarted         = "acquisition.SearchStarted"
	TypeSearchResultsReceived = "acquisition.SearchResultsReceived"
	TypeSearchCompleted       = "acquisition.SearchCompleted"
	TypeReleaseGrabbed        = "acquisition.ReleaseGrabbed"
	TypeReleaseRejected       = "acquisition.ReleaseRejected"
	TypeDownloadProgressed    = "acquisition.DownloadProgressed"
	TypeDownloadCompleted     = "acquisition.DownloadCompleted"
	TypeDownloadFailed        = "acquisition.DownloadFailed"
	TypeCutoffMet             = "acquisition.CutoffMet"
)

type SearchTrigger string

const (
	TriggerRSS         SearchTrigger = "RSS"
	TriggerAuto        SearchTrigger = "AUTO"
	TriggerUserAuto    SearchTrigger = "USER_AUTO"
	TriggerInteractive SearchTrigger = "INTERACTIVE"
	TriggerRegrab      SearchTrigger = "REGRAB"
)

type IndexerRunStatus string

const (
	IndexerDone    IndexerRunStatus = "DONE"
	IndexerFailed  IndexerRunStatus = "FAILED"
	IndexerSkipped IndexerRunStatus = "SKIPPED" // no ID support and the title search found nothing
)

type SearchOutcome string

const (
	OutcomeGrabbed         SearchOutcome = "GRABBED"
	OutcomeNothingAccepted SearchOutcome = "NOTHING_ACCEPTED"
	OutcomePendingDelay    SearchOutcome = "PENDING_DELAY"
	// OutcomeReleasesFound: an interactive search found accepted releases and leaves the pick to the user.
	OutcomeReleasesFound SearchOutcome = "RELEASES_FOUND"
)

type DownloadState string

const (
	StateAwaitingMetadata DownloadState = "AWAITING_METADATA"
	StateDownloading      DownloadState = "DOWNLOADING"
	StateStalled          DownloadState = "STALLED"
	StateCompleted        DownloadState = "COMPLETED"
	StateSeeding          DownloadState = "SEEDING"
	StateImported         DownloadState = "IMPORTED"
	StateImportFailed     DownloadState = "IMPORT_FAILED"
	StateFailed           DownloadState = "FAILED"
	StateRemoved          DownloadState = "REMOVED"
)

type SeedingMode string

const (
	SeedUntilGoals SeedingMode = "SEED_UNTIL_GOALS" // Library hardlinks (copy fallback)
	DontSeed       SeedingMode = "DONT_SEED"        // Library moves
)

// RejectionReason names the decision-engine spec that rejected a release, or why a grabbed release was dropped.
type RejectionReason string

const (
	RejectUnknownTitle         RejectionReason = "UNKNOWN_TITLE"
	RejectUnknownEpisode       RejectionReason = "UNKNOWN_EPISODE"
	RejectUnableToParse        RejectionReason = "UNABLE_TO_PARSE"
	RejectQualityNotAllowed    RejectionReason = "QUALITY_NOT_ALLOWED"
	RejectFormatScoreTooLow    RejectionReason = "FORMAT_SCORE_TOO_LOW"
	RejectUnknownRuntime       RejectionReason = "UNKNOWN_RUNTIME"
	RejectSizeTooSmall         RejectionReason = "SIZE_TOO_SMALL"
	RejectSizeTooLarge         RejectionReason = "SIZE_TOO_LARGE"
	RejectSample               RejectionReason = "SAMPLE"
	RejectRawDisk              RejectionReason = "RAW_DISK"
	RejectSplitEpisode         RejectionReason = "SPLIT_EPISODE"
	RejectBlocklisted          RejectionReason = "BLOCKLISTED"
	RejectAlreadyImported      RejectionReason = "ALREADY_IMPORTED"
	RejectNotEnoughSeeders     RejectionReason = "NOT_ENOUGH_SEEDERS"
	RejectIndexerBackoff       RejectionReason = "INDEXER_BACKOFF"
	RejectUpgradeNotAllowed    RejectionReason = "UPGRADE_NOT_ALLOWED"
	RejectNotAnUpgrade         RejectionReason = "NOT_AN_UPGRADE"
	RejectRepackDifferentGroup RejectionReason = "REPACK_DIFFERENT_GROUP"
	RejectProperNotWanted      RejectionReason = "PROPER_NOT_WANTED"
	RejectSameEpisodesGrabbed  RejectionReason = "SAME_EPISODES_GRABBED"
	RejectAlreadyQueued        RejectionReason = "ALREADY_QUEUED"
	RejectInHistory            RejectionReason = "IN_HISTORY"
	RejectSeasonNotFullyAired  RejectionReason = "SEASON_NOT_FULLY_AIRED"
	RejectSeasonNotWanted      RejectionReason = "SEASON_NOT_WANTED"
	RejectSeasonPackOnly       RejectionReason = "SEASON_PACK_ONLY"
	RejectNotMonitored         RejectionReason = "NOT_MONITORED"
	RejectSearchMismatch       RejectionReason = "SEARCH_MISMATCH"
	RejectNotReleased          RejectionReason = "NOT_RELEASED"
	RejectDelayed              RejectionReason = "DELAYED"
	RejectPending              RejectionReason = "PENDING"
	RejectNotEnoughSpace       RejectionReason = "NOT_ENOUGH_SPACE"
	RejectBlockedFile          RejectionReason = "BLOCKED_FILE"
	RejectMetadataTimeout      RejectionReason = "METADATA_TIMEOUT"
	RejectStalled              RejectionReason = "STALLED"
	RejectImportFailed         RejectionReason = "IMPORT_FAILED"
	RejectDownloadFailed       RejectionReason = "DOWNLOAD_FAILED"
)

type SearchStarted struct {
	Search      domain.SearchID    `json:"search"`
	Title       domain.TitleID     `json:"title"`
	Episodes    []domain.EpisodeID `json:"episodes"` // empty for movies
	Trigger     SearchTrigger      `json:"trigger"`
	Interactive bool               `json:"interactive"`
	Indexers    []string           `json:"indexers"` // names queried
}

func (SearchStarted) Type() string          { return TypeSearchStarted }
func (e SearchStarted) AggregateID() string { return string(e.Title) }
func (SearchStarted) Durable() bool         { return true }

// SearchResultsReceived is ephemeral: one per indexer answer (or failure). Never persisted; drives the
// live search dialog.
type SearchResultsReceived struct {
	Search   domain.SearchID   `json:"search"`
	Indexer  string            `json:"indexer"`
	Status   IndexerRunStatus  `json:"status"`
	Message  string            `json:"message"` // failure reason for FAILED/SKIPPED
	Releases []ReleaseDecision `json:"releases"`
}

func (SearchResultsReceived) Type() string          { return TypeSearchResultsReceived }
func (e SearchResultsReceived) AggregateID() string { return string(e.Search) }
func (SearchResultsReceived) Durable() bool         { return false }

type ReleaseDecision struct {
	Release     domain.ReleaseID `json:"release"` // stable per (search, indexer, guid)
	Name        string           `json:"name"`
	Indexer     string           `json:"indexer"`
	InfoHash    string           `json:"infoHash"` // may be empty until grab
	Quality     domain.Quality   `json:"quality"`
	Formats     []string         `json:"formats"` // matched custom format names
	CFScore     int              `json:"cfScore"`
	SizeBytes   int64            `json:"sizeBytes"`
	Seeders     int              `json:"seeders"`
	PublishedAt time.Time        `json:"publishedAt"`
	Freeleech   bool             `json:"freeleech"`
	Rejections  []Rejection      `json:"rejections"` // empty = accepted
	Rank        int              `json:"rank"`       // position among accepted releases at time of emit
}

type Rejection struct {
	Reason    RejectionReason `json:"reason"`
	Message   string          `json:"message"`
	Temporary bool            `json:"temporary"`
}

type SearchCompleted struct {
	Search         domain.SearchID `json:"search"`
	Title          domain.TitleID  `json:"title"`
	Accepted       int             `json:"accepted"`
	Total          int             `json:"total"`
	FailedIndexers []string        `json:"failedIndexers"`
	// Outcome for interactive searches is GRABBED after a user grab, else RELEASES_FOUND or NOTHING_ACCEPTED.
	Outcome SearchOutcome      `json:"outcome"`
	Grabbed *domain.DownloadID `json:"grabbed"` // set when Outcome = GRABBED
}

func (SearchCompleted) Type() string          { return TypeSearchCompleted }
func (e SearchCompleted) AggregateID() string { return string(e.Title) }
func (SearchCompleted) Durable() bool         { return true }

type ReleaseGrabbed struct {
	Download domain.DownloadID  `json:"download"`
	Title    domain.TitleID     `json:"title"`
	Episodes []domain.EpisodeID `json:"episodes"`
	Release  string             `json:"release"`
	Indexer  string             `json:"indexer"`
	InfoHash string             `json:"infoHash"`
	Quality  domain.Quality     `json:"quality"`
	CFScore  int                `json:"cfScore"`
	Trigger  SearchTrigger      `json:"trigger"`
	Override bool               `json:"override"` // user grabbed a rejected release
}

func (ReleaseGrabbed) Type() string          { return TypeReleaseGrabbed }
func (e ReleaseGrabbed) AggregateID() string { return string(e.Title) }
func (ReleaseGrabbed) Durable() bool         { return true }

type ReleaseRejected struct {
	Title    domain.TitleID `json:"title"`
	Release  string         `json:"release"`
	InfoHash string         `json:"infoHash"`
	Indexer  string         `json:"indexer"`
	// Reason is BLOCKED_FILE, METADATA_TIMEOUT, STALLED, IMPORT_FAILED or DOWNLOAD_FAILED.
	Reason      RejectionReason    `json:"reason"`
	BlockedFile *string            `json:"blockedFile"` // matched file path inside the torrent
	Next        *domain.DownloadID `json:"next"`        // the fallback grab, if one was made
}

func (ReleaseRejected) Type() string          { return TypeReleaseRejected }
func (e ReleaseRejected) AggregateID() string { return string(e.Title) }
func (ReleaseRejected) Durable() bool         { return true }

// DownloadProgressed is ephemeral: emitted on each qBittorrent poll that changes progress or state.
type DownloadProgressed struct {
	Download domain.DownloadID `json:"download"`
	State    DownloadState     `json:"state"` // AWAITING_METADATA, DOWNLOADING, STALLED, COMPLETED, SEEDING
	Progress float64           `json:"progress"`
	SpeedBps int64             `json:"speedBps"`
	ETA      time.Duration     `json:"eta"`
	Ratio    float64           `json:"ratio"`
}

func (DownloadProgressed) Type() string          { return TypeDownloadProgressed }
func (e DownloadProgressed) AggregateID() string { return string(e.Download) }
func (DownloadProgressed) Durable() bool         { return false }

// DownloadFile is one file of a finished torrent, relative to ContentPath.
type DownloadFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// DownloadCompleted is published once, only after the blocked-file backstop passes.
type DownloadCompleted struct {
	Download    domain.DownloadID  `json:"download"`
	Title       domain.TitleID     `json:"title"`
	Episodes    []domain.EpisodeID `json:"episodes"`
	ContentPath string             `json:"contentPath"` // as qBittorrent reports it (remote path mapping already applied)
	Files       []DownloadFile     `json:"files"`       // only files with priority > 0
	ReleaseName string             `json:"releaseName"`
	Quality     domain.Quality     `json:"quality"`
	CFScore     int                `json:"cfScore"`
	SeedingMode SeedingMode        `json:"seedingMode"`
}

func (DownloadCompleted) Type() string          { return TypeDownloadCompleted }
func (e DownloadCompleted) AggregateID() string { return string(e.Title) }
func (DownloadCompleted) Durable() bool         { return true }

type DownloadFailed struct {
	Download domain.DownloadID `json:"download"`
	Title    domain.TitleID    `json:"title"`
	Reason   string            `json:"reason"` // qBittorrent error/missingFiles, stalled past limit, etc.
}

func (DownloadFailed) Type() string          { return TypeDownloadFailed }
func (e DownloadFailed) AggregateID() string { return string(e.Title) }
func (DownloadFailed) Durable() bool         { return true }

type CutoffMet struct {
	File     domain.MediaFileID `json:"file"`
	Title    domain.TitleID     `json:"title"`
	Episodes []domain.EpisodeID `json:"episodes"`
	Quality  domain.Quality     `json:"quality"`
	CFScore  int                `json:"cfScore"`
	Origin   domain.Origin      `json:"origin"` // copied from FileImported: DOWNLOAD | LIBRARY_SCAN | MIGRATION
}

func (CutoffMet) Type() string          { return TypeCutoffMet }
func (e CutoffMet) AggregateID() string { return string(e.Title) }
func (CutoffMet) Durable() bool         { return true }

// Messages lists every event and command this package declares, for the bus registry and the live schemas.
func Messages() []domain.Message {
	return []domain.Message{
		SearchStarted{}, SearchResultsReceived{}, SearchCompleted{},
		ReleaseGrabbed{}, ReleaseRejected{}, DownloadProgressed{},
		DownloadCompleted{}, DownloadFailed{}, CutoffMet{},
	}
}
