// Package probe finds the agent's GPUs and runs short real test encodes and decodes on each, so the
// server schedules work only on what actually works. It never trusts `ffmpeg -encoders`: a compiled-in
// encoder says nothing about the GPU, driver or container access.
package probe

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"magpie/internal/transcode/api/agentproto"
)

const (
	DefaultFFmpeg    = "ffmpeg"
	DefaultNvidiaSMI = "nvidia-smi"
	DefaultWhisper   = "whisper-cli"
	DefaultTimeout   = time.Minute

	samplesDirPerm = 0o750
	versionField   = 2
	whisperHelp    = "--help"
	versionFlag    = "-version"
)

// A hardware decode that cannot start makes ffmpeg fall back to the CPU and still exit 0. These
// warnings are the only sign of it.
var fallbackMarkers = []string{
	"Failed setup for format",
	"hwaccel initialisation returned error",
}

type sample string

const (
	sampleHEVC10 sample = "hevc10.mkv"
	sampleH2648  sample = "h264.mkv"
)

var decodeSamples = map[agentproto.Capability]sample{
	agentproto.CapDecH2648:  sampleH2648,
	agentproto.CapDecHEVC10: sampleHEVC10,
}

// Capabilities lists every tested capability in report order.
var Capabilities = []agentproto.Capability{
	agentproto.CapEncHEVC10,
	agentproto.CapEncAV110,
	agentproto.CapDecH2648,
	agentproto.CapDecHEVC10,
	agentproto.CapPipelineHEVC10,
	agentproto.CapWhisperVulkan,
}

type Prober struct {
	Runner Runner
	// Host is the root filesystem, read for /sys/class/drm.
	Host      fs.FS
	FFmpeg    string
	NvidiaSMI string
	Whisper   string
	// SamplesDir holds the clips the decode tests read.
	SamplesDir string
	Timeout    time.Duration
	// CPU offers the software encoders as a device, for testing without a GPU.
	CPU bool
}

// New returns a prober that runs the real programs from PATH.
func New(samplesDir string) *Prober {
	return &Prober{
		Runner:     ExecRunner{},
		Host:       os.DirFS("/"),
		FFmpeg:     DefaultFFmpeg,
		NvidiaSMI:  DefaultNvidiaSMI,
		Whisper:    DefaultWhisper,
		SamplesDir: samplesDir,
		Timeout:    DefaultTimeout,
	}
}

// FFmpegVersion returns the version from `ffmpeg -version`, or "" when ffmpeg can't run.
func (p *Prober) FFmpegVersion(ctx context.Context) string {
	stdout, _, err := p.run(ctx, p.FFmpeg, versionFlag)
	if err != nil {
		return ""
	}
	first, _, _ := strings.Cut(stdout, "\n")
	fields := strings.Fields(first)
	if len(fields) <= versionField {
		return ""
	}
	return fields[versionField]
}

// Probe tests every capability on every device, or only on deviceID when it is set.
func (p *Prober) Probe(ctx context.Context, deviceID string) []agentproto.DeviceReport {
	var devices []agentproto.DeviceReport
	for _, device := range p.devices(ctx) {
		if deviceID == "" || device.DeviceID == deviceID {
			devices = append(devices, device)
		}
	}
	if len(devices) == 0 {
		return nil
	}

	samples := p.makeSamples(ctx)
	whisperErr := p.whisperError(ctx)
	for i := range devices {
		devices[i].Capabilities = p.test(ctx, devices[i], samples, whisperErr)
	}
	return devices
}

func (p *Prober) test(ctx context.Context, device agentproto.DeviceReport, samples map[sample]result, whisperErr string) []agentproto.CapabilityResult {
	if device.Vendor == agentproto.VendorCPU {
		return p.testCPU(ctx)
	}
	commands := commandsByVendor[device.Vendor]
	results := make([]agentproto.CapabilityResult, 0, len(Capabilities))
	for _, capability := range Capabilities {
		var outcome result
		switch capability {
		case agentproto.CapEncHEVC10, agentproto.CapEncAV110:
			outcome = p.check(ctx, commands.encode(device.DeviceID, commands.encoders[capability]))
		case agentproto.CapDecH2648, agentproto.CapDecHEVC10:
			outcome = p.withSample(samples, decodeSamples[capability], func(path string) result {
				return p.checkDecode(ctx, commands.decode(device.DeviceID, path))
			})
		case agentproto.CapPipelineHEVC10:
			outcome = p.withSample(samples, sampleHEVC10, func(path string) result {
				return p.checkDecode(ctx, commands.pipeline(device.DeviceID, path))
			})
		case agentproto.CapWhisperVulkan:
			outcome = result{err: whisperErr}
			if whisperErr == "" {
				outcome = p.check(ctx, vulkanArgs(device))
			}
		}
		results = append(results, agentproto.CapabilityResult{Capability: capability, OK: outcome.err == "", Error: outcome.err})
	}
	return results
}

