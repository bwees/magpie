package flows_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"magpie/internal/acquisition"
	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/activity"
	activityapi "magpie/internal/activity/api"
	"magpie/internal/domain"
	"magpie/internal/library"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/transcode"
	transcodeapi "magpie/internal/transcode/api"
	"magpie/internal/transcode/api/agentproto"

	"go.uber.org/fx"
)

const (
	transcodeTmdb    = domain.TmdbID(4004)
	transcodeName    = "Transcode Flow"
	transcodeRelease = "Transcode.Flow.2024.1080p.WEB-DL.DDP5.1.H.264-GRP"
	uhdRelease       = "Transcode.Flow.2024.2160p.WEB-DL.DDP5.1.H.265-GRP"
	uhdGroup         = "2160p"
	uhdResolution    = 2160
	editedSource     = "edited after the transcode was verified"

	// shortLease lets an agent that never sends a heartbeat lose its job within the flow.
	shortLease = 500 * time.Millisecond
	shortSweep = 50 * time.Millisecond
	// longAvoid keeps the job away from the only agent until the flow ends.
	longAvoid = time.Hour
)

// transcoded is a downloaded movie and the agent's job that transcoded it.
type transcoded struct {
	file   libraryapi.FileImported
	job    agentproto.JobSpec
	output []byte
}

// downloadMovie requests a movie as the admin, whose requests need no approval, and imports its one release
// with its extra files.
func (s *stack) downloadMovie(tmdb domain.TmdbID, name, releaseName string, info libraryapi.MediaInfo, extras ...string) (domain.TitleID, libraryapi.FileImported) {
	s.t.Helper()
	s.movieOnTMDB(tmdb, name, movieYear)
	item := release(releaseName, releaseSeeders)
	s.answer(item)
	s.request(s.session, tmdb)
	title := s.titleAdded(tmdb)
	return title, s.download(item, info, extras...)
}

// runTranscode takes the job offered for file and runs it as an agent does: accept, fetch the source, encode
// and upload. ffprobe reads the upload as a good HEVC transcode.
func (s *stack) runTranscode(agent *fakeAgent, file libraryapi.FileImported) transcoded {
	s.t.Helper()
	offer := agent.offer()
	if offer.Spec.Kind != agentproto.KindTranscode || offer.Spec.Transcode == nil {
		s.t.Fatalf("offer = %+v, want a transcode", offer.Spec)
	}
	staged := filepath.Join(s.movies, library.StagingDirName, offer.Spec.OutputName)
	s.prober.Set(staged, hevcOf(file.MediaInfo))
	agent.accept(offer.Spec)
	s.awaitJobState(offer.Spec.Job, transcodeapi.StateLeased)
	source := agent.fetchSource(offer.Spec)
	if onDisk, err := os.ReadFile(file.Path); err != nil || !bytes.Equal(source, onDisk) {
		s.t.Fatalf("fetched %q, want the library file (%v)", source, err)
	}
	output := []byte("hevc of " + string(source))
	agent.deliver(offer.Spec, output)
	return transcoded{file: file, job: offer.Spec, output: output}
}

