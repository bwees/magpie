package subsync

import (
	"context"
	"io"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
)

func TestFFmpegArgsDecodeFirstAudioTrackTo16kMono(t *testing.T) {
	args := ffmpegArgs("/media/movie.mkv")
	for _, want := range [][]string{
		{"-i", "/media/movie.mkv"},
		{"-map", "0:a:0"},
		{"-ac", "1"},
		{"-ar", strconv.Itoa(SampleRate)},
		{"-f", "s16le"},
	} {
		at := slices.Index(args, want[0])
		if at < 0 || at+1 >= len(args) || args[at+1] != want[1] {
			t.Errorf("args %v lack %v", args, want)
		}
	}
}

func TestFFmpegDecodesAudio(t *testing.T) {
	binary, err := exec.LookPath(DefaultFFmpegBinary)
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	const seconds = 2
	video := filepath.Join(t.TempDir(), "tone.mka")
	generate := exec.Command(binary, "-loglevel", "error", "-f", "lavfi",
		"-i", "sine=frequency=440:sample_rate=48000:duration="+strconv.Itoa(seconds),
		"-ac", "2", video)
	if out, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("generate fixture: %v: %s", err, out)
	}

	stream, err := FFmpeg{}.DecodeAudio(context.Background(), video)
	if err != nil {
		t.Fatal(err)
	}
	pcm, readErr := io.ReadAll(stream)
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	if readErr != nil {
		t.Fatal(readErr)
	}
	if want := seconds * SampleRate * bytesPerSample; len(pcm) != want {
		t.Errorf("decoded %d bytes, want %d", len(pcm), want)
	}
}

func TestFFmpegReportsMissingInput(t *testing.T) {
	if _, err := exec.LookPath(DefaultFFmpegBinary); err != nil {
		t.Skip("ffmpeg not installed")
	}
	stream, err := FFmpeg{}.DecodeAudio(context.Background(), filepath.Join(t.TempDir(), "missing.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(stream)
	if err := stream.Close(); err == nil {
		t.Error("Close returned nil for a missing input")
	}
}