// errCPUOnlyEncodes is the result of every test but the encodes on the CPU device: it decodes as the
// fallback path does and has no Vulkan.
const errCPUOnlyEncodes = "the CPU device only runs the software encoders"

// testCPU runs the software encoders on the same test clip the GPUs encode.
func (p *Prober) testCPU(ctx context.Context) []agentproto.CapabilityResult {
	results := make([]agentproto.CapabilityResult, 0, len(Capabilities))
	for _, capability := range Capabilities {
		outcome := result{err: errCPUOnlyEncodes}
		if name, ok := cpuEncoders[capability]; ok {
			outcome = p.check(ctx, cpuEncodeArgs(name))
		}
		results = append(results, agentproto.CapabilityResult{Capability: capability, OK: outcome.err == "", Error: outcome.err})
	}
	return results
}

// result is one test's outcome; err is empty when it passed.
type result struct {
	path string
	err  string
}

func (p *Prober) check(ctx context.Context, arguments []string) result {
	_, stderr, err := p.run(ctx, p.FFmpeg, arguments...)
	if err != nil {
		return result{err: errorText(stderr, err)}
	}
	return result{}
}

// checkDecode also fails a decode that silently fell back to the CPU.
func (p *Prober) checkDecode(ctx context.Context, arguments []string) result {
	_, stderr, err := p.run(ctx, p.FFmpeg, arguments...)
	if err != nil {
		return result{err: errorText(stderr, err)}
	}
	for line := range strings.Lines(stderr) {
		for _, marker := range fallbackMarkers {
			if strings.Contains(line, marker) {
				return result{err: strings.TrimSpace(logPrefix.ReplaceAllString(line, ""))}
			}
		}
	}
	return result{}
}

func (p *Prober) withSample(samples map[sample]result, s sample, test func(path string) result) result {
	made := samples[s]
	if made.err != "" {
		return result{err: "making the " + string(s) + " sample: " + made.err}
	}
	return test(made.path)
}

// makeSamples encodes the decode-test clips on the CPU once per probe.
func (p *Prober) makeSamples(ctx context.Context) map[sample]result {
	made := map[sample]result{}
	if err := os.MkdirAll(p.SamplesDir, samplesDirPerm); err != nil {
		for _, s := range []sample{sampleHEVC10, sampleH2648} {
			made[s] = result{err: err.Error()}
		}
		return made
	}
	for _, s := range []sample{sampleHEVC10, sampleH2648} {
		path := filepath.Join(p.SamplesDir, string(s))
		outcome := p.check(ctx, sampleArgs(s, path))
		outcome.path = path
		made[s] = outcome
	}
	return made
}

// whisperError is why whisper.cpp can't run, or "" when it can.
func (p *Prober) whisperError(ctx context.Context) string {
	_, stderr, err := p.run(ctx, p.Whisper, whisperHelp)
	if err != nil {
		return p.Whisper + ": " + errorText(stderr, err)
	}
	return ""
}

func (p *Prober) run(ctx context.Context, name string, arguments ...string) (string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel()
	stdout, stderr, err := p.Runner.Run(ctx, name, arguments...)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return stdout, stderr, fmt.Errorf("%w after %s", errTimedOut, p.Timeout)
	}
	return stdout, stderr, err
}

var errTimedOut = errors.New("timed out")

// ffmpeg prefixes log lines with the component that wrote them, e.g. "[av1_nvenc @ 0x5600c3a1b2c0] ".
var logPrefix = regexp.MustCompile(`^\[[^\]]+ @ 0x[0-9a-f]+\] `)

// errorText is the first line ffmpeg printed, without its component prefix. ffmpeg logs the root cause
// first (e.g. "OpenEncodeSessionEx failed: out of memory") and the generic cascade after it. When the
// program printed nothing or hung, the process error is used.
func errorText(stderr string, err error) string {
	if errors.Is(err, errTimedOut) {
		return err.Error()
	}
	for line := range strings.Lines(stderr) {
		if text := strings.TrimSpace(logPrefix.ReplaceAllString(line, "")); text != "" {
			return text
		}
	}
	return err.Error()
}
