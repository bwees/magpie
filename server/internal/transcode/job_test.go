package transcode

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"testing"
	"time"

	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/transcode/api"
	"magpie/internal/transcode/api/agentproto"
)

func decodeEvents[T any](t *testing.T, payloads []string) []T {
	t.Helper()
	events := make([]T, 0, len(payloads))
	for _, raw := range payloads {
		var e T
		if err := json.Unmarshal([]byte(raw), &e); err != nil {
			t.Fatal(err)
		}
		events = append(events, e)
	}
	return events
}

// Seed 1.
func TestCutoffQueuesATranscodeCappedAtTheResolutionsRate(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))

	f.cutoff(file, domain.OriginDownload)

	job := f.onlyJob(file)
	if job.State != api.StateQueued || job.MaxRateKbps != defaultCap1080Mbps*kbpsPerMbps || job.Attempt != 1 {
		t.Fatalf("job = %+v, want QUEUED with MaxRateKbps %d", job, defaultCap1080Mbps*kbpsPerMbps)
	}
	a, _ := f.connect(capableHello("gpu-a"))
	offer := f.offered(a)
	if offer.Spec.Transcode.MaxRateKbps != 10000 || offer.Spec.Transcode.BufSizeKbps != 20000 ||
		offer.Spec.Transcode.Encoder != EncoderHEVCNVENC || offer.Spec.Source.ETag == "" {
		t.Fatalf("offer = %+v", offer.Spec.Transcode)
	}
}

func TestCutoffSkipsAFileAlreadyInTheTargetCodec(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(hevcOutput())

	f.cutoff(file, domain.OriginDownload)

	skipped := decodeEvents[api.TranscodeSkipped](t, f.published(api.TypeTranscodeSkipped))
	if len(skipped) != 1 || skipped[0].Reason != api.SkipAlreadyTarget {
		t.Fatalf("skips = %+v, want ALREADY_TARGET", skipped)
	}
	f.cutoff(file, domain.OriginDownload)
	if n := len(f.published(api.TypeTranscodeSkipped)); n != 1 {
		t.Fatalf("skipped %d times, want once under one policy", n)
	}
}

// Seed 7.
func TestRedeliveredCutoffMetQueuesOneJob(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))

	f.cutoff(file, domain.OriginDownload)
	f.cutoff(file, domain.OriginDownload)

	f.onlyJob(file)
	if n := len(f.published(api.TypeJobQueued)); n != 1 {
		t.Fatalf("JobQueued published %d times, want once", n)
	}
}

// Seed 29.
func TestLifecycleEventsAcrossALeaseExpiry(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.cutoff(file, domain.OriginLibraryScan)
	a, _ := f.connect(capableHello("gpu-a"))

	spec := f.lease(a)
	f.clock.Advance(DefaultTimings.LeaseTTL + time.Second)
	f.schedule()
	f.inState(spec.Job, api.StateQueued)
	_ = a.conn.CloseNow()
	b, _ := f.connect(capableHello("gpu-b"))
	f.lease(b)

	queued := decodeEvents[api.JobQueued](t, f.published(api.TypeJobQueued))
	if len(queued) != 1 || queued[0].Origin != domain.OriginLibraryScan || queued[0].Title != file.Title {
		t.Fatalf("JobQueued = %+v, want one with origin LIBRARY_SCAN", queued)
	}
	started := decodeEvents[api.JobStarted](t, f.published(api.TypeJobStarted))
	want := []api.JobStarted{
		{Job: spec.Job, Kind: api.JobKindTranscode, MediaFile: file.ID, Agent: a.id, Attempt: 1},
		{Job: spec.Job, Kind: api.JobKindTranscode, MediaFile: file.ID, Agent: b.id, Attempt: 2},
	}
	if !slices.Equal(started, want) {
		t.Fatalf("JobStarted = %+v, want %+v", started, want)
	}
}

