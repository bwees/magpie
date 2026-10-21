package activity

import (
	"context"
	"slices"
	"testing"
	"time"

	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/activity/api"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	platformapi "magpie/internal/platform/api"
	"magpie/internal/platform/bus"
	requestsapi "magpie/internal/requests/api"
	subtitlesapi "magpie/internal/subtitles/api"
	transcodeapi "magpie/internal/transcode/api"

	"gorm.io/gorm"
)

const (
	releaseName  = "Dune.2021.2160p.WEB-DL.DDP5.1.HEVC-GROUP"
	oldSize      = 10_000
	newSize      = 3_800
	savedPercent = 62
	historyDays  = 90
	staleDays    = historyDays + 1
	freshDays    = historyDays - 1
	day          = 24 * time.Hour
	english      = domain.Language("en")
)

func titleAdded(title domain.TitleID, kind domain.MediaKind, episodes ...domain.EpisodeID) catalogapi.TitleAdded {
	return catalogapi.TitleAdded{Title: title, Kind: kind, Monitored: episodes, Origin: domain.OriginRequest}
}

// movieWithFile walks a movie to an imported file and returns the file.
func movieWithFile(h *harness, title domain.TitleID) domain.MediaFileID {
	file := domain.NewID[domain.MediaFileID]()
	download := domain.NewID[domain.DownloadID]()
	h.deliver(titleAdded(title, domain.MediaKindMovie))
	h.deliver(acquisitionapi.ReleaseGrabbed{Download: download, Title: title, Release: releaseName})
	h.deliver(acquisitionapi.DownloadCompleted{Download: download, Title: title, ReleaseName: releaseName})
	h.deliver(libraryapi.FileImported{MediaFile: file, Title: title, ReleaseName: releaseName, Download: &download})
	return file
}

// availableMovie walks a movie to AVAILABLE and returns its file.
func availableMovie(h *harness, title domain.TitleID) domain.MediaFileID {
	file := movieWithFile(h, title)
	h.deliver(libraryapi.ItemAvailable{Title: title})
	return file
}

func assertStage(t *testing.T, row api.PipelineRow, stage api.Stage, failed bool, detail string) {
	t.Helper()
	if row.Stage != stage || row.Failed != failed || row.Detail != detail {
		t.Errorf("row = %s failed=%v %q, want %s failed=%v %q", row.Stage, row.Failed, row.Detail, stage, failed, detail)
	}
}

// Seed 1.
func TestTitleAddedCreatesARequestedRowAndATimelineEntry(t *testing.T) {
	h := newHarness(t)
	title := h.movie()

	h.deliver(titleAdded(title, domain.MediaKindMovie))

	assertStage(t, h.row(title), api.StageRequested, false, "")
	if got := h.timeline(title); len(got) != 1 {
		t.Errorf("timeline has %d entries, want 1", len(got))
	}
}

// Seed 2.
func TestRedeliveredEventChangesNothing(t *testing.T) {
	h := newHarness(t)
	title := h.movie()
	ev := h.deliver(titleAdded(title, domain.MediaKindMovie))
	before := h.row(title)

	h.deliverEvent(ev)

	if got := h.timeline(title); len(got) != 1 {
		t.Errorf("timeline has %d entries, want 1", len(got))
	}
	if got := h.history(); len(got) != 1 {
		t.Errorf("history has %d entries, want 1", len(got))
	}
	if after := h.row(title); !after.Updated.Equal(before.Updated) {
		t.Errorf("pipeline row updated again: %v → %v", before.Updated, after.Updated)
	}
}

// Seed 3.
func TestDownloadAndImportMoveTheStage(t *testing.T) {
	h := newHarness(t)
	title := h.movie()
	download := domain.NewID[domain.DownloadID]()
	h.deliver(titleAdded(title, domain.MediaKindMovie))
	h.deliver(acquisitionapi.SearchStarted{Search: domain.NewID[domain.SearchID](), Title: title})
	assertStage(t, h.row(title), api.StageSearching, false, "")

	h.deliver(acquisitionapi.ReleaseGrabbed{Download: download, Title: title, Release: releaseName})
	row := h.row(title)
	assertStage(t, row, api.StageDownloading, false, "")
	if row.Download == nil || *row.Download != download {
		t.Errorf("download = %v, want %s for the live %%", row.Download, download)
	}

	h.deliver(acquisitionapi.DownloadCompleted{Download: download, Title: title})
	assertStage(t, h.row(title), api.StageImporting, false, "")
	if h.row(title).Download != nil {
		t.Error("download still set after it completed")
	}

	h.deliver(libraryapi.FileImported{MediaFile: domain.NewID[domain.MediaFileID](), Title: title, Download: &download})
	row = h.row(title)
	assertStage(t, row, api.StageImporting, false, detailWaitingForJellyfin)
	if row.Subs == nil || row.Subs.State != api.SubsPending {
		t.Errorf("subs chip = %+v, want PENDING", row.Subs)
	}
}

