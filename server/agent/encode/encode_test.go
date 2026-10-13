package encode

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"magpie/internal/transcode/api/agentproto"
)

var errExit = errors.New("exit status 1")

// rule answers the commands of a program whose line contains every fragment.
type rule struct {
	program   string
	fragments []string
	stdout    string
	stderr    string
	lines     []string
	fail      bool
}

// fakeRunner answers from rules, first match wins, and records every command.
type fakeRunner struct {
	mu    sync.Mutex
	rules []rule
	calls []string
}

func (r *fakeRunner) Run(_ context.Context, c Command) (string, string, error) {
	line := c.String()
	r.mu.Lock()
	r.calls = append(r.calls, line)
	r.mu.Unlock()
	for _, rule := range r.rules {
		if rule.program != c.Name || !containsAll(line, rule.fragments) {
			continue
		}
		for _, l := range rule.lines {
			if c.OnLine != nil {
				c.OnLine(l)
			}
		}
		if rule.fail {
			return rule.stdout, rule.stderr, errExit
		}
		return rule.stdout, rule.stderr, nil
	}
	return "", "", nil
}

func (r *fakeRunner) called(fragments ...string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var matched []string
	for _, call := range r.calls {
		if containsAll(call, fragments) {
			matched = append(matched, call)
		}
	}
	return matched
}

func containsAll(line string, fragments []string) bool {
	for _, f := range fragments {
		if !strings.Contains(line, f) {
			return false
		}
	}
	return true
}

func device(vendor agentproto.Vendor, id string, passed ...agentproto.Capability) agentproto.DeviceReport {
	d := agentproto.DeviceReport{DeviceID: id, Vendor: vendor}
	for _, c := range passed {
		d.Capabilities = append(d.Capabilities, agentproto.CapabilityResult{Capability: c, OK: true})
	}
	return d
}

func source(videoCodec, pixFmt string) Media {
	return Media{Duration: 3600, Streams: []Stream{
		{Index: 0, Type: TypeVideo, Codec: videoCodec, PixFmt: pixFmt, FrameRate: "24000/1001", AvgFrameRate: "24000/1001"},
		{Index: 1, Type: TypeAudio, Codec: "eac3"},
		{Index: 2, Type: TypeAudio, Codec: "aac"},
		{Index: 3, Type: TypeSubtitle, Codec: "subrip"},
	}}
}

func job(d agentproto.DeviceReport, encoder string, src Media) Job {
	return Job{
		Spec: agentproto.JobSpec{Job: "01JOB", DeviceID: d.DeviceID, Transcode: &agentproto.TranscodeParams{
			Codec: "hevc", BitDepth: 10, Encoder: encoder, EncoderArgs: map[string]string{"cq": "24", "preset": "p6"},
			MaxRateKbps: 10000, BufSizeKbps: 20000, KeepAudio: []int{1, 2}, KeepSubs: []int{3},
		}},
		Device: d,
		Input:  Input{Path: "/scratch/jobs/01JOB/source.mkv"},
		Source: src,
		Output: "/scratch/jobs/01JOB/output.mkv",
	}
}

var nvidiaFull = device(agentproto.VendorNVIDIA, "cuda:1", agentproto.CapEncHEVC10, agentproto.CapDecH2648, agentproto.CapDecHEVC10, agentproto.CapPipelineHEVC10)

func TestArgsRunTheWholeEncodeOnTheGPU(t *testing.T) {
	j := job(nvidiaFull, "hevc_nvenc", source("h264", "yuv420p"))

	got := strings.Join(Args(j, HardwareDecode(j)), " ")

	for _, want := range []string{
		"-init_hw_device cuda=gpu:1", "-hwaccel cuda -hwaccel_output_format cuda -i /scratch/jobs/01JOB/source.mkv",
		"-map 0:0 -map 0:1 -map 0:2 -map 0:3 -map 0:t?", "-c copy -c:v:0 hevc_nvenc -vf scale_cuda=format=p010le",
		"-gpu 1 -profile:v main10 -cq 24 -preset p6", "-maxrate 10000k -bufsize 20000k", "-f matroska /scratch/jobs/01JOB/output.mkv",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("args lack %q:\n%s", want, got)
		}
	}
}

