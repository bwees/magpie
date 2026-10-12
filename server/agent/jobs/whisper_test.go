package jobs

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"magpie/agent/encode"
	"magpie/agent/whisper"
	"magpie/internal/domain"
	"magpie/internal/transcode/api/agentproto"
)

const (
	testWhisperModel = "small"
	transcript       = "1\n00:00:01,000 --> 00:00:02,000\nHello\n\n"
	shiftedBy        = 4.0
	shifted          = "1\n00:00:05,000 --> 00:00:06,000\nHello\n\n"
)

// whisperRunner answers ffmpeg's audio extraction and whisper.cpp like a clean transcription.
type whisperRunner struct{}

func (whisperRunner) Run(_ context.Context, c encode.Command) (string, string, error) {
	if c.Name == whisper.DefaultWhisper {
		of := c.Args[slices.Index(c.Args, "-of")+1]
		return "", "", os.WriteFile(of+".srt", []byte(transcript), filePerm)
	}
	return "", "", os.WriteFile(c.Args[len(c.Args)-1], []byte("RIFF"), filePerm)
}

// newTranscriber runs with fake programs and models already on disk, so nothing is downloaded.
func newTranscriber(t *testing.T) *whisper.Transcriber {
	t.Helper()
	models := whisper.NewModels(t.TempDir())
	for _, name := range []string{"ggml-" + testWhisperModel + ".bin", filepath.Base(models.VADURL)} {
		if err := os.WriteFile(filepath.Join(models.Dir, name), []byte("weights"), filePerm); err != nil {
			t.Fatal(err)
		}
	}
	return &whisper.Transcriber{Runner: whisperRunner{}, FFmpeg: encode.DefaultFFmpeg, Whisper: whisper.DefaultWhisper, Models: models}
}

func whisperOffer(id domain.JobID) agentproto.JobOffer {
	return agentproto.JobOffer{Spec: agentproto.JobSpec{
		Job: id, Kind: agentproto.KindWhisper, DeviceID: testDevice,
		Source:    agentproto.SourceRef{URL: agentproto.SourcePath(id), Size: int64(len(sourceContent)), ETag: testETag},
		Whisper:   &agentproto.WhisperParams{Language: "en", AudioStream: 1, StartOffsetSec: shiftedBy, Model: testWhisperModel},
		OutputURL: agentproto.OutputPath(id), OutputName: string(id) + ".srt",
	}}
}

func TestAWhisperJobUploadsTheShiftedSubtitle(t *testing.T) {
	f := newFixture(t, t.TempDir())

	f.accept(t, whisperOffer(domain.NewID[domain.JobID]()))
	result := f.sender.result(t)

	if result.Outcome != agentproto.OutcomeUploaded || result.Verify.OutputSize != int64(len(shifted)) || result.Verify.SHA256 == "" {
		t.Fatalf("result = %+v", result)
	}
	if got := string(f.server.uploaded()); got != shifted {
		t.Fatalf("uploaded %q, want %q", got, shifted)
	}
}

func TestAWhisperJobNeedsAWhisperDevice(t *testing.T) {
	f := newFixture(t, t.TempDir())
	f.manager.SetDevices([]agentproto.DeviceReport{{
		DeviceID: testDevice, Vendor: agentproto.VendorNVIDIA,
		Capabilities: []agentproto.CapabilityResult{{Capability: agentproto.CapEncHEVC10, OK: true}},
	}})

	if ok, reason := f.manager.Offer(whisperOffer(domain.NewID[domain.JobID]())); ok || reason != agentproto.RejectCapability {
		t.Fatalf("accepted %v reason %s, want a CAPABILITY reject", ok, reason)
	}
}
