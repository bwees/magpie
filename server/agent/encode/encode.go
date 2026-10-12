// Package encode runs a transcode job's ffmpeg work on the agent: the encode on the device's hardware
// pipeline, with a CPU-decode fallback, and the checks of the output before it is uploaded.
// Spec: docs/spec/contexts/transcode.md#agent-side-job-run.
package encode

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"magpie/internal/transcode/api/agentproto"
)

const (
	DefaultFFmpeg  = "ffmpeg"
	DefaultFFprobe = "ffprobe"

	cudaPrefix = "cuda:"
	// qsvUploadFrames is the surface pool QSV encoders need from hwupload.
	qsvUploadFrames = "64"
	// subtitlesForMatroska is what mp4 text subtitles become, since Matroska can't hold mov_text.
	subtitlesForMatroska = "srt"
	movText              = "mov_text"
	hevcPrefix           = "hevc_"
	hevcMain10           = "main10"
	matroska             = "matroska"
	softwareTenBit       = "yuv420p10le"
	x265Quiet            = "log-level=error"
	kbps                 = "k"
)

// Encoder builds and runs ffmpeg commands.
type Encoder struct {
	Runner  Runner
	FFmpeg  string
	FFprobe string
}

func New() *Encoder {
	return &Encoder{Runner: ExecRunner{}, FFmpeg: DefaultFFmpeg, FFprobe: DefaultFFprobe}
}

// Job is one encode: the server's spec, the device it runs on, and the probed source.
type Job struct {
	Spec   agentproto.JobSpec
	Device agentproto.DeviceReport
	Input  Input
	Source Media
	Output string
	// Progress receives the share of the source encoded and the speed as a multiple of realtime.
	Progress func(fraction, speed float64)
}

func (j Job) params() agentproto.TranscodeParams {
	if j.Spec.Transcode == nil {
		return agentproto.TranscodeParams{}
	}
	return *j.Spec.Transcode
}

// pipeline is how one vendor's devices decode, convert to 10-bit and encode.
type pipeline struct {
	// device opens the device for hardware decode and filters.
	device func(id string) []string
	// decode keeps decoded frames on the device.
	decode []string
	// gpuFilter converts frames already on the device; cpuFilter uploads frames the CPU decoded.
	gpuFilter, cpuFilter string
	// encoder adds the options that pick the device for the encoder.
	encoder func(id string) []string
}

var pipelines = map[agentproto.Vendor]pipeline{
	agentproto.VendorNVIDIA: {
		device: func(id string) []string {
			return []string{"-init_hw_device", "cuda=gpu:" + strings.TrimPrefix(id, cudaPrefix), "-filter_hw_device", "gpu"}
		},
		decode:    []string{"-hwaccel", "cuda", "-hwaccel_output_format", "cuda"},
		gpuFilter: "scale_cuda=format=p010le",
		cpuFilter: "format=p010le",
		encoder:   func(id string) []string { return []string{"-gpu", strings.TrimPrefix(id, cudaPrefix)} },
	},
	agentproto.VendorIntel: {
		device: func(id string) []string {
			return []string{"-init_hw_device", "vaapi=va:" + id, "-init_hw_device", "qsv=qs@va", "-filter_hw_device", "qs"}
		},
		decode:    []string{"-hwaccel", "qsv", "-hwaccel_output_format", "qsv"},
		gpuFilter: "vpp_qsv=format=p010",
		cpuFilter: "format=p010le,hwupload=extra_hw_frames=" + qsvUploadFrames,
		encoder:   func(string) []string { return nil },
	},
	agentproto.VendorAMD: {
		device: func(id string) []string {
			return []string{"-init_hw_device", "vaapi=va:" + id, "-filter_hw_device", "va"}
		},
		decode:    []string{"-hwaccel", "vaapi", "-hwaccel_output_format", "vaapi"},
		gpuFilter: "scale_vaapi=format=p010",
		cpuFilter: "format=p010le,hwupload",
		encoder:   func(string) []string { return nil },
	},
}

// decodeCapability is the capability a device must have passed to decode the source video itself.
func decodeCapability(video Stream) (agentproto.Capability, bool) {
	switch {
	case video.Codec == "h264" && video.BitDepth() == eightBit:
		return agentproto.CapDecH2648, true
	case video.Codec == "hevc" && video.BitDepth() <= tenBit:
		return agentproto.CapDecHEVC10, true
	}
	return "", false
}

func passed(device agentproto.DeviceReport, c agentproto.Capability) bool {
	return slices.ContainsFunc(device.Capabilities, func(r agentproto.CapabilityResult) bool { return r.Capability == c && r.OK })
}