// Seed 11.
func TestLeaseExpiryRequeuesAwayFromTheAgentForTwoMinutes(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.cutoff(file, domain.OriginDownload)
	a, _ := f.connect(capableHello("gpu-a"))
	spec := f.lease(a)
	f.beat(a, spec.Job, agentproto.StageEncoding, 0.5)

	f.clock.Advance(DefaultTimings.LeaseTTL + time.Second)
	f.schedule()

	job := f.inState(spec.Job, api.StateQueued)
	if job.Attempt != 2 || job.AgentID != nil {
		t.Fatalf("job = %+v, want attempt 2 and no agent", job)
	}
	failed := decodeEvents[api.JobFailed](t, f.published(api.TypeJobFailed))
	if len(failed) != 1 || failed[0].Final {
		t.Fatalf("JobFailed = %+v, want one that isn't final", failed)
	}
	f.clock.Advance(DefaultTimings.AvoidAgent - 2*time.Second)
	f.schedule()
	if job := f.job(spec.Job); job.State != api.StateQueued {
		t.Fatalf("job = %s, want it kept from agent A for 2 minutes", job.State)
	}
	f.clock.Advance(2 * time.Second)
	if offer := f.offered(a); offer.Spec.Job != spec.Job {
		t.Fatalf("offer = %s, want %s once the 2 minutes passed", offer.Spec.Job, spec.Job)
	}
}

// Seed 12.
func TestHeartbeatsKeepAnUploadsLeaseAlive(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.cutoff(file, domain.OriginDownload)
	a, _ := f.connect(capableHello("gpu-a"))
	spec := f.lease(a)

	for beat := range 5 * time.Minute / DefaultTimings.HeartbeatEvery {
		f.clock.Advance(DefaultTimings.HeartbeatEvery)
		f.beat(a, spec.Job, agentproto.StageUploading, float64(beat)/100)
		f.schedule()
	}

	if job := f.job(spec.Job); job.State != api.StateUploading || job.Attempt != 1 {
		t.Fatalf("job = %s attempt %d, want still UPLOADING on attempt 1", job.State, job.Attempt)
	}
}

// Seed 13: a restarted agent back with the job's scratch gets it back inside its avoid window, and the
// server resumes the download at the agent's offset.
func TestRestartedAgentResumesItsJob(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.cutoff(file, domain.OriginDownload)
	hello := capableHello("gpu-a")
	a, _ := f.connect(hello)
	spec := f.lease(a)
	const have = 100
	f.beat(a, spec.Job, agentproto.StageFetching, float64(have)/float64(len(file.content)))

	_ = a.conn.CloseNow()
	f.clock.Advance(DefaultTimings.LeaseTTL + time.Second)
	f.schedule()
	f.inState(spec.Job, api.StateQueued)
	hello.ResumableJobs = []agentproto.ResumableJob{{Job: spec.Job, SourceETag: spec.Source.ETag, BytesHave: have}}
	a, _ = f.connect(hello)
	offer := f.offered(a)
	if offer.Spec.Job != spec.Job || offer.Spec.Source.ETag != spec.Source.ETag {
		t.Fatalf("offer = %+v, want job %s back with the same ETag", offer.Spec, spec.Job)
	}
	a.send(agentproto.TypeJobAccept, agentproto.JobAccept{Job: spec.Job})
	f.inState(spec.Job, api.StateLeased)

	header := http.Header{}
	header.Set("Range", "bytes="+strconv.Itoa(have)+"-")
	header.Set("If-Range", spec.Source.ETag)
	resp := f.request(a.id, http.MethodGet, spec.Source.URL, header, nil)
	if body := readAll(t, resp); resp.StatusCode != http.StatusPartialContent || string(body) != string(file.content[have:]) {
		t.Fatalf("resumed download = %d, %d bytes; want 206 with the rest", resp.StatusCode, len(body))
	}
}

// Seed 14, the server's part.
func TestSourceChangedMidDownloadServesTheWholeNewFile(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.cutoff(file, domain.OriginDownload)
	a, _ := f.connect(capableHello("gpu-a"))
	spec := f.lease(a)
	f.replaceSource(file)

	header := http.Header{}
	header.Set("Range", "bytes=10-")
	header.Set("If-Range", spec.Source.ETag)
	resp := f.request(a.id, http.MethodGet, spec.Source.URL, header, nil)

	body := readAll(t, resp)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("ETag") == spec.Source.ETag || len(body) <= len(file.content) {
		t.Fatalf("download = %d with ETag %s and %d bytes; want 200 with the new file", resp.StatusCode, resp.Header.Get("ETag"), len(body))
	}
	a.send(agentproto.TypeJobResult, agentproto.JobResult{Job: spec.Job, Outcome: agentproto.OutcomeFailed, Error: agentproto.ErrorSourceChanged})
	if job := f.inState(spec.Job, api.StateFailed); job.Error != ReasonSourceChanged {
		t.Fatalf("job error = %q, want %s and no retry", job.Error, ReasonSourceChanged)
	}
}

