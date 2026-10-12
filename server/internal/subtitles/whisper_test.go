package subtitles_test

import (
	"slices"
	"testing"
	"time"

	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	"magpie/internal/subtitles"
	"magpie/internal/subtitles/api"
	"magpie/internal/subtitles/fakes"
	transcodeapi "magpie/internal/transcode/api"
)

const (
	stagedSubtitle = "/tv/Show (2011)/.magpie-staging/job.srt"
	whisperSidecar = "/tv/Show (2011)/Season 01/Show S01E02.en.srt"
	sweepInterval  = 7 * time.Hour
	attemptsLimit  = 3
)

var whisperSRT = []byte("1\n00:00:05,000 --> 00:00:06,000\nHello\n")

// newWhisperHarness is a harness with the Whisper fallback on; it is off by default.
func newWhisperHarness(t *testing.T, global ...domain.Language) *harness {
	t.Helper()
	h := newHarness(t, global...)
	h.setWhisper(true)
	return h
}

func (h *harness) agentJobs() []transcodeapi.RunAgentJob {
	h.t.Helper()
	return ofType[transcodeapi.RunAgentJob](h.t, h.events, transcodeapi.TypeRunAgentJob)
}

func (h *harness) wanted(file domain.MediaFileID, language domain.Language) subtitles.WantedStatus {
	h.t.Helper()
	for _, row := range h.view(file).Wanted {
		if row.Language == language && !row.Forced {
			return row.Status
		}
	}
	h.t.Fatalf("no wanted row for %s", language)
	return ""
}

// whisperDone delivers WhisperCompleted for a subtitle staged at stagedSubtitle.
func (h *harness) whisperDone(file domain.MediaFileID, job domain.JobID) {
	h.t.Helper()
	h.handle(subtitles.NewWhisperHandler(h.service), transcodeapi.WhisperCompleted{
		Job: job, MediaFile: file, Language: english, StagedPath: stagedSubtitle,
	})
}

func (h *harness) whisperFailed(file domain.MediaFileID, final bool) {
	h.t.Helper()
	h.handle(subtitles.NewWhisperHandler(h.service), transcodeapi.JobFailed{
		Job: domain.NewID[domain.JobID](), Kind: transcodeapi.JobKindWhisper, MediaFile: file, Reason: "no", Final: final, Language: english,
	})
}

// Seed 5.
func TestNoMatchInTheOriginalLanguageAsksWhisperOnce(t *testing.T) {
	h := newWhisperHarness(t, english)
	file := h.episodeFile()
	h.imported(file)
	h.runDue()

	want := transcodeapi.RunAgentJob{Kind: transcodeapi.JobKindWhisper, MediaFile: file.ID, Language: english, Model: transcodeapi.WhisperModelLargeV3Turbo}
	if jobs := h.agentJobs(); len(jobs) != 1 || jobs[0] != want {
		t.Fatalf("RunAgentJob = %+v, want once %+v", jobs, want)
	}
	if status := h.wanted(file.ID, english); status != subtitles.StatusWaitingWhisper {
		t.Fatalf("English = %s, want WAITING_WHISPER", status)
	}
	if missing := ofType[api.SubtitleMissing](t, h.events, api.TypeSubtitleMissing); len(missing) != 0 {
		t.Errorf("SubtitleMissing = %+v, want none while Whisper runs", missing)
	}
	if settled := ofType[api.CoverageSettled](t, h.events, api.TypeCoverageSettled); len(settled) != 0 {
		t.Errorf("CoverageSettled = %+v, want none while Whisper runs", settled)
	}

	h.advance(sweepInterval)
	h.sweep()
	if n := len(h.agentJobs()); n != 1 {
		t.Errorf("RunAgentJob sent %d times after a sweep, want once", n)
	}
}

func TestAForcedTrackIsNotWhispered(t *testing.T) {
	h := newWhisperHarness(t)
	h.catalog.SetGlobalLanguages([]catalogapi.WantedLanguage{{Language: english, Forced: true}})
	file := h.episodeFile()
	h.imported(file)
	h.runDue()

	if jobs := h.agentJobs(); len(jobs) != 1 {
		t.Fatalf("RunAgentJob = %+v, want one for the regular track only", jobs)
	}
	if got := h.coverage(file.ID); got[string(english)+"+forced"] != api.CoverageMissing {
		t.Errorf("coverage = %v, want the forced track missing", got)
	}
}

