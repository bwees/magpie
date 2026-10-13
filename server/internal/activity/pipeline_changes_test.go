package activity

import (
	"context"
	"encoding/json"
	"testing"

	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/activity/api"
	"magpie/internal/domain"
	"magpie/internal/platform/bus"
	transcodeapi "magpie/internal/transcode/api"
)

const leaseExpired = "lease expired"

func (h *harness) active() []api.PipelineRow {
	h.t.Helper()
	rows, err := h.repo.ActivePipeline(context.Background(), api.MaxPageLimit)
	if err != nil {
		h.t.Fatal(err)
	}
	return rows
}

func TestARetriedTranscodeGoesBackToQueuedUntilItsNextAttemptStarts(t *testing.T) {
	h := newHarness(t)
	title := h.movie()
	file := availableMovie(h, title)
	job := domain.NewID[domain.JobID]()
	h.deliver(transcodeapi.JobQueued{Job: job, Kind: transcodeapi.JobKindTranscode, MediaFile: file, Title: title})
	h.deliver(transcodeapi.JobStarted{Job: job, Kind: transcodeapi.JobKindTranscode, MediaFile: file, Attempt: 1})

	h.deliver(transcodeapi.JobFailed{Job: job, Kind: transcodeapi.JobKindTranscode, MediaFile: file, Reason: leaseExpired})

	if chip := h.row(title).Transcode; chip == nil || chip.State != api.TranscodeQueued || chip.Job != nil {
		t.Errorf("transcode chip = %+v, want QUEUED", chip)
	}
	if rows := h.active(); len(rows) != 0 {
		t.Errorf("active rows = %+v, want none while the job waits", rows)
	}

	h.deliver(transcodeapi.JobStarted{Job: job, Kind: transcodeapi.JobKindTranscode, MediaFile: file, Attempt: 2})

	if chip := h.row(title).Transcode; chip == nil || chip.State != api.TranscodeRunning || chip.Job == nil || *chip.Job != job {
		t.Errorf("transcode chip = %+v, want RUNNING with job %s", chip, job)
	}
}

func TestACancelledTranscodeClearsItsChip(t *testing.T) {
	h := newHarness(t)
	title := h.movie()
	file := availableMovie(h, title)
	job := domain.NewID[domain.JobID]()
	h.deliver(transcodeapi.JobQueued{Job: job, Kind: transcodeapi.JobKindTranscode, MediaFile: file, Title: title})
	h.deliver(transcodeapi.JobStarted{Job: job, Kind: transcodeapi.JobKindTranscode, MediaFile: file, Attempt: 1})

	h.deliver(transcodeapi.JobCancelled{Job: job, MediaFile: file, Reason: "cancelled"})

	if chip := h.row(title).Transcode; chip != nil {
		t.Errorf("transcode chip = %+v, want none", chip)
	}
	if rows := h.active(); len(rows) != 0 {
		t.Errorf("active rows = %+v, want none", rows)
	}
}

func TestWhisperFailuresLeaveTheTranscodeChipAlone(t *testing.T) {
	h := newHarness(t)
	title := h.movie()
	file := availableMovie(h, title)
	job := domain.NewID[domain.JobID]()
	h.deliver(transcodeapi.JobStarted{Job: job, Kind: transcodeapi.JobKindWhisper, MediaFile: file})

	h.deliver(transcodeapi.JobFailed{Job: job, Kind: transcodeapi.JobKindWhisper, MediaFile: file, Reason: leaseExpired})

	if chip := h.row(title).Transcode; chip != nil {
		t.Errorf("transcode chip = %+v, want none", chip)
	}
}

func TestARejectedDownloadLeavesTheDownloadingStage(t *testing.T) {
	h := newHarness(t)
	title := h.movie()
	download := domain.NewID[domain.DownloadID]()
	h.deliver(titleAdded(title, domain.MediaKindMovie))
	h.deliver(acquisitionapi.ReleaseGrabbed{Download: download, Title: title, Release: releaseName})

	h.deliver(acquisitionapi.ReleaseRejected{Title: title, Release: releaseName, Reason: acquisitionapi.RejectMetadataTimeout, Download: &download})

	row := h.row(title)
	assertStage(t, row, api.StageRequested, false, rejectedNote(acquisitionapi.RejectMetadataTimeout))
	if row.Download != nil {
		t.Errorf("download = %s, want none after the rejection", *row.Download)
	}
	if rows := h.active(); len(rows) != 0 {
		t.Errorf("active rows = %+v, want none", rows)
	}
}

func TestARemovedDownloadLeavesTheDownloadingStage(t *testing.T) {
	h := newHarness(t)
	title := h.movie()
	download := domain.NewID[domain.DownloadID]()
	h.deliver(titleAdded(title, domain.MediaKindMovie))
	h.deliver(acquisitionapi.ReleaseGrabbed{Download: download, Title: title, Release: releaseName})

	h.deliver(acquisitionapi.DownloadRemoved{Download: download, Title: title})

	row := h.row(title)
	assertStage(t, row, api.StageRequested, false, "")
	if row.Download != nil {
		t.Errorf("download = %s, want none after the removal", *row.Download)
	}
}

