package transcode

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/platform/owner"
	"magpie/internal/platform/settings"
	"magpie/internal/transcode/api"
	"magpie/internal/transcode/api/agentproto"
)

// Policy is the transcode target and the thresholds the agent applies. Ticket 25 adds its settings page
// and the skip rules; until then the defaults apply.
type Policy struct {
	TargetCodec  api.TargetCodec             `json:"targetCodec"`
	Preset       api.Preset                  `json:"preset"`
	CapsMbps     map[api.ResolutionClass]int `json:"capsMbps"`
	MinSavingPct int                         `json:"minSavingPct"`
	VMAFSamples  int                         `json:"vmafSamples"`
}

const (
	defaultCap2160Mbps  = 25
	defaultCap1080Mbps  = 10
	defaultCap720Mbps   = 5
	defaultCapSDMbps    = 2
	defaultMinSavingPct = 15
	defaultVMAFSamples  = 6

	kbpsPerMbps = 1000
	// The VBV buffer holds this many seconds at the maximum rate.
	bufSizeFactor = 2
)

var policyKey = settings.NewKey(owner.Transcode, "policy", Policy{
	TargetCodec: api.TargetHEVC10,
	Preset:      api.PresetHigh,
	CapsMbps: map[api.ResolutionClass]int{
		api.Resolution2160: defaultCap2160Mbps,
		api.Resolution1080: defaultCap1080Mbps,
		api.Resolution720:  defaultCap720Mbps,
		api.ResolutionSD:   defaultCapSDMbps,
	},
	MinSavingPct: defaultMinSavingPct,
	VMAFSamples:  defaultVMAFSamples,
})

const policyHashBytes = 8

// Hash changes whenever any policy value changes, so a skip recorded under an older policy is re-evaluated.
func (p Policy) Hash() string {
	encoded, err := json.Marshal(p)
	if err != nil {
		panic(err) // a struct of strings and ints always encodes
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:policyHashBytes])
}

func (p Policy) target() api.Target {
	return api.Target{Codec: p.TargetCodec, Preset: p.Preset, CapsMbps: maps.Clone(p.CapsMbps)}
}

// MaxRateKbps is the bitrate cap for a source of the given size.
func (p Policy) MaxRateKbps(width, height int) int {
	return p.CapsMbps[ResolutionClassOf(width, height)] * kbpsPerMbps
}

// The smallest frame each class holds. Width or height may reach it, so letterboxed and 4:3 sources land
// in their nominal class.
type resolutionFloor struct {
	class         api.ResolutionClass
	width, height int
}

var resolutionFloors = []resolutionFloor{
	{class: api.Resolution2160, width: 3200, height: 1800},
	{class: api.Resolution1080, width: 1600, height: 900},
	{class: api.Resolution720, width: 1100, height: 650},
}

// ResolutionClassOf groups a frame size for the bitrate caps.
func ResolutionClassOf(width, height int) api.ResolutionClass {
	for _, floor := range resolutionFloors {
		if width >= floor.width || height >= floor.height {
			return floor.class
		}
	}
	return api.ResolutionSD
}

// targetVideo is the ffmpeg codec name, bit depth and the capability an agent needs for a target.
type targetVideo struct {
	codec    string
	bitDepth int
	needs    agentproto.Capability
}

const tenBit = 10

var targetVideos = map[api.TargetCodec]targetVideo{
	api.TargetHEVC10: {codec: api.VideoCodecHEVC, bitDepth: tenBit, needs: agentproto.CapEncHEVC10},
	api.TargetAV110:  {codec: api.VideoCodecAV1, bitDepth: tenBit, needs: agentproto.CapEncAV110},
}

// ffmpeg encoder names.
const (
	EncoderHEVCNVENC = "hevc_nvenc"
	EncoderAV1NVENC  = "av1_nvenc"
	EncoderHEVCQSV   = "hevc_qsv"
	EncoderAV1QSV    = "av1_qsv"
	EncoderHEVCVAAPI = "hevc_vaapi"
	EncoderAV1VAAPI  = "av1_vaapi"
	EncoderLibx265   = "libx265"
	EncoderSVTAV1    = "libsvtav1"
)

