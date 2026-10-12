package library_test

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/domain"
	"magpie/internal/library"
	"magpie/internal/library/api"
	"magpie/internal/platform/bus"
)

const (
	oldEpisodeRelease   = "Show.2020.S01E01.720p.HDTV.x264-OLD"
	newEpisodeRelease   = "Show.2020.S01E01.1080p.WEB-DL.DDP5.1.H.264-NEW"
	multiEpisodeRelease = "Show.2020.S01E01-E02.1080p.WEB-DL.DDP5.1.H.264-MULTI"
)

// fileEvents are the FileImported and FileDeleted events in the order they were published.
func (f *importFixture) fileEvents() []bus.Event {
	f.events.mu.Lock()
	defer f.events.mu.Unlock()
	return slices.DeleteFunc(slices.Clone(f.events.events), func(ev bus.Event) bool {
		return ev.Type != api.TypeFileImported && ev.Type != api.TypeFileDeleted
	})
}

func (f *importFixture) importEpisode(t *testing.T, release, file string, episodes ...domain.EpisodeID) acquisitionapi.DownloadCompleted {
	t.Helper()
	f.download(t, file, fullRuntime)
	c := f.completed(f.show.ID, episodes, release, acquisitionapi.SeedUntilGoals, file)
	f.deliver(t, c)
	return c
}

// Seed library #7.
func TestAnUpgradeReplacesTheOldFileOnceTheNewOneIsInPlace(t *testing.T) {
	cases := []struct {
		name           string
		oldExt, newExt string
	}{
		{"onto the same name", ".mkv", ".mkv"},
		{"under a new extension", ".mp4", ".mkv"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newImportFixture(t, oneDevice)
			episode := f.episodes[0].ID
			f.importEpisode(t, oldEpisodeRelease, oldEpisodeRelease+tc.oldExt, episode)
			old := f.imported(t)[0]

			f.importEpisode(t, newEpisodeRelease, newEpisodeRelease+tc.newExt, episode)

			events := f.fileEvents()
			if len(events) != 3 {
				t.Fatalf("%d file events, want FileImported(F1), FileImported(F2), FileDeleted(F1)", len(events))
			}
			upgrade := payload[api.FileImported](t, events[1])
			deleted := payload[api.FileDeleted](t, events[2])
			if upgrade.Replaces == nil || *upgrade.Replaces != old.MediaFile {
				t.Errorf("FileImported.Replaces = %v, want %s", upgrade.Replaces, old.MediaFile)
			}
			if deleted.MediaFile != old.MediaFile || deleted.Reason != api.DeleteUpgraded || deleted.Path != old.Path {
				t.Errorf("FileDeleted = %+v, want %s UPGRADED", deleted, old.MediaFile)
			}
			files, err := f.repo.FilesForTitle(context.Background(), f.show.ID)
			if err != nil || len(files) != 1 || files[0].ID != upgrade.MediaFile {
				t.Errorf("files = %+v, %v; want only the upgrade", files, err)
			}
			content, err := f.files.ReadFile(upgrade.Path)
			if err != nil || string(content) != "content of "+newEpisodeRelease+tc.newExt {
				t.Errorf("new file = %q, %v; want the upgrade's content", content, err)
			}
			if old.Path != upgrade.Path && f.files.Exists(old.Path) {
				t.Errorf("%s is still on disk", old.Path)
			}
		})
	}
}

func TestAnEpisodeFileOnlyReplacesFilesSharingItsEpisodes(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	f.importEpisode(t, "Show.2020.S01E02.720p.HDTV.x264-OLD", "Show.2020.S01E02.720p.HDTV.x264-OLD.mkv", f.episodes[1].ID)
	f.importEpisode(t, newEpisodeRelease, newEpisodeRelease+".mkv", f.episodes[0].ID)

	if deleted := f.events.ofType(api.TypeFileDeleted); len(deleted) != 0 {
		t.Errorf("FileDeleted = %d, want none for another episode", len(deleted))
	}
	if upgrade := f.imported(t)[1]; upgrade.Replaces != nil {
		t.Errorf("Replaces = %v, want nil", *upgrade.Replaces)
	}
}

