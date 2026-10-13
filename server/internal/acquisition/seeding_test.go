package acquisition_test

import (
	"context"
	"net/http"
	"slices"
	"testing"
	"time"

	"magpie/internal/acquisition"
	"magpie/internal/acquisition/api"
	"magpie/internal/acquisition/fakes"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/platform/bus"
)

const (
	contentPath = "/downloads/movie"
	globalRatio = 2.0
	globalDays  = 14
	day         = 24 * time.Hour
)

// completedDownload grabs a release and lets it finish in qBittorrent.
func (f searchFixture) completedDownload(t *testing.T, names ...string) domain.DownloadID {
	t.Helper()
	items := make([]acquisition.TorznabItem, 0, len(names))
	for _, name := range names {
		items = append(items, release(name))
	}
	s := f.searched(t, items...)
	id := f.startedMagnet(t, s, names[0], acquisition.TorrentFile{Path: "Movie.mkv", Size: 1, Priority: 1})
	f.qbit.Update(magnetHash(names[0]), finish(contentPath))
	f.poll(t)
	if d := f.download(t, id); d.State != api.StateCompleted {
		t.Fatalf("download state = %s, want COMPLETED", d.State)
	}
	return id
}

func (f searchFixture) setSeeding(t *testing.T, mode api.SeedingMode) {
	t.Helper()
	if err := acquisition.SeedingModeKey.Set(context.Background(), f.store, mode); err != nil {
		t.Fatal(err)
	}
}

// overrideIndexer changes the settings of the indexer completedDownload searches; a later sync keeps them.
func (f searchFixture) overrideIndexer(t *testing.T, change func(*acquisition.IndexerSettings)) {
	t.Helper()
	f.indexer(t, fastID, "Fast", idCaps, 0)
	settings := acquisition.DefaultIndexerSettings()
	change(&settings)
	if _, err := f.service.UpdateIndexer(context.Background(), fastID, settings); err != nil {
		t.Fatal(err)
	}
}

// deliverLibrary hands a Library event to Acquisition's handlers and runs the jobs they queued.
func (f searchFixture) deliverLibrary(t *testing.T, m domain.Message) {
	t.Helper()
	for _, h := range acquisition.LibraryHandlers(f.downloads) {
		if slices.Contains(h.EventTypes, m.Type()) {
			f.deliver(t, h, bus.EventOf(m))
		}
	}
	queued := f.jobs.queued()
	f.jobs.drop(acquisition.IsImportJob)
	for _, task := range queued {
		if _, err := acquisition.RunDownloadTask(context.Background(), f.downloads, task); err != nil {
			t.Fatal(err)
		}
	}
}

func imported(id domain.DownloadID) libraryapi.FileImported {
	return libraryapi.FileImported{
		MediaFile: domain.NewID[domain.MediaFileID](), Title: domain.NewID[domain.TitleID](),
		Download: &id, Origin: domain.OriginDownload,
	}
}

