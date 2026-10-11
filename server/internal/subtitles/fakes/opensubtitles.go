// Package fakes holds in-memory fakes of Subtitles' ports, for tests and for booting without real adapters.
package fakes

import (
	"context"
	"slices"
	"sync"
	"time"

	"magpie/internal/domain"
	"magpie/internal/platform/scripted"
	"magpie/internal/subtitles"
)

const (
	OpenSubtitlesLogin    scripted.Method = "OpenSubtitles.Login"
	OpenSubtitlesSearch   scripted.Method = "OpenSubtitles.Search"
	OpenSubtitlesDownload scripted.Method = "OpenSubtitles.Download"

	// FreeDailyDownloads is OpenSubtitles' quota for a non-VIP account.
	FreeDailyDownloads = 20
	quotaPeriod        = 24 * time.Hour
	tokenLifetime      = 24 * time.Hour
)

type tokenValue string

// OpenSubtitles serves scripted search results and subtitle files, and counts down a daily quota.
type OpenSubtitles struct {
	scripted.Script

	mu         sync.Mutex
	now        func() time.Time
	user, pass string
	candidates map[subtitles.SearchQuery][]subtitles.Candidate
	content    map[int64][]byte
	remaining  int
	resetAt    time.Time
}

var _ subtitles.OpenSubtitles = (*OpenSubtitles)(nil)

// NewOpenSubtitles starts with no account, no results and a full free quota.
func NewOpenSubtitles() *OpenSubtitles {
	f := &OpenSubtitles{
		now:        time.Now,
		candidates: map[subtitles.SearchQuery][]subtitles.Candidate{},
		content:    map[int64][]byte{},
		remaining:  FreeDailyDownloads,
	}
	f.resetAt = f.now().Add(quotaPeriod)
	return f
}

// SetCredentials sets the only login Login accepts.
func (f *OpenSubtitles) SetCredentials(user, pass string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.user, f.pass = user, pass
}

// AddCandidates makes Search return cands for exactly q.
func (f *OpenSubtitles) AddCandidates(q subtitles.SearchQuery, cands ...subtitles.Candidate) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.candidates[q] = append(f.candidates[q], cands...)
}

// SetContent makes Download return content for fileID.
func (f *OpenSubtitles) SetContent(fileID int64, content []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.content[fileID] = slices.Clone(content)
}

// SetQuota sets the downloads left and when they reset.
func (f *OpenSubtitles) SetQuota(remaining int, resetAt time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.remaining, f.resetAt = remaining, resetAt
}

func (f *OpenSubtitles) Login(ctx context.Context, user, pass string) (subtitles.Token, error) {
	if err := f.Enter(ctx, OpenSubtitlesLogin); err != nil {
		return subtitles.Token{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if user == "" || user != f.user || pass != f.pass {
		return subtitles.Token{}, subtitles.ErrAuth
	}
	return subtitles.Token{Value: string(domain.NewID[tokenValue]()), ExpiresAt: f.now().Add(tokenLifetime)}, nil
}

func (f *OpenSubtitles) Search(ctx context.Context, q subtitles.SearchQuery) ([]subtitles.Candidate, error) {
	if err := f.Enter(ctx, OpenSubtitlesSearch); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]subtitles.Candidate{}, f.candidates[q]...), nil
}

func (f *OpenSubtitles) Download(ctx context.Context, fileID int64) ([]byte, int, time.Time, error) {
	if err := f.Enter(ctx, OpenSubtitlesDownload); err != nil {
		return nil, 0, time.Time{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.remaining <= 0 {
		return nil, 0, f.resetAt, subtitles.ErrQuotaExceeded
	}
	content, ok := f.content[fileID]
	if !ok {
		return nil, f.remaining, f.resetAt, subtitles.ErrUnavailable
	}
	f.remaining--
	return slices.Clone(content), f.remaining, f.resetAt, nil
}
