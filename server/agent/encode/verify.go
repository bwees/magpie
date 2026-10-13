package encode

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"magpie/internal/transcode/api/agentproto"
)

const (
	// vmafWindow is the length of each sampled VMAF window, in seconds.
	vmafWindow = 10
	half       = 0.5
	// vmafPrecision keeps two decimals of the mean.
	vmafPrecision = 100
)

// vmafScore is the line libvmaf logs at the end of a run.
var vmafScore = regexp.MustCompile(`VMAF score: ([0-9.]+)`)

// Verify checks the output against the source and the spec: it decodes cleanly, runs as long, has the
// streams the spec asked for, copied streams are bit-identical, HDR10 metadata survived, and a sampled
// VMAF. The size and SHA-256 are filled in by the caller.
func (e *Encoder) Verify(ctx context.Context, job Job) (agentproto.VerifyReport, error) {
	params := job.params()
	output := Input{Path: job.Output}
	report := agentproto.VerifyReport{DecodeOK: e.decodes(ctx, job)}
	out, err := e.Probe(ctx, output)
	if err != nil {
		return report, nil
	}
	report.DurationDelta = out.Duration - job.Source.Duration
	report.StreamMapOK = streamMapOK(job, out)
	if report.FrameDelta, err = e.frameDelta(ctx, job, output); err != nil {
		return report, err
	}
	if report.CopiedStreamsOK, err = e.copiedStreamsOK(ctx, job); err != nil {
		return report, err
	}
	if params.HDR10 {
		kept, err := e.hdr10Kept(ctx, output)
		if err != nil {
			return report, err
		}
		report.HDR10Kept = &kept
	}
	if params.VMAFSamples > 0 {
		report.VMAFMean = e.sampledVMAF(ctx, job, params.VMAFSamples)
	}
	return report, ctx.Err()
}

// HardwareCheck reports whether the decode check can run on the GPU: the device passed the decode test
// for the output's codec.
func HardwareCheck(job Job) bool {
	if _, ok := pipelines[job.Device.Vendor]; !ok {
		return false
	}
	params := job.params()
	needs, ok := decodeCapability(params.Codec, params.BitDepth)
	return ok && passed(job.Device, needs)
}

// CheckArgs builds the decode check: the whole output decoded to the null muxer, stopping at the first
// error. hardwareDecode keeps the frames on the GPU, since nothing reads them.
func CheckArgs(job Job, hardwareDecode bool) []string {
	args := []string{"-hide_banner", "-nostdin", "-v", "error", "-xerror", "-progress", "pipe:1", "-nostats"}
	if line, ok := pipelines[job.Device.Vendor]; ok && hardwareDecode {
		args = slices.Concat(args, line.device(job.Device.DeviceID), line.decode)
	}
	return slices.Concat(args, Input{Path: job.Output}.args(), []string{"-map", "0:v", "-map", "0:a?", "-f", "null", "-"})
}

// decodes runs the decode check. A hardware check that fails before its first frame runs again on the
// CPU: ffmpeg couldn't open the device or its decoder, which says nothing about the file. One that fails
// after a frame hit an error in the file, and fails the check.
func (e *Encoder) decodes(ctx context.Context, job Job) bool {
	hardware := HardwareCheck(job)
	decoded, err := e.decode(ctx, job, hardware)
	if err != nil && hardware && !decoded && ctx.Err() == nil {
		slog.WarnContext(ctx, "agent: the hardware decode check didn't start; checking on the CPU", "job", job.Spec.Job, "error", err)
		_, err = e.decode(ctx, job, false)
	}
	return err == nil
}

// decode runs the decode check once. decoded reports whether ffmpeg wrote a frame before it stopped.
func (e *Encoder) decode(ctx context.Context, job Job, hardwareDecode bool) (decoded bool, err error) {
	progress := newProgressParser(job)
	_, stderr, err := e.Runner.Run(ctx, Command{Name: e.FFmpeg, Args: CheckArgs(job, hardwareDecode), OnLine: progress.line})
	if err != nil {
		err = fmt.Errorf("ffmpeg: %w: %s", err, firstLine(stderr))
	}
	return progress.frames > 0, err
}

// streamMapOK checks the output holds the encoded video in the target codec and depth, then the kept
// audio and subtitles in the spec's order with their codecs.
func streamMapOK(job Job, out Media) bool {
	params := job.params()
	var got []Stream
	for _, s := range out.Streams {
		if s.Type != TypeAttachment && s.Disposition.AttachedPic == 0 {
			got = append(got, s)
		}
	}
	kept := slices.Concat(params.KeepAudio, params.KeepSubs)
	if len(got) != len(kept)+1 || got[0].Type != TypeVideo || got[0].Codec != params.Codec || got[0].BitDepth() != params.BitDepth {
		return false
	}
	for i, index := range kept {
		source, ok := job.Source.Stream(index)
		if !ok || got[i+1].Type != source.Type || got[i+1].Codec != matroskaCodec(source.Codec) {
			return false
		}
	}
	return true
}

