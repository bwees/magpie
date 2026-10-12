package whisper

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"magpie/agent/encode"
	"magpie/internal/transcode/api/agentproto"
)

const (
	testModel     = "large-v3-turbo"
	modelBytes    = "model weights"
	vadBytes      = "vad weights"
	audioSeconds  = 10.0
	whisperOutput = "1\n00:00:01,000 --> 00:00:02,000\nHello\n\n2\n00:00:03,000 --> 00:00:04,500\nthere\n\n"
)

// runner answers ffmpeg by writing the audio file and whisper.cpp by printing segments and writing its SRT.
type runner struct {
	mu       sync.Mutex
	commands []encode.Command
	failOn   string
}

func (r *runner) Run(_ context.Context, c encode.Command) (string, string, error) {
	r.mu.Lock()
	r.commands = append(r.commands, c)
	r.mu.Unlock()
	if c.Name == r.failOn {
		return "", "something broke\nlast words", errors.New("exit status 1")
	}
	last := c.Args[len(c.Args)-1]
	switch c.Name {
	case encode.DefaultFFmpeg:
		return "", "", os.WriteFile(last, []byte("RIFF"), outputPerm)
	case DefaultWhisper:
		for _, line := range []string{"[00:00:01.000 --> 00:00:02.000]  Hello", "[00:00:03.000 --> 00:00:04.500]  there"} {
			c.OnLine(line)
		}
		of := c.Args[slices.Index(c.Args, "-of")+1]
		return "", "", os.WriteFile(of+srtExtension, []byte(whisperOutput), outputPerm)
	}
	return "", "", nil
}

func (r *runner) command(name string) encode.Command {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.commands {
		if c.Name == name {
			return c
		}
	}
	return encode.Command{}
}

// modelServer serves the model files and counts how often each was fetched.
func modelServer(t *testing.T) (*httptest.Server, map[string]int) {
	t.Helper()
	fetched := map[string]int{}
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		fetched[r.URL.Path]++
		mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "ggml-"+testModel+".bin"):
			_, _ = w.Write([]byte(modelBytes))
		case strings.HasSuffix(r.URL.Path, "silero.bin"):
			_, _ = w.Write([]byte(vadBytes))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server, fetched
}

func newTranscriber(t *testing.T) (*Transcriber, *runner, map[string]int) {
	t.Helper()
	server, fetched := modelServer(t)
	models := &Models{Dir: filepath.Join(t.TempDir(), "models"), ModelsURL: server.URL + "/models", VADURL: server.URL + "/vad/silero.bin", Client: server.Client()}
	r := &runner{}
	return &Transcriber{Runner: r, FFmpeg: encode.DefaultFFmpeg, Whisper: DefaultWhisper, Models: models}, r, fetched
}

func testJob(t *testing.T, device agentproto.DeviceReport, offset float64, progress func(float64)) Job {
	dir := t.TempDir()
	return Job{
		Params:   agentproto.WhisperParams{Language: "en", AudioStream: 1, StartOffsetSec: offset, Model: testModel},
		Device:   device,
		Input:    encode.Input{Path: filepath.Join(dir, "source")},
		Duration: audioSeconds,
		Dir:      dir,
		Output:   filepath.Join(dir, "output.srt"),
		Progress: progress,
	}
}

var nvidia = agentproto.DeviceReport{DeviceID: "cuda:1", Vendor: agentproto.VendorNVIDIA}

// Seed 26, the agent's part.
func TestCuesAreShiftedByTheAudioOffset(t *testing.T) {
	transcriber, _, _ := newTranscriber(t)
	job := testJob(t, nvidia, 4.0, nil)

	if err := transcriber.Transcribe(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(job.Output)
	if err != nil {
		t.Fatal(err)
	}
	want := "1\n00:00:05,000 --> 00:00:06,000\nHello\n\n2\n00:00:07,000 --> 00:00:08,500\nthere\n\n"
	if string(got) != want {
		t.Fatalf("subtitle = %q, want %q", got, want)
	}
}

func TestAudioIsExtractedAndTranscribedWithTheJobsLanguageAndModel(t *testing.T) {
	transcriber, r, fetched := newTranscriber(t)
	var progress []float64
	job := testJob(t, nvidia, 0, func(f float64) { progress = append(progress, f) })

	if err := transcriber.Transcribe(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	extract := r.command(encode.DefaultFFmpeg).String()
	for _, want := range []string{"-i " + job.Input.Path, "-map 0:1", "-ac 1", "-ar 16000", "-c:a pcm_s16le"} {
		if !strings.Contains(extract, want) {
			t.Errorf("ffmpeg %q lacks %q", extract, want)
		}
	}
	transcribe := r.command(DefaultWhisper).String()
	model := filepath.Join(transcriber.Models.Dir, "ggml-"+testModel+".bin")
	vad := filepath.Join(transcriber.Models.Dir, "silero.bin")
	for _, want := range []string{"-m " + model, "-l en", "--vad -vm " + vad, "-osrt", "-dev 1"} {
		if !strings.Contains(transcribe, want) {
			t.Errorf("whisper-cli %q lacks %q", transcribe, want)
		}
	}
	if !slices.Equal(progress, []float64{0.2, 0.45}) {
		t.Errorf("progress = %v, want each segment's end over the duration", progress)
	}

	if err := transcriber.Transcribe(context.Background(), testJob(t, nvidia, 0, nil)); err != nil {
		t.Fatal(err)
	}
	for path, n := range fetched {
		if n != 1 {
			t.Errorf("%s fetched %d times, want once", path, n)
		}
	}
}

func TestDeviceArgs(t *testing.T) {
	cases := []struct {
		device agentproto.DeviceReport
		want   []string
	}{
		{nvidia, []string{"-dev", "1"}},
		{agentproto.DeviceReport{DeviceID: "/dev/dri/renderD129", Vendor: agentproto.VendorIntel}, []string{"-dev", "1"}},
		{agentproto.DeviceReport{DeviceID: "cpu", Vendor: agentproto.VendorCPU}, []string{"-ng"}},
	}
	for _, c := range cases {
		if got := deviceArgs(c.device); !slices.Equal(got, c.want) {
			t.Errorf("deviceArgs(%s) = %v, want %v", c.device.DeviceID, got, c.want)
		}
	}
}

func TestWhisperFailureNamesItsLastLine(t *testing.T) {
	transcriber, r, _ := newTranscriber(t)
	r.failOn = DefaultWhisper
	err := transcriber.Transcribe(context.Background(), testJob(t, nvidia, 0, nil))
	if err == nil || !strings.Contains(err.Error(), "last words") {
		t.Fatalf("err = %v, want whisper.cpp's last line", err)
	}
}

func TestUnknownModelNameIsRefused(t *testing.T) {
	models := NewModels(t.TempDir())
	if _, err := models.Model(context.Background(), "../etc/passwd"); !errors.Is(err, ErrUnknownModel) {
		t.Fatalf("err = %v, want ErrUnknownModel", err)
	}
}
