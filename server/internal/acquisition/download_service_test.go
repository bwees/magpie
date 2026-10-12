package acquisition_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"magpie/internal/acquisition"
	"magpie/internal/acquisition/api"
	"magpie/internal/acquisition/fakes"
	"magpie/internal/domain"
	"magpie/internal/platform/settings"
)

const (
	otherName    = "Movie.Title.2024.1080p.WEB-DL.DDP5.1.H.264-OTHER"
	thirdName    = "Movie.Title.2024.1080p.WEBRip.DDP5.1.x264-THIRD"
	downloadLink = "https://prowlarr.test/1/download?file="
	pieceBytes   = 20
)

func bencodeString(s string) string {
	return fmt.Sprintf("%d:%s", len(s), s)
}

// torrentFile bencodes a multi-file .torrent whose files sit under a root folder called name.
func torrentFile(name string, files ...string) []byte {
	var list strings.Builder
	for _, file := range files {
		list.WriteString("d" + bencodeString("length") + "i1e" + bencodeString("path") + "l")
		for _, segment := range strings.Split(file, "/") {
			list.WriteString(bencodeString(segment))
		}
		list.WriteString("ee")
	}
	info := "d" + bencodeString("files") + "l" + list.String() + "e" +
		bencodeString("name") + bencodeString(name) +
		bencodeString("piece length") + "i16384e" +
		bencodeString("pieces") + bencodeString(strings.Repeat("x", pieceBytes)) + "e"
	return []byte("d" + bencodeString("info") + info + "e")
}

// torrentRelease is a release offered as a .torrent through Prowlarr.
func (f searchFixture) torrentRelease(name string, files ...string) acquisition.TorznabItem {
	item := release(name)
	item.MagnetURI = ""
	item.DownloadURL = downloadLink + name
	f.prowlarr.SetDownload(item.DownloadURL, acquisition.TorrentPayload{Torrent: torrentFile(name, files...)})
	return item
}

