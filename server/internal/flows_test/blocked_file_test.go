package flows_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"magpie/internal/acquisition"
	acquisitionapi "magpie/internal/acquisition/api"
	acquisitionfakes "magpie/internal/acquisition/fakes"
	"magpie/internal/domain"
	"magpie/internal/library"
	libraryapi "magpie/internal/library/api"
)

const (
	blockedTmdb     = domain.TmdbID(2002)
	blockedName     = "Blocked Flow"
	blockedRelease  = "Blocked.Flow.2024.1080p.WEB-DL.DDP5.1.H.264-BAD"
	fallbackRelease = "Blocked.Flow.2024.1080p.WEB-DL.DDP5.1.H.264-GRP"
	archiveFile     = "Blocked.Flow.part1.rar"
	installerFile   = "extras/setup.exe"
)

// everyImport lists Library's imports in every status.
var everyImport = "/api/library/imports?status=" + strings.Join([]string{
	string(library.ImportPending), string(library.ImportImported), string(library.ImportFailed),
	string(library.ImportManual), string(library.ImportRemoved),
}, ",")

// blockedFlow is an admin's request for a movie whose best release hides a blocked file behind a cached
// fallback.
type blockedFlow struct {
	*stack
	blocked, fallback acquisition.TorznabItem
}

func newBlockedFlow(t *testing.T) *blockedFlow {
	t.Helper()
	f := &blockedFlow{
		stack:   newStack(t),
		blocked: release(blockedRelease, topSeeders), fallback: release(fallbackRelease, releaseSeeders),
	}
	f.movieOnTMDB(blockedTmdb, blockedName, movieYear)
	f.answer(f.blocked, f.fallback)
	f.request(f.session, blockedTmdb)
	return f
}

// rejectedAlone lists what is wrong if the blocked release wasn't rejected for file, replaced by the
// fallback and kept away from every other context.
func (f *blockedFlow) rejectedAlone(grab acquisitionapi.ReleaseGrabbed, file string) []string {
	f.t.Helper()
	var problems []string
	rejected := published(f.harness, acquisitionapi.TypeReleaseRejected, func(e acquisitionapi.ReleaseRejected) bool { return e.Release == blockedRelease })
	fallback := published(f.harness, acquisitionapi.TypeReleaseGrabbed, func(e acquisitionapi.ReleaseGrabbed) bool { return e.Release == fallbackRelease })
	switch {
	case len(rejected) != 1 || len(fallback) != 1:
		problems = append(problems, fmt.Sprintf("rejected %+v, fallback grabbed %+v; want one of each", rejected, fallback))
	case rejected[0].Reason != acquisitionapi.RejectBlockedFile || rejected[0].BlockedFile == nil ||
		!strings.HasSuffix(*rejected[0].BlockedFile, file) || rejected[0].Next == nil || *rejected[0].Next != fallback[0].Download:
		problems = append(problems, fmt.Sprintf("rejected = %+v, want BLOCKED_FILE %s with the fallback as next", rejected[0], file))
	}
	if !slices.Contains(f.qbit.Deletions(), acquisitionfakes.Deletion{Hash: hashOf(f.blocked), DeleteFiles: true}) {
		problems = append(problems, fmt.Sprintf("qBittorrent deletions = %+v, want the blocked torrent with its data", f.qbit.Deletions()))
	}
	entries := getJSON[[]acquisition.BlocklistEntryResponse](f.harness, blocklistPath)
	if !slices.ContainsFunc(entries, func(e acquisition.BlocklistEntryResponse) bool {
		return e.ReleaseName == blockedRelease && e.Reason == acquisitionapi.BlocklistBlockedFile
	}) {
		problems = append(problems, fmt.Sprintf("blocklist = %+v, want the blocked release", entries))
	}
	completed := published(f.harness, acquisitionapi.TypeDownloadCompleted, func(e acquisitionapi.DownloadCompleted) bool { return e.Download == grab.Download })
	if len(completed) != 0 {
		problems = append(problems, fmt.Sprintf("the rejected download completed: %+v", completed))
	}
	if imports := getJSON[[]library.ImportResponse](f.harness, everyImport); len(imports) != 0 {
		problems = append(problems, fmt.Sprintf("Library saw imports %+v, want none", imports))
	}
	if n := len(f.outbox(libraryapi.TypeFileImported)) + len(f.outbox(libraryapi.TypeImportFailed)); n != 0 {
		problems = append(problems, fmt.Sprintf("Library published %d import events, want none", n))
	}
	return problems
}

// Flow 2: the best release's magnet lists a .rar once its metadata arrives, so Acquisition drops it,
// blocklists it and grabs the next cached release without searching again.
func TestBlockedFileInMetadataGrabsTheNextCachedRelease(t *testing.T) {
	f := newBlockedFlow(t)
	grab := f.grabbed(f.blocked)

	f.receiveMetadata(f.blocked, torrentFiles(f.blocked, archiveFile, videoOf(f.blocked))...)
	pollForEvent(f.stack, acquisitionapi.TypeReleaseRejected, func(e acquisitionapi.ReleaseRejected) bool { return e.Release == blockedRelease })

	rejected := func() []string { return f.rejectedAlone(grab, archiveFile) }
	settles(t, "the blocked release", rejected)
	if searches := f.outbox(acquisitionapi.TypeSearchStarted); len(searches) != 1 {
		t.Errorf("%d searches started, want only the request's", len(searches))
	}

	f.redeliverAll()
	settles(t, "the blocked release after redelivery", rejected)
}

// Flow 2's backstop: the metadata looked clean, but the finished torrent holds an installer, so
// Acquisition rejects it at completion instead of handing it to Library.
func TestBlockedFileAtCompletionGrabsTheNextCachedRelease(t *testing.T) {
	f := newBlockedFlow(t)
	grab := f.startDownload(f.blocked, videoOf(f.blocked))

	f.receiveMetadata(f.blocked, torrentFiles(f.blocked, videoOf(f.blocked), installerFile)...)
	f.finishTorrent(f.blocked, h264Movie(), videoOf(f.blocked), installerFile)
	pollForEvent(f.stack, acquisitionapi.TypeReleaseRejected, func(e acquisitionapi.ReleaseRejected) bool { return e.Release == blockedRelease })

	rejected := func() []string { return f.rejectedAlone(grab, installerFile) }
	settles(t, "the blocked release", rejected)

	f.redeliverAll()
	settles(t, "the blocked release after redelivery", rejected)
}
