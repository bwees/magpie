package probe

import (
	"context"
	"errors"
	"flag"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"magpie/internal/transcode/api/agentproto"
)

var update = flag.Bool("update", false, "rewrite testdata/commands.golden")

const (
	samplesPlaceholder = "$SAMPLES"
	goldenFile         = "testdata/commands.golden"
	intelNode          = "/dev/dri/renderD128"
	amdNode            = "/dev/dri/renderD129"
	geForce            = "cuda:0"
	workstation        = "cuda:1"
)

var errExit = errors.New("exit status 1")

// rule answers commands for program whose arguments contain every fragment.
type rule struct {
	program   string
	fragments []string
	stdout    string
	stderr    string
	fail      bool
}

type fakeRunner struct {
	mu    sync.Mutex
	rules []rule
	calls []string
}

func (r *fakeRunner) Run(_ context.Context, name string, args ...string) (string, string, error) {
	line := name + " " + strings.Join(args, " ")
	r.mu.Lock()
	r.calls = append(r.calls, line)
	r.mu.Unlock()
	for _, rule := range r.rules {
		if rule.program == name && containsAll(line, rule.fragments) {
			if rule.fail {
				return rule.stdout, rule.stderr, errExit
			}
			return rule.stdout, rule.stderr, nil
		}
	}
	return "", "", nil
}

func containsAll(line string, fragments []string) bool {
	for _, fragment := range fragments {
		if !strings.Contains(line, fragment) {
			return false
		}
	}
	return true
}