// Seed 4.
func TestItemAvailableIsAvailableWhileSubtitlesArePending(t *testing.T) {
	h := newHarness(t)
	title := h.movie()
	file := availableMovie(h, title)
	assertStage(t, h.row(title), api.StageAvailable, false, "")

	h.deliver(subtitlesapi.CoverageSettled{MediaFile: file, Title: title, Satisfied: []domain.Language{english}})

	row := h.row(title)
	assertStage(t, row, api.StageAvailable, false, "")
	if row.Subs == nil || row.Subs.State != api.SubsSettled || len(row.Subs.Satisfied) != 1 || row.Subs.Satisfied[0] != english {
		t.Errorf("subs chip = %+v, want SETTLED with en", row.Subs)
	}
}

// Seed 5.
func TestTranscodeChipFollowsTheJob(t *testing.T) {
	h := newHarness(t)
	title := h.movie()
	file := availableMovie(h, title)
	job := domain.NewID[domain.JobID]()

	h.deliver(transcodeapi.JobQueued{Job: job, Kind: transcodeapi.JobKindTranscode, MediaFile: file, Title: title})
	row := h.row(title)
	if row.Transcode == nil || row.Transcode.State != api.TranscodeQueued {
		t.Fatalf("transcode chip = %+v, want QUEUED", row.Transcode)
	}

	h.deliver(transcodeapi.JobStarted{Job: job, Kind: transcodeapi.JobKindTranscode, MediaFile: file})
	row = h.row(title)
	if row.Transcode == nil || row.Transcode.State != api.TranscodeRunning || row.Transcode.Job == nil || *row.Transcode.Job != job {
		t.Fatalf("transcode chip = %+v, want RUNNING with job %s", row.Transcode, job)
	}

	h.deliver(libraryapi.FileTranscoded{MediaFile: file, OldSize: oldSize, NewSize: newSize})
	row = h.row(title)
	assertStage(t, row, api.StageAvailable, false, "")
	if row.Transcode == nil || row.Transcode.State != api.TranscodeDone || row.Transcode.Saved == nil || *row.Transcode.Saved != savedPercent {
		t.Errorf("transcode chip = %+v, want DONE saving %d%%", row.Transcode, savedPercent)
	}
}

func TestWhisperJobsLeaveTheTranscodeChipAlone(t *testing.T) {
	h := newHarness(t)
	title := h.movie()
	file := availableMovie(h, title)

	h.deliver(transcodeapi.JobQueued{Job: domain.NewID[domain.JobID](), Kind: transcodeapi.JobKindWhisper, MediaFile: file, Title: title})

	if chip := h.row(title).Transcode; chip != nil {
		t.Errorf("transcode chip = %+v, want none", chip)
	}
}

// Seed 6.
func TestUpgradeGrabShowsDownloadingUpgrade(t *testing.T) {
	h := newHarness(t)
	title := h.movie()
	availableMovie(h, title)

	h.deliver(acquisitionapi.ReleaseGrabbed{Download: domain.NewID[domain.DownloadID](), Title: title, Release: releaseName})

	assertStage(t, h.row(title), api.StageDownloading, false, detailUpgrade)
}

// Seed 7.
func TestNothingAcceptedIsAFailedSearch(t *testing.T) {
	h := newHarness(t)
	title := h.movie()
	search := domain.NewID[domain.SearchID]()
	h.deliver(titleAdded(title, domain.MediaKindMovie))
	h.deliver(acquisitionapi.SearchStarted{Search: search, Title: title})

	h.deliver(acquisitionapi.SearchCompleted{Search: search, Title: title, Outcome: acquisitionapi.OutcomeNothingAccepted})

	assertStage(t, h.row(title), api.StageSearching, true, detailNoAcceptedRelease)
	stages, err := h.repo.TitleStages(context.Background(), []domain.TitleID{title})
	if err != nil {
		t.Fatal(err)
	}
	if row, ok := stages[title]; !ok || row.Stage != api.StageSearching || !row.Failed {
		t.Errorf("TitleStages = %+v, want the failed SEARCHING row", stages)
	}
}

