package library_test

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"

	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/domain"
	"magpie/internal/library"
	"magpie/internal/library/api"
	"magpie/internal/library/fakes"
	"magpie/internal/platform/settings"
)

const (
	movieItemID   = "jf-movie"
	episodeItemID = "jf-episode"
	waitForWatch  = 5 * time.Second
	pollEvery     = 10 * time.Millisecond
	watchBackoff  = 10 * time.Millisecond
)

var errJellyfinDown = errors.New("jellyfin: connection refused")

// importMovie imports Dune and returns its FileImported.
func (f *importFixture) importMovie(t *testing.T) api.FileImported {
	t.Helper()
	f.download(t, "Dune.2021.1080p.mkv", 2*time.Hour)
	f.deliver(t, f.movieDownload(acquisitionapi.SeedUntilGoals, "Dune.2021.1080p.mkv"))
	imported := f.imported(t)
	if len(imported) != 1 {
		t.Fatalf("%d FileImported, want 1", len(imported))
	}
	return imported[0]
}

func (f *importFixture) available(t *testing.T) []api.ItemAvailable {
	t.Helper()
	var out []api.ItemAvailable
	for _, ev := range f.events.ofType(api.TypeItemAvailable) {
		out = append(out, payload[api.ItemAvailable](t, ev))
	}
	return out
}

// Seed library #12.
func TestALibraryChangedItemMatchingAFileIsAvailableOnce(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	ctx := context.Background()
	imported := f.importMovie(t)
	f.jellyfin.AddItem(library.JellyfinItem{ID: movieItemID, Path: imported.Path, Type: "Movie"}, time.Now())
	change := library.LibraryChange{Added: []string{movieItemID}}

	if err := f.service.HandleChange(ctx, change); err != nil {
		t.Fatal(err)
	}
	if err := f.service.HandleChange(ctx, change); err != nil {
		t.Fatal(err)
	}

	got := f.available(t)
	want := api.ItemAvailable{Title: f.movie.ID, Episodes: []domain.EpisodeID{}, JellyfinItem: movieItemID}
	if len(got) != 1 || got[0].Title != want.Title || len(got[0].Episodes) != 0 || got[0].JellyfinItem != want.JellyfinItem {
		t.Errorf("ItemAvailable = %+v, want once %+v", got, want)
	}
	if file, err := f.repo.File(ctx, imported.MediaFile); err != nil || !file.Available {
		t.Errorf("file = %+v, %v; want available", file, err)
	}
}

func TestAnUnknownOrUnrelatedItemChangesNothing(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	ctx := context.Background()
	f.importMovie(t)
	f.jellyfin.AddItem(library.JellyfinItem{ID: "jf-other", Path: "/elsewhere/Other (2020).mkv", Type: "Movie"}, time.Now())

	err := f.service.HandleChange(ctx, library.LibraryChange{Added: []string{"jf-other", "jf-gone"}, Removed: []string{movieItemID}})
	if err != nil || len(f.available(t)) != 0 {
		t.Errorf("err = %v, ItemAvailable = %+v; want nothing", err, f.available(t))
	}
}

func TestAConfirmedEpisodeFileMakesItsEpisodesAvailable(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	ctx := context.Background()
	folder := "Show.2020.S01.1080p"
	f.deliver(t, f.completed(f.show.ID, f.episodeIDs(), packRelease, acquisitionapi.SeedUntilGoals, folder, f.packFiles(t, folder)...))
	first := f.imported(t)[0]
	f.jellyfin.AddItem(library.JellyfinItem{ID: episodeItemID, Path: first.Path, Type: "Episode"}, time.Now())

	if err := f.service.HandleChange(ctx, library.LibraryChange{Updated: []string{episodeItemID}}); err != nil {
		t.Fatal(err)
	}

	got := f.available(t)
	if len(got) != 1 || !slices.Equal(got[0].Episodes, []domain.EpisodeID{first.Episodes[0].Episode}) {
		t.Errorf("ItemAvailable = %+v, want the file's episode", got)
	}
	episodes, err := f.repo.AvailableEpisodes(ctx, f.show.ID)
	if err != nil || !slices.Equal(episodes, []domain.EpisodeID{first.Episodes[0].Episode}) {
		t.Errorf("AvailableEpisodes = %v, %v", episodes, err)
	}
}

