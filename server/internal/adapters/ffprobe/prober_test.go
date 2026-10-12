package ffprobe

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"magpie/internal/library"
	"magpie/internal/library/api"
)

func TestMediaInfoFromARecordedProbe(t *testing.T) {
	raw, err := os.ReadFile("testdata/hdr10-movie.json")
	if err != nil {
		t.Fatal(err)
	}
	out, err := decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	info := out.mediaInfo()

	if info.Container != "matroska" || info.Duration != 9312416*time.Millisecond || info.BitrateKbps != 25000 ||
		info.Width != 3840 || info.Height != 2160 || info.HDR != api.HDR10 {
		t.Errorf("info = %+v", info)
	}
	want := []api.Stream{
		{Index: 0, Kind: api.StreamVideo, Codec: "hevc", Language: "en", Default: true, BitDepth: 10, BitrateKbps: 24000},
		{Index: 1, Kind: api.StreamAudio, Codec: "eac3", Language: "de", Default: true, Channels: 6, BitrateKbps: 640, StartTime: 4 * time.Second},
		{Index: 2, Kind: api.StreamSubtitle, Codec: "subrip", Language: "en", Forced: true},
		{Index: 3, Kind: api.StreamSubtitle, Codec: "hdmv_pgs_subtitle", Language: "und", Image: true},
	}
	if !slices.Equal(info.Streams, want) {
		t.Errorf("streams = %+v\nwant %+v", info.Streams, want)
	}
}

func TestProbeReadsARealFile(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if _, probeErr := exec.LookPath(DefaultBinary); err != nil || probeErr != nil {
		t.Skip("ffmpeg or ffprobe not installed")
	}
	video := filepath.Join(t.TempDir(), "clip.mkv")
	generate := exec.Command(ffmpeg, "-loglevel", "error", "-f", "lavfi", "-i", "testsrc=duration=2:size=320x240:rate=25",
		"-f", "lavfi", "-i", "sine=duration=2", "-metadata:s:a:0", "language=fre", video)
	if out, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("generate fixture: %v: %s", err, out)
	}

	info, err := New().Probe(context.Background(), video)
	if err != nil {
		t.Fatal(err)
	}
	if info.Container != "matroska" || info.Width != 320 || info.Duration < time.Second || len(info.Streams) != 2 {
		t.Errorf("info = %+v", info)
	}
	if info.Streams[1].Kind != api.StreamAudio || info.Streams[1].Language != "fr" {
		t.Errorf("audio stream = %+v", info.Streams[1])
	}
}

func TestProbeFailsOnAFileItCantRead(t *testing.T) {
	if _, err := exec.LookPath(DefaultBinary); err != nil {
		t.Skip("ffprobe not installed")
	}
	path := filepath.Join(t.TempDir(), "not-a-video.mkv")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := New().Probe(context.Background(), path); !errors.Is(err, library.ErrProbeFailed) {
		t.Errorf("err = %v, want ErrProbeFailed", err)
	}
}

func TestProbeWithoutFFprobeSaysSo(t *testing.T) {
	p := &Prober{binary: "magpie-no-such-ffprobe"}
	if _, err := p.Probe(context.Background(), "/movie.mkv"); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("err = %v, want ErrNotInstalled", err)
	}
}
