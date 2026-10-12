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

// Policy is the transcode target and the rules that pick which files are transcoded. Admins edit it
// under Settings → Transcode.
type Policy struct {
	Enabled     bool                        `json:"enabled"`
	TargetCodec api.TargetCodec             `json:"targetCodec"`
	Preset      api.Preset                  `json:"preset"`
	CapsMbps    map[api.ResolutionClass]int `json:"capsMbps"`
	// SkipAlreadyTarget keeps files already in the target codec and under their cap.
	SkipAlreadyTarget bool `json:"skipAlreadyTarget"`
	// SkipLowBitrate keeps other files whose bitrate is under LowBitratePct of their cap.
	SkipLowBitrate bool `json:"skipLowBitrate"`
	LowBitratePct  int  `json:"lowBitratePct"`
	MinSavingPct   int  `json:"minSavingPct"`
	// EncoderOverrides replaces the calibrated quality value, keyed by ffmpeg encoder name.
	EncoderOverrides map[string]int `json:"encoderOverrides"`
}

const (
	defaultCap2160Mbps   = 25
	defaultCap1080Mbps   = 10
	defaultCap720Mbps    = 5
	defaultCapSDMbps     = 2
	defaultLowBitratePct = 50
	defaultMinSavingPct  = 15
	// vmafSamples is how many 10-second windows the agent scores with VMAF.
	vmafSamples = 6

	MinCapMbps       = 1
	MaxCapMbps       = 200
	MinLowBitratePct = 10
	MaxLowBitratePct = 100
	MinSavingPct     = 0
	MaxSavingPct     = 90

	kbpsPerMbps = 1000
	// The VBV buffer holds this many seconds at the maximum rate.
	bufSizeFactor = 2
)

var policyKey = settings.NewKey(owner.Transcode, "policy", Policy{
	Enabled:     true,
	TargetCodec: api.TargetHEVC10,
	Preset:      api.PresetHigh,
	CapsMbps: map[api.ResolutionClass]int{
		api.Resolution2160: defaultCap2160Mbps,
		api.Resolution1080: defaultCap1080Mbps,
		api.Resolution720:  defaultCap720Mbps,
		api.ResolutionSD:   defaultCapSDMbps,
	},
	SkipAlreadyTarget: true,
	SkipLowBitrate:    true,
	LowBitratePct:     defaultLowBitratePct,
	MinSavingPct:      defaultMinSavingPct,
	EncoderOverrides:  map[string]int{},
})

// ResolutionClasses from the largest frame down.
var ResolutionClasses = []api.ResolutionClass{api.Resolution2160, api.Resolution1080, api.Resolution720, api.ResolutionSD}

// TargetCodecs are the targets an admin can pick.
var TargetCodecs = []api.TargetCodec{api.TargetHEVC10, api.TargetAV110}

func (p Policy) validate() error {
	if !slices.Contains(TargetCodecs, p.TargetCodec) {
		return fmt.Errorf("%w: unknown target %q", ErrInvalidSetting, p.TargetCodec)
	}
	if !knownPreset(p.Preset) {
		return fmt.Errorf("%w: unknown preset %q", ErrInvalidSetting, p.Preset)
	}
	if len(p.CapsMbps) != len(ResolutionClasses) {
		return fmt.Errorf("%w: give a bitrate cap for each of %v", ErrInvalidSetting, ResolutionClasses)
	}
	for _, class := range ResolutionClasses {
		if limit, ok := p.CapsMbps[class]; !ok || limit < MinCapMbps || limit > MaxCapMbps {
			return fmt.Errorf("%w: the %s cap must be %d–%d Mbps", ErrInvalidSetting, class, MinCapMbps, MaxCapMbps)
		}
	}
	if p.LowBitratePct < MinLowBitratePct || p.LowBitratePct > MaxLowBitratePct {
		return fmt.Errorf("%w: the low-bitrate threshold must be %d–%d%% of the cap", ErrInvalidSetting, MinLowBitratePct, MaxLowBitratePct)
	}
	if p.MinSavingPct < MinSavingPct || p.MinSavingPct > MaxSavingPct {
		return fmt.Errorf("%w: the minimum saving must be %d–%d%%", ErrInvalidSetting, MinSavingPct, MaxSavingPct)
	}
	for encoder, value := range p.EncoderOverrides {
		cal, ok := calibration.Encoders[encoder]
		if !ok {
			return fmt.Errorf("%w: unknown encoder %q", ErrInvalidSetting, encoder)
		}
		if !cal.Quality.contains(value) {
			return fmt.Errorf("%w: %s %s must be %d–%d", ErrInvalidSetting, encoder, cal.Quality.Arg, cal.Quality.Min, cal.Quality.Max)
		}
	}
	return nil
}

