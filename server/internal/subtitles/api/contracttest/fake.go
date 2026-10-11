package contracttest

import (
	"cmp"
	"context"
	"slices"
	"sync"

	"magpie/internal/domain"
	"magpie/internal/platform/scripted"
	"magpie/internal/subtitles/api"
)

const (
	ReaderForFile  scripted.Method = "subtitles.Reader.ForFile"
	ReaderCoverage scripted.Method = "subtitles.Reader.Coverage"
	ReaderQuota    scripted.Method = "subtitles.Reader.Quota"
)

// Fake is the in-memory subtitles.Reader other contexts test against. It passes Run.
type Fake struct {
	scripted.Script

	mu    sync.RWMutex
	files map[domain.MediaFileID]File
	quota api.Quota
}

var _ api.Reader = (*Fake)(nil)

func NewFake(f Fixture) *Fake {
	fake := &Fake{files: map[domain.MediaFileID]File{}, quota: f.Quota}
	for id, file := range f.Files {
		fake.PutFile(id, file)
	}
	return fake
}

// PutFile adds or replaces a file's subtitles and coverage.
func (f *Fake) PutFile(id domain.MediaFileID, file File) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[id] = File{Subtitles: slices.Clone(file.Subtitles), Coverage: slices.Clone(file.Coverage)}
}

func (f *Fake) SetQuota(q api.Quota) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.quota = q
}

func (f *Fake) ForFile(ctx context.Context, file domain.MediaFileID) ([]api.Subtitle, error) {
	if err := f.Enter(ctx, ReaderForFile); err != nil {
		return nil, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	subs := append([]api.Subtitle{}, f.files[file].Subtitles...)
	slices.SortStableFunc(subs, func(a, b api.Subtitle) int {
		return cmp.Or(cmp.Compare(a.Language, b.Language), compareBool(a.Forced, b.Forced), compareBool(a.SDH, b.SDH))
	})
	return subs, nil
}

func (f *Fake) Coverage(ctx context.Context, file domain.MediaFileID) ([]api.LanguageCoverage, error) {
	if err := f.Enter(ctx, ReaderCoverage); err != nil {
		return nil, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	return append([]api.LanguageCoverage{}, f.files[file].Coverage...), nil
}

func (f *Fake) Quota(ctx context.Context) (api.Quota, error) {
	if err := f.Enter(ctx, ReaderQuota); err != nil {
		return api.Quota{}, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.quota, nil
}

// compareBool orders false before true.
func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case b:
		return -1
	default:
		return 1
	}
}