// Seed library #7.
func TestAVideoCoveringEveryEpisodeOfOldFilesReplacesThem(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	first, second := f.episodes[0].ID, f.episodes[1].ID
	f.importEpisode(t, oldEpisodeRelease, oldEpisodeRelease+".mkv", first)
	f.importEpisode(t, "Show.2020.S01E02.720p.HDTV.x264-OLD", "Show.2020.S01E02.720p.HDTV.x264-OLD.mkv", second)
	olds := f.imported(t)

	f.importEpisode(t, multiEpisodeRelease, multiEpisodeRelease+".mkv", first, second)

	deleted := f.events.ofType(api.TypeFileDeleted)
	if len(deleted) != len(olds) {
		t.Fatalf("%d FileDeleted, want one per old file", len(deleted))
	}
	files, err := f.repo.FilesForTitle(context.Background(), f.show.ID)
	if err != nil || len(files) != 1 || len(files[0].Episodes) != 2 {
		t.Errorf("files = %+v, %v; want only the two-episode upgrade", files, err)
	}
	for _, old := range olds {
		if f.files.Exists(old.Path) {
			t.Errorf("%s is still on disk", old.Path)
		}
	}
}

// Seed library #7.
func TestAVideoCoveringPartOfAMultiEpisodeFileWaitsForTheUser(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	first, second := f.episodes[0].ID, f.episodes[1].ID
	f.importEpisode(t, multiEpisodeRelease, multiEpisodeRelease+".mkv", first, second)
	old := f.imported(t)[0]

	c := f.importEpisode(t, newEpisodeRelease, newEpisodeRelease+".mkv", first)

	if imp := f.imp(t, c.Download); imp.Status != library.ImportManual || !strings.Contains(imp.LastError, "multi-episode") {
		t.Errorf("import = %s %q, want MANUAL saying it would split a multi-episode file", imp.Status, imp.LastError)
	}
	if n := len(f.imported(t)); n != 1 {
		t.Errorf("%d FileImported, want only the old file's", n)
	}
	if deleted := f.events.ofType(api.TypeFileDeleted); len(deleted) != 0 {
		t.Errorf("%d FileDeleted, want the old file kept", len(deleted))
	}
	files, err := f.repo.FilesForTitle(context.Background(), f.show.ID)
	if err != nil || len(files) != 1 || files[0].ID != old.MediaFile {
		t.Errorf("files = %+v, %v; want only the old file", files, err)
	}
	if !f.files.Exists(old.Path) {
		t.Errorf("%s was removed", old.Path)
	}
	if path := filepath.Join(filepath.Dir(old.Path), "Show S01E01.mkv"); f.files.Exists(path) {
		t.Errorf("%s was placed for an import waiting for the user", path)
	}
}

// Seed library #7.
func TestAPackCoveringAMultiEpisodeFileReplacesIt(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	f.importEpisode(t, multiEpisodeRelease, multiEpisodeRelease+".mkv", f.episodes[0].ID, f.episodes[1].ID)
	old := f.imported(t)[0]
	folder := "Show.2020.S01.1080p"

	f.deliver(t, f.completed(f.show.ID, f.episodeIDs(), packRelease, acquisitionapi.SeedUntilGoals, folder, f.packFiles(t, folder)...))

	deleted := f.events.ofType(api.TypeFileDeleted)
	if len(deleted) != 1 || payload[api.FileDeleted](t, deleted[0]).MediaFile != old.MediaFile {
		t.Errorf("FileDeleted = %v, want the two-episode file once", deleted)
	}
	files, err := f.repo.FilesForTitle(context.Background(), f.show.ID)
	if err != nil || len(files) != showEpisodes {
		t.Errorf("%d files, %v; want one per pack episode", len(files), err)
	}
	if f.files.Exists(old.Path) {
		t.Errorf("%s is still on disk", old.Path)
	}
}

// Seed library #7.
func TestAMovieReplacesItsOldFile(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	const oldMovie = "Dune.2021.720p.WEB-DL.mp4"
	f.download(t, oldMovie, 2*time.Hour)
	f.deliver(t, f.movieDownload(acquisitionapi.SeedUntilGoals, oldMovie))
	old := f.imported(t)[0]
	const newMovie = "Dune.2021.1080p.WEB-DL.mkv"
	f.download(t, newMovie, 2*time.Hour)

	f.deliver(t, f.movieDownload(acquisitionapi.SeedUntilGoals, newMovie))

	deleted := f.events.ofType(api.TypeFileDeleted)
	if len(deleted) != 1 || payload[api.FileDeleted](t, deleted[0]).MediaFile != old.MediaFile {
		t.Errorf("FileDeleted = %v, want the old movie file once", deleted)
	}
	if f.files.Exists(old.Path) {
		t.Errorf("%s is still on disk", old.Path)
	}
	if !f.files.Exists(f.moviePath()) {
		t.Errorf("%s is missing", f.moviePath())
	}
}
