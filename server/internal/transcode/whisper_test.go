package transcode

import (
	"context"
	"strings"
	"testing"
	"time"

	"magpie/internal/domain"
	"magpie/internal/transcode/api"
	"magpie/internal/transcode/api/agentproto"
)

const (
	english         domain.Language = "en"
	japanese        domain.Language = "ja"
	audioLateBy                     = 4 * time.Second
	englishAudio                    = 1
	whisperSubtitle                 = "1\n00:00:05,000 --> 00:00:06,500\nHello there\n\n"
)

// whisperHello is an agent whose only device passed the Whisper test.
func whisperHello(hostname string) agentproto.Hello {
	hello := capableHello(hostname)
	hello.Devices[0].Capabilities = []agentproto.CapabilityResult{
		{Capability: agentproto.CapEncHEVC10, OK: false, Error: sessionError},
		{Capability: agentproto.CapWhisperVulkan, OK: true},
	}
	return hello
}

func (f *fixture) runAgentJob(file sourceFile, language domain.Language) {
	f.t.Helper()
	run := api.RunAgentJob{Kind: api.JobKindWhisper, MediaFile: file.ID, Language: language, Model: api.WhisperModelLargeV3Turbo}
	f.handle(newRunAgentJobHandler(f.jobs), run)
}

// Seed 25.
func TestWhisperJobRunsOnlyOnAWhisperDeviceAndCompletes(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.runAgentJob(file, english)
	f.connect(capableHello("encoder-only"))
	whisper, _ := f.connect(whisperHello("whisper-box"))

	spec := f.lease(whisper)
	if spec.Kind != agentproto.KindWhisper || spec.Transcode != nil || spec.Whisper == nil || !strings.HasSuffix(spec.OutputName, ".srt") {
		t.Fatalf("spec = %+v, want a Whisper job uploading an SRT", spec)
	}
	f.uploadAndReport(whisper, spec, []byte(whisperSubtitle))
	f.runVerify()

	staged := f.job(spec.Job).StagingPath
	completed := decodeEvents[api.WhisperCompleted](t, f.published(api.TypeWhisperCompleted))
	want := api.WhisperCompleted{Job: spec.Job, MediaFile: file.ID, Language: english, StagedPath: staged}
	if len(completed) != 1 || completed[0] != want {
		t.Fatalf("WhisperCompleted = %+v, want %+v", completed, want)
	}
	if job := f.job(spec.Job); job.State != api.StateCompleted || job.FinishedAt == nil || !exists(staged) {
		t.Fatalf("job = %s, staged kept = %v; want COMPLETED with the SRT left for Subtitles", job.State, exists(staged))
	}
}

// Seed 25: an SRT without cues fails the job.
func TestEmptyWhisperSubtitleFailsTheJob(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.runAgentJob(file, english)
	whisper, _ := f.connect(whisperHello("whisper-box"))
	spec := f.lease(whisper)
	f.uploadAndReport(whisper, spec, []byte("\n"))
	staged := f.job(spec.Job).StagingPath

	f.runVerify()

	failed := decodeEvents[api.JobFailed](t, f.published(api.TypeJobFailed))
	want := api.JobFailed{Job: spec.Job, Kind: api.JobKindWhisper, MediaFile: file.ID, Reason: ReasonNoCues, Final: true, Language: english}
	if len(failed) != 1 || failed[0] != want {
		t.Fatalf("JobFailed = %+v, want %+v", failed, want)
	}
	if n := len(f.published(api.TypeWhisperCompleted)); n != 0 {
		t.Errorf("WhisperCompleted published %d times, want none", n)
	}
	f.lifetime.Wait()
	if job := f.job(spec.Job); job.State != api.StateFailed || exists(staged) {
		t.Fatalf("job = %s, staged kept = %v; want FAILED and the upload removed", job.State, exists(staged))
	}
}

