package acquisition_test

import (
	"context"
	"slices"
	"testing"

	"magpie/internal/acquisition"
	"magpie/internal/acquisition/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	librarytest "magpie/internal/library/api/contracttest"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/parse"
	requestsapi "magpie/internal/requests/api"

	"github.com/mikestefanello/backlite"
)

const reportedHash = "3a929e0159bfe761ca2f4b0b3b8dfe0de806c9d9"

func (f searchFixture) libraryFile(name string, transcoded bool, hash string) libraryapi.MediaFile {
	file := libraryapi.MediaFile{
		ID: domain.NewID[domain.MediaFileID](), Title: f.title.ID, Episodes: []domain.EpisodeRef{},
		Path: "/movies/Movie Title (2024)/Movie Title (2024).mkv", Quality: profileQuality(parse.QualityWEBDL1080p),
		ReleaseName: name, InfoHash: hash, Transcoded: transcoded, Origin: domain.OriginDownload,
	}
	f.library.Put(librarytest.StoredFile{File: file})
	return file
}

func (f searchFixture) regrab(t *testing.T) {
	t.Helper()
	f.regrabIssue(t, domain.NewID[domain.IssueID]())
}

// regrabIssue delivers the issue's REGRAB in a fresh envelope, as a repeated action would arrive.
func (f searchFixture) regrabIssue(t *testing.T, issue domain.IssueID, removed ...domain.DownloadID) {
	t.Helper()
	action := requestsapi.IssueActionRequested{Issue: issue, Title: f.title.ID, Action: requestsapi.IssueActionRegrab, RemovedDownloads: removed}
	f.deliver(t, acquisition.NewIssueActionHandler(f.searches), bus.EventOf(action))
}

