package probe

import (
	"strings"

	"magpie/internal/transcode/api/agentproto"
)

// ffmpeg arguments for the test encodes. Every test reads a short synthetic clip and writes to the null
// muxer, so it exercises the GPU without touching disk. Spec: .scratch/v1-spec/research/05-gpu-encoder-matrix.md §3, §7.
const (
	testSource     = "testsrc2=size=1280x720:rate=30"
	testFrames     = "30"
	vulkanSource   = "nullsrc=size=64x64"
	vulkanFrames   = "1"
	scaledWidth    = "640"
	scaledHeight   = "360"
	tenBitSoftware = "p010le"
	tenBitHardware = "p010"
	hevcProfile    = "main10"

	sampleHEVC10Pixels = "yuv420p10le"
	sampleH264Pixels   = "yuv420p"
	x265Quiet          = "log-level=error"

	// QSV encoders keep a pool of surfaces, so hwupload must allocate extra frames for them.
	qsvUploadFrames = "64"

	// ffmpeg hardware device types, and the names the tests give the devices they open.
	hwCUDA      = "cuda"
	hwVAAPI     = "vaapi"
	hwQSV       = "qsv"
	hwDRM       = "drm"
	hwVulkan    = "vulkan"
	cudaDevice  = "gpu"
	vaapiDevice = "va"
	qsvDevice   = "qs"
	drmDevice   = "dr"
	vulkanName  = "vk"

	hevcEncoderPrefix = "hevc_"
)

const (
	encoderHEVCNVENC = "hevc_nvenc"
	encoderAV1NVENC  = "av1_nvenc"
	encoderHEVCQSV   = "hevc_qsv"
	encoderAV1QSV    = "av1_qsv"
	encoderHEVCVAAPI = "hevc_vaapi"
	encoderAV1VAAPI  = "av1_vaapi"
	encoderLibx265   = "libx265"
	encoderLibx264   = "libx264"
)

func baseArgs() []string {
	return []string{"-hide_banner", "-nostdin", "-v", "warning"}
}

func nullOutput() []string {
	return []string{"-f", "null", "-"}
}

func testInput() []string {
	return []string{"-f", "lavfi", "-i", testSource, "-frames:v", testFrames}
}

func args(parts ...[]string) []string {
	var all []string
	for _, part := range parts {
		all = append(all, part...)
	}
	return all
}

// encoder picks the hardware encoder for a capability; HEVC asks for the Main 10 profile.
func encoder(name string) []string {
	if strings.HasPrefix(name, hevcEncoderPrefix) {
		return []string{"-c:v", name, "-profile:v", hevcProfile}
	}
	return []string{"-c:v", name}
}

// vendorCommands builds each capability test for one hardware path.
type vendorCommands struct {
	encoders map[agentproto.Capability]string
	// encode feeds software 10-bit frames to the hardware encoder.
	encode func(device, encoder string) []string
	// decode decodes a sample on the GPU, keeping frames on the device.
	decode func(device, sample string) []string
	// pipeline decodes, scales and encodes HEVC 10-bit without leaving the GPU.
	pipeline func(device, sample string) []string
}

var commandsByVendor = map[agentproto.Vendor]vendorCommands{
	agentproto.VendorNVIDIA: nvidiaCommands,
	agentproto.VendorIntel:  intelCommands,
	agentproto.VendorAMD:    amdCommands,
}

func cudaIndex(device string) string {
	return strings.TrimPrefix(device, cudaDevicePrefix)
}

var nvidiaCommands = vendorCommands{
	encoders: map[agentproto.Capability]string{
		agentproto.CapEncHEVC10: encoderHEVCNVENC,
		agentproto.CapEncAV110:  encoderAV1NVENC,
	},
	encode: func(device, name string) []string {
		return args(baseArgs(), testInput(), []string{"-vf", "format=" + tenBitSoftware}, encoder(name),
			[]string{"-gpu", cudaIndex(device)}, nullOutput())
	},
	decode: func(device, sample string) []string {
		return args(baseArgs(), nvidiaInit(device), []string{"-i", sample}, nullOutput())
	},
	pipeline: func(device, sample string) []string {
		scale := "scale_cuda=w=" + scaledWidth + ":h=" + scaledHeight + ":format=" + tenBitSoftware
		return args(baseArgs(), nvidiaInit(device), []string{"-i", sample, "-vf", scale}, encoder(encoderHEVCNVENC),
			[]string{"-gpu", cudaIndex(device)}, nullOutput())
	},
}

func nvidiaInit(device string) []string {
	return []string{
		"-init_hw_device", hwCUDA + "=" + cudaDevice + ":" + cudaIndex(device),
		"-hwaccel", hwCUDA, "-hwaccel_output_format", hwCUDA,
	}
}

