package transcode

import "time"

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
