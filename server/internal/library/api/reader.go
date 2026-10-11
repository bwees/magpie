// Package api is Library's public contract: the media-file Reader, the events Library publishes and the
// migration Importer. Spec: docs/spec/contexts/library.md.
package api

import (
	"context"
	"errors"
	"time"

	"magpie/internal/domain"
)

var ErrNotFound = errors.New("library: not found")

// Reader is read-only and backed by Library's repository. It never touches the disk or Jellyfin.
type Reader interface {
	// FilesForTitle returns the current media files of a title, newest first (by ImportedAt).
	// A title without files returns an empty slice, not ErrNotFound.
	FilesForTitle(ctx context.Context, title domain.TitleID) ([]MediaFile, error)
	// FileForEpisode returns the file covering an episode, or ErrNotFound.
	FileForEpisode(ctx context.Context, ep domain.EpisodeID) (MediaFile, error)
	// File returns one media file, or ErrNotFound.
	File(ctx context.Context, id domain.MediaFileID) (MediaFile, error)
	// SourceForProcessing gives Transcode a read-only absolute path, the current ETag,
	// and a staging directory on the same filesystem. Transcode serves these bytes to agents
	// itself and writes the uploaded result into StagingDir; it never touches Path.
	// ErrNotFound if the file is unknown.
	SourceForProcessing(ctx context.Context, id domain.MediaFileID) (ProcessingSource, error)
	// AvailableEpisodes returns the title's episodes whose files Jellyfin has confirmed.
	// A movie or a title without confirmed files returns an empty slice.
	AvailableEpisodes(ctx context.Context, title domain.TitleID) ([]domain.EpisodeID, error)
	// Coverage returns per-title file coverage for the TV page, one entry per requested title in input
	// order: episodes with a file and episodes Jellyfin confirmed. Titles without files return zero
	// counts, not ErrNotFound.
	Coverage(ctx context.Context, titles []domain.TitleID) ([]TitleCoverage, error)
}

type MediaFile struct {
	ID          domain.MediaFileID
	Title       domain.TitleID
	Episodes    []domain.EpisodeRef
	RootFolder  string
	Path        string
	Size        int64
	Quality     domain.Quality
	ReleaseName string    // original grabbed release; empty for scanned files with no name
	OSHash      string    // OpenSubtitles hash of the original file
	MediaInfo   MediaInfo // streams, duration, HDR format, bitrate
	Origin      domain.Origin
	Download    *domain.DownloadID // source download, nil for scanned files
	InfoHash    string             // source torrent hash; "" if unknown (scan, some migrations)
	Transcoded  bool
	ImportedAt  time.Time
	Available   bool
}

type TitleCoverage struct {
	Title             domain.TitleID
	EpisodesWithFile  int
	EpisodesAvailable int // confirmed in Jellyfin
	SizeBytes         int64
}

type ProcessingSource struct {
	Path       string // absolute, read-only for the caller
	ETag       string
	Size       int64
	StagingDir string // same filesystem as Path; caller may create files here only
	MediaInfo  MediaInfo
}

type StreamKind string

const (
	StreamVideo    StreamKind = "VIDEO"
	StreamAudio    StreamKind = "AUDIO"
	StreamSubtitle StreamKind = "SUBTITLE"
)

type HDRFormat string

const (
	HDRNone        HDRFormat = "NONE"
	HDR10          HDRFormat = "HDR10"
	HDR10Plus      HDRFormat = "HDR10_PLUS"
	HDRDolbyVision HDRFormat = "DOLBY_VISION"
	HDRHLG         HDRFormat = "HLG"
)

// MediaInfo is what ffprobe reported for a file.
type MediaInfo struct {
	Container   string        `json:"container"`
	Duration    time.Duration `json:"duration"`
	BitrateKbps int           `json:"bitrateKbps"`
	Width       int           `json:"width"`
	Height      int           `json:"height"`
	HDR         HDRFormat     `json:"hdr"`
	Streams     []Stream      `json:"streams"`
}

type Stream struct {
	Index       int             `json:"index"` // source stream index, as ffprobe numbers it
	Kind        StreamKind      `json:"kind"`
	Codec       string          `json:"codec"`
	Language    domain.Language `json:"language"` // "und" when untagged
	Forced      bool            `json:"forced"`
	Default     bool            `json:"default"`
	Image       bool            `json:"image"` // image subtitle (PGS/VobSub); false for text
	Channels    int             `json:"channels"`
	BitDepth    int             `json:"bitDepth"`
	BitrateKbps int             `json:"bitrateKbps"`
}