var encoders = map[agentproto.Vendor]map[api.TargetCodec]string{
	agentproto.VendorNVIDIA: {api.TargetHEVC10: EncoderHEVCNVENC, api.TargetAV110: EncoderAV1NVENC},
	agentproto.VendorIntel:  {api.TargetHEVC10: EncoderHEVCQSV, api.TargetAV110: EncoderAV1QSV},
	agentproto.VendorAMD:    {api.TargetHEVC10: EncoderHEVCVAAPI, api.TargetAV110: EncoderAV1VAAPI},
	agentproto.VendorCPU:    {api.TargetHEVC10: EncoderLibx265, api.TargetAV110: EncoderSVTAV1},
}

// encoderArgs are the quality settings per encoder until ticket 25's calibration table replaces them.
var encoderArgs = map[string]map[string]string{
	EncoderHEVCNVENC: {"preset": "p6", "tune": "hq", "rc": "vbr", "cq": "24", "b:v": "0", "spatial-aq": "1"},
	EncoderAV1NVENC:  {"preset": "p6", "tune": "hq", "rc": "vbr", "cq": "30", "b:v": "0", "spatial-aq": "1"},
	EncoderHEVCQSV:   {"preset": "slower", "global_quality": "24"},
	EncoderAV1QSV:    {"preset": "slower", "global_quality": "28"},
	EncoderHEVCVAAPI: {"rc_mode": "QVBR", "global_quality": "24"},
	EncoderAV1VAAPI:  {"rc_mode": "QVBR", "global_quality": "30"},
	EncoderLibx265:   {"preset": "medium", "crf": "22"},
	EncoderSVTAV1:    {"preset": "8", "crf": "30"},
}

// AlreadyTarget reports whether the file's first video stream is in the target codec.
func (p Policy) AlreadyTarget(info libraryapi.MediaInfo) bool {
	video, ok := firstVideo(info)
	return ok && strings.EqualFold(video.Codec, targetVideos[p.TargetCodec].codec)
}

func firstVideo(info libraryapi.MediaInfo) (libraryapi.Stream, bool) {
	i := slices.IndexFunc(info.Streams, func(s libraryapi.Stream) bool { return s.Kind == libraryapi.StreamVideo })
	if i < 0 {
		return libraryapi.Stream{}, false
	}
	return info.Streams[i], true
}

// streamsOf lists the source indexes of one kind of stream, in source order.
func streamsOf(info libraryapi.MediaInfo, kind libraryapi.StreamKind) []int {
	indexes := []int{}
	for _, s := range info.Streams {
		if s.Kind == kind {
			indexes = append(indexes, s.Index)
		}
	}
	return indexes
}

// jobPlan is what a queued transcode job keeps until it is offered.
type jobPlan struct {
	TargetCodec api.TargetCodec
	MaxRateKbps int
	HDR10       bool
}

// transcodeParams builds the encode for one device. Ticket 25 narrows the kept tracks to the title's
// languages; until then every audio and subtitle stream is kept.
func transcodeParams(policy Policy, plan jobPlan, vendor agentproto.Vendor, source libraryapi.MediaInfo) (agentproto.TranscodeParams, error) {
	video, ok := targetVideos[plan.TargetCodec]
	if !ok {
		return agentproto.TranscodeParams{}, fmt.Errorf("transcode: unknown target %q", plan.TargetCodec)
	}
	encoder, ok := encoders[vendor][plan.TargetCodec]
	if !ok {
		return agentproto.TranscodeParams{}, fmt.Errorf("transcode: no %s encoder for %s devices", plan.TargetCodec, vendor)
	}
	return agentproto.TranscodeParams{
		Codec:        video.codec,
		BitDepth:     video.bitDepth,
		Encoder:      encoder,
		EncoderArgs:  maps.Clone(encoderArgs[encoder]),
		MaxRateKbps:  plan.MaxRateKbps,
		BufSizeKbps:  plan.MaxRateKbps * bufSizeFactor,
		KeepAudio:    streamsOf(source, libraryapi.StreamAudio),
		KeepSubs:     streamsOf(source, libraryapi.StreamSubtitle),
		HDR10:        plan.HDR10,
		VMAFSamples:  policy.VMAFSamples,
		MinSavingPct: policy.MinSavingPct,
	}, nil
}

// Job priorities: higher is offered first.
const (
	priorityBacklog  = 0
	priorityDownload = 1
	priorityWhisper  = 2
)

// transcodePriority runs downloads ahead of files found by a library scan or a migration.
func transcodePriority(origin domain.Origin) int {
	if origin == domain.OriginDownload {
		return priorityDownload
	}
	return priorityBacklog
}
