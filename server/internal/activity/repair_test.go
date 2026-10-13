package activity

import (
	"context"
	"testing"
	"time"

	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/activity/api"
	"magpie/internal/domain"
	"magpie/internal/platform/bus"
	transcodeapi "magpie/internal/transcode/api"
)

// grabbedMovie walks a movie to a download in progress and returns the download.
func grabbedMovie(h *harness, title domain.TitleID) domain.DownloadID {
	download := domain.NewID[domain.DownloadID]()
	h.deliver(titleAdded(title, domain.MediaKindMovie))
	h.deliver(acquisitionapi.ReleaseGrabbed{Download: download, Title: title, Release: releaseName})
	return download
}

// transcodingMovie walks an available movie to a running transcode and returns the job.
func transcodingMovie(h *harness, title domain.TitleID) domain.JobID {
	file := availableMovie(h, title)
	job := domain.NewID[domain.JobID]()
	h.deliver(transcodeapi.JobStarted{Job: job, Kind: transcodeapi.JobKindTranscode, MediaFile: file})
	return job
}

func (h *harness) repair() {
	h.t.Helper()
	if err := h.repairer.Run(context.Background()); err != nil {
		h.t.Fatal(err)
	}
}

func TestRepairFreesRowsOfDownloadsThatLeftQBittorrent(t *testing.T) {
	h := newHarness(t)
	dropped, live := h.movie(), h.movie()
	removedAt := time.Now()
	h.acquisition.PutDownload(acquisitionapi.Download{ID: grabbedMovie(h, dropped), Title: dropped, RemovedAt: &removedAt})
	h.acquisition.PutDownload(acquisitionapi.Download{ID: grabbedMovie(h, live), Title: live})

	h.repair()

	assertStage(t, h.row(dropped), api.StageRequested, false, "")
	if row := h.row(live); row.Stage != api.StageDownloading || row.Download == nil {
		t.Errorf("live download row = %+v, want still downloading", row)
	}
}

func TestRepairedRowWithNothingAcceptedOpensAPipelineFailure(t *testing.T) {
	h := newHarness(t)
	title := h.movie()
	search := domain.NewID[domain.SearchID]()
	grabbedMovie(h, title)
	h.deliver(acquisitionapi.SearchStarted{Search: search, Title: title})
	h.deliver(acquisitionapi.SearchCompleted{Search: search, Title: title, Outcome: acquisitionapi.OutcomeNothingAccepted})

	h.repair()

	assertStage(t, h.row(title), api.StageSearching, true, detailNoAcceptedRelease)
	assertOpenKinds(t, h.attention(), api.AttentionPipelineFailed)
}

func TestRepairQueuesTranscodesWhoseJobLostItsAgent(t *testing.T) {
	h := newHarness(t)
	requeued, encoding, cancelled := h.movie(), h.movie(), h.movie()
	h.transcode.PutJob(transcodeapi.Job{ID: transcodingMovie(h, requeued), State: transcodeapi.StateQueued})
	h.transcode.PutJob(transcodeapi.Job{ID: transcodingMovie(h, encoding), State: transcodeapi.StateEncoding})
	h.transcode.PutJob(transcodeapi.Job{ID: transcodingMovie(h, cancelled), State: transcodeapi.StateCancelled})

	h.repair()

	if chip := h.row(requeued).Transcode; chip == nil || chip.State != api.TranscodeQueued {
		t.Errorf("requeued job chip = %+v, want QUEUED", chip)
	}
	if chip := h.row(encoding).Transcode; chip == nil || chip.State != api.TranscodeRunning {
		t.Errorf("encoding job chip = %+v, want RUNNING", chip)
	}
	if chip := h.row(cancelled).Transcode; chip != nil {
		t.Errorf("cancelled job chip = %+v, want none", chip)
	}
	if rows := h.active(); len(rows) != 1 || rows[0].Title != encoding {
		t.Errorf("active rows = %+v, want only the encoding movie", rows)
	}
}

// The repair doesn't count as an event, so one still on its way applies after it.
func TestEventsStillQueuedApplyAfterTheRepair(t *testing.T) {
	h := newHarness(t)
	title := h.movie()
	download := grabbedMovie(h, title)
	removedAt := time.Now()
	h.acquisition.PutDownload(acquisitionapi.Download{ID: download, Title: title, RemovedAt: &removedAt})

	completed := bus.EventOf(acquisitionapi.DownloadCompleted{Download: download, Title: title})

	h.repair()
	h.deliverEvent(completed)

	assertStage(t, h.row(title), api.StageImporting, false, "")
}