func TestNoReleasesIsNotAFailedSearch(t *testing.T) {
	h := newHarness(t)
	title := h.movie()
	search := domain.NewID[domain.SearchID]()
	h.deliver(titleAdded(title, domain.MediaKindMovie))
	h.deliver(acquisitionapi.SearchStarted{Search: search, Title: title})

	h.deliver(acquisitionapi.SearchCompleted{Search: search, Title: title, Outcome: acquisitionapi.OutcomeNoReleases})

	assertStage(t, h.row(title), api.StageRequested, false, detailNoReleases)
}

func TestUnreleasedEpisodeIsNoLongerSearching(t *testing.T) {
	h := newHarness(t)
	title, episodes := h.series([2]int{1, 1}, [2]int{1, 2})
	search := domain.NewID[domain.SearchID]()
	h.deliver(titleAdded(title, domain.MediaKindSeries, episodes...))
	h.deliver(acquisitionapi.SearchStarted{Search: search, Title: title, Episodes: episodes})
	h.deliver(acquisitionapi.SearchCompleted{Search: search, Title: title, Outcome: acquisitionapi.OutcomeNothingAccepted})

	h.deliver(catalogapi.EpisodesUnreleased{Title: title, Episodes: episodes[1:]})

	for _, row := range h.pipeline(title) {
		switch *row.Episode {
		case episodes[0]:
			assertStage(t, row, api.StageSearching, true, detailNoAcceptedRelease)
		case episodes[1]:
			assertStage(t, row, api.StageRequested, false, "")
		}
	}
}

// Seed 8.
func TestOlderEventArrivingLateIsIgnored(t *testing.T) {
	h := newHarness(t)
	title, episodes := h.series([2]int{1, 1})
	h.deliver(titleAdded(title, domain.MediaKindSeries, episodes...))
	older := bus.EventOf(acquisitionapi.SearchStarted{Search: domain.NewID[domain.SearchID](), Title: title, Episodes: episodes})
	newer := bus.EventOf(acquisitionapi.ReleaseGrabbed{Download: domain.NewID[domain.DownloadID](), Title: title, Episodes: episodes})

	h.deliverEvent(newer)
	h.deliverEvent(older)

	row := h.row(title)
	assertStage(t, row, api.StageDownloading, false, "")
}

// Seed 9.
func TestTitleRemovedDropsTimelineAndPipelineButKeepsHistory(t *testing.T) {
	h := newHarness(t)
	title := h.movie()
	h.deliver(titleAdded(title, domain.MediaKindMovie))
	h.deliver(acquisitionapi.SearchStarted{Search: domain.NewID[domain.SearchID](), Title: title})

	h.deliver(catalogapi.TitleRemoved{Title: title})
	h.deliver(libraryapi.FileDeleted{MediaFile: domain.NewID[domain.MediaFileID](), Title: title, Reason: libraryapi.DeleteTitleRemoved})

	if got := h.timeline(title); len(got) != 0 {
		t.Errorf("timeline has %d entries, want 0", len(got))
	}
	if got := h.pipeline(title); len(got) != 0 {
		t.Errorf("pipeline has %d rows, want 0", len(got))
	}
	if got := h.history(); len(got) != 4 {
		t.Errorf("history has %d entries, want 4", len(got))
	}
}

func TestUpgradedFileDeletionChangesNothing(t *testing.T) {
	h := newHarness(t)
	title := h.movie()
	file := availableMovie(h, title)
	before := h.row(title)

	h.deliver(libraryapi.FileDeleted{MediaFile: file, Title: title, Reason: libraryapi.DeleteUpgraded})

	after := h.row(title)
	assertStage(t, after, api.StageAvailable, false, "")
	if !after.Updated.Equal(before.Updated) || after.Subs == nil {
		t.Errorf("row changed: %+v → %+v", before, after)
	}
}

func TestRemovedFileGoesBackToRequested(t *testing.T) {
	h := newHarness(t)
	title := h.movie()
	file := availableMovie(h, title)

	h.deliver(libraryapi.FileDeleted{MediaFile: file, Title: title, Reason: libraryapi.DeleteRemoved})

	row := h.row(title)
	assertStage(t, row, api.StageRequested, false, "")
	if row.Subs != nil || row.Transcode != nil {
		t.Errorf("chips = %+v / %+v, want none", row.Subs, row.Transcode)
	}
}