// Intel uses QSV on top of a VA-API device opened on its render node.
var intelCommands = vendorCommands{
	encoders: map[agentproto.Capability]string{
		agentproto.CapEncHEVC10: encoderHEVCQSV,
		agentproto.CapEncAV110:  encoderAV1QSV,
	},
	encode: func(device, name string) []string {
		upload := "format=" + tenBitSoftware + ",hwupload=extra_hw_frames=" + qsvUploadFrames
		return args(baseArgs(), qsvInit(device), testInput(), []string{"-vf", upload}, encoder(name), nullOutput())
	},
	decode: func(device, sample string) []string {
		return args(baseArgs(), qsvInit(device), qsvDecode(), []string{"-i", sample}, nullOutput())
	},
	pipeline: func(device, sample string) []string {
		scale := "vpp_qsv=w=" + scaledWidth + ":h=" + scaledHeight + ":format=" + tenBitHardware
		return args(baseArgs(), qsvInit(device), qsvDecode(), []string{"-i", sample, "-vf", scale},
			encoder(encoderHEVCQSV), nullOutput())
	},
}

func qsvInit(device string) []string {
	return []string{
		"-init_hw_device", hwVAAPI + "=" + vaapiDevice + ":" + device,
		"-init_hw_device", hwQSV + "=" + qsvDevice + "@" + vaapiDevice,
		"-filter_hw_device", qsvDevice,
	}
}

func qsvDecode() []string {
	return []string{"-hwaccel", hwQSV, "-hwaccel_output_format", hwQSV}
}

// AMD on Linux uses VA-API through Mesa.
var amdCommands = vendorCommands{
	encoders: map[agentproto.Capability]string{
		agentproto.CapEncHEVC10: encoderHEVCVAAPI,
		agentproto.CapEncAV110:  encoderAV1VAAPI,
	},
	encode: func(device, name string) []string {
		upload := "format=" + tenBitSoftware + ",hwupload"
		return args(baseArgs(), vaapiInit(device), testInput(), []string{"-vf", upload}, encoder(name), nullOutput())
	},
	decode: func(device, sample string) []string {
		return args(baseArgs(), vaapiInit(device), vaapiDecode(), []string{"-i", sample}, nullOutput())
	},
	pipeline: func(device, sample string) []string {
		scale := "scale_vaapi=w=" + scaledWidth + ":h=" + scaledHeight + ":format=" + tenBitHardware
		return args(baseArgs(), vaapiInit(device), vaapiDecode(), []string{"-i", sample, "-vf", scale},
			encoder(encoderHEVCVAAPI), nullOutput())
	},
}

func vaapiInit(device string) []string {
	return []string{"-init_hw_device", hwVAAPI + "=" + vaapiDevice + ":" + device, "-filter_hw_device", vaapiDevice}
}

func vaapiDecode() []string {
	return []string{"-hwaccel", hwVAAPI, "-hwaccel_output_format", hwVAAPI}
}

// vaapiInfoArgs opens a render node at verbose level, which logs the VA-API driver's vendor string.
func vaapiInfoArgs(device string) []string {
	return args([]string{"-hide_banner", "-nostdin", "-v", "verbose"}, vaapiInit(device),
		[]string{"-f", "lavfi", "-i", vulkanSource, "-frames:v", vulkanFrames}, nullOutput())
}

// vulkanArgs derives a Vulkan device from the GPU, the way whisper.cpp's Vulkan backend reaches it.
func vulkanArgs(report agentproto.DeviceReport) []string {
	parent := []string{"-init_hw_device", hwDRM + "=" + drmDevice + ":" + report.DeviceID}
	parentName := drmDevice
	if report.Vendor == agentproto.VendorNVIDIA {
		parent = []string{"-init_hw_device", hwCUDA + "=" + cudaDevice + ":" + cudaIndex(report.DeviceID)}
		parentName = cudaDevice
	}
	return args(baseArgs(), parent, []string{"-init_hw_device", hwVulkan + "=" + vulkanName + "@" + parentName},
		[]string{"-f", "lavfi", "-i", vulkanSource, "-frames:v", vulkanFrames}, nullOutput())
}

const encoderSVTAV1 = "libsvtav1"

var cpuEncoders = map[agentproto.Capability]string{
	agentproto.CapEncHEVC10: encoderLibx265,
	agentproto.CapEncAV110:  encoderSVTAV1,
}

// cpuEncodeArgs encodes the test clip at 10-bit with a software encoder.
func cpuEncodeArgs(encoder string) []string {
	codec := []string{"-pix_fmt", sampleHEVC10Pixels, "-c:v", encoder}
	if encoder == encoderLibx265 {
		codec = append(codec, "-x265-params", x265Quiet)
	}
	return args(baseArgs(), testInput(), codec, nullOutput())
}

// sampleArgs encodes a decode-test sample on the CPU.
func sampleArgs(s sample, path string) []string {
	codec := []string{"-pix_fmt", sampleH264Pixels, "-c:v", encoderLibx264}
	if s == sampleHEVC10 {
		codec = []string{"-pix_fmt", sampleHEVC10Pixels, "-c:v", encoderLibx265, "-x265-params", x265Quiet}
	}
	return args(baseArgs(), testInput(), codec, []string{"-y", path})
}
