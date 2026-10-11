// Package api is Acquisition's public contract: the Reader for its own HTTP layer, the events Acquisition
// publishes and the migration Importer. Spec: docs/spec/contexts/acquisition.md.
package api

import (
	"context"
	"errors"
	"time"

	"magpie/internal/domain"
)

var ErrNotFound = errors.New("acquisition: not found")

// Reader is read-only and backed by Acquisition's repository. Requests is the only other context that
// depends on it, for the default profile of new requests (see the context map). The UI's profile pickers
// call GET /api/acquisition/profiles directly.
type Reader interface {
	// Profile returns a quality profile with items, groups, sizes and format scores.
	// Returns ErrNotFound for unknown IDs.
	Profile(ctx context.Context, id domain.ProfileID) (Profile, error)
	// Profiles lists all profiles, ordered by name.
	Profiles(ctx context.Context) ([]ProfileSummary, error)
	// DefaultProfile returns the default profile for new requests ("" before the first start seeds it).
	DefaultProfile(ctx context.Context) (domain.ProfileID, error)
	// ActiveDownloads lists downloads not yet removed from qBittorrent, newest first (by GrabbedAt).
	ActiveDownloads(ctx context.Context) ([]Download, error)
	// DownloadsForTitle lists every download for a title, including removed ones, newest first.
	DownloadsForTitle(ctx context.Context, title domain.TitleID) ([]Download, error)
	// Blocklist lists entries for a title (or all when title is empty), newest first, expired
	// temporary entries included.
	Blocklist(ctx context.Context, title domain.TitleID) ([]BlocklistEntry, error)
	// CutoffStatus reports whether a library file meets its profile cutoff and whether upgrades are still allowed.
	// Returns ErrNotFound for a file Acquisition has no score for.
	CutoffStatus(ctx context.Context, file domain.MediaFileID) (CutoffStatus, error)
}

type CutoffStatus struct {
	File            domain.MediaFileID
	Quality         domain.Quality
	CFScore         int
	CutoffMet       bool // quality ≥ cutoff AND CF score ≥ cutoff_format_score
	UpgradesAllowed bool // false once transcoded, or profile.upgrade_allowed == false
}

type PropersMode string

const (
	PropersPreferAndUpgrade PropersMode = "PREFER_AND_UPGRADE"
	PropersDoNotUpgrade     PropersMode = "DO_NOT_UPGRADE"
	PropersDoNotPrefer      PropersMode = "DO_NOT_PREFER"
)

// QualityPreset names a first-run resolution preset: one group holding every source at that resolution.
type QualityPreset string

const (
	Preset720p  QualityPreset = "720P"
	Preset1080p QualityPreset = "1080P"
	Preset2160p QualityPreset = "2160P"
)

type Profile struct {
	ID             domain.ProfileID
	Name           string
	UpgradeAllowed bool
	// Preset is set for a profile built from a resolution preset; TRaSH sync keeps its formats attached.
	Preset QualityPreset
	// Items are in preference order, most preferred first. Members of a group rank equally.
	Items []ProfileItem
	// Cutoff is the index into Items of the cutoff quality or group.
	Cutoff                int
	MinFormatScore        int
	CutoffFormatScore     int // "upgrade until"
	MinUpgradeFormatScore int // minimum CF score increment for an upgrade
	PropersMode           PropersMode
	FormatScores          []FormatScore
	TrashProfileID        string // "" when not linked to a TRaSH guide profile
	// TrashScoreSet picks which of a format's TRaSH scores apply; "" means TRaSH's "default" set.
	TrashScoreSet string
}

// ProfileItem is one quality or one group of equally ranked qualities.
type ProfileItem struct {
	Group     *QualityGroup    // nil for a single quality
	Qualities []domain.Quality // one entry for a single quality, the members for a group
	Allowed   bool
	Sizes     SizeLimits
}

type QualityGroup struct {
	ID   int // ≥ 1000, so it never clashes with a quality ID
	Name string
}

// SizeLimits are MB per minute of runtime.
type SizeLimits struct {
	MinMBPerMin, PreferredMBPerMin, MaxMBPerMin float64
}

type FormatScore struct {
	Format       CustomFormatID
	Name         string
	Score        int
	UserOverride bool // TRaSH sync never overwrites it
}

type ProfileSummary struct {
	ID             domain.ProfileID
	Name           string
	UpgradeAllowed bool
	Preset         QualityPreset
	TrashProfileID string
}

type Download struct {
	ID          domain.DownloadID
	Title       domain.TitleID
	Episodes    []domain.EpisodeID
	Search      *domain.SearchID
	ReleaseName string
	Indexer     string
	InfoHash    string
	Quality     domain.Quality
	CFScore     int
	State       DownloadState
	Progress    float64 // 0..1, last polled value
	SeedingMode SeedingMode
	GrabbedAt   time.Time
	CompletedAt *time.Time
	RemovedAt   *time.Time // set once the torrent left qBittorrent
}

type BlocklistReason string

const (
	BlocklistBlockedFile     BlocklistReason = "BLOCKED_FILE"
	BlocklistMetadataTimeout BlocklistReason = "METADATA_TIMEOUT"
	BlocklistStalled         BlocklistReason = "STALLED"
	BlocklistImportFailed    BlocklistReason = "IMPORT_FAILED"
	BlocklistDownloadFailed  BlocklistReason = "DOWNLOAD_FAILED"
	BlocklistUser            BlocklistReason = "USER"
	BlocklistIssue           BlocklistReason = "ISSUE"
	BlocklistMigrated        BlocklistReason = "MIGRATED"
)

type BlocklistEntry struct {
	ID          string
	Title       domain.TitleID
	InfoHash    string // "" when blocked by release name + indexer
	ReleaseName string
	Indexer     string
	Reason      BlocklistReason
	At          time.Time
	ExpiresAt   *time.Time // nil = permanent
}
