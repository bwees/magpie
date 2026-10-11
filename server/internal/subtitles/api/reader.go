// Package api is Subtitles' public contract: the subtitle Reader and the events Subtitles publishes.
// Spec: docs/spec/contexts/subtitles.md.
package api

import (
	"context"
	"time"

	"magpie/internal/domain"
)

// Source says where a subtitle came from.
type Source string

const (
	SourceEmbedded Source = "EMBEDDED"
	SourceRelease  Source = "RELEASE"
	SourceProvider Source = "PROVIDER"
	SourceWhisper  Source = "WHISPER"
)

// CoverageStatus is a wanted language's state for one file. Searching and waiting for Whisper read as pending.
type CoverageStatus string

const (
	CoverageSatisfied CoverageStatus = "SATISFIED"
	CoveragePending   CoverageStatus = "PENDING"
	CoverageMissing   CoverageStatus = "MISSING"
)

// Reader is read-only and backed by Subtitles' repository.
type Reader interface {
	// ForFile returns every subtitle known for a media file, embedded tracks included, ordered by
	// language, then Forced, then SDH. An unknown file returns an empty slice, no error.
	ForFile(ctx context.Context, file domain.MediaFileID) ([]Subtitle, error)
	// Coverage returns per wanted language whether it's satisfied, pending or missing, for the title page,
	// in wanted-language order. An unknown file returns an empty slice, no error.
	Coverage(ctx context.Context, file domain.MediaFileID) ([]LanguageCoverage, error)
	// Quota returns OpenSubtitles downloads left today and when they reset.
	Quota(ctx context.Context) (Quota, error)
}

type Subtitle struct {
	Language domain.Language
	Forced   bool
	SDH      bool
	Source   Source
	Score    *int // percent of max, PROVIDER only
	Synced   bool
	Path     string // empty for EMBEDDED
}

type LanguageCoverage struct {
	Language domain.Language
	Forced   bool
	Status   CoverageStatus
	Source   *Source // set when Status is CoverageSatisfied
}

type Quota struct {
	Remaining int
	ResetAt   time.Time
}