func TestANewJellyfinItemForAConfirmedFileIsAvailableAgain(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	ctx := context.Background()
	imported := f.importMovie(t)
	for _, id := range []string{movieItemID, "jf-movie-rescanned"} {
		f.jellyfin.AddItem(library.JellyfinItem{ID: id, Path: imported.Path, Type: "Movie"}, time.Now())
		if err := f.service.HandleChange(ctx, library.LibraryChange{Added: []string{id}}); err != nil {
			t.Fatal(err)
		}
	}
	if got := f.available(t); len(got) != 2 || got[1].JellyfinItem != "jf-movie-rescanned" {
		t.Errorf("ItemAvailable = %+v, want one per Jellyfin item", got)
	}
}

// Seed library #13.
func TestTheReconcileCatchesAnItemTheWebSocketMissed(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	ctx := context.Background()
	imported := f.importMovie(t)
	f.jellyfin.AddItem(library.JellyfinItem{ID: movieItemID, Path: imported.Path, Type: "Movie"}, time.Now())

	if err := f.service.ReconcileJellyfin(ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.service.ReconcileJellyfin(ctx); err != nil {
		t.Fatal(err)
	}

	if got := f.available(t); len(got) != 1 || got[0].JellyfinItem != movieItemID {
		t.Errorf("ItemAvailable = %+v, want one for the missed item", got)
	}
}

func TestTheReconcileReadsFromALittleBeforeItsLastRun(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	ctx := context.Background()
	imported := f.importMovie(t)
	lastRun := time.Date(2026, 10, 11, 3, 0, 0, 0, time.UTC)
	library.SetClock(f.service, func() time.Time { return lastRun })
	if err := f.service.ReconcileJellyfin(ctx); err != nil {
		t.Fatal(err)
	}

	f.jellyfin.AddItem(library.JellyfinItem{ID: movieItemID, Path: imported.Path, Type: "Movie"}, lastRun.Add(-time.Minute))
	library.SetClock(f.service, func() time.Time { return lastRun.Add(24 * time.Hour) })
	if err := f.service.ReconcileJellyfin(ctx); err != nil {
		t.Fatal(err)
	}

	if got := f.available(t); len(got) != 1 {
		t.Errorf("ItemAvailable = %+v; an item saved just before the last run by Jellyfin's clock counts", got)
	}
}

func TestEachImportRefreshesItsFoldersOnce(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	folder := "Show.2020.S01.1080p"

	f.deliver(t, f.completed(f.show.ID, f.episodeIDs(), packRelease, acquisitionapi.SeedUntilGoals, folder, f.packFiles(t, folder)...))

	want := []string{filepath.Join(f.layout.tv, "Show (2020) [tmdbid-1399]", "Season 01")}
	if got := f.jellyfin.Refreshed(); !slices.Equal(got, want) {
		t.Errorf("refreshed %v, want %v", got, want)
	}
}

// Seed library #16.
func TestARefreshErrorKeepsTheImportAndTheReconcileRetriesIt(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	ctx := context.Background()
	f.jellyfin.FailAlways(fakes.JellyfinLibraryRefreshPath, errJellyfinDown)

	imported := f.importMovie(t)

	if _, err := f.repo.File(ctx, imported.MediaFile); err != nil {
		t.Fatalf("the import didn't commit: %v", err)
	}
	if n := f.jellyfin.Called(fakes.JellyfinLibraryRefreshPath); n != 1 {
		t.Errorf("RefreshPath called %d times, want once", n)
	}

	f.jellyfin.FailAlways(fakes.JellyfinLibraryRefreshPath, nil)
	if err := f.service.ReconcileJellyfin(ctx); err != nil {
		t.Fatal(err)
	}
	if got, want := f.jellyfin.Refreshed(), []string{filepath.Dir(imported.Path)}; !slices.Equal(got, want) {
		t.Errorf("refreshed %v, want the unconfirmed file's folder %v", got, want)
	}

	f.jellyfin.AddItem(library.JellyfinItem{ID: movieItemID, Path: imported.Path, Type: "Movie"}, time.Now())
	if err := f.service.ReconcileJellyfin(ctx); err != nil {
		t.Fatal(err)
	}
	if len(f.available(t)) != 1 || len(f.jellyfin.Refreshed()) != 1 {
		t.Errorf("after Jellyfin found it: ItemAvailable %+v, refreshed %v; want available and no further refresh", f.available(t), f.jellyfin.Refreshed())
	}
}

func TestTheWatcherFollowsChangesAndReconnectsUntilStopped(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	imported := f.importMovie(t)
	library.SetWatchBackoff(f.service, watchBackoff, watchBackoff)
	f.jellyfin.FailNext(fakes.JellyfinLibraryChanges, errJellyfinDown)
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		f.service.WatchJellyfin(ctx)
		close(stopped)
	}()

	waitUntil(t, "the watcher connects after a failed attempt", func() bool { return f.jellyfin.Streams() == 1 })
	f.jellyfin.DropStreams()
	waitUntil(t, "the watcher reconnects after the socket drops", func() bool { return f.jellyfin.Streams() == 1 })

	f.jellyfin.AddItem(library.JellyfinItem{ID: movieItemID, Path: imported.Path, Type: "Movie"}, time.Now())
	f.jellyfin.PushChange(library.LibraryChange{Added: []string{movieItemID}})
	waitUntil(t, "ItemAvailable is published", func() bool { return len(f.available(t)) == 1 })

	cancel()
	select {
	case <-stopped:
	case <-time.After(waitForWatch):
		t.Fatal("WatchJellyfin didn't return after ctx ended")
	}
}

