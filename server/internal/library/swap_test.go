package library_test

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/domain"
	"magpie/internal/library"
	"magpie/internal/library/api"
	transcodeapi "magpie/internal/transcode/api"
)

const movieRuntime = 2 * time.Hour

func movieStreams() []api.Stream {
	return []api.Stream{
		{Index: 0, Kind: api.StreamVideo, Codec: "h264"},
		{Index: 1, Kind: api.StreamAudio, Codec: "aac", Language: "en"},
		{Index: 2, Kind: api.StreamSubtitle, Codec: "mov_text", Language: "en"},
	}
}

// importMP4 imports Dune as an .mp4 and returns its record.
func (f *importFixture) importMP4(t *testing.T, content string) api.MediaFile {
	t.Helper()
	rel := "Dune.2021.1080p.mp4"
	source := f.download(t, rel, movieRuntime)
	if err := f.files.WriteFile(source, []byte(content)); err != nil {
		t.Fatal(err)
	}
	f.prober.Set(source, api.MediaInfo{Container: "mov,mp4", Duration: movieRuntime, Streams: movieStreams()})
	f.deliver(t, f.movieDownload(acquisitionapi.DontSeed, rel))
	events := f.imported(t)
	file, err := f.repo.File(context.Background(), events[len(events)-1].MediaFile)
	if err != nil {
		t.Fatal(err)
	}
	return file
}

// stage writes a transcode of the file into its staging dir, as Transcode's upload would.
func (f *importFixture) stage(t *testing.T, file api.MediaFile) transcodeapi.TranscodeVerified {
	t.Helper()
	ctx := context.Background()
	source, err := f.repo.SourceForProcessing(ctx, file.ID)
	if err != nil {
		t.Fatal(err)
	}
	job := domain.NewID[domain.JobID]()
	staged := filepath.Join(source.StagingDir, string(job)+".mkv")
	if err := f.files.MkdirAll(source.StagingDir); err != nil {
		t.Fatal(err)
	}
	if err := f.files.WriteFile(staged, []byte("hevc transcode")); err != nil {
		t.Fatal(err)
	}
	streams := movieStreams()
	streams[0].Codec = transcodeapi.VideoCodecHEVC
	f.prober.Set(staged, api.MediaInfo{Container: "matroska,webm", Duration: movieRuntime + 200*time.Millisecond, Streams: streams})
	sum, err := f.hasher.SHA256(staged)
	if err != nil {
		t.Fatal(err)
	}
	return transcodeapi.TranscodeVerified{
		Job: job, MediaFile: file.ID, StagedPath: staged, SHA256: sum, SourceETag: source.ETag,
		Stats: transcodeapi.TranscodeStats{OldSize: file.Size, VideoCodec: transcodeapi.VideoCodecHEVC, DroppedAudio: []int{}, DroppedSubs: []int{}},
	}
}