// Seed 15.
func TestSourceChangedBeforeTheSwapFailsVerification(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.cutoff(file, domain.OriginDownload)
	a, _ := f.connect(capableHello("gpu-a"))
	spec := f.lease(a)
	output := []byte("hevc output")
	f.uploadAndReport(a, spec, output)
	f.prober.set(f.job(spec.Job).StagingPath, hevcOutput())

	f.replaceSource(file)
	f.runVerify()

	if job := f.job(spec.Job); job.State != api.StateFailed || job.Error != ReasonSourceChanged {
		t.Fatalf("job = %s %q, want FAILED SOURCE_CHANGED", job.State, job.Error)
	}
	if n := len(f.published(api.TypeTranscodeVerified)); n != 0 {
		t.Fatalf("TranscodeVerified published %d times, want none", n)
	}
}

// The happy path through the server: upload, the server's checks, then TranscodeVerified for Library.
func TestVerifiedUploadPublishesTranscodeVerified(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.cutoff(file, domain.OriginDownload)
	a, _ := f.connect(capableHello("gpu-a"))
	spec := f.lease(a)
	output := []byte("hevc output")
	f.uploadAndReport(a, spec, output)
	staged := f.job(spec.Job).StagingPath
	f.prober.set(staged, hevcOutput())

	f.runVerify()

	verified := decodeEvents[api.TranscodeVerified](t, f.published(api.TypeTranscodeVerified))
	if len(verified) != 1 {
		t.Fatalf("TranscodeVerified = %+v, want one", verified)
	}
	got := verified[0]
	if got.StagedPath != staged || got.SourceETag != "library-"+string(file.ID) || got.Stats.NewSize != int64(len(output)) ||
		got.Stats.OldSize != file.Size || got.Stats.VideoCodec != api.VideoCodecHEVC || got.Stats.Agent != a.id {
		t.Fatalf("TranscodeVerified = %+v", got)
	}
	if staged != stagedPath(f.root+"/.magpie-staging", spec.Job) {
		t.Fatalf("staged at %s, want the library's staging dir", staged)
	}
	if job := f.job(spec.Job); job.State != api.StateVerified {
		t.Fatalf("job = %s, want VERIFIED", job.State)
	}
}

// Seed 16.
func TestDigestMismatchEmptiesTheStagedFile(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.cutoff(file, domain.OriginDownload)
	a, _ := f.connect(capableHello("gpu-a"))
	spec := f.lease(a)
	output := []byte("hevc output")

	resp := f.put(a.id, spec.Job, 0, []byte("hevc outpuX"), output, true)

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("PUT = %d, want 422", resp.StatusCode)
	}
	head := f.request(a.id, http.MethodHead, spec.OutputURL, nil, nil)
	if head.Header.Get(agentproto.HeaderUploadOffset) != "0" {
		t.Fatalf("offset after a digest mismatch = %s, want 0", head.Header.Get(agentproto.HeaderUploadOffset))
	}
	if resp := f.put(a.id, spec.Job, 0, output, output, true); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("re-upload = %d, want 204", resp.StatusCode)
	}
}

// Seed 17.
func TestOffsetMismatchAnswersTheCommittedOffset(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.cutoff(file, domain.OriginDownload)
	a, _ := f.connect(capableHello("gpu-a"))
	spec := f.lease(a)
	const committed, ahead = 4 << 20, 8 << 20
	output := make([]byte, 2*ahead)

	if resp := f.put(a.id, spec.Job, 0, output[:committed], output, false); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("first chunk = %d", resp.StatusCode)
	}
	resp := f.put(a.id, spec.Job, ahead, output[ahead:], output, true)

	if resp.StatusCode != http.StatusConflict || resp.Header.Get(agentproto.HeaderUploadOffset) != "4194304" {
		t.Fatalf("PUT at 8 MiB = %d with Upload-Offset %s, want 409 with 4194304", resp.StatusCode, resp.Header.Get(agentproto.HeaderUploadOffset))
	}
}

// Seed 18.
func TestAgentVerificationFailureFailsTheJobWithoutAnUpload(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.cutoff(file, domain.OriginDownload)
	a, _ := f.connect(capableHello("gpu-a"))
	spec := f.lease(a)
	report := passingReport([]byte("output"))
	report.CopiedStreamsOK = false

	a.send(agentproto.TypeJobResult, agentproto.JobResult{Job: spec.Job, Outcome: agentproto.OutcomeFailed, Error: agentproto.ErrorVerifyFailed, Verify: report})

	if job := f.inState(spec.Job, api.StateFailed); job.Error != ReasonCopiedStreams {
		t.Fatalf("job error = %q, want %q", job.Error, ReasonCopiedStreams)
	}
	if exists(f.job(spec.Job).StagingPath) || len(f.published(api.TypeTranscodeVerified)) != 0 {
		t.Fatal("an output was staged or verified")
	}
}