func TestImportFailedFindsTheDownloadsTitle(t *testing.T) {
	h := newHarness(t)
	title := h.movie()
	download := domain.NewID[domain.DownloadID]()
	h.deliver(titleAdded(title, domain.MediaKindMovie))
	h.deliver(acquisitionapi.ReleaseGrabbed{Download: download, Title: title, Release: releaseName})
	h.deliver(acquisitionapi.DownloadCompleted{Download: download, Title: title})

	h.deliver(libraryapi.ImportFailed{Download: download, Reason: "no video file"})

	assertStage(t, h.row(title), api.StageDownloading, true, "no video file")
	if got := h.timeline(title); got[0].Type != libraryapi.TypeImportFailed {
		t.Errorf("newest timeline entry = %s, want ImportFailed", got[0].Type)
	}
}

func TestMonitoringChangesWantedRows(t *testing.T) {
	h := newHarness(t)
	title, episodes := h.series([2]int{1, 1}, [2]int{1, 2})
	h.deliver(titleAdded(title, domain.MediaKindSeries, episodes[0]))
	h.deliver(catalogapi.MonitoringChanged{Title: title, Added: episodes[1:]})
	if got := h.pipeline(title); len(got) != 2 {
		t.Fatalf("pipeline has %d rows, want 2", len(got))
	}

	h.deliver(libraryapi.FileImported{MediaFile: domain.NewID[domain.MediaFileID](), Title: title, Episodes: []domain.EpisodeRef{{Episode: episodes[1], Season: 1, Number: 2}}})
	h.deliver(libraryapi.ItemAvailable{Title: title, Episodes: episodes[1:]})
	h.deliver(catalogapi.MonitoringChanged{Title: title, Removed: episodes[:1]})

	stages, err := h.repo.TitleStages(context.Background(), []domain.TitleID{title})
	if err != nil {
		t.Fatal(err)
	}
	if got := stages[title]; got.Stage != api.StageAvailable {
		t.Errorf("summary stage = %s, want AVAILABLE once the requested episode is unmonitored", got.Stage)
	}
	if got := h.pipeline(title); len(got) != 2 {
		t.Errorf("pipeline has %d rows, want 2: unmonitored rows are kept", len(got))
	}
}

func TestEpisodeGroupChangeReordersRows(t *testing.T) {
	h := newHarness(t)
	title, episodes := h.series([2]int{1, 1}, [2]int{1, 2})
	h.deliver(titleAdded(title, domain.MediaKindSeries, episodes...))

	h.deliver(catalogapi.EpisodeGroupChanged{Title: title, Moves: []catalogapi.EpisodeMove{
		{Episode: episodes[0], FromSeason: 1, FromNo: 1, ToSeason: 1, ToNo: 2},
		{Episode: episodes[1], FromSeason: 1, FromNo: 2, ToSeason: 1, ToNo: 1},
	}})

	rows := h.pipeline(title)
	if *rows[0].Episode != episodes[1] || *rows[1].Episode != episodes[0] {
		t.Errorf("rows are not in the new episode order")
	}
}

func TestARenamedFileReadsOnTheTitleTimeline(t *testing.T) {
	h := newHarness(t)
	title, episodes := h.series([2]int{1, 1})
	h.deliver(libraryapi.FileRenamed{
		MediaFile: domain.NewID[domain.MediaFileID](), Title: title,
		Episodes: []domain.EpisodeRef{{Episode: episodes[0], Season: 2, Number: 1}},
		OldPath:  "/tv/Show/Season 01/Show S01E01.mkv", NewPath: "/tv/Show/Season 02/Show S02E01.mkv",
	})

	got := h.timeline(title)
	if len(got) != 1 || got[0].Text != "Renamed to Show S02E01.mkv" || got[0].Detail != "/tv/Show/Season 01/Show S01E01.mkv" ||
		!slices.Equal(got[0].Episodes, episodes) {
		t.Errorf("timeline = %+v, want one rename entry for the episode", got)
	}
}