// searched runs an interactive search over one indexer offering items and waits for it to finish.
func (f searchFixture) searched(t *testing.T, items ...acquisition.TorznabItem) acquisition.SearchSnapshot {
	t.Helper()
	f.indexer(t, fastID, "Fast", idCaps, 0, items...)
	f.start(t)
	done := f.completed(t)
	snapshot, err := f.searches.Search(done.Search)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func releaseNamed(t *testing.T, s acquisition.SearchSnapshot, name string) domain.ReleaseID {
	t.Helper()
	for _, r := range s.Releases {
		if r.Name == name {
			return r.Release
		}
	}
	t.Fatalf("no release %s in the search", name)
	return ""
}

func (f searchFixture) grab(t *testing.T, s acquisition.SearchSnapshot, name string) acquisition.GrabResult {
	t.Helper()
	result, err := f.downloads.Grab(context.Background(), s.ID, releaseNamed(t, s, name), false)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// poll runs one qBittorrent poll and the check jobs it queued.
func (f searchFixture) poll(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if err := f.downloads.Poll(ctx, f.jobs); err != nil {
		t.Fatal(err)
	}
	queued := f.jobs.tasks
	f.jobs.tasks = nil
	for _, task := range queued {
		id, isCheck := acquisition.DownloadCheckTarget(task)
		if !isCheck {
			f.jobs.tasks = append(f.jobs.tasks, task)
			continue
		}
		if err := f.downloads.Check(ctx, domain.DownloadID(id)); err != nil {
			t.Fatal(err)
		}
	}
}

func (f searchFixture) blocklist(t *testing.T) []api.BlocklistEntry {
	t.Helper()
	entries, err := f.repo.Blocklist(context.Background(), f.title.ID)
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

func (f searchFixture) download(t *testing.T, id domain.DownloadID) api.Download {
	t.Helper()
	list, err := f.repo.DownloadsForTitle(context.Background(), f.title.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range list {
		if d.ID == id {
			return d
		}
	}
	t.Fatalf("no download %s", id)
	return api.Download{}
}

func (f searchFixture) setOptions(t *testing.T, change func(*acquisition.DownloadOptions)) {
	t.Helper()
	ctx := context.Background()
	o, err := f.downloads.Options(ctx)
	if err != nil {
		t.Fatal(err)
	}
	change(&o)
	if _, err := f.downloads.SetOptions(ctx, o); err != nil {
		t.Fatal(err)
	}
}

func magnetHash(name string) string {
	return fakes.HashOf(acquisition.AddRequest{Magnet: magnet(name)})
}

func torrentHash(name string, files ...string) string {
	return fakes.HashOf(acquisition.AddRequest{Torrent: torrentFile(name, files...)})
}

func (f searchFixture) assertRejected(t *testing.T, reason api.RejectionReason, next *domain.DownloadID) api.ReleaseRejected {
	t.Helper()
	rejected := payload[api.ReleaseRejected](t, f.events.waitFor(t, api.TypeReleaseRejected, 1)[0])
	if rejected.Reason != reason || rejected.Title != f.title.ID {
		t.Errorf("ReleaseRejected = %+v, want %s", rejected, reason)
	}
	if (next == nil) != (rejected.Next == nil) || (next != nil && *next != *rejected.Next) {
		t.Errorf("ReleaseRejected.Next = %v, want %v", rejected.Next, next)
	}
	return rejected
}

func (f searchFixture) assertBlocklisted(t *testing.T, reason api.BlocklistReason, expires *time.Time) api.BlocklistEntry {
	t.Helper()
	entries := f.blocklist(t)
	if len(entries) != 1 || entries[0].Reason != reason {
		t.Fatalf("blocklist = %+v, want one %s entry", entries, reason)
	}
	got := entries[0].ExpiresAt
	if (expires == nil) != (got == nil) || (expires != nil && !got.Equal(*expires)) {
		t.Errorf("%s entry expires %v, want %v", reason, got, expires)
	}
	return entries[0]
}

// Seed acquisition #7.
func TestABlockedTorrentIsNeverAddedAndTheNextReleaseIsGrabbed(t *testing.T) {
	f := newSearchFixture(t)
	clean := []string{"Movie.mkv"}
	s := f.searched(t, f.torrentRelease(acceptedName, "Movie.exe", "Movie.mkv"), f.torrentRelease(otherName, clean...))

	result := f.grab(t, s, acceptedName)

	if result.Status != acquisition.GrabRejected || result.BlockedFile == nil || *result.BlockedFile != "Movie.exe" || result.Next == nil {
		t.Fatalf("grab = %+v, want REJECTED for Movie.exe with a next grab", result)
	}
	if n := f.qbit.Called(fakes.QBittorrentAddTorrent); n != 1 {
		t.Errorf("AddTorrent called %d times, want once, for the next release", n)
	}
	nextHash := torrentHash(otherName, clean...)
	if added, ok := f.qbit.Added(nextHash); !ok || !added.Stopped || !f.qbit.Started(nextHash) {
		t.Errorf("next release added %+v (found %v), want added stopped and then started", added, ok)
	}
	rejected := f.assertRejected(t, api.RejectBlockedFile, result.Next)
	if rejected.BlockedFile == nil || *rejected.BlockedFile != "Movie.exe" || rejected.Release != acceptedName || rejected.InfoHash == "" {
		t.Errorf("ReleaseRejected = %+v", rejected)
	}
	entry := f.assertBlocklisted(t, api.BlocklistBlockedFile, nil)
	if entry.InfoHash != rejected.InfoHash || entry.ReleaseName != acceptedName || entry.Indexer != "Fast" {
		t.Errorf("blocklist entry = %+v", entry)
	}
	grabbed := payload[api.ReleaseGrabbed](t, f.events.waitFor(t, api.TypeReleaseGrabbed, 1)[0])
	if grabbed.Download != *result.Next || grabbed.Release != otherName || grabbed.Override {
		t.Errorf("ReleaseGrabbed = %+v, want the next release", grabbed)
	}
	if d := f.download(t, *result.Next); d.State != api.StateDownloading {
		t.Errorf("next download state = %s", d.State)
	}
}

// Seed acquisition #11, fed by a rejection.
func TestABlocklistedReleaseIsRejectedByTheNextSearch(t *testing.T) {
	f := newSearchFixture(t)
	s := f.searched(t, f.torrentRelease(acceptedName, "Movie.exe"))
	f.grab(t, s, acceptedName)

	f.start(t)
	again := f.events.waitFor(t, api.TypeSearchCompleted, 2)[1]
	snapshot, err := f.searches.Search(payload[api.SearchCompleted](t, again).Search)
	if err != nil {
		t.Fatal(err)
	}
	if r := snapshot.Releases[0]; len(r.Rejections) == 0 || r.Rejections[0].Reason != api.RejectBlocklisted {
		t.Errorf("release = %+v, want BLOCKLISTED", r)
	}
}

// Seed acquisition #8.
func TestABlockedMagnetIsDeletedWhenItsMetadataArrives(t *testing.T) {
	f := newSearchFixture(t)
	s := f.searched(t, release(acceptedName), release(otherName))

	result := f.grab(t, s, acceptedName)
	if result.Status != acquisition.GrabChecking {
		t.Fatalf("grab = %+v, want CHECKING", result)
	}
	hash := magnetHash(acceptedName)
	if added, _ := f.qbit.Added(hash); added.StopCondition != acquisition.StopMetadataReceived || added.Magnet == "" {
		t.Errorf("magnet added as %+v, want stopCondition=MetadataReceived", added)
	}
	f.qbit.SetFiles(hash, acquisition.TorrentFile{Path: "Show.S01E01.rar", Size: 1, Priority: 1})
	f.poll(t)

	if !slices.Contains(f.qbit.Deletions(), fakes.Deletion{Hash: hash, DeleteFiles: true}) {
		t.Errorf("deletions = %+v, want %s with its files", f.qbit.Deletions(), hash)
	}
	if n := f.qbit.Called(fakes.QBittorrentStart); n != 0 {
		t.Errorf("Start called %d times before the check passed", n)
	}
	f.assertBlocklisted(t, api.BlocklistBlockedFile, nil)
	grabbed := f.events.waitFor(t, api.TypeReleaseGrabbed, 2)
	next := payload[api.ReleaseGrabbed](t, grabbed[1])
	rejected := f.assertRejected(t, api.RejectBlockedFile, &next.Download)
	if rejected.BlockedFile == nil || *rejected.BlockedFile != "Show.S01E01.rar" {
		t.Errorf("BlockedFile = %v", rejected.BlockedFile)
	}
	if d := f.download(t, *result.Download); d.State != api.StateFailed || d.RemovedAt == nil {
		t.Errorf("rejected download = %+v, want FAILED and removed", d)
	}
}

func TestACleanMagnetStartsAfterItsCheck(t *testing.T) {
	f := newSearchFixture(t)
	s := f.searched(t, release(acceptedName))
	result := f.grab(t, s, acceptedName)
	hash := magnetHash(acceptedName)

	f.poll(t)
	if f.qbit.Started(hash) {
		t.Fatal("started before the metadata arrived")
	}
	f.qbit.SetFiles(hash, acquisition.TorrentFile{Path: "Movie.mkv", Size: 1, Priority: 1})
	f.poll(t)

	if !f.qbit.Started(hash) {
		t.Error("a clean magnet wasn't started")
	}
	if d := f.download(t, *result.Download); d.State != api.StateDownloading {
		t.Errorf("state = %s, want DOWNLOADING", d.State)
	}
	if n := len(f.events.ofType(api.TypeReleaseRejected)); n != 0 {
		t.Errorf("%d rejections for a clean magnet", n)
	}
}

// Seed acquisition #9.
func TestAMetadataTimeoutDeletesAndBlocklistsFor24Hours(t *testing.T) {
	f := newSearchFixture(t)
	s := f.searched(t, release(acceptedName))
	f.grab(t, s, acceptedName)
	hash := magnetHash(acceptedName)

	f.clock.Advance(acquisition.DefaultMetadataTimeout - time.Second)
	f.poll(t)
	if len(f.qbit.Deletions()) != 0 {
		t.Fatal("deleted before the metadata timeout")
	}
	f.clock.Advance(2 * time.Second)
	f.poll(t)

	if !slices.Equal(f.qbit.Deletions(), []fakes.Deletion{{Hash: hash, DeleteFiles: true}}) {
		t.Errorf("deletions = %+v", f.qbit.Deletions())
	}
	expires := f.clock.Now().UTC().Add(acquisition.TemporaryBlockFor)
	f.assertBlocklisted(t, api.BlocklistMetadataTimeout, &expires)
	f.assertRejected(t, api.RejectMetadataTimeout, nil)

	var searches []acquisition.SearchRequest
	for _, task := range f.jobs.tasks {
		if req, ok := acquisition.SearchTaskOf(task); ok {
			searches = append(searches, req)
		}
	}
	if len(searches) != 1 || searches[0].Title != f.title.ID || searches[0].Trigger != api.TriggerAuto {
		t.Errorf("queued searches = %+v, want one AUTO search for the title", searches)
	}
}

// Seed acquisition #10.
func TestWithTheCheckOffABlockedTorrentIsAddedAndStarted(t *testing.T) {
	f := newSearchFixture(t)
	f.setOptions(t, func(o *acquisition.DownloadOptions) { o.BlockedFilesEnabled = false })
	files := []string{"Movie.rar"}
	s := f.searched(t, f.torrentRelease(acceptedName, files...))

	result := f.grab(t, s, acceptedName)

	if result.Status != acquisition.GrabDownloading || !f.qbit.Started(torrentHash(acceptedName, files...)) {
		t.Errorf("grab = %+v, want it added and started", result)
	}
}

// Seed acquisition #24.
func TestAnOldQBittorrentRefusesGrabs(t *testing.T) {
	f := newSearchFixture(t)
	f.qbit.SetVersion(acquisition.WebAPIVersion{Major: 2, Minor: 11})
	s := f.searched(t, f.torrentRelease(acceptedName, "Movie.mkv"))

	if err := f.downloads.CheckClient(context.Background()); !errors.Is(err, acquisition.ErrUnsupportedVersion) {
		t.Errorf("CheckClient = %v, want ErrUnsupportedVersion", err)
	}
	_, err := f.downloads.Grab(context.Background(), s.ID, releaseNamed(t, s, acceptedName), false)
	if !errors.Is(err, acquisition.ErrUnsupportedVersion) {
		t.Errorf("grab = %v, want ErrUnsupportedVersion", err)
	}
	if f.qbit.Called(fakes.QBittorrentAddTorrent) != 0 || f.prowlarr.Called(fakes.ProwlarrDownload) != 0 {
		t.Error("a grab was attempted")
	}
	if _, err := f.downloads.Grab(context.Background(), s.ID, releaseNamed(t, s, acceptedName), false); !errors.Is(err, acquisition.ErrUnsupportedVersion) {
		t.Errorf("a refused grab used up the release: %v", err)
	}
}

// startedMagnet grabs a release as a magnet and lets it pass its metadata check.
func (f searchFixture) startedMagnet(t *testing.T, s acquisition.SearchSnapshot, name string, files ...acquisition.TorrentFile) domain.DownloadID {
	t.Helper()
	result := f.grab(t, s, name)
	f.qbit.SetFiles(magnetHash(name), files...)
	f.poll(t)
	if !f.qbit.Started(magnetHash(name)) {
		t.Fatalf("%s didn't pass its check", name)
	}
	return *result.Download
}

func finish(contentPath string) func(*acquisition.Torrent) {
	return func(tr *acquisition.Torrent) {
		tr.Progress, tr.Status, tr.ContentPath = 1, acquisition.TorrentUploading, contentPath
	}
}

// Seed acquisition #27.
func TestTheCompletionBackstopRejectsALateBlockedFile(t *testing.T) {
	f := newSearchFixture(t)
	s := f.searched(t, release(acceptedName), release(otherName))
	movie := acquisition.TorrentFile{Path: "Movie.mkv", Size: 1, Priority: 1}
	id := f.startedMagnet(t, s, acceptedName, movie)
	hash := magnetHash(acceptedName)

	f.qbit.SetFiles(hash, movie, acquisition.TorrentFile{Path: "extras/setup.exe", Size: 1, Priority: 1})
	f.qbit.Update(hash, finish("/downloads/movie"))
	f.poll(t)

	if n := len(f.events.ofType(api.TypeDownloadCompleted)); n != 0 {
		t.Errorf("%d DownloadCompleted for a blocked download", n)
	}
	if !slices.Contains(f.qbit.Deletions(), fakes.Deletion{Hash: hash, DeleteFiles: true}) {
		t.Errorf("deletions = %+v", f.qbit.Deletions())
	}
	f.assertBlocklisted(t, api.BlocklistBlockedFile, nil)
	next := payload[api.ReleaseGrabbed](t, f.events.waitFor(t, api.TypeReleaseGrabbed, 2)[1])
	rejected := f.assertRejected(t, api.RejectBlockedFile, &next.Download)
	if rejected.BlockedFile == nil || *rejected.BlockedFile != "extras/setup.exe" || next.Release != otherName {
		t.Errorf("rejected %+v, next %+v", rejected, next)
	}
	if d := f.download(t, id); d.State != api.StateFailed {
		t.Errorf("state = %s, want FAILED", d.State)
	}
}

// Seed acquisition #21's payload, through the poll.
func TestACompletedDownloadIsPublishedOnceWithItsWantedFiles(t *testing.T) {
	f := newSearchFixture(t)
	s := f.searched(t, release(acceptedName))
	episode := acquisition.TorrentFile{Path: "S01E01.mkv", Size: 7, Priority: 1}
	id := f.startedMagnet(t, s, acceptedName, episode, acquisition.TorrentFile{Path: "sample.mkv", Size: 1, Priority: 0})

	f.qbit.Update(magnetHash(acceptedName), finish("/downloads/movie"))
	f.poll(t)
	f.poll(t)

	completed := f.events.waitFor(t, api.TypeDownloadCompleted, 1)
	time.Sleep(quietPeriod)
	if n := len(f.events.ofType(api.TypeDownloadCompleted)); n != 1 {
		t.Fatalf("%d DownloadCompleted, want exactly one", n)
	}
	got := payload[api.DownloadCompleted](t, completed[0])
	wantFiles := []api.DownloadFile{{Path: "S01E01.mkv", Size: 7}}
	if got.Download != id || got.ContentPath != "/downloads/movie" || !slices.Equal(got.Files, wantFiles) || got.SeedingMode != api.SeedUntilGoals {
		t.Errorf("DownloadCompleted = %+v", got)
	}
	if d := f.download(t, id); d.State != api.StateCompleted || d.CompletedAt == nil {
		t.Errorf("download = %+v, want COMPLETED", d)
	}
}

// Seed acquisition #28.
func TestAStalledDownloadFailsAfterTheLimit(t *testing.T) {
	f := newSearchFixture(t)
	s := f.searched(t, f.torrentRelease(acceptedName, "Movie.mkv"), f.torrentRelease(otherName, "Movie.mkv"))
	result := f.grab(t, s, acceptedName)
	hash := torrentHash(acceptedName, "Movie.mkv")
	f.qbit.Update(hash, func(tr *acquisition.Torrent) { tr.Progress, tr.Status = 0.4, acquisition.TorrentStalledDL })
	f.poll(t)

	f.clock.Advance(acquisition.DefaultStalledLimit + time.Minute)
	f.poll(t)

	failed := payload[api.DownloadFailed](t, f.events.waitFor(t, api.TypeDownloadFailed, 1)[0])
	if failed.Download != *result.Download || failed.Title != f.title.ID || failed.Reason == "" {
		t.Errorf("DownloadFailed = %+v", failed)
	}
	expires := f.clock.Now().UTC().Add(acquisition.TemporaryBlockFor)
	f.assertBlocklisted(t, api.BlocklistStalled, &expires)
	next := payload[api.ReleaseGrabbed](t, f.events.waitFor(t, api.TypeReleaseGrabbed, 2)[1])
	f.assertRejected(t, api.RejectStalled, &next.Download)
	if !slices.Contains(f.qbit.Deletions(), fakes.Deletion{Hash: hash, DeleteFiles: true}) {
		t.Errorf("deletions = %+v", f.qbit.Deletions())
	}
}

func TestALongerStalledLimitLeavesTheDownloadAlone(t *testing.T) {
	const longerLimit, stall = 12 * time.Hour, 7 * time.Hour
	f := newSearchFixture(t)
	f.setOptions(t, func(o *acquisition.DownloadOptions) { o.StalledLimit = settings.Duration(longerLimit) })
	s := f.searched(t, f.torrentRelease(acceptedName, "Movie.mkv"))
	f.grab(t, s, acceptedName)
	f.poll(t)

	f.clock.Advance(stall)
	f.poll(t)

	if len(f.qbit.Deletions()) != 0 || len(f.events.ofType(api.TypeDownloadFailed)) != 0 {
		t.Errorf("a %s stall under a %s limit was dropped", stall, longerLimit)
	}
}

func TestAnErroredDownloadFailsAndIsBlocklistedPermanently(t *testing.T) {
	f := newSearchFixture(t)
	s := f.searched(t, f.torrentRelease(acceptedName, "Movie.mkv"))
	result := f.grab(t, s, acceptedName)
	f.qbit.Update(torrentHash(acceptedName, "Movie.mkv"), func(tr *acquisition.Torrent) { tr.Status = acquisition.TorrentMissingFiles })

	f.poll(t)

	failed := payload[api.DownloadFailed](t, f.events.waitFor(t, api.TypeDownloadFailed, 1)[0])
	if failed.Download != *result.Download {
		t.Errorf("DownloadFailed = %+v", failed)
	}
	f.assertBlocklisted(t, api.BlocklistDownloadFailed, nil)
	f.assertRejected(t, api.RejectDownloadFailed, nil)
}

func TestProgressIsEmittedOnlyWhenItChanges(t *testing.T) {
	f := newSearchFixture(t)
	s := f.searched(t, f.torrentRelease(acceptedName, "Movie.mkv"))
	result := f.grab(t, s, acceptedName)
	hash := torrentHash(acceptedName, "Movie.mkv")
	f.qbit.Update(hash, func(tr *acquisition.Torrent) {
		tr.Progress, tr.DownloadBps, tr.Status = 0.25, 1<<20, acquisition.TorrentDownloading
	})

	f.poll(t)
	f.poll(t)
	f.qbit.Update(hash, func(tr *acquisition.Torrent) { tr.Progress = 0.5 })
	f.poll(t)

	progress := f.events.waitFor(t, api.TypeDownloadProgressed, 2)
	time.Sleep(quietPeriod)
	if n := len(f.events.ofType(api.TypeDownloadProgressed)); n != 2 {
		t.Errorf("%d DownloadProgressed, want 2", n)
	}
	last := payload[api.DownloadProgressed](t, progress[1])
	if last.Download != *result.Download || last.Progress != 0.5 || last.State != api.StateDownloading {
		t.Errorf("DownloadProgressed = %+v", last)
	}
	views, err := f.downloads.Downloads(context.Background())
	if err != nil || len(views) != 1 || views[0].Live == nil || views[0].Live.Progress != 0.5 || views[0].TitleName != f.title.Name {
		t.Errorf("downloads = %+v, %v", views, err)
	}
}

func TestPollPaceFollowsActiveDownloads(t *testing.T) {
	f := newSearchFixture(t)
	ctx := context.Background()
	if pace, err := f.downloads.PollInterval(ctx); err != nil || pace != acquisition.IdlePollInterval {
		t.Errorf("idle pace = %s, %v", pace, err)
	}
	s := f.searched(t, release(acceptedName))
	f.grab(t, s, acceptedName)
	if pace, err := f.downloads.PollInterval(ctx); err != nil || pace != acquisition.ActivePollInterval {
		t.Errorf("active pace = %s, %v", pace, err)
	}
}

func TestRemovingADownloadDeletesItWithoutBlocklisting(t *testing.T) {
	f := newSearchFixture(t)
	s := f.searched(t, release(acceptedName))
	result := f.grab(t, s, acceptedName)

	if _, err := f.downloads.Remove(context.Background(), *result.Download, true); err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(f.qbit.Deletions(), []fakes.Deletion{{Hash: magnetHash(acceptedName), DeleteFiles: true}}) {
		t.Errorf("deletions = %+v", f.qbit.Deletions())
	}
	if d := f.download(t, *result.Download); d.State != api.StateRemoved || d.RemovedAt == nil {
		t.Errorf("download = %+v, want REMOVED", d)
	}
	if len(f.blocklist(t)) != 0 {
		t.Error("removing a download blocklisted it")
	}
	views, err := f.downloads.Downloads(context.Background())
	if err != nil || len(views) != 0 {
		t.Errorf("downloads after removal = %+v, %v", views, err)
	}
}

func TestATorrentRemovedInQBittorrentIsMarkedRemoved(t *testing.T) {
	f := newSearchFixture(t)
	s := f.searched(t, release(acceptedName))
	result := f.grab(t, s, acceptedName)
	if err := f.qbit.Delete(context.Background(), false, magnetHash(acceptedName)); err != nil {
		t.Fatal(err)
	}

	f.clock.Advance(2 * time.Minute)
	f.poll(t)

	if d := f.download(t, *result.Download); d.State != api.StateRemoved {
		t.Errorf("state = %s, want REMOVED", d.State)
	}
}

func TestQueuedDownloadsFeedTheQueueSpec(t *testing.T) {
	f := newSearchFixture(t)
	s := f.searched(t, release(acceptedName), release(thirdName))
	f.grab(t, s, acceptedName)

	f.start(t)
	again := payload[api.SearchCompleted](t, f.events.waitFor(t, api.TypeSearchCompleted, 2)[1])
	snapshot, err := f.searches.Search(again.Search)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range snapshot.Releases {
		reasons := []api.RejectionReason{}
		for _, rejection := range r.Rejections {
			reasons = append(reasons, rejection.Reason)
		}
		if !slices.Contains(reasons, api.RejectAlreadyQueued) {
			t.Errorf("%s rejected for %v, want ALREADY_QUEUED", r.Name, reasons)
		}
	}
}

func TestDownloadOptionsValidate(t *testing.T) {
	valid := acquisition.DownloadOptions{
		BlockedFilesEnabled: true,
		BlockedPatterns:     []string{"*.exe", "regex:^setup\\."},
		MetadataTimeout:     settings.Duration(acquisition.DefaultMetadataTimeout),
		StalledLimit:        settings.Duration(acquisition.DefaultStalledLimit),
	}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid options = %v", err)
	}
	for name, change := range map[string]func(*acquisition.DownloadOptions){
		"bad regex":             func(o *acquisition.DownloadOptions) { o.BlockedPatterns = []string{"regex:("} },
		"folder glob":           func(o *acquisition.DownloadOptions) { o.BlockedPatterns = []string{"extras/*.exe"} },
		"metadata timeout low":  func(o *acquisition.DownloadOptions) { o.MetadataTimeout = settings.Duration(time.Second) },
		"metadata timeout high": func(o *acquisition.DownloadOptions) { o.MetadataTimeout = settings.Duration(2 * time.Hour) },
		"stalled limit low":     func(o *acquisition.DownloadOptions) { o.StalledLimit = settings.Duration(time.Minute) },
		"stalled limit high":    func(o *acquisition.DownloadOptions) { o.StalledLimit = settings.Duration(73 * time.Hour) },
	} {
		o := valid
		change(&o)
		if err := o.Validate(); !errors.Is(err, acquisition.ErrInvalidDownloadOptions) {
			t.Errorf("%s = %v, want ErrInvalidDownloadOptions", name, err)
		}
	}
}