// swappedProblems lists what is wrong if the transcode didn't replace the file and stop its upgrades.
func (s *stack) swappedProblems(done transcoded) []string {
	s.t.Helper()
	var problems []string
	id := done.file.MediaFile
	started := published(s.harness, transcodeapi.TypeJobStarted, func(e transcodeapi.JobStarted) bool { return e.Job == done.job.Job })
	verified := published(s.harness, transcodeapi.TypeTranscodeVerified, func(e transcodeapi.TranscodeVerified) bool { return e.Job == done.job.Job })
	swapped := published(s.harness, libraryapi.TypeFileTranscoded, func(e libraryapi.FileTranscoded) bool { return e.MediaFile == id })
	if len(started) != 1 || len(verified) != 1 || len(swapped) != 1 || swapped[0].NewSize != int64(len(done.output)) {
		problems = append(problems, fmt.Sprintf("started %+v, verified %+v, transcoded %+v; want one each", started, verified, swapped))
	}
	if got := s.libraryFile(id); !got.Transcoded || got.Path != done.file.Path {
		problems = append(problems, fmt.Sprintf("library file = %+v, want transcoded in place", got))
	}
	if content, err := os.ReadFile(done.file.Path); err != nil || !bytes.Equal(content, done.output) {
		problems = append(problems, fmt.Sprintf("file content = %q, %v; want the agent's output", content, err))
	}
	if job := s.transcodeJob(done.job.Job); job.State != transcodeapi.StateSwapped {
		problems = append(problems, fmt.Sprintf("job = %+v, want SWAPPED", job))
	}
	cutoff := s.cutoff(done.file.Title)
	if !slices.ContainsFunc(cutoff, func(c acquisition.CutoffStatusResponse) bool { return c.File == id && !c.UpgradesAllowed }) {
		problems = append(problems, fmt.Sprintf("cutoff = %+v, want upgrades stopped", cutoff))
	}
	return problems
}

// Flow 4: a downloaded file meets its cutoff, Transcode queues it, an agent transcodes it, Library swaps the
// output in, and Acquisition stops upgrading it even once a better quality is wanted.
func TestDownloadedFileIsTranscodedAndNoLongerUpgraded(t *testing.T) {
	s := newStack(t)
	agent := connectAgent(s.harness, agentproto.CapEncHEVC10)
	title, file := s.downloadMovie(transcodeTmdb, transcodeName, transcodeRelease, h264Movie())

	queued := awaitEvent(s.harness, transcodeapi.TypeJobQueued, func(e transcodeapi.JobQueued) bool { return e.MediaFile == file.MediaFile })
	if queued.Kind != transcodeapi.JobKindTranscode || queued.Origin != domain.OriginDownload {
		t.Fatalf("JobQueued = %+v, want a DOWNLOAD transcode", queued)
	}
	done := s.runTranscode(agent, file)
	swapped := func() []string { return s.swappedProblems(done) }
	settles(t, "the transcoded file", swapped)

	s.editDefaultProfile(func(p *acquisition.ProfileRequest) {
		p.Items = append([]acquisition.ProfileItemRequest{s.qualityGroup(uhdGroup, uhdResolution)}, p.Items...)
		p.Cutoff = 0
	})
	eventually(t, "the file falls below the raised cutoff", func() bool {
		cutoff := s.cutoff(title)
		return len(cutoff) == 1 && !cutoff[0].CutoffMet
	})
	s.answer(release(uhdRelease, releaseSeeders))
	search := s.searchByHand(title)
	if len(search.Releases) != 1 || !slices.ContainsFunc(search.Releases[0].Rejections, func(r acquisitionapi.Rejection) bool {
		return r.Reason == acquisitionapi.RejectUpgradeNotAllowed
	}) {
		t.Fatalf("releases = %+v, want the 2160p release rejected as an upgrade of a transcoded file", search.Releases)
	}

	s.redeliverAll()
	settles(t, "the transcoded file after redelivery", swapped)
}

// sourceEditor rewrites a library file when Library first looks at a staged transcode, as an edit landing
// between Transcode's verify and Library's swap would.
type sourceEditor struct {
	library.FileSystem
	mu   sync.Mutex
	edit func()
}

func (e *sourceEditor) arm(edit func()) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.edit = edit
}

func (e *sourceEditor) Stat(path string) (library.FileInfo, error) {
	if strings.Contains(path, library.StagingDirName) {
		e.mu.Lock()
		edit := e.edit
		e.edit = nil
		e.mu.Unlock()
		if edit != nil {
			edit()
		}
	}
	return e.FileSystem.Stat(path)
}

