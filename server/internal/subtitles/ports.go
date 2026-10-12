package subtitles

import (
	"context"
	"errors"
	"fmt"
	"time"

	"magpie/internal/domain"
)

// OpenSubtitles is implemented by internal/adapters/opensubtitles. The app API key is built in;
// the adapter reads the user's login from platform connection config and keeps the token.
type OpenSubtitles interface {
	// Login exchanges the configured login for a fresh token, which later calls use. ErrAuth on bad or
	// missing credentials.
	Login(ctx context.Context) (Token, error)
	// Search queries by moviehash plus tmdb/imdb id, season, episode and language. Max 5 req/s.
	Search(ctx context.Context, q SearchQuery) ([]Candidate, error)
	// Download returns the subtitle content and the remaining daily quota. ErrAuth without a valid token;
	// ErrQuotaExceeded when no downloads are left before resetAt.
	Download(ctx context.Context, fileID int64) (content []byte, remaining int, resetAt time.Time, err error)
}

type Token struct {
	Value     string
	ExpiresAt time.Time
	VIP       bool
	// AllowedDownloads is the account's daily download quota.
	AllowedDownloads int
}

// SearchQuery is comparable so fakes can key results by it. Season and Episode are 0 for movies; for an
// episode, Tmdb and Imdb identify the series.
type SearchQuery struct {
	MovieHash string
	Tmdb      domain.TmdbID
	Imdb      string // "" when unknown
	Season    int
	Episode   int
	Language  domain.Language
}

// Candidate is one OpenSubtitles search result, with the attributes scoring needs. Tmdb and Imdb identify
// the movie, or the series for an episode.
type Candidate struct {
	FileID            int64
	Language          domain.Language
	ReleaseName       string
	HashMatch         bool // moviehash_match
	HearingImpaired   bool
	Forced            bool // foreign_parts_only
	MachineTranslated bool
	AITranslated      bool
	Tmdb              domain.TmdbID
	Imdb              string
	Year              int
	Season            int
	Episode           int
	FPS               float64
}

// SidecarFiles writes and removes subtitle files next to videos. Implemented by internal/adapters/disk.
type SidecarFiles interface {
	// Write replaces the file at path with content.
	Write(path string, content []byte) error
	// Remove deletes path. A missing file is not an error.
	Remove(path string) error
}

var (
	// ErrAuth: re-login once, then pause and notify.
	ErrAuth = errors.New("opensubtitles: authentication failed")
	// ErrQuotaExceeded: pause downloads until the quota resets.
	ErrQuotaExceeded = errors.New("opensubtitles: daily download quota exceeded")
	// ErrRateLimited: back off with Retry-After. The adapter returns it as a *RetryAfterError.
	ErrRateLimited = errors.New("opensubtitles: rate limited")
	// ErrUnavailable: retry with backoff.
	ErrUnavailable = errors.New("opensubtitles: unavailable")
)

// RetryAfterError is ErrRateLimited with the wait OpenSubtitles asked for.
type RetryAfterError struct {
	Wait time.Duration
}

func (e *RetryAfterError) Error() string {
	return fmt.Sprintf("%s: retry after %s", ErrRateLimited, e.Wait)
}

func (e *RetryAfterError) Is(target error) bool { return target == ErrRateLimited }

// Syncer is implemented by internal/adapters/subsync.
type Syncer interface {
	// Sync aligns subtitle timing to the video's speech (Go port of ffsubsync: VAD + FFT offset search).
	// Returns the shifted subtitle and the applied offset; ErrNoSpeech if alignment is not confident.
	Sync(ctx context.Context, videoPath string, srt []byte) (synced []byte, offset time.Duration, err error)
}

// ErrNoSpeech means the video has no usable speech or the subtitle doesn't line up with it
// confidently. The caller keeps the subtitle unsynced.
var ErrNoSpeech = errors.New("subtitles: no confident alignment between speech and subtitle")