// verify hands TranscodeVerified to Library's handler and runs the swap it queued.
func (f *importFixture) verify(t *testing.T, v transcodeapi.TranscodeVerified) {
	t.Helper()
	ctx := context.Background()
	if err := library.Handle(ctx, f.db, library.NewTranscodeVerifiedHandler(f.service), v); err != nil {
		t.Fatal(err)
	}
	for _, task := range f.queue.drain() {
		ran, err := library.RunSwapTask(ctx, f.service, task)
		if !ran {
			t.Fatalf("unexpected task %T", task)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

func (f *importFixture) rejections(t *testing.T) []api.TranscodeSwapRejected {
	t.Helper()
	var out []api.TranscodeSwapRejected
	for _, ev := range f.events.ofType(api.TypeTranscodeSwapRejected) {
		out = append(out, payload[api.TranscodeSwapRejected](t, ev))
	}
	return out
}

// Seed library #9.
func TestAVerifiedTranscodeIsSwappedIn(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	file := f.importMP4(t, "h264 original")
	v := f.stage(t, file)

	f.verify(t, v)

	got, err := f.repo.File(context.Background(), file.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got.Path, ".mkv") || !got.Transcoded || got.OSHash != file.OSHash || got.ReleaseName != file.ReleaseName {
		t.Fatalf("file = %+v, want an .mkv marked transcoded with the original OSHash", got)
	}
	if f.files.Exists(file.Path) || f.files.Exists(v.StagedPath) || !f.files.Exists(got.Path) {
		t.Fatal("want the .mp4 and the staged file gone and the .mkv in place")
	}
	if got.MediaInfo.Streams[0].Codec != transcodeapi.VideoCodecHEVC {
		t.Fatalf("media info = %+v, want the transcode's", got.MediaInfo)
	}
	var transcoded []api.FileTranscoded
	for _, ev := range f.events.ofType(api.TypeFileTranscoded) {
		transcoded = append(transcoded, payload[api.FileTranscoded](t, ev))
	}
	if len(transcoded) != 1 || transcoded[0].Path != got.Path || transcoded[0].OldSize != file.Size || transcoded[0].NewSize != got.Size {
		t.Fatalf("FileTranscoded = %+v", transcoded)
	}
	if refreshed := f.jellyfin.Refreshed(); !slices.Contains(refreshed, filepath.Dir(got.Path)) {
		t.Fatalf("Jellyfin refreshed %v, want the swapped file's folder", refreshed)
	}
	source, err := f.repo.SourceForProcessing(context.Background(), file.ID)
	if err != nil || source.ETag == v.SourceETag {
		t.Fatalf("ETag after the swap = %q, %v; want a new one", source.ETag, err)
	}

	f.verify(t, v)
	if n := len(f.events.ofType(api.TypeFileTranscoded)) + len(f.rejections(t)); n != 1 {
		t.Fatalf("a redelivered TranscodeVerified published %d more events, want none", n-1)
	}
}

// Seed library #10.
func TestATranscodeOfAChangedFileIsRejected(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	file := f.importMP4(t, "h264 original")
	v := f.stage(t, file)
	if err := f.files.WriteFile(file.Path, []byte("upgraded in place")); err != nil {
		t.Fatal(err)
	}

	f.verify(t, v)

	if got := f.rejections(t); len(got) != 1 || got[0].Reason != api.SwapSourceChanged || got[0].Job != v.Job {
		t.Fatalf("rejections = %+v, want SOURCE_CHANGED", got)
	}
	if content, _ := f.files.ReadFile(file.Path); string(content) != "upgraded in place" {
		t.Fatal("the file was touched")
	}
	if f.files.Exists(v.StagedPath) {
		t.Fatal("the staged file was kept")
	}
}

// Seed library #22. An NFS share gets a new device number each time a node mounts it, so the server
// moving to another node doesn't change the file.
func TestATranscodeIsSwappedInAfterTheLibraryIsRemounted(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	file := f.importMP4(t, "h264 original")
	v := f.stage(t, file)
	f.files.Remount("/data", remountedDevice)

	f.verify(t, v)

	if got := f.rejections(t); len(got) != 0 {
		t.Fatalf("rejections = %+v, want none", got)
	}
	if got, _ := f.repo.File(context.Background(), file.ID); !got.Transcoded {
		t.Fatalf("file = %+v, want it transcoded", got)
	}
}

// Seed library #11.
func TestATranscodeWithTheWrongHashIsRejected(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	file := f.importMP4(t, "h264 original")
	v := f.stage(t, file)
	v.SHA256 = strings.Repeat("0", len(v.SHA256))

	f.verify(t, v)

	if got := f.rejections(t); len(got) != 1 || got[0].Reason != api.SwapHashMismatch {
		t.Fatalf("rejections = %+v, want HASH_MISMATCH", got)
	}
	if got, _ := f.repo.File(context.Background(), file.ID); got.Transcoded || got.Path != file.Path {
		t.Fatalf("file = %+v, want it untouched", got)
	}
}

func TestATranscodeMissingAStreamIsRejected(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	file := f.importMP4(t, "h264 original")
	v := f.stage(t, file)
	v.Stats.DroppedSubs = []int{2}

	f.verify(t, v)

	if got := f.rejections(t); len(got) != 1 || got[0].Reason != api.SwapProbeFailed {
		t.Fatalf("rejections = %+v, want PROBE_FAILED", got)
	}
}

func TestATranscodeOfADeletedFileIsRejected(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	file := f.importMP4(t, "h264 original")
	v := f.stage(t, file)
	v.MediaFile = domain.NewID[domain.MediaFileID]()

	f.verify(t, v)

	if got := f.rejections(t); len(got) != 1 || got[0].Reason != api.SwapFileGone {
		t.Fatalf("rejections = %+v, want FILE_GONE", got)
	}
}

// Seed library #18.
func TestSourceForProcessingFollowsTheFileOnDisk(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	first := f.importMP4(t, "first release")
	ctx := context.Background()

	source, err := f.repo.SourceForProcessing(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	info := f.stat(t, first.Path)
	if source.Path != first.Path || source.Size != info.Size || source.StagingDir != filepath.Join(f.layout.movies, library.StagingDirName) {
		t.Fatalf("source = %+v", source)
	}
	if same, err := f.files.SameDevice(filepath.Dir(source.StagingDir), source.Path); err != nil || !same {
		t.Fatalf("staging dir on the same device = %v, %v", same, err)
	}

	upgrade := f.importMP4(t, "the upgrade")
	replaced, err := f.repo.SourceForProcessing(ctx, upgrade.ID)
	if err != nil {
		t.Fatal(err)
	}
	if replaced.Path != source.Path || replaced.ETag == source.ETag {
		t.Fatalf("after the upgrade ETag = %q, before %q; want a new one at the same path", replaced.ETag, source.ETag)
	}
}