// Flow 4's refusal: the file changes after Transcode verified the output but before Library swaps it, so
// Library keeps the changed file and the job fails.
func TestASourceChangedBeforeTheSwapIsKept(t *testing.T) {
	editor := &sourceEditor{}
	s := newStack(t, fx.Decorate(func(fs library.FileSystem) library.FileSystem {
		editor.FileSystem = fs
		return editor
	}))
	agent := connectAgent(s.harness, agentproto.CapEncHEVC10)
	_, file := s.downloadMovie(transcodeTmdb, transcodeName, transcodeRelease, h264Movie())
	editor.arm(func() { writeFile(t, file.Path, []byte(editedSource)) })

	done := s.runTranscode(agent, file)
	staged := filepath.Join(s.movies, library.StagingDirName, done.job.OutputName)
	rejected := func() []string {
		var problems []string
		refused := published(s.harness, libraryapi.TypeTranscodeSwapRejected, func(e libraryapi.TranscodeSwapRejected) bool { return e.Job == done.job.Job })
		if len(refused) != 1 || refused[0].Reason != libraryapi.SwapSourceChanged {
			problems = append(problems, fmt.Sprintf("swap rejections = %+v, want one SOURCE_CHANGED", refused))
		}
		if n := len(published(s.harness, libraryapi.TypeFileTranscoded, func(e libraryapi.FileTranscoded) bool { return e.MediaFile == file.MediaFile })); n != 0 {
			problems = append(problems, fmt.Sprintf("%d FileTranscoded, want none", n))
		}
		if job := s.transcodeJob(done.job.Job); job.State != transcodeapi.StateFailed {
			problems = append(problems, fmt.Sprintf("job = %+v, want FAILED", job))
		}
		if got := s.libraryFile(file.MediaFile); got.Transcoded {
			problems = append(problems, fmt.Sprintf("library file = %+v, want it untouched", got))
		}
		if content, err := os.ReadFile(file.Path); err != nil || string(content) != editedSource {
			problems = append(problems, fmt.Sprintf("file content = %q, %v; want the edit kept", content, err))
		}
		if exists(staged) {
			problems = append(problems, "the staged output is still on disk")
		}
		return problems
	}
	settles(t, "the refused swap", rejected)

	s.redeliverAll()
	settles(t, "the refused swap after redelivery", rejected)
}

// A lost lease puts the job back in the queue, away from the silent agent, so the file leaves Live now.
func TestALostLeaseTakesTheTranscodeOffLiveNow(t *testing.T) {
	s := newStack(t, fx.Decorate(func(timings transcode.Timings) transcode.Timings {
		timings.LeaseTTL, timings.SweepEvery, timings.AvoidAgent = shortLease, shortSweep, longAvoid
		return timings
	}))
	agent := connectAgent(s.harness, agentproto.CapEncHEVC10)
	title, _ := s.downloadMovie(transcodeTmdb, transcodeName, transcodeRelease, h264Movie())
	offer := agent.offer()
	agent.accept(offer.Spec)

	queued := func() []string {
		var problems []string
		failed := published(s.harness, transcodeapi.TypeJobFailed, func(e transcodeapi.JobFailed) bool { return e.Job == offer.Spec.Job })
		if len(failed) != 1 || failed[0].Final {
			problems = append(problems, fmt.Sprintf("job failures = %+v, want one to retry", failed))
		}
		rows := getJSON[[]activity.PipelineRowResponse](s.harness, pipelinePath+titleQuery+string(title))
		if len(rows) != 1 || rows[0].Transcode == nil || rows[0].Transcode.State != activityapi.TranscodeQueued {
			problems = append(problems, fmt.Sprintf("pipeline = %+v, want the transcode queued", rows))
		}
		active := getJSON[[]activity.PipelineRowResponse](s.harness, pipelinePath+activeQuery)
		if slices.ContainsFunc(active, func(r activity.PipelineRowResponse) bool { return r.TitleID == string(title) }) {
			problems = append(problems, fmt.Sprintf("Live now = %+v, want the title gone", active))
		}
		return problems
	}
	settles(t, "the lost lease", queued)

	s.redeliverAll()
	settles(t, "the lost lease after redelivery", queued)
}
