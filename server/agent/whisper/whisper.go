// Package whisper runs a Whisper job on the agent: it extracts the chosen audio track as 16 kHz mono
// PCM, transcribes it with whisper.cpp and Silero VAD, and shifts every cue by the audio's start offset.
// Spec: docs/spec/contexts/transcode.md#agent-side-job-run, step 5.
package whisper

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"magpie/agent/encode"
	"magpie/internal/platform/srt"
	"magpie/internal/transcode/api/agentproto"
)

const (
	DefaultWhisper = "whisper-cli"

	audioName      = "audio.wav"
	transcriptBase = "transcript"
	srtExtension   = ".srt"
	outputPerm     = 0o640

	// whisper.cpp reads 16 kHz mono 16-bit PCM.
	sampleRate = "16000"
	monoAudio  = "1"
	pcm16      = "pcm_s16le"

	cudaPrefix       = "cuda:"
	renderNodePrefix = "/dev/dri/renderD"
	// firstRenderNode is renderD128, the first GPU's render node; Vulkan numbers devices from 0.
	firstRenderNode = 128
)

// Transcriber runs ffmpeg and whisper.cpp. Tests replace the Runner with recorded answers.
type Transcriber struct {
	Runner  encode.Runner
	FFmpeg  string
	Whisper string
	Models  *Models
}

func New(models *Models) *Transcriber {
	return &Transcriber{Runner: encode.ExecRunner{}, FFmpeg: encode.DefaultFFmpeg, Whisper: DefaultWhisper, Models: models}
}

// Job is one transcription.
type Job struct {
	Params agentproto.WhisperParams
	Device agentproto.DeviceReport
	Input  encode.Input
	// Duration is the source's length in seconds, for progress.
	Duration float64
	// Dir holds the extracted audio and whisper.cpp's own output.
	Dir    string
	Output string
	// Progress receives the share of the audio transcribed.
	Progress func(fraction float64)
}

// Transcribe writes the job's subtitle to Output.
func (t *Transcriber) Transcribe(ctx context.Context, job Job) error {
	audio := filepath.Join(job.Dir, audioName)
	if err := t.run(ctx, encode.Command{Name: t.FFmpeg, Args: extractArgs(job, audio)}); err != nil {
		return fmt.Errorf("extracting audio stream %d: %w", job.Params.AudioStream, err)
	}
	model, err := t.Models.Model(ctx, job.Params.Model)
	if err != nil {
		return err
	}
	vad, err := t.Models.VAD(ctx)
	if err != nil {
		return err
	}
	transcript := filepath.Join(job.Dir, transcriptBase)
	transcribe := encode.Command{
		Name:   t.Whisper,
		Args:   transcribeArgs(job, model, vad, audio, transcript),
		OnLine: progressOf(job),
	}
	if err := t.run(ctx, transcribe); err != nil {
		return fmt.Errorf("whisper.cpp: %w", err)
	}
	raw, err := os.ReadFile(transcript + srtExtension)
	if err != nil {
		return err
	}
	cues, err := srt.Parse(raw)
	if err != nil {
		return err
	}
	offset := time.Duration(job.Params.StartOffsetSec * float64(time.Second))
	return os.WriteFile(job.Output, srt.Format(srt.Shift(cues, offset)), outputPerm)
}

func (t *Transcriber) run(ctx context.Context, cmd encode.Command) error {
	_, stderr, err := t.Runner.Run(ctx, cmd)
	if err != nil {
		return fmt.Errorf("%w: %s", err, lastLine(stderr))
	}
	return nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

func extractArgs(job Job, audio string) []string {
	args := append([]string{"-hide_banner", "-nostdin", "-v", "error"}, job.Input.Args...)
	return append(args, "-i", job.Input.Path, "-map", "0:"+strconv.Itoa(job.Params.AudioStream), "-vn",
		"-ac", monoAudio, "-ar", sampleRate, "-c:a", pcm16, "-y", audio)
}

func transcribeArgs(job Job, model, vad, audio, transcript string) []string {
	args := []string{
		"-m", model, "-f", audio, "-l", string(job.Params.Language),
		"--vad", "-vm", vad, "-osrt", "-of", transcript, "-np",
	}
	return append(args, deviceArgs(job.Device)...)
}

// deviceArgs picks the GPU whisper.cpp runs on. Vulkan numbers devices from 0 in the order the render
// nodes and CUDA indexes have; the CPU device runs without a GPU.
func deviceArgs(device agentproto.DeviceReport) []string {
	if device.Vendor == agentproto.VendorCPU {
		return []string{"-ng"}
	}
	if index, ok := strings.CutPrefix(device.DeviceID, cudaPrefix); ok {
		return []string{"-dev", index}
	}
	if node, ok := strings.CutPrefix(device.DeviceID, renderNodePrefix); ok {
		if n, err := strconv.Atoi(node); err == nil {
			return []string{"-dev", strconv.Itoa(n - firstRenderNode)}
		}
	}
	return nil
}

// segmentLine is how whisper.cpp prints each segment it transcribed: [00:01:02.500 --> 00:01:04.000]  text
var segmentLine = regexp.MustCompile(`^\[\d+:\d{2}:\d{2}\.\d{3} --> (\d+):(\d{2}):(\d{2})\.(\d{3})\]`)

// progressOf reports how far into the audio the last printed segment ends.
func progressOf(job Job) func(string) {
	return func(line string) {
		match := segmentLine.FindStringSubmatch(line)
		if match == nil || job.Progress == nil || job.Duration <= 0 {
			return
		}
		units := []time.Duration{time.Hour, time.Minute, time.Second, time.Millisecond}
		var end time.Duration
		for i, part := range match[1:] {
			n, _ := strconv.Atoi(part)
			end += time.Duration(n) * units[i]
		}
		job.Progress(min(end.Seconds()/job.Duration, 1))
	}
}
