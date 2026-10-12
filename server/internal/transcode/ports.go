package transcode

import (
	"context"
	"fmt"
	"time"

	libraryapi "magpie/internal/library/api"
)

// FileStat is used for staging and source checks. Transcode has no third-party adapters; its only
// external party is the agent, through the protocol in api/agentproto.
type FileStat interface {
	// Stat returns size, mtime and inode for an absolute path; ETag is derived from them.
	// A missing path returns an error matching fs.ErrNotExist.
	Stat(path string) (FileInfo, error)
}

type FileInfo struct {
	Size    int64
	ModTime time.Time
	Inode   uint64
	Device  uint64
}

// ETag is the strong HTTP entity tag of the file: it changes whenever the file is replaced or written.
func (i FileInfo) ETag() string {
	return fmt.Sprintf(`"%d-%d-%d"`, i.Inode, i.Size, i.ModTime.UnixNano())
}

// MediaProber reads a staged output with ffprobe, for the server's own check before Library swaps it.
type MediaProber interface {
	// Probe returns the file's container, duration and streams. An unreadable file is an error.
	Probe(ctx context.Context, path string) (libraryapi.MediaInfo, error)
}
