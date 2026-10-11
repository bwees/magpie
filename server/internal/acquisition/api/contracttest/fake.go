package contracttest

import (
	"cmp"
	"context"
	"slices"
	"sync"

	"magpie/internal/acquisition/api"
	"magpie/internal/domain"
	"magpie/internal/platform/scripted"
)

const (
	ReaderProfile           scripted.Method = "acquisition.Reader.Profile"
	ReaderProfiles          scripted.Method = "acquisition.Reader.Profiles"
	ReaderActiveDownloads   scripted.Method = "acquisition.Reader.ActiveDownloads"
	ReaderDownloadsForTitle scripted.Method = "acquisition.Reader.DownloadsForTitle"
	ReaderBlocklist         scripted.Method = "acquisition.Reader.Blocklist"
	ReaderCutoffStatus      scripted.Method = "acquisition.Reader.CutoffStatus"
)

// Fake is the in-memory acquisition.Reader. It passes Run.
type Fake struct {
	scripted.Script

	mu         sync.RWMutex
	profiles   map[domain.ProfileID]api.Profile
	downloads  map[domain.DownloadID]api.Download
	blocklist  map[string]api.BlocklistEntry
	fileScores map[domain.MediaFileID]FileScore
}

var _ api.Reader = (*Fake)(nil)

func NewFake(f Fixture) *Fake {
	fake := &Fake{
		profiles:   map[domain.ProfileID]api.Profile{},
		downloads:  map[domain.DownloadID]api.Download{},
		blocklist:  map[string]api.BlocklistEntry{},
		fileScores: map[domain.MediaFileID]FileScore{},
	}
	for _, p := range f.Profiles {
		fake.PutProfile(p)
	}
	for _, d := range f.Downloads {
		fake.PutDownload(d)
	}
	for _, e := range f.Blocklist {
		fake.PutBlocklistEntry(e)
	}
	for _, s := range f.FileScores {
		fake.PutFileScore(s)
	}
	return fake
}

func (f *Fake) PutProfile(p api.Profile) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.profiles[p.ID] = p
}

func (f *Fake) PutDownload(d api.Download) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.downloads[d.ID] = d
}

func (f *Fake) PutBlocklistEntry(e api.BlocklistEntry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.blocklist[e.ID] = e
}

func (f *Fake) PutFileScore(s FileScore) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fileScores[s.File] = s
}

func (f *Fake) Profile(ctx context.Context, id domain.ProfileID) (api.Profile, error) {
	if err := f.Enter(ctx, ReaderProfile); err != nil {
		return api.Profile{}, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	p, ok := f.profiles[id]
	if !ok {
		return api.Profile{}, api.ErrNotFound
	}
	return p, nil
}

func (f *Fake) Profiles(ctx context.Context) ([]api.ProfileSummary, error) {
	if err := f.Enter(ctx, ReaderProfiles); err != nil {
		return nil, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	summaries := []api.ProfileSummary{}
	for _, p := range f.profiles {
		summaries = append(summaries, api.ProfileSummary{ID: p.ID, Name: p.Name, UpgradeAllowed: p.UpgradeAllowed, Preset: p.Preset, TrashProfileID: p.TrashProfileID})
	}
	slices.SortFunc(summaries, func(a, b api.ProfileSummary) int { return cmp.Compare(a.Name, b.Name) })
	return summaries, nil
}

func (f *Fake) ActiveDownloads(ctx context.Context) ([]api.Download, error) {
	if err := f.Enter(ctx, ReaderActiveDownloads); err != nil {
		return nil, err
	}
	return f.downloadsNewestFirst(func(d api.Download) bool { return d.RemovedAt == nil }), nil
}

func (f *Fake) DownloadsForTitle(ctx context.Context, title domain.TitleID) ([]api.Download, error) {
	if err := f.Enter(ctx, ReaderDownloadsForTitle); err != nil {
		return nil, err
	}
	return f.downloadsNewestFirst(func(d api.Download) bool { return d.Title == title }), nil
}

func (f *Fake) Blocklist(ctx context.Context, title domain.TitleID) ([]api.BlocklistEntry, error) {
	if err := f.Enter(ctx, ReaderBlocklist); err != nil {
		return nil, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	entries := []api.BlocklistEntry{}
	for _, e := range f.blocklist {
		if title == "" || e.Title == title {
			entries = append(entries, e)
		}
	}
	slices.SortFunc(entries, func(a, b api.BlocklistEntry) int {
		return cmp.Or(b.At.Compare(a.At), cmp.Compare(b.ID, a.ID))
	})
	return entries, nil
}

func (f *Fake) CutoffStatus(ctx context.Context, file domain.MediaFileID) (api.CutoffStatus, error) {
	if err := f.Enter(ctx, ReaderCutoffStatus); err != nil {
		return api.CutoffStatus{}, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	score, ok := f.fileScores[file]
	if !ok {
		return api.CutoffStatus{}, api.ErrNotFound
	}
	profile, known := f.profiles[score.Profile]
	return api.CutoffStatus{
		File:            score.File,
		Quality:         score.Quality,
		CFScore:         score.CFScore,
		CutoffMet:       score.CutoffMet,
		UpgradesAllowed: known && profile.UpgradeAllowed && !score.Transcoded,
	}, nil
}

func (f *Fake) downloadsNewestFirst(keep func(api.Download) bool) []api.Download {
	f.mu.RLock()
	defer f.mu.RUnlock()
	downloads := []api.Download{}
	for _, d := range f.downloads {
		if keep(d) {
			downloads = append(downloads, d)
		}
	}
	slices.SortFunc(downloads, func(a, b api.Download) int {
		return cmp.Or(b.GrabbedAt.Compare(a.GrabbedAt), cmp.Compare(b.ID, a.ID))
	})
	return downloads
}
