package acquisition

import (
	"slices"

	"magpie/internal/acquisition/api"
	"magpie/internal/domain"
	"magpie/internal/platform/parse"
)

const (
	firstGroupID = 1000

	// Sizes are MB per minute. Radarr treats 2000 as unlimited and TRaSH prefers just under it.
	presetPreferredMBPerMin = 1999
	presetMaxMBPerMin       = 2000

	presetUpgradeIncrement = 1
)

// Preset group members: every source at the resolution, without disc images or raw captures.
var (
	presetSources   = []parse.QualitySource{parse.SourceWebDL, parse.SourceWebRip, parse.SourceTV, parse.SourceBluray}
	presetModifiers = []parse.QualityModifier{parse.ModifierNone, parse.ModifierRemux}
)

// Formats every preset scores, by TRaSH name: release-group tiers order the sources inside the
// group, and the unwanted formats carry TRaSH's -10000.
var presetCommonFormats = []string{
	"Remux Tier 01", "Remux Tier 02", "Remux Tier 03",
	"WEB Tier 01", "WEB Tier 02", "WEB Tier 03", "WEB Scene",
	"Repack/Proper", "Repack2", "Repack3",
	"BR-DISK", "LQ", "LQ (Release Title)", "3D", "Extras", "Upscaled", "Bad Dual Groups",
}

var (
	presetHDFormats  = []string{"HD Bluray Tier 01", "HD Bluray Tier 02", "HD Bluray Tier 03", "x265 (HD)"}
	presetUHDFormats = []string{"UHD Bluray Tier 01", "UHD Bluray Tier 02", "UHD Bluray Tier 03"}
)

type presetDefinition struct {
	name       string
	resolution parse.Resolution
	// minMBPerMin is the lowest TRaSH minimum of the group's qualities across its movie and series tables.
	minMBPerMin float64
	formats     []string
}

var presets = map[api.QualityPreset]presetDefinition{
	api.Preset720p:  {name: "720p", resolution: parse.Resolution720p, minMBPerMin: 10, formats: slices.Concat(presetCommonFormats, presetHDFormats)},
	api.Preset1080p: {name: "1080p", resolution: parse.Resolution1080p, minMBPerMin: 12.5, formats: slices.Concat(presetCommonFormats, presetHDFormats)},
	api.Preset2160p: {name: "2160p", resolution: parse.Resolution2160p, minMBPerMin: 25, formats: slices.Concat(presetCommonFormats, presetUHDFormats)},
}

// FirstRunPresets are created on the first start, in this order.
var FirstRunPresets = []api.QualityPreset{api.Preset720p, api.Preset1080p, api.Preset2160p}

// DefaultPreset is the default profile for new requests after the first run.
const DefaultPreset = api.Preset1080p

func knownPreset(p api.QualityPreset) bool {
	_, ok := presets[p]
	return ok
}

// PresetProfile builds a resolution preset: one allowed group holding every source at the
// resolution, which is also the cutoff. Upgrade-until is 0, so reaching the resolution is enough.
// It has no ID and no format scores; the service attaches those.
func PresetProfile(preset api.QualityPreset) api.Profile {
	def := presets[preset]
	var members []domain.Quality
	for _, q := range parse.Qualities() {
		if q.Resolution == def.resolution && slices.Contains(presetSources, q.Source) && slices.Contains(presetModifiers, q.Modifier) {
			members = append(members, q.Quality(0))
		}
	}
	return api.Profile{
		Name:           def.name,
		UpgradeAllowed: true,
		Preset:         preset,
		Items: []api.ProfileItem{{
			Group:     &api.QualityGroup{ID: firstGroupID, Name: def.name},
			Qualities: members,
			Allowed:   true,
			Sizes:     api.SizeLimits{MinMBPerMin: def.minMBPerMin, PreferredMBPerMin: presetPreferredMBPerMin, MaxMBPerMin: presetMaxMBPerMin},
		}},
		Cutoff:                0,
		MinUpgradeFormatScore: presetUpgradeIncrement,
		PropersMode:           api.PropersPreferAndUpgrade,
		FormatScores:          []api.FormatScore{},
	}
}

// presetFormatNames lists the TRaSH format names a preset scores.
func presetFormatNames(preset api.QualityPreset) []string {
	return presets[preset].formats
}