// runQueuedSearch runs the one queued search job and returns its snapshot once it completed.
func (f searchFixture) runQueuedSearch(t *testing.T, completedBefore int) acquisition.SearchSnapshot {
	t.Helper()
	tasks := f.searchTasks()
	if len(tasks) != 1 {
		t.Fatalf("searches = %+v, want one queued", tasks)
	}
	f.jobs.drop(func(task backlite.Task) bool { _, ok := acquisition.SearchTaskOf(task); return ok })
	if err := f.searches.RunSearchJob(context.Background(), tasks[0]); err != nil {
		t.Fatal(err)
	}
	f.settle(t)
	done := payload[api.SearchCompleted](t, f.events.waitFor(t, api.TypeSearchCompleted, completedBefore+1)[completedBefore])
	snapshot, err := f.searches.Search(done.Search)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func rejectionReasons(r api.ReleaseDecision) []api.RejectionReason {
	reasons := make([]api.RejectionReason, 0, len(r.Rejections))
	for _, rejection := range r.Rejections {
		reasons = append(reasons, rejection.Reason)
	}
	return reasons
}

// Seed acquisition #17.
func TestATranscodedFileIsNotUpgradedButARegrabReplacesIt(t *testing.T) {
	f := newSearchFixture(t)
	f.useScoredProfile(t, cutoffAt2160p)
	f.libraryFile(low1080p, true, reportedHash)

	found := f.searched(t, release(high2160p))
	if reasons := rejectionReasons(found.Releases[0]); !slices.Contains(reasons, api.RejectUpgradeNotAllowed) {
		t.Fatalf("better release over a transcoded file: %v, want UPGRADE_NOT_ALLOWED", reasons)
	}

	f.regrab(t)

	entries := f.blocklist(t)
	if len(entries) != 1 || entries[0].InfoHash != reportedHash || entries[0].Reason != api.BlocklistIssue || entries[0].ExpiresAt != nil {
		t.Fatalf("blocklist = %+v, want the file's hash blocked for good as ISSUE", entries)
	}
	tasks := f.searchTasks()
	if len(tasks) != 1 || tasks[0].Trigger != api.TriggerRegrab || tasks[0].Title != f.title.ID {
		t.Fatalf("searches = %+v, want one REGRAB search", tasks)
	}
	f.jobs.drop(func(task backlite.Task) bool { _, ok := acquisition.SearchTaskOf(task); return ok })
	if err := f.searches.RunSearchJob(context.Background(), tasks[0]); err != nil {
		t.Fatal(err)
	}
	f.settle(t)
	if grabs := f.grabbed(t); len(grabs) != 1 || grabs[0].Release != high2160p || grabs[0].Trigger != api.TriggerRegrab {
		t.Errorf("grabs = %+v, want the REGRAB search to grab %s", grabs, high2160p)
	}
}

func TestARegrabOfAFileWithoutAHashBlocksItsReleaseAtItsIndexer(t *testing.T) {
	f := newSearchFixture(t)
	f.useScoredProfile(t, cutoffAt2160p)
	id := f.completedDownload(t, low1080p)
	file := f.libraryFile(low1080p, false, "")
	file.Download = &id
	f.library.Put(librarytest.StoredFile{File: file})

	f.regrab(t)
	f.regrab(t)

	issues := slices.DeleteFunc(f.blocklist(t), func(e api.BlocklistEntry) bool { return e.Reason != api.BlocklistIssue })
	if len(issues) != 1 || issues[0].ReleaseName != low1080p || issues[0].Indexer != "Fast" || issues[0].InfoHash != "" {
		t.Errorf("ISSUE entries = %+v, want one by release name at Fast", issues)
	}
}

func TestARegrabBlocksTheReleaseAtAnIndexerThatReportsNoHash(t *testing.T) {
	f := newSearchFixture(t)
	id := f.completedDownload(t, acceptedName)
	file := f.libraryFile(acceptedName, false, magnetHash(acceptedName))
	file.Download = &id
	f.library.Put(librarytest.StoredFile{File: file})

	f.regrab(t)
	snapshot := f.runQueuedSearch(t, 1)

	entries := slices.DeleteFunc(f.blocklist(t), func(e api.BlocklistEntry) bool { return e.Reason != api.BlocklistIssue })
	if len(entries) != 1 || entries[0].InfoHash != magnetHash(acceptedName) || entries[0].Indexer != "Fast" {
		t.Fatalf("ISSUE entries = %+v, want the hash and the source indexer", entries)
	}
	found := slices.IndexFunc(snapshot.Releases, func(r api.ReleaseDecision) bool { return r.Name == acceptedName })
	if found < 0 {
		t.Fatalf("releases = %+v, want %s offered again", snapshot.Releases, acceptedName)
	}
	if reasons := rejectionReasons(snapshot.Releases[found]); !slices.Contains(reasons, api.RejectBlocklisted) {
		t.Errorf("%s rejected for %v, want BLOCKLISTED", acceptedName, reasons)
	}
}

// Seed acquisition #41.
func TestARegrabRunsOncePerIssue(t *testing.T) {
	f := newSearchFixture(t)
	f.searched(t, release(acceptedName))
	f.libraryFile(low1080p, false, reportedHash)
	issue := domain.NewID[domain.IssueID]()

	f.regrabIssue(t, issue)
	f.runQueuedSearch(t, 1)
	f.regrabIssue(t, issue)

	if entries := f.blocklist(t); len(entries) != 1 {
		t.Errorf("blocklist = %+v, want one ISSUE entry", entries)
	}
	if tasks := f.searchTasks(); len(tasks) != 0 {
		t.Errorf("searches after the repeat = %+v, want none", tasks)
	}
}

// Seed acquisition #42.
func TestARegrabDoesNotWaitForADownloadTheAdminRemoved(t *testing.T) {
	f := newSearchFixture(t)
	waiting := f.completedDownload(t, acceptedName)
	f.deliverLibrary(t, manualImport(waiting))

	f.regrabIssue(t, domain.NewID[domain.IssueID](), waiting)
	snapshot := f.runQueuedSearch(t, 1)

	if len(snapshot.Releases) == 0 {
		t.Fatal("the REGRAB search found nothing")
	}
	for _, r := range snapshot.Releases {
		if slices.Contains(rejectionReasons(r), api.RejectAlreadyQueued) {
			t.Errorf("%s rejected ALREADY_QUEUED, want the removed download not to hold the items", r.Name)
		}
	}
}

// Seed acquisition #42, without RemovedDownloads.
func TestARegrabStillWaitsForADownloadNobodyRemoved(t *testing.T) {
	f := newSearchFixture(t)
	waiting := f.completedDownload(t, acceptedName)
	f.deliverLibrary(t, manualImport(waiting))

	f.regrab(t)
	snapshot := f.runQueuedSearch(t, 1)

	if len(snapshot.Releases) == 0 {
		t.Fatal("the REGRAB search found nothing")
	}
	for _, r := range snapshot.Releases {
		if !slices.Contains(rejectionReasons(r), api.RejectAlreadyQueued) {
			t.Errorf("%s rejected for %v, want ALREADY_QUEUED", r.Name, rejectionReasons(r))
		}
	}
}

func TestARegrabOfAScannedFileOnlySearches(t *testing.T) {
	f := newSearchFixture(t)
	f.libraryFile("", false, "")

	f.regrab(t)

	if entries := f.blocklist(t); len(entries) != 0 {
		t.Errorf("blocklist = %+v, want nothing to block", entries)
	}
	if tasks := f.searchTasks(); len(tasks) != 1 || tasks[0].Trigger != api.TriggerRegrab {
		t.Errorf("searches = %+v, want one REGRAB search", tasks)
	}
}