const policyHashBytes = 8

// Hash changes whenever a value that decides or shapes a transcode changes, including the calibration
// table's version, so a skip recorded under an older policy is evaluated again. Turning transcoding off
// and on again keeps the hash.
func (p Policy) Hash() string {
	hashed := struct {
		Policy
		Enabled     bool `json:"enabled,omitempty"`
		Calibration int  `json:"calibration"`
	}{Policy: p, Calibration: calibration.Version}
	if len(hashed.EncoderOverrides) == 0 {
		hashed.EncoderOverrides = nil
	}
	encoded, err := json.Marshal(hashed)
	if err != nil {
		panic(err) // a struct of strings, ints and maps of them always encodes
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

// targetVideo is the ffmpeg codec name, bit depth, label and the capability an agent needs for a target.
type targetVideo struct {
	codec    string
	bitDepth int
	label    string
	needs    agentproto.Capability
}

const tenBit = 10

var targetVideos = map[api.TargetCodec]targetVideo{
	api.TargetHEVC10: {codec: api.VideoCodecHEVC, bitDepth: tenBit, label: "HEVC 10-bit", needs: agentproto.CapEncHEVC10},
	api.TargetAV110:  {codec: api.VideoCodecAV1, bitDepth: tenBit, label: "AV1 10-bit", needs: agentproto.CapEncAV110},
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

func firstVideo(info libraryapi.MediaInfo) (libraryapi.Stream, bool) {
	i := slices.IndexFunc(info.Streams, func(s libraryapi.Stream) bool { return s.Kind == libraryapi.StreamVideo })
	if i < 0 {
		return libraryapi.Stream{}, false
	}
	return info.Streams[i], true
}

// videoBitrateKbps is the video stream's own bitrate when the file tags it, else the whole file's.
func videoBitrateKbps(info libraryapi.MediaInfo) int {
	if video, ok := firstVideo(info); ok && video.BitrateKbps > 0 {
		return video.BitrateKbps
	}
	return info.BitrateKbps
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

// fileFacts is what the policy needs to know about one eligible file.
type fileFacts struct {
	Info     libraryapi.MediaInfo
	Excluded bool
	// DoneUnderPolicy: Magpie already transcoded the file, or found it not worth it, under this policy.
	DoneUnderPolicy bool
	// CapableAgentSeen: some agent, online or not, once passed the target's test encode.
	CapableAgentSeen bool
}

// policyResult is what evaluating a file decides.
type policyResult string

const (
	resultQueue policyResult = "QUEUE"
	resultSkip  policyResult = "SKIP"
	resultDone  policyResult = "DONE"
)

type decision struct {
	Result policyResult
	Skip   api.SkipReason
	Detail string
	Plan   jobPlan
}

func skipped(reason api.SkipReason, detail string) decision {
	return decision{Result: resultSkip, Skip: reason, Detail: detail}
}

// decide applies the policy's rules to one file; the first that matches wins.
func (p Policy) decide(facts fileFacts) decision {
	target := targetVideos[p.TargetCodec]
	video, _ := firstVideo(facts.Info)
	class := ResolutionClassOf(facts.Info.Width, facts.Info.Height)
	capKbps := p.CapsMbps[class] * kbpsPerMbps
	bitrate := videoBitrateKbps(facts.Info)
	isTarget := strings.EqualFold(video.Codec, target.codec)

	switch {
	case facts.Excluded:
		return skipped(api.SkipExcluded, "Excluded from transcoding")
	case facts.Info.HDR == libraryapi.HDRDolbyVision || facts.Info.HDR == libraryapi.HDR10Plus:
		return skipped(api.SkipHDRDynamic, fmt.Sprintf("%s keeps its original encode", hdrLabels[facts.Info.HDR]))
	case facts.DoneUnderPolicy:
		return decision{Result: resultDone}
	case p.SkipAlreadyTarget && isTarget && bitrate <= capKbps:
		return skipped(api.SkipAlreadyTarget, fmt.Sprintf("Already %s at %s, within the %s cap of %d Mbps",
			strings.ToUpper(target.codec), mbps(bitrate), class, p.CapsMbps[class]))
	case p.SkipLowBitrate && !isTarget && bitrate > 0 && bitrate*percent < p.LowBitratePct*capKbps:
		return skipped(api.SkipLowBitrate, fmt.Sprintf("%s is under %d%% of the %s cap of %d Mbps",
			mbps(bitrate), p.LowBitratePct, class, p.CapsMbps[class]))
	case !facts.CapableAgentSeen:
		return skipped(api.SkipNoCapableAgent, "No agent has passed a "+target.label+" test encode")
	}
	return decision{Result: resultQueue, Plan: jobPlan{
		TargetCodec: p.TargetCodec,
		MaxRateKbps: capKbps,
		HDR10:       facts.Info.HDR == libraryapi.HDR10,
		PolicyHash:  p.Hash(),
	}}
}

var hdrLabels = map[libraryapi.HDRFormat]string{
	libraryapi.HDRDolbyVision: "Dolby Vision",
	libraryapi.HDR10Plus:      "HDR10+",
}

func mbps(kbps int) string {
	return fmt.Sprintf("%.1f Mbps", float64(kbps)/kbpsPerMbps)
}

// jobPlan is what a queued transcode job keeps until it is offered.
type jobPlan struct {
	TargetCodec api.TargetCodec
	MaxRateKbps int
	HDR10       bool
	PolicyHash  string
}

// transcodeParams builds the encode for one device, keeping the audio and subtitle streams in keep.
func transcodeParams(policy Policy, plan jobPlan, vendor agentproto.Vendor, source libraryapi.MediaInfo, keep keptLanguages) (agentproto.TranscodeParams, error) {
	video, ok := targetVideos[plan.TargetCodec]
	if !ok {
		return agentproto.TranscodeParams{}, fmt.Errorf("transcode: unknown target %q", plan.TargetCodec)
	}
	encoder, ok := encoders[vendor][plan.TargetCodec]
	if !ok {
		return agentproto.TranscodeParams{}, fmt.Errorf("transcode: no %s encoder for %s devices", plan.TargetCodec, vendor)
	}
	args, err := encoderArgs(encoder, policy.Preset, policy.EncoderOverrides)
	if err != nil {
		return agentproto.TranscodeParams{}, err
	}
	audio, subs := keptTracks(source, keep)
	return agentproto.TranscodeParams{
		Codec:        video.codec,
		BitDepth:     video.bitDepth,
		Encoder:      encoder,
		EncoderArgs:  args,
		MaxRateKbps:  plan.MaxRateKbps,
		BufSizeKbps:  plan.MaxRateKbps * bufSizeFactor,
		KeepAudio:    audio,
		KeepSubs:     subs,
		HDR10:        plan.HDR10,
		VMAFSamples:  vmafSamples,
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
