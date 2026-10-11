package subtitles

import (
	"context"
	"errors"
	"time"

	"magpie/internal/domain"
)

// OpenSubtitles is implemented by internal/adapters/opensubtitles. The app API key is built in;
// the user's login lives in platform connection config.
type OpenSubtitles interface {
	// Login exchanges the user's credentials for a token. ErrAuth on bad credentials.
	Login(ctx context.Context, user, pass string) (Token, error)
	// Search queries by moviehash plus tmdb/imdb id, season, episode and language. Max 5 req/s.
	Search(ctx context.Context, q SearchQuery) ([]Candidate, error)
	// Download returns the subtitle content and the remaining daily quota.
	// ErrQuotaExceeded when no downloads are left before resetAt.
	Download(ctx context.Context, fileID int64) (content []byte, remaining int, resetAt time.Time, err error)
}

type Token struct {
	Value     string
	ExpiresAt time.Time
	VIP       bool
}

// SearchQuery is comparable so fakes can key results by it. Season and Episode are 0 for movies.
type SearchQuery struct {
	MovieHash string
	Tmdb      domain.TmdbID
	Imdb      string // "" when unknown
	Season    int
	Episode   int
	Language  domain.Language
}

// Candidate is one OpenSubtitles search result, with the attributes scoring needs.
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

var (
	// ErrAuth: re-login once, then pause and notify.
	ErrAuth = errors.New("opensubtitles: authentication failed")
	// ErrQuotaExceeded: pause downloads until the quota resets.
	ErrQuotaExceeded = errors.New("opensubtitles: daily download quota exceeded")
	// ErrRateLimited: back off with Retry-After.
	ErrRateLimited = errors.New("opensubtitles: rate limited")
	// ErrUnavailable: retry with backoff.
	ErrUnavailable = errors.New("opensubtitles: unavailable")
)

// Syncer is implemented by internal/adapters/subsync.
type Syncer interface {
	// Sync aligns subtitle timing to the video's speech (Go port of ffsubsync: VAD + FFT offset search).
	// Returns the shifted subtitle and the applied offset; ErrNoSpeech if alignment is not confident.
	Sync(ctx context.Context, videoPath string, srt []byte) (synced []byte, offset time.Duration, err error)
}

// ErrNoSpeech means the video has no usable speech or the subtitle doesn't line up with it
// confidently. The caller keeps the subtitle unsynced.
var ErrNoSpeech = errors.New("subtitles: no confident alignment between speech and subtitle")