// Seed 26, the server's part.
func TestWhisperOffsetIsTheAudioStartAfterTheVideo(t *testing.T) {
	f := newFixture(t)
	info := h264Movie(0)
	info.Streams[englishAudio].StartTime = audioLateBy
	file := f.addFile(info)
	f.runAgentJob(file, english)
	whisper, _ := f.connect(whisperHello("whisper-box"))

	params := f.offered(whisper).Spec.Whisper
	want := agentproto.WhisperParams{Language: english, AudioStream: englishAudio, StartOffsetSec: audioLateBy.Seconds(), Model: api.WhisperModelLargeV3Turbo}
	if params == nil || *params != want {
		t.Fatalf("WhisperParams = %+v, want %+v", params, want)
	}
}

func TestWhisperListensToTheTrackInItsLanguage(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.runAgentJob(file, japanese)
	whisper, _ := f.connect(whisperHello("whisper-box"))

	params := f.offered(whisper).Spec.Whisper
	if params == nil || params.AudioStream != englishAudio+1 || params.Language != japanese {
		t.Fatalf("WhisperParams = %+v, want the Japanese track", params)
	}
}

func TestWhisperJobIsOfferedAheadOfAnEarlierTranscode(t *testing.T) {
	f := newFixture(t)
	movie := f.addFile(h264Movie(0))
	f.cutoff(movie, domain.OriginDownload)
	f.clock.Advance(time.Minute)
	f.runAgentJob(movie, english)
	hello := capableHello("both")
	hello.Devices[0].Capabilities = append(hello.Devices[0].Capabilities, agentproto.CapabilityResult{Capability: agentproto.CapWhisperVulkan, OK: true})
	agent, _ := f.connect(hello)

	if spec := f.offered(agent).Spec; spec.Kind != agentproto.KindWhisper {
		t.Fatalf("first offer is a %s job, want the Whisper one", spec.Kind)
	}
}

func TestRepeatedRunAgentJobQueuesOneJob(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.runAgentJob(file, english)
	f.runAgentJob(file, english)

	job := f.onlyJob(file)
	if job.Kind != api.JobKindWhisper || job.Language != english || job.WhisperModel != api.WhisperModelLargeV3Turbo || job.Priority != priorityWhisper {
		t.Fatalf("job = %+v", job)
	}
	queued := decodeEvents[api.JobQueued](t, f.published(api.TypeJobQueued))
	if len(queued) != 1 || queued[0].Kind != api.JobKindWhisper {
		t.Fatalf("JobQueued = %+v, want one Whisper job", queued)
	}
}

func TestCancelledTranscodeJobPublishesJobCancelled(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.cutoff(file, domain.OriginDownload)
	job := f.onlyJob(file)

	if err := f.jobs.Cancel(context.Background(), job.ID, ReasonCancelled); err != nil {
		t.Fatal(err)
	}

	cancelled := decodeEvents[api.JobCancelled](t, f.published(api.TypeJobCancelled))
	want := api.JobCancelled{Job: job.ID, MediaFile: file.ID, Reason: ReasonCancelled}
	if len(cancelled) != 1 || cancelled[0] != want {
		t.Fatalf("JobCancelled = %+v, want %+v", cancelled, want)
	}
	if failed := f.published(api.TypeJobFailed); len(failed) != 0 {
		t.Fatalf("JobFailed = %+v, want none for a transcode", failed)
	}
}

func TestCancelledWhisperJobRepliesWithAFinalJobFailed(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.runAgentJob(file, english)
	job := f.onlyJob(file)

	if err := f.jobs.Cancel(context.Background(), job.ID, ReasonCancelled); err != nil {
		t.Fatal(err)
	}

	failed := decodeEvents[api.JobFailed](t, f.published(api.TypeJobFailed))
	want := api.JobFailed{Job: job.ID, Kind: api.JobKindWhisper, MediaFile: file.ID, Reason: ReasonCancelled, Final: true, Language: english}
	if len(failed) != 1 || failed[0] != want {
		t.Fatalf("JobFailed = %+v, want %+v", failed, want)
	}
}
