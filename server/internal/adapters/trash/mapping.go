package trash

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strconv"

	"magpie/internal/acquisition"
	"magpie/internal/acquisition/api"
	"magpie/internal/platform/parse"
)

// defaultScoreSet is the trash_scores key a profile without trash_score_set uses.
const defaultScoreSet = "default"

// Sonarr's and Radarr's custom-format condition implementations.
const (
	implReleaseTitle    = "ReleaseTitleSpecification"
	implReleaseGroup    = "ReleaseGroupSpecification"
	implEdition         = "EditionSpecification"
	implSource          = "SourceSpecification"
	implResolution      = "ResolutionSpecification"
	implQualityModifier = "QualityModifierSpecification"
	implLanguage        = "LanguageSpecification"
	implIndexerFlag     = "IndexerFlagSpecification"
	implReleaseType     = "ReleaseTypeSpecification"
	implSize            = "SizeSpecification"
	implYear            = "YearSpecification"
)

type specJSON struct {
	Name           string `json:"name"`
	Implementation string `json:"implementation"`
	Negate         bool   `json:"negate"`
	Required       bool   `json:"required"`
	Fields         struct {
		Value          json.RawMessage `json:"value"`
		Min            float64         `json:"min"`
		Max            float64         `json:"max"`
		ExceptLanguage bool            `json:"exceptLanguage"`
	} `json:"fields"`
}

type customFormatJSON struct {
	TrashID             string         `json:"trash_id"`
	TrashScores         map[string]int `json:"trash_scores"`
	Name                string         `json:"name"`
	IncludeWhenRenaming bool           `json:"includeCustomFormatWhenRenaming"`
	Specifications      []specJSON     `json:"specifications"`
}

type profileItemJSON struct {
	Name    string   `json:"name"`
	Allowed bool     `json:"allowed"`
	Items   []string `json:"items"`
}

type qualityProfileJSON struct {
	TrashID               string            `json:"trash_id"`
	Name                  string            `json:"name"`
	ScoreSet              string            `json:"trash_score_set"`
	UpgradeAllowed        bool              `json:"upgradeAllowed"`
	Cutoff                string            `json:"cutoff"`
	MinFormatScore        int               `json:"minFormatScore"`
	CutoffFormatScore     int               `json:"cutoffFormatScore"`
	MinUpgradeFormatScore int               `json:"minUpgradeFormatScore"`
	Items                 []profileItemJSON `json:"items"`
	FormatItems           map[string]string `json:"formatItems"` // format name → trash_id
}

type qualitySizeJSON struct {
	Qualities []struct {
		Quality   string  `json:"quality"`
		Min       float64 `json:"min"`
		Preferred float64 `json:"preferred"`
		Max       float64 `json:"max"`
	} `json:"qualities"`
}

// Enum values as Radarr and Sonarr number them in their APIs.
var (
	radarrSources = map[int]parse.QualitySource{
		0: parse.SourceUnknown, 1: parse.SourceCam, 2: parse.SourceTelesync, 3: parse.SourceTelecine, 4: parse.SourceWorkprint,
		5: parse.SourceDVD, 6: parse.SourceTV, 7: parse.SourceWebDL, 8: parse.SourceWebRip, 9: parse.SourceBluray,
	}
	// Sonarr keeps raw captures (Raw-HD, Remux) as their own sources.
	sonarrSources = map[int]parse.QualitySource{
		0: parse.SourceUnknown, 1: parse.SourceTV, 2: acquisition.SourceTVRaw, 3: parse.SourceWebDL, 4: parse.SourceWebRip,
		5: parse.SourceDVD, 6: parse.SourceBluray, 7: acquisition.SourceBlurayRaw,
	}
	radarrModifiers = map[int]parse.QualityModifier{
		0: parse.ModifierNone, 1: parse.ModifierRegional, 2: parse.ModifierScreener, 3: parse.ModifierRawHD, 4: parse.ModifierBRDisk, 5: parse.ModifierRemux,
	}
	indexerFlags = map[acquisition.TrashApp]map[int]acquisition.IndexerFlag{
		acquisition.TrashRadarr: {1: acquisition.FlagFreeleech, 32: acquisition.FlagInternal},
		acquisition.TrashSonarr: {1: acquisition.FlagFreeleech, 8: acquisition.FlagInternal},
	}
	releaseTypes = map[int]acquisition.ReleaseType{
		1: acquisition.ReleaseTypeSingleEpisode, 2: acquisition.ReleaseTypeMultiEpisode, 3: acquisition.ReleaseTypeSeasonPack,
	}
	// The language IDs TRaSH uses; both apps number them the same.
	languages = map[int]string{
		-2: acquisition.LanguageOriginal, 1: "en", 2: "fr", 4: "de", 8: "ja", 10: "zh", 21: "ko",
	}
)