func TestAFailedDownloadLeavesItsFallbackDownloading(t *testing.T) {
	h := newHarness(t)
	title, episodes := h.series([2]int{1, 1}, [2]int{1, 2})
	stalled, fallback := domain.NewID[domain.DownloadID](), domain.NewID[domain.DownloadID]()
	h.deliver(titleAdded(title, domain.MediaKindSeries, episodes...))
	h.deliver(acquisitionapi.ReleaseGrabbed{Download: stalled, Title: title, Episodes: episodes, Release: releaseName})
	h.deliver(acquisitionapi.ReleaseGrabbed{Download: fallback, Title: title, Episodes: episodes[:1], Release: releaseName})

	h.deliver(acquisitionapi.DownloadFailed{Download: stalled, Title: title, Reason: "stalled"})

	rows := h.pipeline(title)
	if rows[0].Stage != api.StageDownloading || rows[0].Failed || rows[0].Download == nil || *rows[0].Download != fallback {
		t.Errorf("first episode = %+v, want downloading the fallback", rows[0])
	}
	assertStage(t, rows[1], api.StageDownloading, true, "stalled")
}

func TestARejectedUpgradeIsAvailableAgain(t *testing.T) {
	h := newHarness(t)
	title := h.movie()
	availableMovie(h, title)
	upgrade := domain.NewID[domain.DownloadID]()
	h.deliver(acquisitionapi.ReleaseGrabbed{Download: upgrade, Title: title, Release: releaseName})

	h.deliver(acquisitionapi.ReleaseRejected{Title: title, Release: releaseName, Reason: acquisitionapi.RejectStalled, Download: &upgrade})

	assertStage(t, h.row(title), api.StageAvailable, false, "")
}

// The fallback is grabbed before the rejection is published, so its ReleaseGrabbed has already moved the
// episodes it covers.
func TestARejectionKeepsTheFallbackDownloading(t *testing.T) {
	h := newHarness(t)
	title, episodes := h.series([2]int{1, 1}, [2]int{1, 2})
	pack, fallback := domain.NewID[domain.DownloadID](), domain.NewID[domain.DownloadID]()
	h.deliver(titleAdded(title, domain.MediaKindSeries, episodes...))
	h.deliver(acquisitionapi.ReleaseGrabbed{Download: pack, Title: title, Episodes: episodes, Release: releaseName})
	h.deliver(acquisitionapi.ReleaseGrabbed{Download: fallback, Title: title, Episodes: episodes[:1], Release: releaseName})

	h.deliver(acquisitionapi.ReleaseRejected{
		Title: title, Release: releaseName, Reason: acquisitionapi.RejectBlockedFile, Download: &pack, Next: &fallback,
	})

	rows := h.pipeline(title)
	if rows[0].Stage != api.StageDownloading || rows[0].Download == nil || *rows[0].Download != fallback {
		t.Errorf("first episode = %+v, want downloading the fallback", rows[0])
	}
	assertStage(t, rows[1], api.StageRequested, false, rejectedNote(acquisitionapi.RejectBlockedFile))
	if rows[1].Download != nil {
		t.Errorf("second episode download = %s, want none", *rows[1].Download)
	}
}

func TestARejectionBeforeAGrabOnlyNotesTheSearch(t *testing.T) {
	h := newHarness(t)
	title := h.movie()
	h.deliver(titleAdded(title, domain.MediaKindMovie))
	h.deliver(acquisitionapi.SearchStarted{Search: domain.NewID[domain.SearchID](), Title: title})

	h.deliver(acquisitionapi.ReleaseRejected{Title: title, Release: releaseName, Reason: acquisitionapi.RejectBlockedFile})

	assertStage(t, h.row(title), api.StageSearching, false, rejectedNote(acquisitionapi.RejectBlockedFile))
}

func TestAStoredRejectionWithoutADownloadStillDecodes(t *testing.T) {
	h := newHarness(t)
	title := h.movie()
	download := domain.NewID[domain.DownloadID]()
	h.deliver(titleAdded(title, domain.MediaKindMovie))
	h.deliver(acquisitionapi.ReleaseGrabbed{Download: download, Title: title, Release: releaseName})
	stored, err := json.Marshal(map[string]any{"title": title, "release": releaseName, "reason": acquisitionapi.RejectStalled})
	if err != nil {
		t.Fatal(err)
	}

	h.deliverEvent(bus.NewEvent(acquisitionapi.TypeReleaseRejected, string(title), json.RawMessage(stored)))

	assertStage(t, h.row(title), api.StageDownloading, false, rejectedNote(acquisitionapi.RejectStalled))
}
