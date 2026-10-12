package transcode

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"

	"magpie/internal/transcode/api"
)

// The calibration table maps each quality preset to every encoder's own quality value. It ships in the
// binary and isn't user data; its version is part of the policy hash.
//
//go:embed calibration.json
var calibrationJSON []byte

// QualityKnob is the encoder option that sets quality, and its range. Lower values mean higher quality
// on every encoder Magpie uses.
type QualityKnob struct {
	Arg string `json:"arg"`
	Min int    `json:"min"`
	Max int    `json:"max"`
}

func (k QualityKnob) contains(value int) bool {
	return value >= k.Min && value <= k.Max
}

// EncoderCalibration is one encoder's fixed options and its quality value per preset.
type EncoderCalibration struct {
	Quality QualityKnob        `json:"quality"`
	Args    map[string]string  `json:"args"`
	Presets map[api.Preset]int `json:"presets"`
}

type calibrationTable struct {
	Version  int                           `json:"version"`
	Encoders map[string]EncoderCalibration `json:"encoders"`
}

var calibration = mustParseCalibration(calibrationJSON)

func mustParseCalibration(raw []byte) calibrationTable {
	var table calibrationTable
	if err := json.Unmarshal(raw, &table); err != nil {
		panic(fmt.Sprintf("transcode: the embedded calibration table doesn't parse: %v", err))
	}
	return table
}

// Presets from the highest quality to the smallest files.
var presets = []api.Preset{api.PresetArchive, api.PresetHigh, api.PresetBalanced, api.PresetSmall}

// Calibrations lists every encoder's calibration, by encoder name.
func Calibrations() map[string]EncoderCalibration {
	return maps.Clone(calibration.Encoders)
}

// encoderArgs are the options for one encode: the encoder's fixed options plus its quality value for the
// preset, or the admin's override for that encoder.
func encoderArgs(encoder string, preset api.Preset, overrides map[string]int) (map[string]string, error) {
	cal, ok := calibration.Encoders[encoder]
	if !ok {
		return nil, fmt.Errorf("transcode: no calibration for encoder %s", encoder)
	}
	quality, ok := overrides[encoder]
	if !ok {
		quality, ok = cal.Presets[preset]
	}
	if !ok {
		return nil, fmt.Errorf("transcode: no %s calibration for preset %s", encoder, preset)
	}
	args := maps.Clone(cal.Args)
	args[cal.Quality.Arg] = strconv.Itoa(quality)
	return args, nil
}

func knownPreset(p api.Preset) bool {
	return slices.Contains(presets, p)
}