// toPort maps a format, or reports false (and logs why) when a spec can't be expressed in Magpie.
func (f customFormatJSON) toPort(app acquisition.TrashApp) (acquisition.TrashCustomFormat, bool) {
	format := acquisition.TrashCustomFormat{
		TrashID:             f.TrashID,
		Name:                f.Name,
		Scores:              f.TrashScores,
		IncludeWhenRenaming: f.IncludeWhenRenaming,
		Specs:               make([]api.FormatSpec, 0, len(f.Specifications)),
	}
	for _, s := range f.Specifications {
		spec, err := s.toPort(app)
		if err != nil {
			slog.Warn("skipping TRaSH custom format", "app", app, "trash_id", f.TrashID, "name", f.Name, "error", err)
			return acquisition.TrashCustomFormat{}, false
		}
		format.Specs = append(format.Specs, spec)
	}
	return format, true
}

func (s specJSON) toPort(app acquisition.TrashApp) (api.FormatSpec, error) {
	spec := api.FormatSpec{Name: s.Name, Negate: s.Negate, Required: s.Required}
	var err error
	switch s.Implementation {
	case implReleaseTitle:
		spec.Kind = api.SpecReleaseTitle
		err = json.Unmarshal(s.Fields.Value, &spec.Value)
	case implReleaseGroup:
		spec.Kind = api.SpecReleaseGroup
		err = json.Unmarshal(s.Fields.Value, &spec.Value)
	case implEdition:
		spec.Kind = api.SpecEdition
		err = json.Unmarshal(s.Fields.Value, &spec.Value)
	case implSource:
		spec.Kind = api.SpecSource
		spec.Value, err = enumValue(s, sourcesFor(app))
	case implResolution:
		spec.Kind = api.SpecResolution
		var lines int
		err = json.Unmarshal(s.Fields.Value, &lines)
		spec.Value = strconv.Itoa(lines)
	case implQualityModifier:
		spec.Kind = api.SpecQualityModifier
		spec.Value, err = enumValue(s, radarrModifiers)
	case implLanguage:
		if s.Fields.ExceptLanguage {
			return spec, fmt.Errorf("spec %q: exceptLanguage is not supported", s.Name)
		}
		spec.Kind = api.SpecLanguage
		spec.Value, err = enumValue(s, languages)
	case implIndexerFlag:
		spec.Kind = api.SpecIndexerFlag
		spec.Value, err = enumValue(s, indexerFlags[app])
	case implReleaseType:
		spec.Kind = api.SpecReleaseType
		spec.Value, err = enumValue(s, releaseTypes)
	case implSize:
		spec.Kind, spec.Min, spec.Max = api.SpecSize, s.Fields.Min, s.Fields.Max
	case implYear:
		spec.Kind, spec.Min, spec.Max = api.SpecYear, s.Fields.Min, s.Fields.Max
	default:
		return spec, fmt.Errorf("spec %q: unsupported implementation %q", s.Name, s.Implementation)
	}
	return spec, err
}

func sourcesFor(app acquisition.TrashApp) map[int]parse.QualitySource {
	if app == acquisition.TrashSonarr {
		return sonarrSources
	}
	return radarrSources
}

func enumValue[T ~string](s specJSON, values map[int]T) (string, error) {
	var number int
	if err := json.Unmarshal(s.Fields.Value, &number); err != nil {
		return "", fmt.Errorf("spec %q: %w", s.Name, err)
	}
	value, ok := values[number]
	if !ok {
		return "", fmt.Errorf("spec %q: unknown %s value %d", s.Name, s.Implementation, number)
	}
	return string(value), nil
}

func (p qualityProfileJSON) toPort() acquisition.TrashQualityProfile {
	scoreSet := p.ScoreSet
	if scoreSet == "" {
		scoreSet = defaultScoreSet
	}
	profile := acquisition.TrashQualityProfile{
		TrashID:               p.TrashID,
		Name:                  p.Name,
		ScoreSet:              scoreSet,
		UpgradeAllowed:        p.UpgradeAllowed,
		Cutoff:                p.Cutoff,
		MinFormatScore:        p.MinFormatScore,
		CutoffFormatScore:     p.CutoffFormatScore,
		MinUpgradeFormatScore: p.MinUpgradeFormatScore,
		Items:                 make([]acquisition.TrashProfileItem, 0, len(p.Items)),
		Formats:               make([]string, 0, len(p.FormatItems)),
	}
	for _, item := range p.Items {
		profile.Items = append(profile.Items, acquisition.TrashProfileItem{Name: item.Name, Qualities: item.Items, Allowed: item.Allowed})
	}
	for _, trashID := range p.FormatItems {
		profile.Formats = append(profile.Formats, trashID)
	}
	slices.Sort(profile.Formats)
	return profile
}

func (t qualitySizeJSON) toPort() []acquisition.TrashQualitySize {
	sizes := make([]acquisition.TrashQualitySize, 0, len(t.Qualities))
	for _, q := range t.Qualities {
		sizes = append(sizes, acquisition.TrashQualitySize{
			Quality: q.Quality,
			Sizes:   api.SizeLimits{MinMBPerMin: q.Min, PreferredMBPerMin: q.Preferred, MaxMBPerMin: q.Max},
		})
	}
	return sizes
}
