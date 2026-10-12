package contracttest

import (
	"cmp"
	"context"
	"path/filepath"
	"slices"
	"sync"

	"magpie/internal/domain"
	"magpie/internal/library/api"
	"magpie/internal/platform/scripted"
)

const (
	ReaderFilesForTitle       scripted.Method = "library.Reader.FilesForTitle"
	ReaderFilesForTitles      scripted.Method = "library.Reader.FilesForTitles"
	ReaderFileForEpisode      scripted.Method = "library.Reader.FileForEpisode"
	ReaderFile                scripted.Method = "library.Reader.File"
	ReaderSourceForProcessing scripted.Method = "library.Reader.SourceForProcessing"
	ReaderAvailableEpisodes   scripted.Method = "library.Reader.AvailableEpisodes"
	ReaderCoverage            scripted.Method = "library.Reader.Coverage"

	// stagingDirName is the folder under each root that holds transcoded files until Library swaps them.
	stagingDirName = ".magpie-staging"
)

// Fake is the in-memory library.Reader other contexts test against. It passes Run.
type Fake struct {
	scripted.Script

	mu    sync.RWMutex
	files map[domain.MediaFileID]StoredFile
}

var _ api.Reader = (*Fake)(nil)

func NewFake(f Fixture) *Fake {
	fake := &Fake{files: map[domain.MediaFileID]StoredFile{}}
	for _, file := range f.Files {
		fake.Put(file)
	}
	return fake
}

// Put adds or replaces a stored file.
func (f *Fake) Put(file StoredFile) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[file.File.ID] = file
}

// Remove drops a file, as a delete or an upgrade would.
func (f *Fake) Remove(id domain.MediaFileID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.files, id)
}

func (f *Fake) FilesForTitle(ctx context.Context, title domain.TitleID) ([]api.MediaFile, error) {
	if err := f.Enter(ctx, ReaderFilesForTitle); err != nil {
		return nil, err
	}
	return f.newestFirst(func(m api.MediaFile) bool { return m.Title == title }), nil
}

func (f *Fake) FilesForTitles(ctx context.Context, titles []domain.TitleID, filter api.FileFilter) ([]api.MediaFile, error) {
	if err := f.Enter(ctx, ReaderFilesForTitles); err != nil {
		return nil, err
	}
	files := []api.MediaFile{}
	for _, title := range titles {
		files = append(files, f.newestFirst(func(m api.MediaFile) bool {
			return m.Title == title && (filter.Transcoded == nil || m.Transcoded == *filter.Transcoded)
		})...)
	}
	return files, nil
}

func (f *Fake) FileForEpisode(ctx context.Context, ep domain.EpisodeID) (api.MediaFile, error) {
	if err := f.Enter(ctx, ReaderFileForEpisode); err != nil {
		return api.MediaFile{}, err
	}
	files := f.newestFirst(func(m api.MediaFile) bool {
		return slices.ContainsFunc(m.Episodes, func(ref domain.EpisodeRef) bool { return ref.Episode == ep })
	})
	if len(files) == 0 {
		return api.MediaFile{}, api.ErrNotFound
	}
	return files[0], nil
}

func (f *Fake) File(ctx context.Context, id domain.MediaFileID) (api.MediaFile, error) {
	if err := f.Enter(ctx, ReaderFile); err != nil {
		return api.MediaFile{}, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	stored, ok := f.files[id]
	if !ok {
		return api.MediaFile{}, api.ErrNotFound
	}
	return stored.File, nil
}

func (f *Fake) SourceForProcessing(ctx context.Context, id domain.MediaFileID) (api.ProcessingSource, error) {
	if err := f.Enter(ctx, ReaderSourceForProcessing); err != nil {
		return api.ProcessingSource{}, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	stored, ok := f.files[id]
	if !ok {
		return api.ProcessingSource{}, api.ErrNotFound
	}
	return api.ProcessingSource{
		Path:       stored.File.Path,
		ETag:       stored.ETag,
		Size:       stored.File.Size,
		StagingDir: filepath.Join(stored.File.RootFolder, stagingDirName),
		MediaInfo:  stored.File.MediaInfo,
	}, nil
}

func (f *Fake) AvailableEpisodes(ctx context.Context, title domain.TitleID) ([]domain.EpisodeID, error) {
	if err := f.Enter(ctx, ReaderAvailableEpisodes); err != nil {
		return nil, err
	}
	refs := []domain.EpisodeRef{}
	for _, m := range f.newestFirst(func(m api.MediaFile) bool { return m.Title == title && m.Available }) {
		refs = append(refs, m.Episodes...)
	}
	slices.SortFunc(refs, func(a, b domain.EpisodeRef) int {
		return cmp.Or(cmp.Compare(a.Season, b.Season), cmp.Compare(a.Number, b.Number))
	})
	episodes := []domain.EpisodeID{}
	for _, ref := range refs {
		if !slices.Contains(episodes, ref.Episode) {
			episodes = append(episodes, ref.Episode)
		}
	}
	return episodes, nil
}

func (f *Fake) Coverage(ctx context.Context, titles []domain.TitleID) ([]api.TitleCoverage, error) {
	if err := f.Enter(ctx, ReaderCoverage); err != nil {
		return nil, err
	}
	coverage := make([]api.TitleCoverage, 0, len(titles))
	for _, title := range titles {
		withFile := map[domain.EpisodeID]bool{}
		available := map[domain.EpisodeID]bool{}
		c := api.TitleCoverage{Title: title}
		for _, m := range f.newestFirst(func(m api.MediaFile) bool { return m.Title == title }) {
			c.SizeBytes += m.Size
			for _, ref := range m.Episodes {
				withFile[ref.Episode] = true
				if m.Available {
					available[ref.Episode] = true
				}
			}
		}
		c.EpisodesWithFile = len(withFile)
		c.EpisodesAvailable = len(available)
		coverage = append(coverage, c)
	}
	return coverage, nil
}

func (f *Fake) newestFirst(keep func(api.MediaFile) bool) []api.MediaFile {
	f.mu.RLock()
	defer f.mu.RUnlock()
	files := []api.MediaFile{}
	for _, stored := range f.files {
		if keep(stored.File) {
			files = append(files, stored.File)
		}
	}
	slices.SortFunc(files, func(a, b api.MediaFile) int {
		return cmp.Or(b.ImportedAt.Compare(a.ImportedAt), cmp.Compare(b.ID, a.ID))
	})
	return files
}