// Seed 20, the HTTP part: the third transfer over the cap is told to come back.
func TestTransferCapRefusesASourceDownloadWhileFull(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.cutoff(file, domain.OriginDownload)
	a, _ := f.connect(capableHello("gpu-a"))
	spec := f.lease(a)
	for _, job := range []domain.JobID{domain.NewID[domain.JobID](), domain.NewID[domain.JobID]()} {
		if !f.gate.Acquire(TransferHolder{Job: job, Direction: DirectionOutput}, RankUpload, defaultTransferCap) {
			t.Fatal("upload refused under the cap")
		}
	}

	resp := f.request(a.id, http.MethodGet, spec.Source.URL, nil, nil)

	if resp.StatusCode != http.StatusServiceUnavailable || resp.Header.Get(agentproto.HeaderRetryAfter) != "5" {
		t.Fatalf("download = %d Retry-After %q, want 503 and 5", resp.StatusCode, resp.Header.Get(agentproto.HeaderRetryAfter))
	}
}

// Seed 22.
func TestThreeFailedAttemptsFailTheJobUntilARetry(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.cutoff(file, domain.OriginDownload)
	agents := []*testAgent{}
	for _, name := range []string{"gpu-a", "gpu-b", "gpu-c"} {
		a, _ := f.connect(capableHello(name))
		agents = append(agents, a)
	}

	var job domain.JobID
	for attempt := 1; attempt <= MaxAttempts; attempt++ {
		offer := f.offeredToAny(agents)
		job = offer.Spec.Job
		holder := f.agentNamed(agents, offer)
		holder.send(agentproto.TypeJobAccept, agentproto.JobAccept{Job: job})
		f.inState(job, api.StateLeased)
		holder.send(agentproto.TypeJobResult, agentproto.JobResult{Job: job, Outcome: agentproto.OutcomeFailed, Error: "encoder crashed"})
		f.eventually(job, "the attempt to end", func(r jobRow) bool { return r.State == api.StateFailed || r.Attempt == attempt+1 })
		f.clock.Advance(retryBackoff[len(retryBackoff)-1])
	}

	failed := decodeEvents[api.JobFailed](t, f.published(api.TypeJobFailed))
	if len(failed) != MaxAttempts || !failed[MaxAttempts-1].Final || failed[0].Final {
		t.Fatalf("JobFailed = %+v, want %d with only the last final", failed, MaxAttempts)
	}
	if row := f.job(job); row.State != api.StateFailed {
		t.Fatalf("job = %s, want FAILED", row.State)
	}
	if err := f.jobs.Retry(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if row := f.job(job); row.State != api.StateQueued || row.Attempt != 1 {
		t.Fatalf("after retry job = %s attempt %d, want QUEUED attempt 1", row.State, row.Attempt)
	}
}

func (f *fixture) offeredToAny(agents []*testAgent) agentproto.JobOffer {
	f.t.Helper()
	f.schedule()
	row := f.firstOffered()
	if row == nil {
		f.t.Fatal("no agent was offered a job")
	}
	for _, a := range agents {
		if *row.AgentID == a.id {
			return decodeBody[agentproto.JobOffer](f.t, a.expect(agentproto.TypeJobOffer))
		}
	}
	f.t.Fatal("the offer went to an unknown agent")
	return agentproto.JobOffer{}
}

func (f *fixture) firstOffered() *jobRow {
	var rows []jobRow
	if err := f.db.Read(context.Background()).Where("state = ?", api.StateOffered).Find(&rows).Error; err != nil {
		f.t.Fatal(err)
	}
	if len(rows) == 0 {
		return nil
	}
	return &rows[0]
}

func (f *fixture) agentNamed(agents []*testAgent, offer agentproto.JobOffer) *testAgent {
	row := f.job(offer.Spec.Job)
	for _, a := range agents {
		if a.id == *row.AgentID {
			return a
		}
	}
	f.t.Fatal("offer went to an unknown agent")
	return nil
}

// Seeds 23 and 31.
func TestDeletedFileCancelsItsJobOnTheAgent(t *testing.T) {
	for _, reason := range []libraryapi.DeleteReason{libraryapi.DeleteRemoved, libraryapi.DeleteUpgraded} {
		t.Run(string(reason), func(t *testing.T) {
			f := newFixture(t)
			file := f.addFile(h264Movie(0))
			f.cutoff(file, domain.OriginDownload)
			a, _ := f.connect(capableHello("gpu-a"))
			spec := f.lease(a)
			f.beat(a, spec.Job, agentproto.StageUploading, 0)
			if resp := f.put(a.id, spec.Job, 0, []byte("part"), []byte("partial"), false); resp.StatusCode != http.StatusNoContent {
				t.Fatalf("chunk = %d", resp.StatusCode)
			}
			staged := f.job(spec.Job).StagingPath

			f.handle(newFileDeletedHandler(f.jobs), libraryapi.FileDeleted{MediaFile: file.ID, Title: file.Title, Path: file.Path, Reason: reason})

			if cancel := decodeBody[agentproto.Cancel](t, a.expect(agentproto.TypeCancel)); cancel.Job != spec.Job {
				t.Fatalf("cancel = %+v, want job %s", cancel, spec.Job)
			}
			if job := f.job(spec.Job); job.State != api.StateCancelled {
				t.Fatalf("job = %s, want CANCELLED", job.State)
			}
			if exists(staged) {
				t.Fatal("staged output left behind")
			}
		})
	}
}

func TestRemovedTitleCancelsItsJobs(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.cutoff(file, domain.OriginDownload)

	f.handle(newTitleRemovedHandler(f.jobs), catalogapi.TitleRemoved{Title: file.Title})

	if job := f.onlyJob(file); job.State != api.StateCancelled || job.Error != ReasonTitleRemoved {
		t.Fatalf("job = %s %q, want CANCELLED", job.State, job.Error)
	}
}

// verified runs a job through to VERIFIED.
func (f *fixture) verified() (sourceFile, agentproto.JobSpec, *testAgent) {
	f.t.Helper()
	file := f.addFile(h264Movie(0))
	f.cutoff(file, domain.OriginDownload)
	a, _ := f.connect(capableHello("gpu-a"))
	spec := f.lease(a)
	f.uploadAndReport(a, spec, []byte("hevc output"))
	f.prober.set(f.job(spec.Job).StagingPath, hevcOutput())
	f.runVerify()
	f.inState(spec.Job, api.StateVerified)
	return file, spec, a
}

// Seed 24.
func TestFileTranscodedTwiceSwapsOnce(t *testing.T) {
	f := newFixture(t)
	file, spec, _ := f.verified()
	transcoded := libraryapi.FileTranscoded{MediaFile: file.ID, Path: file.Path, OldSize: file.Size, NewSize: 1}

	f.handle(newFileTranscodedHandler(f.jobs), transcoded)
	finished := f.job(spec.Job).FinishedAt
	f.handle(newFileTranscodedHandler(f.jobs), transcoded)

	if job := f.job(spec.Job); job.State != api.StateSwapped || !job.FinishedAt.Equal(*finished) {
		t.Fatalf("job = %+v, want SWAPPED once", job)
	}
}

// Seed 30.
func TestSwapRejectedFailsOrRetriesByReason(t *testing.T) {
	t.Run("source changed fails for good", func(t *testing.T) {
		f := newFixture(t)
		file, spec, _ := f.verified()
		staged := f.job(spec.Job).StagingPath

		f.handle(newSwapRejectedHandler(f.jobs), libraryapi.TranscodeSwapRejected{Job: spec.Job, MediaFile: file.ID, Reason: libraryapi.SwapSourceChanged})

		if job := f.job(spec.Job); job.State != api.StateFailed {
			t.Fatalf("job = %s, want FAILED", job.State)
		}
		if exists(staged) {
			t.Fatal("staged output left behind")
		}
	})
	t.Run("hash mismatch retries", func(t *testing.T) {
		f := newFixture(t)
		file, spec, _ := f.verified()

		f.handle(newSwapRejectedHandler(f.jobs), libraryapi.TranscodeSwapRejected{Job: spec.Job, MediaFile: file.ID, Reason: libraryapi.SwapHashMismatch})

		if job := f.job(spec.Job); job.State != api.StateQueued || job.Attempt != 2 || job.NotBefore == nil {
			t.Fatalf("job = %+v, want QUEUED attempt 2 after a backoff", job)
		}
	})
}

// Seed 27.
func TestAnotherAgentCantReadTheSource(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.cutoff(file, domain.OriginDownload)
	a, _ := f.connect(capableHello("gpu-a"))
	spec := f.lease(a)
	b, _ := f.connect(capableHello("gpu-b"))

	if resp := f.request(b.id, http.MethodGet, spec.Source.URL, nil, nil); resp.StatusCode != http.StatusConflict {
		t.Fatalf("agent B's download = %d, want 409", resp.StatusCode)
	}
	if resp := f.request(b.id, http.MethodHead, spec.OutputURL, nil, nil); resp.StatusCode != http.StatusConflict {
		t.Fatalf("agent B's HEAD = %d, want 409", resp.StatusCode)
	}
}

func TestTheOfferedAgentMayFetchBeforeItsAcceptLands(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.cutoff(file, domain.OriginDownload)
	a, _ := f.connect(capableHello("gpu-a"))
	spec := f.offered(a).Spec

	resp := f.request(a.id, http.MethodGet, spec.Source.URL, nil, nil)

	if body := readAll(t, resp); resp.StatusCode != http.StatusOK || len(body) != len(file.content) {
		t.Fatalf("download while OFFERED = %d with %d bytes, want the whole source", resp.StatusCode, len(body))
	}
}

// Seed 33.
func TestRetryGoesToAnotherAgent(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.cutoff(file, domain.OriginDownload)
	a, _ := f.connect(capableHello("gpu-a"))
	spec := f.lease(a)
	a.send(agentproto.TypeJobResult, agentproto.JobResult{Job: spec.Job, Outcome: agentproto.OutcomeFailed, Error: "encoder crashed"})
	f.inState(spec.Job, api.StateQueued)
	b, _ := f.connect(capableHello("gpu-b"))

	f.clock.Advance(retryBackoff[0])
	f.schedule()

	if job := f.job(spec.Job); job.State != api.StateOffered || *job.AgentID != b.id {
		t.Fatalf("job = %s to %v, want OFFERED to B", job.State, job.AgentID)
	}
}

func TestOfferTimeoutRequeuesWithoutAnAttempt(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.cutoff(file, domain.OriginDownload)
	a, _ := f.connect(capableHello("gpu-a"))
	spec := f.offered(a).Spec

	f.clock.Advance(DefaultTimings.OfferTimeout + time.Second)
	if err := f.jobs.expireOffers(context.Background()); err != nil {
		t.Fatal(err)
	}

	if job := f.job(spec.Job); job.State != api.StateQueued || job.Attempt != 1 {
		t.Fatalf("job = %s attempt %d, want QUEUED attempt 1", job.State, job.Attempt)
	}
	a.send(agentproto.TypeJobAccept, agentproto.JobAccept{Job: spec.Job})
	if cancel := decodeBody[agentproto.Cancel](t, a.expect(agentproto.TypeCancel)); cancel.Job != spec.Job {
		t.Fatalf("late accept got %+v, want a cancel", cancel)
	}
}

func TestNotWorthItRecordsASkip(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.cutoff(file, domain.OriginDownload)
	a, _ := f.connect(capableHello("gpu-a"))
	spec := f.lease(a)

	a.send(agentproto.TypeJobResult, agentproto.JobResult{Job: spec.Job, Outcome: agentproto.OutcomeNotWorthIt, Error: "9% smaller"})

	f.inState(spec.Job, api.StateNotWorthIt)
	skipped := decodeEvents[api.TranscodeSkipped](t, f.published(api.TypeTranscodeSkipped))
	if len(skipped) != 1 || skipped[0].Reason != api.SkipNotWorthIt {
		t.Fatalf("skips = %+v, want NOT_WORTH_IT", skipped)
	}
	f.cutoff(file, domain.OriginDownload)
	if n := len(f.published(api.TypeJobQueued)); n != 1 {
		t.Fatalf("JobQueued %d times, want no new job under the same policy", n)
	}
}

func TestStrayJobInAHeartbeatIsCancelled(t *testing.T) {
	f := newFixture(t)
	a, _ := f.connect(capableHello("gpu-a"))
	stray := domain.NewID[domain.JobID]()

	a.send(agentproto.TypeHeartbeat, agentproto.Heartbeat{Jobs: []agentproto.JobHeartbeat{{Job: stray, Stage: agentproto.StageEncoding}}})

	if cancel := decodeBody[agentproto.Cancel](t, a.expect(agentproto.TypeCancel)); cancel.Job != stray {
		t.Fatalf("cancel = %+v, want %s", cancel, stray)
	}
}
