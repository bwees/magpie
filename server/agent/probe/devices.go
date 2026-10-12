package probe

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"strings"

	"magpie/internal/transcode/api/agentproto"
)

const (
	cudaDevicePrefix = "cuda:"
	devDRI           = "/dev/dri/"

	// drmGlob is relative to the host root the prober reads (fs.FS paths have no leading slash).
	drmGlob          = "sys/class/drm/renderD*"
	pciVendorFile    = "device/vendor"
	pciDeviceFile    = "device/device"
	kernelDriverLink = "device/driver"

	pciVendorNVIDIA = "0x10de"
	pciVendorIntel  = "0x8086"
	pciVendorAMD    = "0x1002"

	// GeForce cards allow this many concurrent NVENC sessions; other cards have no published limit.
	geForceSessionLimit = 12
	geForceMarker       = "GeForce"

	nvidiaFieldSeparator = ","
	nvidiaFields         = 3

	vaapiDriverPrefix = "VAAPI driver: "
	vaapiNameSep      = " for "
	vaapiVersionSep   = " - "
	vaapiDetailsSep   = " ("
)

var nvidiaQuery = []string{"--query-gpu=index,name,driver_version", "--format=csv,noheader"}

// DRM nodes of NVIDIA cards are skipped: those GPUs are reached through CUDA.
var drmVendors = map[string]agentproto.Vendor{
	pciVendorIntel: agentproto.VendorIntel,
	pciVendorAMD:   agentproto.VendorAMD,
}

// CPUDeviceID names the software encoders when the agent offers them.
const (
	CPUDeviceID   = "cpu"
	cpuDeviceName = "CPU (software encoders)"
)

// devices lists every GPU the agent can see, NVIDIA cards first, then the CPU when it is offered.
func (p *Prober) devices(ctx context.Context) []agentproto.DeviceReport {
	devices := append(p.nvidiaDevices(ctx), p.drmDevices(ctx)...)
	if p.CPU {
		devices = append(devices, agentproto.DeviceReport{DeviceID: CPUDeviceID, Vendor: agentproto.VendorCPU, Name: cpuDeviceName})
	}
	return devices
}

func (p *Prober) nvidiaDevices(ctx context.Context) []agentproto.DeviceReport {
	stdout, stderr, err := p.run(ctx, p.NvidiaSMI, nvidiaQuery...)
	if err != nil {
		slog.DebugContext(ctx, "probe: no NVIDIA GPUs", "error", errorText(stderr, err))
		return nil
	}
	var devices []agentproto.DeviceReport
	for line := range strings.Lines(stdout) {
		fields := strings.Split(line, nvidiaFieldSeparator)
		if len(fields) != nvidiaFields {
			continue
		}
		index, name, driver := strings.TrimSpace(fields[0]), strings.TrimSpace(fields[1]), strings.TrimSpace(fields[2])
		device := agentproto.DeviceReport{
			DeviceID: cudaDevicePrefix + index,
			Vendor:   agentproto.VendorNVIDIA,
			Name:     name,
			Driver:   driver,
		}
		if strings.Contains(name, geForceMarker) {
			device.MaxSessions = geForceSessionLimit
		}
		devices = append(devices, device)
	}
	return devices
}

func (p *Prober) drmDevices(ctx context.Context) []agentproto.DeviceReport {
	nodes, err := fs.Glob(p.Host, drmGlob)
	if err != nil {
		slog.WarnContext(ctx, "probe: listing DRM nodes", "error", err)
		return nil
	}
	var devices []agentproto.DeviceReport
	for _, node := range nodes {
		vendorID := p.readSys(node, pciVendorFile)
		vendor, ok := drmVendors[vendorID]
		if !ok {
			continue
		}
		deviceID := devDRI + path.Base(node)
		name, driver := p.vaapiIdentity(ctx, deviceID)
		if name == "" {
			name = fmt.Sprintf("%s GPU %s", vendor, p.readSys(node, pciDeviceFile))
		}
		if driver == "" {
			driver = p.kernelDriver(node)
		}
		devices = append(devices, agentproto.DeviceReport{DeviceID: deviceID, Vendor: vendor, Name: name, Driver: driver})
	}
	return devices
}

func (p *Prober) readSys(node, file string) string {
	raw, err := fs.ReadFile(p.Host, path.Join(node, file))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func (p *Prober) kernelDriver(node string) string {
	target, err := fs.ReadLink(p.Host, path.Join(node, kernelDriverLink))
	if err != nil {
		return ""
	}
	return path.Base(target)
}

// vaapiIdentity opens the node through VA-API and reads the driver's own description of the GPU.
func (p *Prober) vaapiIdentity(ctx context.Context, deviceID string) (name, driver string) {
	_, stderr, err := p.run(ctx, p.FFmpeg, vaapiInfoArgs(deviceID)...)
	if err != nil {
		return "", ""
	}
	for line := range strings.Lines(stderr) {
		if _, description, found := strings.Cut(line, vaapiDriverPrefix); found {
			return parseVAAPIDriver(description)
		}
	}
	return "", ""
}

// parseVAAPIDriver splits a VA-API vendor string such as
// "Intel iHD driver for Intel(R) Gen Graphics - 24.1.0 ()" or
// "Mesa Gallium driver 24.0.5 for AMD Radeon RX 7900 XTX (radeonsi, navi31, …)" into GPU name and driver.
func parseVAAPIDriver(description string) (name, driver string) {
	description = strings.TrimSuffix(strings.TrimSpace(description), ".")
	driver, gpu, found := strings.Cut(description, vaapiNameSep)
	if !found {
		return "", description
	}
	gpu, version, hasVersion := strings.Cut(gpu, vaapiVersionSep)
	name, _, _ = strings.Cut(gpu, vaapiDetailsSep)
	if hasVersion {
		version, _, _ = strings.Cut(version, vaapiDetailsSep)
		driver += " " + strings.TrimSpace(version)
	}
	return strings.TrimSpace(name), driver
}
