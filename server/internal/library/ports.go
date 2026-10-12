package library

import (
	"context"
	"errors"
	"io/fs"
	"time"

	"magpie/internal/library/api"
)

// FileSystem wraps all disk mutation so tests can use an in-memory fake.
// Missing paths return errors matching fs.ErrNotExist.
type FileSystem interface {
	Stat(path string) (FileInfo, error)
	SameDevice(a, b string) (bool, error)
	Hardlink(src, dst string) error
	Copy(ctx context.Context, src, dst string) error
	// Move renames on one device, or copies then deletes across devices.
	Move(src, dst string) error
	// Rename is atomic; it fails with ErrCrossDevice when src and dst are on different devices.
	Rename(src, dst string) error
	Remove(path string) error
	MkdirAll(path string) error
	// Walk visits root and everything under it in lexical order. Returning fs.SkipDir from fn on a
	// directory skips its contents.
	Walk(root string, fn func(path string, info FileInfo) error) error
}

// FileInfo describes one path; ETag is derived from (Device, Inode, Size, ModTime).
type FileInfo struct {
	Path    string
	Size    int64
	Mode    fs.FileMode
	ModTime time.Time
	Device  uint64
	Inode   uint64
}

func (i FileInfo) IsDir() bool { return i.Mode.IsDir() }

// DownloadLocation is implemented by internal/adapters/qbittorrent, for the hardlink check.
type DownloadLocation interface {
	// SavePath is where qBittorrent saves Magpie's downloads, as a path the server can read.
	SavePath(ctx context.Context) (string, error)
}

type MediaProber interface {
	// Probe runs ffprobe. probesize 50 MB, retried at 150 MB with longer analyzeduration.
	// ErrProbeFailed when ffprobe can't read the file.
	Probe(ctx context.Context, path string) (api.MediaInfo, error)
}

type Hasher interface {
	OSHash(path string) (string, error) // size + uint64 LE sum of the first and last 64 KiB
	SHA256(path string) (string, error)
}

// JellyfinLibrary is implemented by internal/adapters/jellyfin. Requests send only
// Authorization: MediaBrowser Token="…". Network/5xx errors return ErrJellyfinUnavailable.
type JellyfinLibrary interface {
	// RefreshPath calls POST /Library/Media/Updated for one path.
	RefreshPath(ctx context.Context, path string) error
}

var (
	ErrCrossDevice         = errors.New("filesystem: rename across devices")
	ErrProbeFailed         = errors.New("ffprobe: could not read media")
	ErrJellyfinUnavailable = errors.New("jellyfin: unavailable")
)