func TestSeverity(t *testing.T) {
	file := domain.NewID[domain.MediaFileID]()
	title := domain.NewID[domain.TitleID]()
	for _, tc := range []struct {
		event domain.Message
		want  string
	}{
		{acquisitionapi.DownloadFailed{Title: title}, api.SeverityError},
		{libraryapi.ImportFailed{}, api.SeverityError},
		{transcodeapi.JobFailed{MediaFile: file, Final: true}, api.SeverityError},
		{transcodeapi.JobFailed{MediaFile: file}, api.SeverityWarn},
		{acquisitionapi.ReleaseRejected{Title: title}, api.SeverityWarn},
		{libraryapi.TranscodeSwapRejected{MediaFile: file}, api.SeverityWarn},
		{transcodeapi.AgentOffline{}, api.SeverityWarn},
		{acquisitionapi.SearchCompleted{Title: title, Outcome: acquisitionapi.OutcomeNothingAccepted}, api.SeverityWarn},
		{acquisitionapi.SearchCompleted{Title: title, Outcome: acquisitionapi.OutcomeNoReleases}, api.SeverityInfo},
		{acquisitionapi.SearchCompleted{Title: title, Outcome: acquisitionapi.OutcomeGrabbed}, api.SeverityInfo},
		{acquisitionapi.ReleaseGrabbed{Title: title}, api.SeverityInfo},
		{requestsapi.RequestDeclined{}, api.SeverityInfo},
	} {
		if got := describe(bus.EventOf(tc.event)).Severity; got != tc.want {
			t.Errorf("%s severity = %s, want %s", tc.event.Type(), got, tc.want)
		}
	}
}

