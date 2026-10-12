package library

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// FileKind is how an import treats a file of a download.
type FileKind string

const (
	FileVideo    FileKind = "VIDEO"
	FileSubtitle FileKind = "SUBTITLE"
	FileIgnored  FileKind = "IGNORED"
)

var (
	videoExtensions    = []string{".mkv", ".mp4", ".m4v", ".avi", ".ts"}
	subtitleExtensions = []string{".srt", ".ass", ".ssa", ".sub", ".idx", ".sup", ".vtt"}
)

func fileKind(path string) FileKind {
	ext := strings.ToLower(filepath.Ext(path))
	switch {
	case slices.Contains(videoExtensions, ext):
		return FileVideo
	case slices.Contains(subtitleExtensions, ext):
		return FileSubtitle
	}
	return FileIgnored
}

const etagShape = "%d:%d:%d:%d"

// etagOf changes whenever the file at a path is replaced or modified: it hashes the device, inode, size
// and modification time.
func etagOf(info FileInfo) string {
	sum := sha256.Sum256(fmt.Appendf(nil, etagShape, info.Device, info.Inode, info.Size, info.ModTime.UnixNano()))
	return hex.EncodeToString(sum[:etagBytes])
}

const etagBytes = 16

// sampleLimit is the longest a sample can run for titles up to a given runtime, from Sonarr and Radarr.
type sampleLimit struct {
	upTo  time.Duration
	limit time.Duration
}

var sampleLimits = []sampleLimit{
	{upTo: 3 * time.Minute, limit: 15 * time.Second},
	{upTo: 10 * time.Minute, limit: 90 * time.Second},
	{upTo: 30 * time.Minute, limit: 300 * time.Second},
}

const (
	longestSampleLimit = 600 * time.Second
	// unknownRuntime stands in for a title or episode without a runtime, as Sonarr does.
	unknownRuntime = 45 * time.Minute
)

// isSample reports whether a video of this duration is a sample of a title expected to run expected.
// A video whose duration couldn't be read counts as a sample.
func isSample(duration, expected time.Duration) bool {
	if duration <= 0 {
		return true
	}
	if expected <= 0 {
		expected = unknownRuntime
	}
	limit := longestSampleLimit
	for _, s := range sampleLimits {
		if expected <= s.upTo {
			limit = s.limit
			break
		}
	}
	return duration <= limit
}
