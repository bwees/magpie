package api

import "magpie/internal/domain"

const (
	TypeFileImported          = "library.FileImported"
	TypeImportFailed          = "library.ImportFailed"
	TypeImportRemoved         = "library.ImportRemoved"
	TypeFileTranscoded        = "library.FileTranscoded"
	TypeFileDeleted           = "library.FileDeleted"
	TypeFileRenamed           = "library.FileRenamed"
	TypeItemAvailable         = "library.ItemAvailable"
	TypeFolderFound           = "library.FolderFound"
	TypeTranscodeSwapRejected = "library.TranscodeSwapRejected"
)

type DeleteReason string

const (
	DeleteUpgraded     DeleteReason = "UPGRADED"
	DeleteRemoved      DeleteReason = "REMOVED"
	DeleteTitleRemoved DeleteReason = "TITLE_REMOVED"
)

type SwapRejectReason string

const (
	SwapSourceChanged SwapRejectReason = "SOURCE_CHANGED"
	SwapHashMismatch  SwapRejectReason = "HASH_MISMATCH"
	SwapProbeFailed   SwapRejectReason = "PROBE_FAILED"
	SwapFileGone      SwapRejectReason = "FILE_GONE"
)

// Sidecar is a subtitle shipped in the release, already placed next to the video.
type Sidecar struct {
	Path     string          `json:"path"`
	Language domain.Language `json:"language"`
	Forced   bool            `json:"forced"`
	SDH      bool            `json:"sdh"`
}

// FileImported is durable and published in the same transaction that inserts the media file row.
type FileImported struct {
	MediaFile   domain.MediaFileID  `json:"mediaFile"`
	Title       domain.TitleID      `json:"title"`
	Episodes    []domain.EpisodeRef `json:"episodes"`
	Path        string              `json:"path"`
	Quality     domain.Quality      `json:"quality"`
	ReleaseName string              `json:"releaseName"`
	OSHash      string              `json:"osHash"`
	Size        int64               `json:"size"`
	MediaInfo   MediaInfo           `json:"mediaInfo"`
	// Replaces is informational: the file this upgrade replaced. Cleanup is driven by FileDeleted{UPGRADED}.
	Replaces *domain.MediaFileID `json:"replaces"`
	Download *domain.DownloadID  `json:"download"` // nil for scanned/migrated files
	Sidecars []Sidecar           `json:"sidecars"`
	Origin   domain.Origin       `json:"origin"` // DOWNLOAD | LIBRARY_SCAN | MIGRATION
}

func (FileImported) Type() string          { return TypeFileImported }
func (e FileImported) AggregateID() string { return string(e.Title) }
func (FileImported) Durable() bool         { return true }

type ImportFailed struct {
	Download  domain.DownloadID `json:"download"`
	Reason    string            `json:"reason"`
	Retryable bool              `json:"retryable"`
	// Manual is set when the import waits for the user to map its files or remove it, so the download
	// must keep its data.
	Manual bool `json:"manual"`
}

func (ImportFailed) Type() string          { return TypeImportFailed }
func (e ImportFailed) AggregateID() string { return string(e.Download) }
func (ImportFailed) Durable() bool         { return true }

// ImportRemoved is published when the user removes a failed or manual import (Remove & blocklist).
type ImportRemoved struct {
	Download    domain.DownloadID `json:"download"`
	Title       domain.TitleID    `json:"title"`
	ReleaseName string            `json:"releaseName"`
}

func (ImportRemoved) Type() string          { return TypeImportRemoved }
func (e ImportRemoved) AggregateID() string { return string(e.Download) }
func (ImportRemoved) Durable() bool         { return true }

type FileTranscoded struct {
	MediaFile domain.MediaFileID `json:"mediaFile"`
	Path      string             `json:"path"`
	OldSize   int64              `json:"oldSize"`
	NewSize   int64              `json:"newSize"`
}

func (FileTranscoded) Type() string          { return TypeFileTranscoded }
func (e FileTranscoded) AggregateID() string { return string(e.MediaFile) }
func (FileTranscoded) Durable() bool         { return true }

// FileDeleted is the only cleanup signal for a removed file, whatever the reason.
type FileDeleted struct {
	MediaFile domain.MediaFileID  `json:"mediaFile"`
	Title     domain.TitleID      `json:"title"`
	Episodes  []domain.EpisodeRef `json:"episodes"`
	Path      string              `json:"path"`
	Reason    DeleteReason        `json:"reason"`
}

func (FileDeleted) Type() string          { return TypeFileDeleted }
func (e FileDeleted) AggregateID() string { return string(e.Title) }
func (FileDeleted) Durable() bool         { return true }

// SidecarMove is one file that moved with a renamed video because it shares the video's basename.
type SidecarMove struct {
	OldPath string `json:"oldPath"`
	NewPath string `json:"newPath"`
}

// FileRenamed is published in the same transaction that records a media file's new path.
type FileRenamed struct {
	MediaFile domain.MediaFileID  `json:"mediaFile"`
	Title     domain.TitleID      `json:"title"`
	Episodes  []domain.EpisodeRef `json:"episodes"` // the numbers after the rename
	OldPath   string              `json:"oldPath"`
	NewPath   string              `json:"newPath"`
	Sidecars  []SidecarMove       `json:"sidecars"`
}

func (FileRenamed) Type() string          { return TypeFileRenamed }
func (e FileRenamed) AggregateID() string { return string(e.Title) }
func (FileRenamed) Durable() bool         { return true }

type ItemAvailable struct {
	Title        domain.TitleID     `json:"title"`
	Episodes     []domain.EpisodeID `json:"episodes"`
	JellyfinItem string             `json:"jellyfinItem"`
}

func (ItemAvailable) Type() string          { return TypeItemAvailable }
func (e ItemAvailable) AggregateID() string { return string(e.Title) }
func (ItemAvailable) Durable() bool         { return true }

type FolderFound struct {
	Path        string           `json:"path"`
	Kind        domain.MediaKind `json:"kind"`
	ParsedTitle string           `json:"parsedTitle"`
	Year        *int             `json:"year"`
	TmdbHint    *domain.TmdbID   `json:"tmdbHint"`
}

func (FolderFound) Type() string          { return TypeFolderFound }
func (e FolderFound) AggregateID() string { return e.Path }
func (FolderFound) Durable() bool         { return true }

type TranscodeSwapRejected struct {
	Job       domain.JobID       `json:"job"`
	MediaFile domain.MediaFileID `json:"mediaFile"`
	Reason    SwapRejectReason   `json:"reason"`
}

func (TranscodeSwapRejected) Type() string          { return TypeTranscodeSwapRejected }
func (e TranscodeSwapRejected) AggregateID() string { return string(e.MediaFile) }
func (TranscodeSwapRejected) Durable() bool         { return true }

// Messages lists every event and command this package declares, for the bus registry and the live schemas.
func Messages() []domain.Message {
	return []domain.Message{
		FileImported{}, ImportFailed{}, ImportRemoved{}, FileTranscoded{}, FileDeleted{}, FileRenamed{},
		ItemAvailable{}, FolderFound{}, TranscodeSwapRejected{},
	}
}