func TestArgsDecodeH264TenBitOnTheCPU(t *testing.T) {
	for _, tt := range []struct {
		vendor  agentproto.Vendor
		id      string
		encoder string
		filter  string
	}{
		{agentproto.VendorNVIDIA, "cuda:0", "hevc_nvenc", "-vf format=p010le"},
		{agentproto.VendorIntel, "/dev/dri/renderD128", "hevc_qsv", "-vf format=p010le,hwupload=extra_hw_frames=64"},
		{agentproto.VendorAMD, "/dev/dri/renderD129", "hevc_vaapi", "-vf format=p010le,hwupload"},
	} {
		t.Run(string(tt.vendor), func(t *testing.T) {
			d := device(tt.vendor, tt.id, agentproto.CapEncHEVC10, agentproto.CapDecH2648, agentproto.CapPipelineHEVC10)
			j := job(d, tt.encoder, source("h264", "yuv420p10le"))

			if HardwareDecode(j) {
				t.Fatal("no GPU decodes H.264 10-bit")
			}
			got := strings.Join(Args(j, false), " ")
			if strings.Contains(got, "-hwaccel ") || !strings.Contains(got, tt.filter) || !strings.Contains(got, "-c:v:0 "+tt.encoder) {
				t.Fatalf("args =\n%s\nwant a CPU decode uploaded with %q", got, tt.filter)
			}
		})
	}
}

func TestArgsForTheCPUEncoder(t *testing.T) {
	j := job(device(agentproto.VendorCPU, "cpu", agentproto.CapEncHEVC10), "libx265", source("h264", "yuv420p"))

	got := strings.Join(Args(j, HardwareDecode(j)), " ")

	if strings.Contains(got, "-init_hw_device") || !strings.Contains(got, "-c:v:0 libx265 -pix_fmt yuv420p10le -x265-params log-level=error") {
		t.Fatalf("args =\n%s", got)
	}
}

func TestArgsConvertMP4TextSubtitles(t *testing.T) {
	src := source("h264", "yuv420p")
	src.Streams[3].Codec = movText

	got := strings.Join(Args(job(nvidiaFull, "hevc_nvenc", src), true), " ")

	if !strings.Contains(got, "-c:s:0 srt") {
		t.Fatalf("args =\n%s\nwant mov_text converted for Matroska", got)
	}
}

func TestAFailedHardwarePipelineRetriesWithTheCPUDecoding(t *testing.T) {
	runner := &fakeRunner{rules: []rule{
		{program: DefaultFFmpeg, fragments: []string{"-hwaccel cuda"}, stderr: "[h264 @ 0x1] No decoder surfaces left\n", fail: true},
		{program: DefaultFFmpeg, fragments: []string{"-vf format=p010le"}, lines: []string{"speed=2.5x", "out_time_us=1800000000"}},
	}}
	e := &Encoder{Runner: runner, FFmpeg: DefaultFFmpeg, FFprobe: DefaultFFprobe}
	j := job(nvidiaFull, "hevc_nvenc", source("h264", "yuv420p"))
	var fraction, speed float64
	j.Progress = func(f, s float64) { fraction, speed = f, s }

	if err := e.Transcode(context.Background(), j); err != nil {
		t.Fatal(err)
	}

	if n := len(runner.called("-hwaccel cuda")); n != 1 {
		t.Fatalf("hardware attempts = %d, want 1", n)
	}
	if fraction != 0.5 || speed != 2.5 {
		t.Fatalf("progress = %v at %vx, want 0.5 at 2.5x", fraction, speed)
	}
}

const (
	outputProbe = `{"format":{"duration":"3600.4"},"streams":[
		{"index":0,"codec_type":"video","codec_name":"hevc","pix_fmt":"p010le"},
		{"index":1,"codec_type":"audio","codec_name":"eac3"},
		{"index":2,"codec_type":"audio","codec_name":"aac"},
		{"index":3,"codec_type":"subtitle","codec_name":"subrip"}]}`
	packets       = `{"streams":[{"nb_read_packets":"86314"}]}`
	sourceHashes  = "0,a,SHA256=aa\n1,a,SHA256=bb\n2,s,SHA256=cc\n"
	changedHashes = "0,a,SHA256=aa\n1,a,SHA256=b0\n2,s,SHA256=cc\n"
)

func verifyRunner(outputHashes string) *fakeRunner {
	return &fakeRunner{rules: []rule{
		{program: DefaultFFprobe, fragments: []string{"-count_packets"}, stdout: packets},
		{program: DefaultFFprobe, fragments: []string{"output.mkv"}, stdout: outputProbe},
		{program: DefaultFFmpeg, fragments: []string{"source.mkv", "streamhash"}, stdout: sourceHashes},
		{program: DefaultFFmpeg, fragments: []string{"output.mkv", "streamhash"}, stdout: outputHashes},
		{program: DefaultFFmpeg, fragments: []string{"libvmaf"}, stderr: "[Parsed_libvmaf_0 @ 0x1] VMAF score: 95.5\n"},
	}}
}