// Seed 17.
func TestPruneDropsHistoryOlderThanTheRetention(t *testing.T) {
	h := newHarness(t)
	stale := domain.NewID[domain.TitleID]()
	fresh := domain.NewID[domain.TitleID]()
	err := h.db.Write(context.Background(), func(tx *gorm.DB) error {
		for _, row := range []historyRow{
			{ID: string(stale), EventID: string(stale), Type: catalogapi.TypeTitleAdded, Severity: api.SeverityInfo, Summary: "stale", EpisodeIDs: "[]", At: ago(staleDays * day)},
			{ID: string(fresh), EventID: string(fresh), Type: catalogapi.TypeTitleAdded, Severity: api.SeverityInfo, Summary: "fresh", EpisodeIDs: "[]", At: ago(freshDays * day)},
		} {
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := h.service.PruneHistory(context.Background()); err != nil {
		t.Fatal(err)
	}

	got := h.history()
	if len(got) != 1 || got[0].Text != "fresh" {
		t.Errorf("history = %+v, want only the fresh row", got)
	}
}

// Seed 18.
func TestRequestAttentionOpensAndClearsOnce(t *testing.T) {
	h := newHarness(t)
	request := domain.NewID[domain.RequestID]()
	h.deliver(transcodeapi.AgentOffline{Agent: domain.NewID[domain.AgentID]()})
	h.attentionChanges()

	created := h.deliver(requestsapi.RequestCreated{Request: request, Kind: domain.MediaKindMovie})
	opened := h.attentionChanges()
	if len(opened) != 1 || !opened[0].Open || opened[0].Kind != api.AttentionRequestPending || opened[0].Ref != string(request) || opened[0].Count != 2 {
		t.Fatalf("changes = %+v, want one open REQUEST_PENDING with count 2", opened)
	}

	approved := h.deliver(requestsapi.RequestApproved{Request: request})
	cleared := h.attentionChanges()
	if len(cleared) != 1 || cleared[0].Open || cleared[0].Item != opened[0].Item || cleared[0].Count != 1 {
		t.Fatalf("changes = %+v, want the item cleared with count 1", cleared)
	}

	h.deliverEvent(created)
	h.deliverEvent(approved)
	if changes := h.attentionChanges(); len(changes) != 0 {
		t.Errorf("redelivery published %+v", changes)
	}
	if items := h.attention(); len(items) != 1 {
		t.Errorf("open items = %+v, want only the agent", items)
	}
}

func TestCancelledRequestClearsItsAttentionItem(t *testing.T) {
	h := newHarness(t)
	request := domain.NewID[domain.RequestID]()

	h.deliver(requestsapi.RequestCreated{Request: request, Kind: domain.MediaKindSeries})
	h.deliver(requestsapi.RequestCancelled{Request: request})

	if items := h.attention(); len(items) != 0 {
		t.Errorf("open items = %+v, want none after the cancel", items)
	}
}

// Seed 19.
func TestAgentOfflineOpensOneItemUntilOnline(t *testing.T) {
	h := newHarness(t)
	agent := domain.NewID[domain.AgentID]()

	h.deliver(transcodeapi.AgentOffline{Agent: agent})
	h.deliver(transcodeapi.AgentOffline{Agent: agent})
	items := h.attention()
	if len(items) != 1 || items[0].Kind != api.AttentionAgentOffline || items[0].Ref != string(agent) {
		t.Fatalf("open items = %+v, want one AGENT_OFFLINE", items)
	}

	h.deliver(transcodeapi.AgentOnline{Agent: agent})
	if items := h.attention(); len(items) != 0 {
		t.Errorf("open items = %+v, want none", items)
	}
}

func TestAttentionKinds(t *testing.T) {
	h := newHarness(t)
	title := h.movie()
	file := availableMovie(h, title)
	issue := domain.NewID[domain.IssueID]()

	h.deliver(requestsapi.IssueReported{Issue: issue, Title: title, Kind: requestsapi.IssueKindVideo})
	h.deliver(transcodeapi.JobFailed{Job: domain.NewID[domain.JobID](), Kind: transcodeapi.JobKindTranscode, MediaFile: file, Final: false})
	h.deliver(platformapi.ServiceHealthChanged{Service: platformapi.ServiceProwlarr, Healthy: false})
	assertOpenKinds(t, h.attention(), api.AttentionIssueOpen, api.AttentionServiceDown)

	h.deliver(transcodeapi.JobFailed{Job: domain.NewID[domain.JobID](), Kind: transcodeapi.JobKindTranscode, MediaFile: file, Final: true})
	assertOpenKinds(t, h.attention(), api.AttentionIssueOpen, api.AttentionServiceDown, api.AttentionTranscodeFailed)

	h.deliver(requestsapi.IssueResolved{Issue: issue})
	h.deliver(platformapi.ServiceHealthChanged{Service: platformapi.ServiceProwlarr, Healthy: true})
	h.deliver(transcodeapi.JobQueued{Job: domain.NewID[domain.JobID](), Kind: transcodeapi.JobKindTranscode, MediaFile: file, Title: title})
	assertOpenKinds(t, h.attention())
}

func TestPipelineFailureOpensOneItemPerTitle(t *testing.T) {
	h := newHarness(t)
	title, episodes := h.series([2]int{1, 1}, [2]int{1, 2})
	search := domain.NewID[domain.SearchID]()
	h.deliver(titleAdded(title, domain.MediaKindSeries, episodes...))
	h.deliver(acquisitionapi.SearchStarted{Search: search, Title: title, Episodes: episodes})

	h.deliver(acquisitionapi.SearchCompleted{Search: search, Title: title, Outcome: acquisitionapi.OutcomeNothingAccepted})
	items := h.attention()
	if len(items) != 1 || items[0].Kind != api.AttentionPipelineFailed || items[0].Title == nil || *items[0].Title != title {
		t.Fatalf("open items = %+v, want one PIPELINE_FAILED for the title", items)
	}

	h.deliver(acquisitionapi.SearchStarted{Search: domain.NewID[domain.SearchID](), Title: title, Episodes: episodes})
	if items := h.attention(); len(items) != 0 {
		t.Errorf("open items = %+v, want none once no row is failed", items)
	}
}

func TestUnmonitoringAFailedSeasonClearsItsFailure(t *testing.T) {
	h := newHarness(t)
	title, episodes := h.series([2]int{1, 1}, [2]int{1, 2})
	search := domain.NewID[domain.SearchID]()
	h.deliver(titleAdded(title, domain.MediaKindSeries, episodes...))
	h.deliver(acquisitionapi.SearchStarted{Search: search, Title: title, Episodes: episodes})
	h.deliver(acquisitionapi.SearchCompleted{Search: search, Title: title, Outcome: acquisitionapi.OutcomeNothingAccepted})

	h.deliver(catalogapi.MonitoringChanged{Title: title, Added: []domain.EpisodeID{}, Removed: episodes})

	for _, row := range h.pipeline(title) {
		assertStage(t, row, api.StageRequested, false, "")
	}
	assertOpenKinds(t, h.attention())
}

func assertOpenKinds(t *testing.T, items []api.AttentionItem, kinds ...string) {
	t.Helper()
	got := make([]string, 0, len(items))
	for _, item := range items {
		got = append(got, item.Kind)
	}
	if !slices.Equal(got, kinds) {
		t.Errorf("open kinds = %v, want %v", got, kinds)
	}
}