// Seed 17.
func TestWhisperUsesTheConfiguredModel(t *testing.T) {
	h := newWhisperHarness(t, english)
	cfg, err := h.service.Settings(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	cfg.WhisperModel = transcodeapi.WhisperModelMedium
	if _, err := h.service.SetSettings(h.ctx, cfg); err != nil {
		t.Fatal(err)
	}
	h.imported(h.movieFile())
	h.runDue()

	if jobs := h.agentJobs(); len(jobs) != 1 || jobs[0].Model != transcodeapi.WhisperModelMedium {
		t.Fatalf("RunAgentJob = %+v, want the medium model", jobs)
	}
}

// Seed 7.
func TestRedeliveredWhisperResultStoresOneUnsyncedSidecar(t *testing.T) {
	h := newWhisperHarness(t, english)
	file := h.episodeFile()
	h.imported(file)
	h.runDue()
	h.sidecars.Put(stagedSubtitle, whisperSRT)
	job := domain.NewID[domain.JobID]()

	h.whisperDone(file.ID, job)
	h.whisperDone(file.ID, job)

	if content, ok := h.sidecars.Read(whisperSidecar); !ok || string(content) != string(whisperSRT) {
		t.Fatalf("sidecar = %q, %v; want the Whisper subtitle", content, ok)
	}
	if paths := h.sidecars.Paths(); len(paths) != 1 {
		t.Errorf("files = %v, want the staged subtitle moved", paths)
	}
	added := ofType[api.SubtitleAdded](t, h.events, api.TypeSubtitleAdded)
	want := api.SubtitleAdded{MediaFile: file.ID, Language: english, Source: api.SourceWhisper}
	if len(added) != 1 || added[0] != want {
		t.Fatalf("SubtitleAdded = %+v, want once %+v", added, want)
	}
	if subs := h.view(file.ID).Subtitles; len(subs) != 1 || subs[0].Synced || subs[0].Source != api.SourceWhisper {
		t.Fatalf("subtitles = %+v, want one unsynced Whisper row", subs)
	}
	if n := h.syncer.Called(fakes.SyncerSync); n != 0 {
		t.Errorf("synced %d times, want Whisper output never synced", n)
	}
	settled := ofType[api.CoverageSettled](t, h.events, api.TypeCoverageSettled)
	if len(settled) != 1 || !slices.Equal(settled[0].Satisfied, []domain.Language{english}) {
		t.Errorf("CoverageSettled = %+v, want English satisfied once", settled)
	}
}

func TestWhisperResultForAFoundTrackIsDropped(t *testing.T) {
	h := newWhisperHarness(t, english)
	file := h.episodeFile()
	h.imported(file)
	h.runDue()
	h.os.AddCandidates(h.episodeQuery(english), perfectEpisode(50, english))
	h.os.SetContent(50, srt)
	if _, err := h.service.Download(h.ctx, file.ID, english, false, 50); err != nil {
		t.Fatal(err)
	}
	h.sidecars.Put(stagedSubtitle, whisperSRT)

	h.whisperDone(file.ID, domain.NewID[domain.JobID]())

	if content, _ := h.sidecars.Read(whisperSidecar); string(content) != string(srt) {
		t.Errorf("sidecar = %q, want the OpenSubtitles one kept", content)
	}
	if _, staged := h.sidecars.Read(stagedSubtitle); staged {
		t.Error("the staged subtitle was left behind")
	}
}

func TestWhisperFailureMarksTheLanguageMissingAndTheSweepRetriesThreeTimes(t *testing.T) {
	h := newWhisperHarness(t, english)
	file := h.episodeFile()
	h.imported(file)
	h.runDue()

	h.whisperFailed(file.ID, false)
	if status := h.wanted(file.ID, english); status != subtitles.StatusWaitingWhisper {
		t.Fatalf("English = %s after a retryable failure, want still WAITING_WHISPER", status)
	}

	for range attemptsLimit {
		h.whisperFailed(file.ID, true)
		h.advance(sweepInterval)
		h.sweep()
	}

	if n := len(h.agentJobs()); n != attemptsLimit {
		t.Fatalf("RunAgentJob sent %d times, want %d", n, attemptsLimit)
	}
	if status := h.wanted(file.ID, english); status != subtitles.StatusMissing {
		t.Fatalf("English = %s, want MISSING once Whisper's attempts ran out", status)
	}
	missing := ofType[api.SubtitleMissing](t, h.events, api.TypeSubtitleMissing)
	want := api.SubtitleMissing{MediaFile: file.ID, Language: english, Reason: api.MissingWhisperFailed}
	if len(missing) != 1 || missing[0] != want {
		t.Fatalf("SubtitleMissing = %+v, want once %+v", missing, want)
	}
}

func TestWhisperResultWithoutItsStagedFileCountsAsAFailure(t *testing.T) {
	h := newWhisperHarness(t, english)
	file := h.episodeFile()
	h.imported(file)
	h.runDue()

	h.whisperDone(file.ID, domain.NewID[domain.JobID]())

	if got := h.coverage(file.ID); got[string(english)] != api.CoverageMissing {
		t.Fatalf("coverage = %v, want English missing", got)
	}
	missing := ofType[api.SubtitleMissing](t, h.events, api.TypeSubtitleMissing)
	if len(missing) != 1 || missing[0].Reason != api.MissingWhisperFailed {
		t.Fatalf("SubtitleMissing = %+v, want WHISPER_FAILED", missing)
	}
}