func TestVerifyReportsAGoodOutput(t *testing.T) {
	runner := verifyRunner(sourceHashes)
	e := &Encoder{Runner: runner, FFmpeg: DefaultFFmpeg, FFprobe: DefaultFFprobe}
	j := job(nvidiaFull, "hevc_nvenc", source("h264", "yuv420p"))
	j.Spec.Transcode.VMAFSamples = 2

	report, err := e.Verify(context.Background(), j)
	if err != nil {
		t.Fatal(err)
	}

	if !report.DecodeOK || !report.StreamMapOK || !report.CopiedStreamsOK || report.FrameDelta != 0 || report.HDR10Kept != nil {
		t.Fatalf("report = %+v", report)
	}
	if report.DurationDelta < 0.39 || report.DurationDelta > 0.41 || report.VMAFMean == nil || *report.VMAFMean != 95.5 {
		t.Fatalf("report = %+v, want 0.4 s longer and VMAF 95.5", report)
	}
	if calls := runner.called("streamhash", "source.mkv", "-map 0:1 -map 0:2 -map 0:3"); len(calls) != 1 {
		t.Fatalf("source streamhash calls = %v", runner.calls)
	}
}

// Seed 18, the agent's part: a copied stream that differs from the source fails the check.
func TestVerifyCatchesAChangedCopiedStream(t *testing.T) {
	e := &Encoder{Runner: verifyRunner(changedHashes), FFmpeg: DefaultFFmpeg, FFprobe: DefaultFFprobe}

	report, err := e.Verify(context.Background(), job(nvidiaFull, "hevc_nvenc", source("h264", "yuv420p")))

	if err != nil || report.CopiedStreamsOK {
		t.Fatalf("report = %+v, %v; want CopiedStreamsOK false", report, err)
	}
}

func TestVerifyChecksHDR10Metadata(t *testing.T) {
	runner := verifyRunner(sourceHashes)
	runner.rules = append([]rule{{program: DefaultFFprobe, fragments: []string{"side_data_list"}, stdout: `{"frames":[{"side_data_list":[]}]}`}}, runner.rules...)
	e := &Encoder{Runner: runner, FFmpeg: DefaultFFmpeg, FFprobe: DefaultFFprobe}
	j := job(nvidiaFull, "hevc_nvenc", source("hevc", "yuv420p10le"))
	j.Spec.Transcode.HDR10 = true

	report, err := e.Verify(context.Background(), j)

	if err != nil || report.HDR10Kept == nil || *report.HDR10Kept {
		t.Fatalf("report = %+v, %v; want HDR10Kept false", report, err)
	}
}

func TestCheckArgsDecodeTheOutputOnTheGPU(t *testing.T) {
	for _, tt := range []struct {
		vendor  agentproto.Vendor
		id      string
		encoder string
		want    string
	}{
		{agentproto.VendorNVIDIA, "cuda:1", "hevc_nvenc", "-init_hw_device cuda=gpu:1 -filter_hw_device gpu -hwaccel cuda -hwaccel_output_format cuda"},
		{agentproto.VendorIntel, "/dev/dri/renderD128", "hevc_qsv", "-init_hw_device vaapi=va:/dev/dri/renderD128 -init_hw_device qsv=qs@va -filter_hw_device qs -hwaccel qsv -hwaccel_output_format qsv"},
		{agentproto.VendorAMD, "/dev/dri/renderD129", "hevc_vaapi", "-init_hw_device vaapi=va:/dev/dri/renderD129 -filter_hw_device va -hwaccel vaapi -hwaccel_output_format vaapi"},
	} {
		t.Run(string(tt.vendor), func(t *testing.T) {
			j := job(device(tt.vendor, tt.id, agentproto.CapEncHEVC10, agentproto.CapDecHEVC10), tt.encoder, source("h264", "yuv420p10le"))

			if !HardwareCheck(j) {
				t.Fatal("a device that decodes HEVC 10-bit checks the HEVC 10-bit output itself, whatever the source")
			}
			got := strings.Join(CheckArgs(j, true), " ")
			want := "-hide_banner -nostdin -v error -xerror -progress pipe:1 -nostats " + tt.want +
				" -i /scratch/jobs/01JOB/output.mkv -map 0:v -map 0:a? -f null -"
			if got != want {
				t.Fatalf("args =\n%s\nwant\n%s", got, want)
			}
		})
	}
}