// matroskaCodec is the codec a copied stream has in the MKV output.
func matroskaCodec(codec string) string {
	if codec == movText {
		return "subrip"
	}
	return codec
}

// frameDelta compares the video frame counts, for constant-rate sources only.
func (e *Encoder) frameDelta(ctx context.Context, job Job, output Input) (int, error) {
	video, ok := job.Source.Video()
	if !ok || !video.ConstantRate() {
		return 0, nil
	}
	source, err := e.videoFrames(ctx, job.Input)
	if err != nil {
		return 0, err
	}
	encoded, err := e.videoFrames(ctx, output)
	return encoded - source, err
}

func (e *Encoder) videoFrames(ctx context.Context, in Input) (int, error) {
	args := slices.Concat([]string{"-v", "error", "-select_streams", "v:0", "-count_packets", "-show_entries",
		"stream=nb_read_packets", "-print_format", "json"}, in.Args, []string{in.Path})
	stdout, _, err := e.Runner.Run(ctx, Command{Name: e.FFprobe, Args: args})
	if err != nil {
		return 0, err
	}
	var out probeOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil || len(out.Streams) == 0 {
		return 0, err
	}
	return strconv.Atoi(out.Streams[0].FramesInStream)
}

// copiedStreamsOK compares the streamhash of every stream copied verbatim, in source and output.
func (e *Encoder) copiedStreamsOK(ctx context.Context, job Job) (bool, error) {
	params := job.params()
	var sourceMaps, outputMaps []string
	for i, index := range slices.Concat(params.KeepAudio, params.KeepSubs) {
		if s, ok := job.Source.Stream(index); ok && s.Codec == movText {
			continue
		}
		sourceMaps = append(sourceMaps, "-map", "0:"+strconv.Itoa(index))
		outputMaps = append(outputMaps, "-map", "0:"+strconv.Itoa(i+1))
	}
	if len(sourceMaps) == 0 {
		return true, nil
	}
	source, err := e.streamHash(ctx, job.Input, sourceMaps)
	if err != nil {
		return false, err
	}
	output, err := e.streamHash(ctx, Input{Path: job.Output}, outputMaps)
	return err == nil && source == output, err
}

func (e *Encoder) streamHash(ctx context.Context, in Input, maps []string) (string, error) {
	args := slices.Concat([]string{"-hide_banner", "-nostdin", "-v", "error"}, in.args(), maps,
		[]string{"-c", "copy", "-f", "streamhash", "-hash", "sha256", "-"})
	stdout, _, err := e.Runner.Run(ctx, Command{Name: e.FFmpeg, Args: args})
	return stdout, err
}

func (e *Encoder) hdr10Kept(ctx context.Context, output Input) (bool, error) {
	args := []string{"-v", "error", "-select_streams", "v:0", "-read_intervals", "%+#1", "-show_frames",
		"-show_entries", "frame=side_data_list", "-print_format", "json", output.Path}
	stdout, _, err := e.Runner.Run(ctx, Command{Name: e.FFprobe, Args: args})
	return strings.Contains(stdout, sideDataMastering), err
}

// sampledVMAF scores evenly spread windows of the output against the source. It returns nil when
// ffmpeg has no libvmaf or any window fails: VMAF informs, it doesn't gate.
func (e *Encoder) sampledVMAF(ctx context.Context, job Job, samples int) *float64 {
	var scores []float64
	for i := range samples {
		start := max(0, job.Source.Duration*(float64(i)+half)/float64(samples)-vmafWindow*half)
		at := []string{"-ss", strconv.FormatFloat(start, 'f', 3, 64), "-t", strconv.Itoa(vmafWindow)}
		args := slices.Concat([]string{"-hide_banner", "-nostdin", "-v", "info"}, at, Input{Path: job.Output}.args(), at,
			job.Input.args(), []string{"-lavfi", "[0:v][1:v]libvmaf", "-f", "null", "-"})
		_, stderr, err := e.Runner.Run(ctx, Command{Name: e.FFmpeg, Args: args})
		match := vmafScore.FindStringSubmatch(stderr)
		if err != nil || match == nil {
			return nil
		}
		score, err := strconv.ParseFloat(match[1], 64)
		if err != nil {
			return nil
		}
		scores = append(scores, score)
	}
	mean := 0.0
	for _, s := range scores {
		mean += s / float64(len(scores))
	}
	mean = math.Round(mean*vmafPrecision) / vmafPrecision
	return &mean
}