func waitUntil(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(waitForWatch)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting until %s", what)
		}
		time.Sleep(pollEvery)
	}
}

func TestAFileJellyfinHasntConfirmedForADayIsOverdue(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	ctx := context.Background()
	imported := f.importMovie(t)
	file, err := f.repo.File(ctx, imported.MediaFile)
	if err != nil {
		t.Fatal(err)
	}

	library.SetClock(f.service, func() time.Time { return file.ImportedAt.Add(23 * time.Hour) })
	if f.service.JellyfinOverdue(file) {
		t.Error("overdue before a day has passed")
	}
	library.SetClock(f.service, func() time.Time { return file.ImportedAt.Add(25 * time.Hour) })
	if !f.service.JellyfinOverdue(file) {
		t.Error("not overdue after a day")
	}
	file.Available = true
	if f.service.JellyfinOverdue(file) {
		t.Error("a confirmed file is overdue")
	}
}

func TestReconcileScheduleStaysWithinItsBounds(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	ctx := context.Background()

	saved, err := f.service.Schedules(ctx)
	if err != nil || saved.JellyfinReconcile.Std() != library.DefaultJellyfinReconcile {
		t.Errorf("default = %+v, %v", saved, err)
	}
	for _, interval := range []time.Duration{library.MinJellyfinReconcile - time.Hour, library.MaxJellyfinReconcile + time.Hour} {
		err := f.service.SetSchedules(ctx, library.Schedules{JellyfinReconcile: settings.Duration(interval)})
		if !errors.Is(err, library.ErrInvalidSchedules) {
			t.Errorf("%s err = %v, want ErrInvalidSchedules", interval, err)
		}
	}
	if err := f.service.SetSchedules(ctx, library.Schedules{JellyfinReconcile: settings.Duration(library.MinJellyfinReconcile)}); err != nil {
		t.Fatal(err)
	}
}
