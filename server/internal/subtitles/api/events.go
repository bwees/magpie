package api

import "magpie/internal/domain"

const (
	TypeSubtitleAdded   = "subtitles.SubtitleAdded"
	TypeSubtitleMissing = "subtitles.SubtitleMissing"
	TypeCoverageSettled = "subtitles.CoverageSettled"
)

// MissingReason says why a wanted language ended MISSING.
type MissingReason string

const (
	MissingNoMatch             MissingReason = "NO_MATCH"
	MissingQuotaExhausted      MissingReason = "QUOTA_EXHAUSTED"
	MissingWhisperFailed       MissingReason = "WHISPER_FAILED"
	MissingNotOriginalLanguage MissingReason = "NOT_ORIGINAL_LANGUAGE"
)

// SubtitleAdded is durable. A subtitle was stored for a file.
type SubtitleAdded struct {
	MediaFile domain.MediaFileID `json:"mediaFile"`
	Language  domain.Language    `json:"language"`
	Forced    bool               `json:"forced"`
	SDH       bool               `json:"sdh"`
	Source    Source             `json:"source"`
	Synced    bool               `json:"synced"`
	// Replaces is true when an upgrade replaced an earlier provider subtitle.
	Replaces bool `json:"replaces"`
}

// SubtitleMissing is durable. Published once per (file, language), when the language moves to MISSING.
type SubtitleMissing struct {
	MediaFile domain.MediaFileID `json:"mediaFile"`
	Language  domain.Language    `json:"language"`
	Forced    bool               `json:"forced"`
	Reason    MissingReason      `json:"reason"`
}

// CoverageSettled is durable. No wanted language for the file is PENDING, SEARCHING or WAITING_WHISPER any more.
// Drives the "Subs" chip on the title page.
type CoverageSettled struct {
	MediaFile domain.MediaFileID `json:"mediaFile"`
	Title     domain.TitleID     `json:"title"`
	Satisfied []domain.Language  `json:"satisfied"`
	Missing   []domain.Language  `json:"missing"`
}

func (SubtitleAdded) Type() string          { return TypeSubtitleAdded }
func (e SubtitleAdded) AggregateID() string { return string(e.MediaFile) }
func (SubtitleAdded) Durable() bool         { return true }

func (SubtitleMissing) Type() string          { return TypeSubtitleMissing }
func (e SubtitleMissing) AggregateID() string { return string(e.MediaFile) }
func (SubtitleMissing) Durable() bool         { return true }

func (CoverageSettled) Type() string          { return TypeCoverageSettled }
func (e CoverageSettled) AggregateID() string { return string(e.MediaFile) }
func (CoverageSettled) Durable() bool         { return true }

// Messages lists every event and command this package declares, for the bus registry and the live schemas.
func Messages() []domain.Message {
	return []domain.Message{SubtitleAdded{}, SubtitleMissing{}, CoverageSettled{}}
}
