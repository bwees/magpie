package api

import (
	"context"
	"time"

	"magpie/internal/domain"
)

// CustomFormatID is a ULID.
type CustomFormatID string

// Importer is the migration entrypoint (platform migration job only). Each call is idempotent on the
// source keys and runs in one Acquisition transaction. Spec: docs/spec/11-migration.md.
type Importer interface {
	// ImportCustomFormats runs first, since profiles reference formats. Same name and same specs → one format.
	ImportCustomFormats(ctx context.Context, in []ImportCustomFormat) (map[domain.SourceKey]CustomFormatID, error)
	// ImportQualityProfiles adds profiles to the one pool shared by movies and TV.
	ImportQualityProfiles(ctx context.Context, in []ImportQualityProfile) (map[domain.SourceKey]domain.ProfileID, error)
	ImportDelayProfiles(ctx context.Context, in []ImportDelayProfile) error
	// ImportBlocklist stores rows as permanent MIGRATED entries. Rows without a hash match by title + indexer.
	ImportBlocklist(ctx context.Context, in []ImportBlockedRelease) error
}

type FormatSpecKind string

const (
	SpecReleaseTitle    FormatSpecKind = "RELEASE_TITLE"
	SpecReleaseGroup    FormatSpecKind = "RELEASE_GROUP"
	SpecEdition         FormatSpecKind = "EDITION"
	SpecSource          FormatSpecKind = "SOURCE"
	SpecResolution      FormatSpecKind = "RESOLUTION"
	SpecQualityModifier FormatSpecKind = "QUALITY_MODIFIER"
	SpecLanguage        FormatSpecKind = "LANGUAGE"
	SpecIndexerFlag     FormatSpecKind = "INDEXER_FLAG"
	SpecReleaseType     FormatSpecKind = "RELEASE_TYPE"
	SpecSize            FormatSpecKind = "SIZE"
	SpecYear            FormatSpecKind = "YEAR"
)

// FormatSpec is one custom-format condition, in the Sonarr/Radarr shape TRaSH also uses.
type FormatSpec struct {
	Kind     FormatSpecKind
	Name     string
	Negate   bool
	Required bool
	// Value is a regex for title/group/edition specs, else the enum name of a source, resolution,
	// modifier, language, flag or release type.
	Value string
	// Min and Max bound SIZE (GB) and YEAR specs.
	Min, Max float64
}

type ImportCustomFormat struct {
	Source              domain.SourceKey
	Name                string
	IncludeWhenRenaming bool
	Specs               []FormatSpec
}

// ImportProfileItem is one quality, or a group of equally ranked qualities when Group is set.
type ImportProfileItem struct {
	Group     string           // "" for a single quality
	Qualities []domain.Quality // Magpie (Radarr) quality IDs
	Allowed   bool
	Sizes     SizeLimits // the source app's global size table, copied in
}

type ImportQualityProfile struct {
	Source         domain.SourceKey
	Name           string
	UpgradeAllowed bool
	// Items are in preference order, most preferred first.
	Items []ImportProfileItem
	// Cutoff is the index into Items of the cutoff quality or group.
	Cutoff                int
	MinFormatScore        int
	CutoffFormatScore     int
	MinUpgradeFormatScore int
	FormatScores          map[CustomFormatID]int // IDs returned by ImportCustomFormats
}

type ImportDelayProfile struct {
	Source domain.SourceKey
	// Delay is the torrent delay; Usenet fields are dropped.
	Delay                    time.Duration
	BypassIfHighestQuality   bool
	BypassIfAboveFormatScore *int
	Tags                     []string
}

type ImportBlockedRelease struct {
	Source      domain.SourceKey
	Title       domain.TitleID
	InfoHash    string // recovered from the grabbed history row; "" when none matched
	ReleaseName string
	Indexer     string // " (Prowlarr)" suffix stripped
	At          time.Time
}