func (f searchFixture) checkGoals(t *testing.T) {
	t.Helper()
	if err := f.downloads.CheckSeedingGoals(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// Seed acquisition #19.
func TestASeedingDownloadIsRemovedWithItsDataOnceTheRatioGoalIsMet(t *testing.T) {
	f := newSearchFixture(t)
	id := f.completedDownload(t, acceptedName)
	hash := magnetHash(acceptedName)

	f.deliverLibrary(t, imported(id))
	if len(f.qbit.Deletions()) != 0 {
		t.Fatalf("deletions = %+v, want the torrent to keep seeding", f.qbit.Deletions())
	}
	if d := f.download(t, id); d.State != api.StateImported || d.RemovedAt != nil {
		t.Errorf("download = %+v, want IMPORTED and still in qBittorrent", d)
	}

	f.qbit.Update(hash, func(tr *acquisition.Torrent) { tr.Ratio = globalRatio - 0.1 })
	f.checkGoals(t)
	if len(f.qbit.Deletions()) != 0 {
		t.Fatal("removed before the ratio goal")
	}
	f.qbit.Update(hash, func(tr *acquisition.Torrent) { tr.Ratio = globalRatio })
	f.checkGoals(t)

	if !slices.Equal(f.qbit.Deletions(), []fakes.Deletion{{Hash: hash, DeleteFiles: true}}) {
		t.Errorf("deletions = %+v, want the torrent with its data", f.qbit.Deletions())
	}
	if d := f.download(t, id); d.State != api.StateImported || d.RemovedAt == nil {
		t.Errorf("download = %+v, want IMPORTED and removed", d)
	}
}

func TestTheSeedingTimeGoalAlsoRemovesTheDownload(t *testing.T) {
	f := newSearchFixture(t)
	id := f.completedDownload(t, acceptedName)
	hash := magnetHash(acceptedName)
	f.deliverLibrary(t, imported(id))

	f.qbit.Update(hash, func(tr *acquisition.Torrent) { tr.SeedingTime = globalDays*day - time.Minute })
	f.checkGoals(t)
	if len(f.qbit.Deletions()) != 0 {
		t.Fatal("removed before the seeding time goal")
	}
	f.qbit.Update(hash, func(tr *acquisition.Torrent) { tr.SeedingTime = globalDays * day })
	f.checkGoals(t)

	if !slices.Equal(f.qbit.Deletions(), []fakes.Deletion{{Hash: hash, DeleteFiles: true}}) {
		t.Errorf("deletions = %+v", f.qbit.Deletions())
	}
}

func TestAnIndexerGoalWinsOverTheGlobalGoal(t *testing.T) {
	f := newSearchFixture(t)
	ratio := 1.0
	f.overrideIndexer(t, func(s *acquisition.IndexerSettings) { s.SeedRatio = &ratio })
	id := f.completedDownload(t, acceptedName)
	hash := magnetHash(acceptedName)
	f.deliverLibrary(t, imported(id))

	f.qbit.Update(hash, func(tr *acquisition.Torrent) { tr.Ratio = ratio })
	f.checkGoals(t)

	if !slices.Equal(f.qbit.Deletions(), []fakes.Deletion{{Hash: hash, DeleteFiles: true}}) {
		t.Errorf("deletions = %+v, want removal at the indexer's ratio", f.qbit.Deletions())
	}
}

// Seed acquisition #20.
func TestADontSeedDownloadIsRemovedWithoutItsDataOnImport(t *testing.T) {
	f := newSearchFixture(t)
	f.setSeeding(t, api.DontSeed)
	id := f.completedDownload(t, acceptedName)

	f.deliverLibrary(t, imported(id))
	f.deliverLibrary(t, imported(id))

	want := []fakes.Deletion{{Hash: magnetHash(acceptedName), DeleteFiles: false}}
	if !slices.Equal(f.qbit.Deletions(), want) {
		t.Errorf("deletions = %+v, want %+v once", f.qbit.Deletions(), want)
	}
	if d := f.download(t, id); d.State != api.StateImported || d.RemovedAt == nil {
		t.Errorf("download = %+v, want IMPORTED and removed", d)
	}
}

// Seed acquisition #20, with an indexer override.
func TestAnIndexerOverrideKeepsSeedingDespiteAGlobalDontSeed(t *testing.T) {
	f := newSearchFixture(t)
	f.setSeeding(t, api.DontSeed)
	f.overrideIndexer(t, func(s *acquisition.IndexerSettings) { s.SeedingOverride = acquisition.OverrideSeedUntilGoals })
	id := f.completedDownload(t, acceptedName)

	f.deliverLibrary(t, imported(id))

	if len(f.qbit.Deletions()) != 0 {
		t.Errorf("deletions = %+v, want the torrent to stay", f.qbit.Deletions())
	}
}

// Seed acquisition #21, the indexer override part.
func TestDownloadCompletedCarriesTheIndexerOverride(t *testing.T) {
	f := newSearchFixture(t)
	f.overrideIndexer(t, func(s *acquisition.IndexerSettings) { s.SeedingOverride = acquisition.OverrideDontSeed })

	f.completedDownload(t, acceptedName)

	completed := payload[api.DownloadCompleted](t, f.events.waitFor(t, api.TypeDownloadCompleted, 1)[0])
	if completed.SeedingMode != api.DontSeed || completed.InfoHash != magnetHash(acceptedName) {
		t.Errorf("DownloadCompleted = %+v, want DONT_SEED from the indexer and the info hash", completed)
	}
}

func TestDownloadOptionsCarryTheGlobalSeedingSettings(t *testing.T) {
	f := newSearchFixture(t)
	handler := serveSearches(f, admin())
	const path = "/api/acquisition/download-options"

	if got := decode[acquisition.DownloadOptionsResponse](t, do(handler, http.MethodGet, path, "")); got.SeedingMode != api.SeedUntilGoals ||
		got.SeedRatio != acquisition.DefaultSeedRatio || got.SeedDays != acquisition.DefaultSeedDays {
		t.Errorf("defaults = %+v, want seed until ratio 2 or 14 days", got)
	}
	body := `{"blockedFilesEnabled":true,"blockedPatterns":[],"metadataTimeoutMinutes":5,"stalledLimitMinutes":360,"seedingMode":"DONT_SEED","seedRatio":1.5,"seedDays":7}`
	if rec := do(handler, http.MethodPut, path, body); rec.Code != http.StatusOK {
		t.Fatalf("PUT = %d %s", rec.Code, rec.Body)
	}
	got := decode[acquisition.DownloadOptionsResponse](t, do(handler, http.MethodGet, path, ""))
	if got.SeedingMode != api.DontSeed || got.SeedRatio != 1.5 || got.SeedDays != 7 {
		t.Errorf("saved = %+v", got)
	}

	for _, bad := range []string{
		`{"blockedFilesEnabled":true,"blockedPatterns":[],"metadataTimeoutMinutes":5,"stalledLimitMinutes":360,"seedingMode":"SOMETIMES","seedRatio":2,"seedDays":14}`,
		`{"blockedFilesEnabled":true,"blockedPatterns":[],"metadataTimeoutMinutes":5,"stalledLimitMinutes":360,"seedingMode":"DONT_SEED","seedRatio":101,"seedDays":14}`,
	} {
		if rec := do(handler, http.MethodPut, path, bad); rec.Code != http.StatusBadRequest {
			t.Errorf("PUT %s = %d, want 400", bad, rec.Code)
		}
	}
}

// Seed acquisition #22.
func TestAFailedImportIsBlocklistedDeletedAndTheNextReleaseGrabbed(t *testing.T) {
	f := newSearchFixture(t)
	id := f.completedDownload(t, acceptedName, otherName)

	f.deliverLibrary(t, libraryapi.ImportFailed{Download: id, Reason: "Couldn't tell which episode it is"})

	hash := magnetHash(acceptedName)
	if !slices.Equal(f.qbit.Deletions(), []fakes.Deletion{{Hash: hash, DeleteFiles: true}}) {
		t.Errorf("deletions = %+v, want the torrent with its data", f.qbit.Deletions())
	}
	entry := f.assertBlocklisted(t, api.BlocklistImportFailed, nil)
	if entry.InfoHash != hash {
		t.Errorf("blocklist entry = %+v", entry)
	}
	grabbed := f.events.waitFor(t, api.TypeReleaseGrabbed, 2)
	next := payload[api.ReleaseGrabbed](t, grabbed[1])
	if next.Release != otherName {
		t.Errorf("next grab = %+v, want %s", next, otherName)
	}
	f.assertRejected(t, api.RejectImportFailed, &id, &next.Download)
	if d := f.download(t, id); d.State != api.StateImportFailed || d.RemovedAt == nil {
		t.Errorf("download = %+v, want IMPORT_FAILED and removed", d)
	}

	f.deliverLibrary(t, libraryapi.ImportFailed{Download: id, Reason: "again"})
	if n := len(f.qbit.Deletions()); n != 1 {
		t.Errorf("%d deletions after a second ImportFailed, want 1", n)
	}
}

func manualImport(id domain.DownloadID) libraryapi.ImportFailed {
	return libraryapi.ImportFailed{Download: id, Reason: "Couldn't tell which episode Extras.mkv is", Manual: true}
}

func (f searchFixture) downloadView(t *testing.T, id domain.DownloadID) acquisition.DownloadView {
	t.Helper()
	views, err := f.downloads.Downloads(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range views {
		if v.ID == id {
			return v
		}
	}
	t.Fatalf("download %s isn't in the Downloads tab", id)
	return acquisition.DownloadView{}
}

// Seed acquisition #38.
func TestAManualImportKeepsTheTorrentAndGrabsNothing(t *testing.T) {
	f := newSearchFixture(t)
	id := f.completedDownload(t, acceptedName, otherName)
	hash := magnetHash(acceptedName)

	f.deliverLibrary(t, manualImport(id))
	f.deliverLibrary(t, manualImport(id))

	if d := f.download(t, id); d.State != api.StateImportPendingUser || d.RemovedAt != nil {
		t.Errorf("download = %+v, want IMPORT_PENDING_USER and still in qBittorrent", d)
	}
	if len(f.qbit.Deletions()) != 0 {
		t.Errorf("deletions = %+v, want the torrent and its data kept", f.qbit.Deletions())
	}
	if entries := f.blocklist(t); len(entries) != 0 {
		t.Errorf("blocklist = %+v, want nothing", entries)
	}
	if n := len(f.events.ofType(api.TypeReleaseRejected)); n != 0 {
		t.Errorf("%d ReleaseRejected, want none", n)
	}
	if n := len(f.events.ofType(api.TypeReleaseGrabbed)); n != 1 {
		t.Errorf("%d ReleaseGrabbed, want only the first grab", n)
	}

	f.qbit.Update(hash, func(tr *acquisition.Torrent) { tr.Ratio = globalRatio })
	f.poll(t)
	f.checkGoals(t)
	if len(f.qbit.Deletions()) != 0 {
		t.Errorf("deletions = %+v, want the seeding goal to leave a waiting download", f.qbit.Deletions())
	}
	if v := f.downloadView(t, id); v.Live == nil || v.Live.State != api.StateImportPendingUser {
		t.Errorf("Downloads tab live = %+v, want IMPORT_PENDING_USER", v.Live)
	}
}

// Seed acquisition #38, the queue part.
func TestAManualImportCountsAsQueuedForItsItems(t *testing.T) {
	f := newSearchFixture(t)
	id := f.completedDownload(t, acceptedName)
	f.deliverLibrary(t, manualImport(id))

	f.start(t)
	again := payload[api.SearchCompleted](t, f.events.waitFor(t, api.TypeSearchCompleted, 2)[1])
	snapshot, err := f.searches.Search(again.Search)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range snapshot.Releases {
		if !slices.ContainsFunc(r.Rejections, func(rej api.Rejection) bool { return rej.Reason == api.RejectAlreadyQueued }) {
			t.Errorf("%s rejected for %+v, want ALREADY_QUEUED", r.Name, r.Rejections)
		}
	}
}

// Seed acquisition #39.
func TestMappingAWaitingImportMarksItImportedAndAppliesTheSeedingMode(t *testing.T) {
	f := newSearchFixture(t)
	f.setSeeding(t, api.DontSeed)
	id := f.completedDownload(t, acceptedName)
	f.deliverLibrary(t, manualImport(id))

	f.deliverLibrary(t, imported(id))

	want := []fakes.Deletion{{Hash: magnetHash(acceptedName), DeleteFiles: false}}
	if !slices.Equal(f.qbit.Deletions(), want) {
		t.Errorf("deletions = %+v, want %+v", f.qbit.Deletions(), want)
	}
	if d := f.download(t, id); d.State != api.StateImported || d.RemovedAt == nil {
		t.Errorf("download = %+v, want IMPORTED and removed", d)
	}
}

// Seed acquisition #40.
func TestRemovingAWaitingImportBlocklistsDeletesAndGrabsTheNext(t *testing.T) {
	f := newSearchFixture(t)
	id := f.completedDownload(t, acceptedName, otherName)
	f.deliverLibrary(t, manualImport(id))
	removed := libraryapi.ImportRemoved{Download: id, Title: f.title.ID, ReleaseName: acceptedName}

	f.deliverLibrary(t, removed)

	hash := magnetHash(acceptedName)
	if !slices.Equal(f.qbit.Deletions(), []fakes.Deletion{{Hash: hash, DeleteFiles: true}}) {
		t.Errorf("deletions = %+v, want the torrent with its data", f.qbit.Deletions())
	}
	if entry := f.assertBlocklisted(t, api.BlocklistImportFailed, nil); entry.InfoHash != hash {
		t.Errorf("blocklist entry = %+v", entry)
	}
	next := payload[api.ReleaseGrabbed](t, f.events.waitFor(t, api.TypeReleaseGrabbed, 2)[1])
	if next.Release != otherName {
		t.Errorf("next grab = %+v, want %s", next, otherName)
	}
	f.assertRejected(t, api.RejectImportFailed, &id, &next.Download)
	if d := f.download(t, id); d.State != api.StateImportFailed || d.RemovedAt == nil {
		t.Errorf("download = %+v, want IMPORT_FAILED and removed", d)
	}

	f.deliverLibrary(t, removed)
	if n := len(f.qbit.Deletions()); n != 1 {
		t.Errorf("%d deletions after a redelivered ImportRemoved, want 1", n)
	}
	if n := len(f.blocklist(t)); n != 1 {
		t.Errorf("%d blocklist rows after a redelivered ImportRemoved, want 1", n)
	}
}

func TestRemovingAnImportedDownloadLeavesItAlone(t *testing.T) {
	f := newSearchFixture(t)
	id := f.completedDownload(t, acceptedName, otherName)
	f.deliverLibrary(t, imported(id))

	f.deliverLibrary(t, libraryapi.ImportRemoved{Download: id, Title: f.title.ID, ReleaseName: acceptedName})

	if len(f.qbit.Deletions()) != 0 {
		t.Errorf("deletions = %+v, want the imported torrent kept", f.qbit.Deletions())
	}
	if d := f.download(t, id); d.State != api.StateImported || d.RemovedAt != nil {
		t.Errorf("download = %+v, want IMPORTED", d)
	}
}