// HardwareDecode reports whether the job can run without leaving the GPU: the device decodes the source
// and passed the full pipeline test. H.264 10-bit, for one, never can.
func HardwareDecode(job Job) bool {
	if _, ok := pipelines[job.Device.Vendor]; !ok {
		return false
	}
	video, ok := job.Source.Video()
	if !ok {
		return false
	}
	needs, ok := decodeCapability(video)
	return ok && passed(job.Device, needs) && passed(job.Device, agentproto.CapPipelineHEVC10)
}

// Args builds the ffmpeg command line of the encode. hardwareDecode keeps frames on the GPU; otherwise
// the CPU decodes and the frames are uploaded to the encoder.
func Args(job Job, hardwareDecode bool) []string {
	params := job.params()
	args := []string{"-hide_banner", "-nostdin", "-y", "-v", "error", "-progress", "pipe:1", "-nostats"}
	line, hardware := pipelines[job.Device.Vendor]
	if hardware {
		args = append(args, line.device(job.Device.DeviceID)...)
		if hardwareDecode {
			args = append(args, line.decode...)
		}
	}
	args = append(args, job.Input.args()...)

	video, _ := job.Source.Video()
	args = append(args, "-map", "0:"+strconv.Itoa(video.Index))
	for _, index := range slices.Concat(params.KeepAudio, params.KeepSubs) {
		args = append(args, "-map", "0:"+strconv.Itoa(index))
	}
	args = append(args, "-map", "0:t?", "-map_metadata", "0", "-map_chapters", "0", "-c", "copy")
	for i, index := range params.KeepSubs {
		if s, ok := job.Source.Stream(index); ok && s.Codec == movText {
			args = append(args, "-c:s:"+strconv.Itoa(i), subtitlesForMatroska)
		}
	}

	args = append(args, "-c:v:0", params.Encoder)
	switch {
	case hardware && hardwareDecode:
		args = append(args, "-vf", line.gpuFilter)
	case hardware:
		args = append(args, "-vf", line.cpuFilter)
	default:
		args = append(args, "-pix_fmt", softwareTenBit)
	}
	if hardware {
		args = append(args, line.encoder(job.Device.DeviceID)...)
		if strings.HasPrefix(params.Encoder, hevcPrefix) {
			args = append(args, "-profile:v", hevcMain10)
		}
	}
	if params.Encoder == "libx265" {
		args = append(args, "-x265-params", x265Quiet)
	}
	for _, key := range slices.Sorted(maps.Keys(params.EncoderArgs)) {
		args = append(args, "-"+key, params.EncoderArgs[key])
	}
	if params.MaxRateKbps > 0 {
		args = append(args, "-maxrate", strconv.Itoa(params.MaxRateKbps)+kbps, "-bufsize", strconv.Itoa(params.BufSizeKbps)+kbps)
	}
	return append(args, "-f", matroska, job.Output)
}

// Transcode encodes the job. A hardware pipeline that fails is tried once more with the CPU decoding,
// since a decoder the capability tests passed can still choke on one file.
func (e *Encoder) Transcode(ctx context.Context, job Job) error {
	hardware := HardwareDecode(job)
	err := e.encode(ctx, job, hardware)
	if err == nil || !hardware || ctx.Err() != nil {
		return err
	}
	if fallback := e.encode(ctx, job, false); fallback != nil {
		return errors.Join(err, fallback)
	}
	return nil
}

func (e *Encoder) encode(ctx context.Context, job Job, hardwareDecode bool) error {
	progress := progressParser{duration: job.Source.Duration, report: job.Progress}
	_, stderr, err := e.Runner.Run(ctx, Command{Name: e.FFmpeg, Args: Args(job, hardwareDecode), OnLine: progress.line})
	if err != nil {
		return fmt.Errorf("ffmpeg: %w: %s", err, firstLine(stderr))
	}
	return nil
}

// progressParser reads ffmpeg's -progress key=value lines.
type progressParser struct {
	duration float64
	speed    float64
	report   func(fraction, speed float64)
}

const (
	progressOutTime = "out_time_us"
	progressSpeed   = "speed"
	speedSuffix     = "x"
	microsPerSecond = 1e6
)

func (p *progressParser) line(line string) {
	key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
	if !ok || p.report == nil {
		return
	}
	switch key {
	case progressSpeed:
		if speed, err := strconv.ParseFloat(strings.TrimSuffix(value, speedSuffix), 64); err == nil {
			p.speed = speed
		}
	case progressOutTime:
		micros, err := strconv.ParseFloat(value, 64)
		if err != nil || p.duration <= 0 {
			return
		}
		p.report(min(1, max(0, micros/microsPerSecond/p.duration)), p.speed)
	}
}
