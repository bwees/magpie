package library_test

import (
	"context"
	"slices"
	"testing"

	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/domain"
	"magpie/internal/library/api"
	"magpie/internal/platform/bus"
)

const (
	oldEpisodeRelease = "Show.2020.S01E01.720p.HDTV.x264-OLD"
	newEpisodeRelease = "Show.2020.S01E01.1080p.WEB-DL.DDP5.1.H.264-NEW"
)

// fileEvents are the FileImported and FileDeleted events in the order they were published.
func (f *importFixture) fileEvents() []bus.Event {
	f.events.mu.Lock()
	defer f.events.mu.Unlock()
	return slices.DeleteFunc(slices.Clone(f.events.events), func(ev bus.Event) bool {
		return ev.Type != api.TypeFileImported && ev.Type != api.TypeFileDeleted
	})
}

func (f *importFixture) importEpisode(t *testing.T, release, file string, episode domain.EpisodeID) {
	t.Helper()
	f.download(t, file, fullRuntime)
	f.deliver(t, f.completed(f.show.ID, []domain.EpisodeID{episode}, release, acquisitionapi.SeedUntilGoals, file))
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