func recorded(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func drmNode(vendor, device, driver string) map[string]*fstest.MapFile {
	return map[string]*fstest.MapFile{
		"vendor": {Data: []byte(vendor + "\n")},
		"device": {Data: []byte(device + "\n")},
		"driver": {Data: []byte("../../../bus/pci/drivers/" + driver), Mode: fs.ModeSymlink},
	}
}

// hostWithGPUs has an Intel and an AMD render node, plus the NVIDIA node CUDA covers.
func hostWithGPUs() fstest.MapFS {
	host := fstest.MapFS{}
	nodes := map[string]map[string]*fstest.MapFile{
		"renderD128": drmNode(pciVendorIntel, "0x56a0", "i915"),
		"renderD129": drmNode(pciVendorAMD, "0x744c", "amdgpu"),
		"renderD130": drmNode(pciVendorNVIDIA, "0x2786", "nvidia"),
	}
	for node, files := range nodes {
		for name, file := range files {
			host["sys/class/drm/"+node+"/device/"+name] = file
		}
	}
	host["sys/class/drm/card0/device/vendor"] = &fstest.MapFile{Data: []byte(pciVendorIntel)}
	return host
}

func TestTheCPUDeviceTestsOnlyTheSoftwareEncoders(t *testing.T) {
	p, runner := newTestProber(fstest.MapFS{},
		rule{program: DefaultNvidiaSMI, fail: true, stderr: exec.ErrNotFound.Error()},
		rule{program: DefaultFFmpeg, fragments: []string{"-c:v " + encoderSVTAV1}, stderr: "Unknown encoder 'libsvtav1'\n", fail: true},
	)
	withSamplesDir(t, p)
	p.CPU = true

	reports := p.Probe(context.Background(), "")

	if len(reports) != 1 || reports[0].DeviceID != CPUDeviceID || reports[0].Vendor != agentproto.VendorCPU {
		t.Fatalf("reports = %+v, want the CPU device alone", reports)
	}
	if hevc := capability(t, reports[0], agentproto.CapEncHEVC10); !hevc.OK {
		t.Errorf("HEVC = %+v", hevc)
	}
	if av1 := capability(t, reports[0], agentproto.CapEncAV110); av1.OK || av1.Error != "Unknown encoder 'libsvtav1'" {
		t.Errorf("AV1 = %+v", av1)
	}
	if dec := capability(t, reports[0], agentproto.CapDecHEVC10); dec.OK {
		t.Errorf("decode = %+v, want not tested", dec)
	}
	if calls := strings.Join(runner.calls, "\n"); !strings.Contains(calls, "-pix_fmt yuv420p10le -c:v libx265 -x265-params log-level=error -f null -") {
		t.Errorf("calls =\n%s", calls)
	}
}

func newTestProber(host fs.FS, rules ...rule) (*Prober, *fakeRunner) {
	runner := &fakeRunner{rules: rules}
	return &Prober{
		Runner:    runner,
		Host:      host,
		FFmpeg:    DefaultFFmpeg,
		NvidiaSMI: DefaultNvidiaSMI,
		Whisper:   DefaultWhisper,
		Timeout:   time.Second,
	}, runner
}

func gpuRules(t *testing.T) []rule {
	return []rule{
		{program: DefaultNvidiaSMI, stdout: recorded(t, "nvidia-smi.txt")},
		{program: DefaultFFmpeg, fragments: []string{"-v verbose", intelNode}, stderr: recorded(t, "vaapi-intel.txt")},
		{program: DefaultFFmpeg, fragments: []string{"-v verbose", amdNode}, stderr: recorded(t, "vaapi-amd.txt")},
	}
}

func capability(t *testing.T, report agentproto.DeviceReport, c agentproto.Capability) agentproto.CapabilityResult {
	t.Helper()
	for _, result := range report.Capabilities {
		if result.Capability == c {
			return result
		}
	}
	t.Fatalf("%s has no %s result", report.DeviceID, c)
	return agentproto.CapabilityResult{}
}

func withSamplesDir(t *testing.T, p *Prober) {
	t.Helper()
	p.SamplesDir = t.TempDir()
}

func TestProbeFindsEveryGPU(t *testing.T) {
	p, _ := newTestProber(hostWithGPUs(), gpuRules(t)...)
	withSamplesDir(t, p)

	reports := p.Probe(context.Background(), "")

	type identity struct {
		id          string
		vendor      agentproto.Vendor
		name        string
		driver      string
		maxSessions int
	}
	var got []identity
	for _, r := range reports {
		got = append(got, identity{r.DeviceID, r.Vendor, r.Name, r.Driver, r.MaxSessions})
		if len(r.Capabilities) != len(Capabilities) {
			t.Errorf("%s has %d results, want %d", r.DeviceID, len(r.Capabilities), len(Capabilities))
		}
	}
	want := []identity{
		{geForce, agentproto.VendorNVIDIA, "NVIDIA GeForce RTX 4070", "560.35.03", geForceSessionLimit},
		{workstation, agentproto.VendorNVIDIA, "NVIDIA RTX A4000", "560.35.03", 0},
		{intelNode, agentproto.VendorIntel, "Intel(R) Gen Graphics", "Intel iHD driver 24.4.4", 0},
		{amdNode, agentproto.VendorAMD, "AMD Radeon RX 7900 XTX", "Mesa Gallium driver 26.0.0", 0},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("devices =\n%+v\nwant\n%+v", got, want)
	}
}

func TestProbeReportsEachFailureWithItsError(t *testing.T) {
	rules := append([]rule{
		{program: DefaultFFmpeg, fragments: []string{"-c:v " + encoderAV1NVENC, "-gpu 0"}, stderr: recorded(t, "nvenc-av1-unsupported.txt"), fail: true},
		{program: DefaultFFmpeg, fragments: []string{"-hwaccel qsv", string(sampleHEVC10)}, stderr: recorded(t, "qsv-hevc-decode-fallback.txt")},
	}, gpuRules(t)...)
	p, _ := newTestProber(hostWithGPUs(), rules...)
	withSamplesDir(t, p)

	reports := p.Probe(context.Background(), "")

	nvidia, intel := reports[0], reports[2]
	if av1 := capability(t, nvidia, agentproto.CapEncAV110); av1.OK || av1.Error != "OpenEncodeSessionEx failed: unsupported device (2): (no details)" {
		t.Errorf("GeForce AV1 = %+v", av1)
	}
	if hevc := capability(t, nvidia, agentproto.CapEncHEVC10); !hevc.OK || hevc.Error != "" {
		t.Errorf("GeForce HEVC = %+v", hevc)
	}
	if dec := capability(t, intel, agentproto.CapDecHEVC10); dec.OK || !strings.Contains(dec.Error, "Failed setup for format qsv") {
		t.Errorf("Intel HEVC decode = %+v, want the CPU fallback reported as a failure", dec)
	}
	if pipeline := capability(t, intel, agentproto.CapPipelineHEVC10); pipeline.OK {
		t.Errorf("Intel pipeline = %+v, want failed: it decodes HEVC on the GPU first", pipeline)
	}
	if dec := capability(t, intel, agentproto.CapDecH2648); !dec.OK {
		t.Errorf("Intel H.264 decode = %+v", dec)
	}
}

func TestProbeOfOneDeviceTestsOnlyThatDevice(t *testing.T) {
	p, runner := newTestProber(hostWithGPUs(), gpuRules(t)...)
	withSamplesDir(t, p)

	reports := p.Probe(context.Background(), amdNode)

	if len(reports) != 1 || reports[0].DeviceID != amdNode {
		t.Fatalf("reports = %+v, want only %s", reports, amdNode)
	}
	for _, call := range runner.calls {
		isTest := strings.Contains(call, "-v warning -init_hw_device") || strings.Contains(call, "-gpu ")
		if isTest && !strings.Contains(call, amdNode) {
			t.Errorf("tested another device: %s", call)
		}
	}
}

func TestMissingSampleEncoderFailsOnlyTheTestsThatNeedIt(t *testing.T) {
	rules := append([]rule{
		{program: DefaultFFmpeg, fragments: []string{"-c:v " + encoderLibx265}, stderr: "Unknown encoder 'libx265'\n", fail: true},
	}, gpuRules(t)...)
	p, _ := newTestProber(hostWithGPUs(), rules...)
	withSamplesDir(t, p)

	amd := p.Probe(context.Background(), amdNode)[0]

	for _, c := range []agentproto.Capability{agentproto.CapDecHEVC10, agentproto.CapPipelineHEVC10} {
		if result := capability(t, amd, c); result.OK || result.Error != "making the hevc10.mkv sample: Unknown encoder 'libx265'" {
			t.Errorf("%s = %+v", c, result)
		}
	}
	if dec := capability(t, amd, agentproto.CapDecH2648); !dec.OK {
		t.Errorf("H.264 decode = %+v", dec)
	}
}

func TestMissingWhisperFailsWhisperOnly(t *testing.T) {
	rules := append([]rule{{program: DefaultWhisper, fail: true}}, gpuRules(t)...)
	p, _ := newTestProber(hostWithGPUs(), rules...)
	withSamplesDir(t, p)

	intel := p.Probe(context.Background(), intelNode)[0]

	if whisper := capability(t, intel, agentproto.CapWhisperVulkan); whisper.OK || whisper.Error != DefaultWhisper+": "+errExit.Error() {
		t.Errorf("whisper = %+v", whisper)
	}
	if enc := capability(t, intel, agentproto.CapEncHEVC10); !enc.OK {
		t.Errorf("HEVC encode = %+v", enc)
	}
}

func TestNoGPUsRunsNoTests(t *testing.T) {
	p, runner := newTestProber(fstest.MapFS{}, rule{program: DefaultNvidiaSMI, fail: true, stderr: exec.ErrNotFound.Error()})

	if reports := p.Probe(context.Background(), ""); reports != nil {
		t.Fatalf("reports = %+v, want none", reports)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %v, want only the nvidia-smi query", runner.calls)
	}
}

// The golden file is the reviewed list of every command a probe runs; regenerate with -update.
func TestProbeCommands(t *testing.T) {
	p, runner := newTestProber(hostWithGPUs(), gpuRules(t)...)
	withSamplesDir(t, p)

	p.Probe(context.Background(), "")

	got := strings.ReplaceAll(strings.Join(runner.calls, "\n")+"\n", p.SamplesDir, samplesPlaceholder)
	if *update {
		if err := os.WriteFile(goldenFile, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if want := recorded(t, filepath.Base(goldenFile)); got != want {
		t.Fatalf("commands changed; rerun with -update and review the diff:\n%s", got)
	}
}

func TestFFmpegVersion(t *testing.T) {
	p, _ := newTestProber(fstest.MapFS{}, rule{program: DefaultFFmpeg, fragments: []string{versionFlag}, stdout: recorded(t, "ffmpeg-version.txt")})

	if got := p.FFmpegVersion(context.Background()); got != "7.1.2-Jellyfin" {
		t.Fatalf("version = %q", got)
	}
}

func TestParseVAAPIDriver(t *testing.T) {
	tests := []struct{ description, name, driver string }{
		{"Intel iHD driver for Intel(R) Gen Graphics - 24.4.4 ().", "Intel(R) Gen Graphics", "Intel iHD driver 24.4.4"},
		{"Mesa Gallium driver 26.0.0 for AMD Radeon RX 7900 XTX (radeonsi, navi31, LLVM 19.1.7, DRM 3.61, 6.12.0-1-amd64).", "AMD Radeon RX 7900 XTX", "Mesa Gallium driver 26.0.0"},
		{"Splitted-Desktop Systems VDPAU backend", "", "Splitted-Desktop Systems VDPAU backend"},
	}
	for _, tt := range tests {
		if name, driver := parseVAAPIDriver(tt.description); name != tt.name || driver != tt.driver {
			t.Errorf("parseVAAPIDriver(%q) = %q, %q; want %q, %q", tt.description, name, driver, tt.name, tt.driver)
		}
	}
}