func TestCheckArgsDecodeOnTheCPUWithoutTheDecodeTest(t *testing.T) {
	av1 := job(device(agentproto.VendorNVIDIA, "cuda:0", agentproto.CapEncAV110, agentproto.CapDecHEVC10), "av1_nvenc", source("hevc", "yuv420p10le"))
	av1.Spec.Transcode.Codec = "av1"
	for name, j := range map[string]Job{
		"decode test failed": job(device(agentproto.VendorAMD, "/dev/dri/renderD128", agentproto.CapEncHEVC10), "hevc_vaapi", source("hevc", "yuv420p10le")),
		"no AV1 decode test": av1,
		"CPU encoder":        job(device(agentproto.VendorCPU, "cpu", agentproto.CapEncHEVC10, agentproto.CapDecHEVC10), "libx265", source("h264", "yuv420p")),
	} {
		t.Run(name, func(t *testing.T) {
			if HardwareCheck(j) {
				t.Fatal("checked on the GPU")
			}
			got := strings.Join(CheckArgs(j, false), " ")
			want := "-hide_banner -nostdin -v error -xerror -progress pipe:1 -nostats -i /scratch/jobs/01JOB/output.mkv -map 0:v -map 0:a? -f null -"
			if got != want {
				t.Fatalf("args =\n%s\nwant\n%s", got, want)
			}
		})
	}
}

// checkRunner answers the output's checks, with the hardware decode check as given.
func checkRunner(hardware rule) *fakeRunner {
	runner := verifyRunner(sourceHashes)
	hardware.program, hardware.fragments = DefaultFFmpeg, []string{"-xerror", "-hwaccel"}
	runner.rules = append([]rule{hardware}, runner.rules...)
	return runner
}

func TestTheCheckReportsItsProgress(t *testing.T) {
	e := &Encoder{Runner: checkRunner(rule{lines: []string{"frame=43200", "speed=11.8x", "out_time_us=1800000000"}}), FFmpeg: DefaultFFmpeg, FFprobe: DefaultFFprobe}
	j := job(nvidiaFull, "hevc_nvenc", source("h264", "yuv420p"))
	var fraction, speed float64
	j.Progress = func(f, s float64) { fraction, speed = f, s }

	report, err := e.Verify(context.Background(), j)

	if err != nil || !report.DecodeOK {
		t.Fatalf("report = %+v, %v", report, err)
	}
	if fraction != 0.5 || speed != 11.8 {
		t.Fatalf("progress = %v at %vx, want 0.5 at 11.8x", fraction, speed)
	}
}

func TestAHardwareCheckThatCannotStartRunsOnTheCPU(t *testing.T) {
	runner := checkRunner(rule{
		lines:  []string{"frame=0", "out_time_us=N/A", "progress=end"},
		stderr: "[AVHWDeviceContext @ 0x1] Failed to initialise VAAPI connection: -1 (unknown libva error).\n", fail: true,
	})
	e := &Encoder{Runner: runner, FFmpeg: DefaultFFmpeg, FFprobe: DefaultFFprobe}

	report, err := e.Verify(context.Background(), job(nvidiaFull, "hevc_nvenc", source("h264", "yuv420p")))

	if err != nil || !report.DecodeOK {
		t.Fatalf("report = %+v, %v; want the CPU's clean decode", report, err)
	}
	if calls := runner.called("-xerror"); len(calls) != 2 || strings.Contains(calls[1], "-hwaccel") {
		t.Fatalf("decode checks = %v, want the GPU then the CPU", calls)
	}
}

func TestAHardwareCheckThatHitsAnErrorFailsTheOutput(t *testing.T) {
	runner := checkRunner(rule{
		lines:  []string{"frame=51234", "out_time_us=2137000000", "progress=end"},
		stderr: "[hevc @ 0x1] Error constructing the frame RPS.\n", fail: true,
	})
	e := &Encoder{Runner: runner, FFmpeg: DefaultFFmpeg, FFprobe: DefaultFFprobe}

	report, err := e.Verify(context.Background(), job(nvidiaFull, "hevc_nvenc", source("h264", "yuv420p")))

	if err != nil || report.DecodeOK {
		t.Fatalf("report = %+v, %v; want DecodeOK false", report, err)
	}
	if calls := runner.called("-xerror"); len(calls) != 1 {
		t.Fatalf("decode checks = %v, want no CPU run after a decode error", calls)
	}
}

func TestVerifyFailsAWrongStreamMap(t *testing.T) {
	j := job(nvidiaFull, "hevc_nvenc", source("h264", "yuv420p"))
	j.Spec.Transcode.KeepAudio = []int{1}

	report, err := (&Encoder{Runner: verifyRunner(sourceHashes), FFmpeg: DefaultFFmpeg, FFprobe: DefaultFFprobe}).Verify(context.Background(), j)

	if err != nil || report.StreamMapOK {
		t.Fatalf("report = %+v, %v; want StreamMapOK false: the output kept an audio stream the spec dropped", report, err)
	}
}
