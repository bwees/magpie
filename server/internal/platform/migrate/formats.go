package migrate

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/domain"
	"magpie/internal/platform/parse"

	"github.com/dlclark/regexp2/v2"
)

// Values of Magpie's custom-format specs that only Acquisition's own package names.
const (
	sourceBlurayRaw = "BLURAY_RAW"
	sourceTVRaw     = "TV_RAW"
	languageOrigin  = "ORIGINAL"
	flagFreeleech   = "FREELEECH"
	flagInternal    = "INTERNAL"

	releaseSingleEpisode = "SINGLE_EPISODE"
	releaseMultiEpisode  = "MULTI_EPISODE"
	releaseSeasonPack    = "SEASON_PACK"
)

// blockScore is TRaSH's "never grab" score, which release profiles and profile languages become.
const blockScore = -10000

// sourceTranslation is what one app source value becomes: a Magpie source, plus a quality modifier for
// Sonarr's raw sources, which Magpie splits into source and modifier.
type sourceTranslation struct {
	source   parse.QualitySource
	modifier parse.QualityModifier
}

// Sonarr's QualitySource values.
const (
	sonarrSourceUnknown = iota
	sonarrSourceTelevision
	sonarrSourceTelevisionRaw
	sonarrSourceWeb
	sonarrSourceWebRip
	sonarrSourceDVD
	sonarrSourceBluray
	sonarrSourceBlurayRaw
)

// Radarr's Source values.
const (
	radarrSourceUnknown = iota
	radarrSourceCam
	radarrSourceTelesync
	radarrSourceTelecine
	radarrSourceWorkprint
	radarrSourceDVD
	radarrSourceTV
	radarrSourceWebDL
	radarrSourceWebRip
	radarrSourceBluray
)

var appSources = map[App]map[int]sourceTranslation{
	AppSonarr: {
		sonarrSourceUnknown:       {source: parse.SourceUnknown},
		sonarrSourceTelevision:    {source: parse.SourceTV},
		sonarrSourceTelevisionRaw: {source: parse.SourceTV, modifier: parse.ModifierRawHD},
		sonarrSourceWeb:           {source: parse.SourceWebDL},
		sonarrSourceWebRip:        {source: parse.SourceWebRip},
		sonarrSourceDVD:           {source: parse.SourceDVD},
		sonarrSourceBluray:        {source: parse.SourceBluray},
		sonarrSourceBlurayRaw:     {source: parse.SourceBluray, modifier: parse.ModifierRemux},
	},
	AppRadarr: {
		radarrSourceUnknown:   {source: parse.SourceUnknown},
		radarrSourceCam:       {source: parse.SourceCam},
		radarrSourceTelesync:  {source: parse.SourceTelesync},
		radarrSourceTelecine:  {source: parse.SourceTelecine},
		radarrSourceWorkprint: {source: parse.SourceWorkprint},
		radarrSourceDVD:       {source: parse.SourceDVD},
		radarrSourceTV:        {source: parse.SourceTV},
		radarrSourceWebDL:     {source: parse.SourceWebDL},
		radarrSourceWebRip:    {source: parse.SourceWebRip},
		radarrSourceBluray:    {source: parse.SourceBluray},
	},
}

// Radarr's quality modifiers, in its enum order.
var radarrModifiers = []parse.QualityModifier{
	parse.ModifierNone, parse.ModifierRegional, parse.ModifierScreener, parse.ModifierRawHD, parse.ModifierBRDisk, parse.ModifierRemux,
}

// Sonarr's release types, in its enum order; Unknown has no Magpie counterpart.
var sonarrReleaseTypes = []string{"", releaseSingleEpisode, releaseMultiEpisode, releaseSeasonPack}

// Magpie's values per spec kind, offered when a spec needs editing.
var specOptions = map[acquisitionapi.FormatSpecKind][]string{
	acquisitionapi.SpecSource: {
		string(parse.SourceUnknown), string(parse.SourceCam), string(parse.SourceTelesync), string(parse.SourceTelecine),
		string(parse.SourceWorkprint), string(parse.SourceDVD), string(parse.SourceTV), string(parse.SourceWebDL),
		string(parse.SourceWebRip), string(parse.SourceBluray), sourceBlurayRaw, sourceTVRaw,
	},
	acquisitionapi.SpecQualityModifier: {
		string(parse.ModifierNone), string(parse.ModifierRegional), string(parse.ModifierScreener),
		string(parse.ModifierRawHD), string(parse.ModifierBRDisk), string(parse.ModifierRemux),
	},
	acquisitionapi.SpecIndexerFlag: {flagFreeleech, flagInternal},
	acquisitionapi.SpecReleaseType: {releaseSingleEpisode, releaseMultiEpisode, releaseSeasonPack},
}

// flagPrefixes are tracker-family prefixes in *arr flag names ("G_Freeleech", "AHD_Internal").
var flagPrefixes = []string{"g_", "ahd_", "ptp_"}

var magpieFlags = map[string]string{"freeleech": flagFreeleech, "internal": flagInternal}

// languageQualifier is a regional note in an *arr language name, e.g. "Portuguese (Brazil)".
var languageQualifier = regexp.MustCompile(`\s*\(.*\)\s*$`)

const (
	arrLanguageOriginal = "original"
	arrLanguageAny      = "any"
)

// regexTerm is a release-profile term written as a regex: /x26[45]/i.
var regexTerm = regexp.MustCompile(`^/(.+)/i?$`)

// PlannedFormat is a custom format to import, translated into Magpie's terms. Specs that need the user
// are left out of Specs and listed as pending reviews.
type PlannedFormat struct {
	Source              domain.SourceKey            `json:"source"`
	App                 App                         `json:"app"`
	Name                string                      `json:"name"`
	IncludeWhenRenaming bool                        `json:"includeWhenRenaming"`
	Specs               []acquisitionapi.FormatSpec `json:"specs"`
	Pending             []PendingSpec               `json:"pending"`
}

// PendingSpec is a spec waiting for its review; Spec holds what was translated (name, negate, required).
type PendingSpec struct {
	Review string                    `json:"review"`
	Spec   acquisitionapi.FormatSpec `json:"spec"`
}

// translator turns one app's custom formats into Magpie's.
type translator struct {
	lib       ArrLibrary
	languages map[int]string
	flags     map[int]string
	reviews   []SpecReview
	warnings  []string
}

func newTranslator(lib ArrLibrary) *translator {
	t := &translator{lib: lib, languages: map[int]string{}, flags: map[int]string{}}
	for _, l := range lib.Languages {
		t.languages[l.ID] = l.Name
	}
	for _, f := range lib.IndexerFlags {
		t.flags[f.ID] = f.Name
	}
	return t
}

func (t *translator) formats() []PlannedFormat {
	formats := make([]PlannedFormat, 0, len(t.lib.CustomFormats))
	for _, cf := range t.lib.CustomFormats {
		formats = append(formats, t.format(cf))
	}
	return formats
}

func (t *translator) format(cf ArrCustomFormat) PlannedFormat {
	f := PlannedFormat{
		Source: recordKey(t.lib.App, recordCustomFormat, cf.ID), App: t.lib.App, Name: strings.TrimSpace(cf.Name),
		IncludeWhenRenaming: cf.IncludeWhenRenaming, Specs: []acquisitionapi.FormatSpec{}, Pending: []PendingSpec{},
	}
	sources := len(slices.DeleteFunc(slices.Clone(cf.Specs), func(s ArrSpec) bool { return s.Implementation != ImplSource }))
	for i, spec := range cf.Specs {
		translated, reason := t.spec(spec, sources)
		switch {
		case reason == "":
			f.Specs = append(f.Specs, translated...)
		case len(translated) == 0 && strings.HasPrefix(reason, dropPrefix):
			t.warnings = append(t.warnings, fmt.Sprintf("%s: %s", f.Name, reason))
		default:
			review := SpecReview{
				ID: fmt.Sprintf("%s#%d", f.Source, i), Format: f.Source, FormatName: f.Name, App: t.lib.App,
				SpecName: spec.Name, Implementation: spec.Implementation, RawValue: spec.Value, Reason: reason,
				Kind: specKind(spec.Implementation), Options: specOptions[specKind(spec.Implementation)],
				Decision: SpecPending,
			}
			t.reviews = append(t.reviews, review)
			base := acquisitionapi.FormatSpec{Kind: review.Kind, Name: spec.Name, Negate: spec.Negate, Required: spec.Required}
			f.Pending = append(f.Pending, PendingSpec{Review: review.ID, Spec: base})
		}
	}
	return f
}

// dropPrefix starts the reason of a spec that is dropped with a warning rather than reviewed.
const dropPrefix = "dropped: "

var implKinds = map[string]acquisitionapi.FormatSpecKind{
	ImplReleaseTitle:    acquisitionapi.SpecReleaseTitle,
	ImplReleaseGroup:    acquisitionapi.SpecReleaseGroup,
	ImplEdition:         acquisitionapi.SpecEdition,
	ImplSize:            acquisitionapi.SpecSize,
	ImplResolution:      acquisitionapi.SpecResolution,
	ImplYear:            acquisitionapi.SpecYear,
	ImplReleaseType:     acquisitionapi.SpecReleaseType,
	ImplQualityModifier: acquisitionapi.SpecQualityModifier,
	ImplSource:          acquisitionapi.SpecSource,
	ImplLanguage:        acquisitionapi.SpecLanguage,
	ImplIndexerFlag:     acquisitionapi.SpecIndexerFlag,
}

func specKind(impl string) acquisitionapi.FormatSpecKind {
	return implKinds[impl]
}

// spec translates one condition. A non-empty reason means it needs review, or, with dropPrefix, that it was
// dropped. sources is how many Source conditions the format has.
func (t *translator) spec(s ArrSpec, sources int) ([]acquisitionapi.FormatSpec, string) {
	kind, known := implKinds[s.Implementation]
	if !known {
		return nil, fmt.Sprintf("Magpie has no %q condition", s.Implementation)
	}
	out := acquisitionapi.FormatSpec{Kind: kind, Name: s.Name, Negate: s.Negate, Required: s.Required, Value: s.Value}
	switch kind {
	case acquisitionapi.SpecSize, acquisitionapi.SpecYear:
		out.Value, out.Min, out.Max = "", s.Min, s.Max
	case acquisitionapi.SpecSource:
		return t.source(out, s, sources)
	case acquisitionapi.SpecQualityModifier:
		modifier, ok := enumValue(s.Value, radarrModifiers)
		if !ok {
			return nil, fmt.Sprintf("unknown quality modifier %s", s.Value)
		}
		out.Value = string(modifier)
	case acquisitionapi.SpecReleaseType:
		releaseType, ok := enumValue(s.Value, sonarrReleaseTypes)
		if !ok || releaseType == "" {
			return nil, fmt.Sprintf("release type %s has no Magpie counterpart", s.Value)
		}
		out.Value = releaseType
	case acquisitionapi.SpecLanguage:
		language, ok := t.language(s.Value)
		if !ok {
			return nil, fmt.Sprintf("language %q has no Magpie counterpart", t.languageName(s.Value))
		}
		out.Value = language
	case acquisitionapi.SpecIndexerFlag:
		flag, ok := t.flag(s.Value)
		if !ok {
			return nil, fmt.Sprintf("%sindexer flag %q has no Magpie counterpart", dropPrefix, t.flagName(s.Value))
		}
		out.Value = flag
	}
	return []acquisitionapi.FormatSpec{out}, ""
}

// source translates a Source condition by meaning. Sonarr's raw sources become a source and a modifier,
// which is exact only when no other Source condition shares the format, and never when negated.
func (t *translator) source(out acquisitionapi.FormatSpec, s ArrSpec, sources int) ([]acquisitionapi.FormatSpec, string) {
	id, err := strconv.Atoi(s.Value)
	translation, ok := appSources[t.lib.App][id]
	if err != nil || !ok {
		return nil, fmt.Sprintf("unknown source %s", s.Value)
	}
	out.Value = string(translation.source)
	if translation.modifier == "" {
		return []acquisitionapi.FormatSpec{out}, ""
	}
	if sources > 1 || s.Negate {
		return nil, fmt.Sprintf("%s splits into a source and a modifier, which is only exact as the format's one Source condition", s.Name)
	}
	modifier := acquisitionapi.FormatSpec{Kind: acquisitionapi.SpecQualityModifier, Name: s.Name, Required: s.Required, Value: string(translation.modifier)}
	return []acquisitionapi.FormatSpec{out, modifier}, ""
}

func enumValue[T any](raw string, values []T) (T, bool) {
	var zero T
	i, err := strconv.Atoi(raw)
	if err != nil || i < 0 || i >= len(values) {
		return zero, false
	}
	return values[i], true
}

func (t *translator) languageName(raw string) string {
	id, _ := strconv.Atoi(raw)
	if name, ok := t.languages[id]; ok {
		return name
	}
	return raw
}

func (t *translator) language(raw string) (string, bool) {
	return languageByName(t.languageName(raw))
}

// languageByName reads an *arr language name as Magpie's ISO 639-1 code, or ORIGINAL.
func languageByName(name string) (string, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == arrLanguageOriginal {
		return languageOrigin, true
	}
	code, ok := parse.LanguageCode(languageQualifier.ReplaceAllString(name, ""))
	return string(code), ok
}

func (t *translator) flagName(raw string) string {
	id, _ := strconv.Atoi(raw)
	if name, ok := t.flags[id]; ok {
		return name
	}
	return raw
}

func (t *translator) flag(raw string) (string, bool) {
	name := strings.ToLower(t.flagName(raw))
	for _, prefix := range flagPrefixes {
		name = strings.TrimPrefix(name, prefix)
	}
	flag, ok := magpieFlags[name]
	return flag, ok
}

// releaseProfileFormats turns each enabled release profile into formats: one matching any ignored term and
// one matching releases that lack a required term. Both score blockScore in every profile of the app.
func (t *translator) releaseProfileFormats() []PlannedFormat {
	var formats []PlannedFormat
	for _, rp := range t.lib.ReleaseProfiles {
		if !rp.Enabled {
			continue
		}
		key := recordKey(t.lib.App, recordReleaseProfile, rp.ID)
		name := cmp.Or(strings.TrimSpace(rp.Name), strconv.Itoa(rp.ID))
		if rp.IndexerID != 0 || len(rp.Tags) > 0 {
			t.warnings = append(t.warnings, fmt.Sprintf("release profile %s: its indexer and tag scope is lost; its formats apply everywhere", name))
		}
		if len(rp.Ignored) > 0 {
			f := PlannedFormat{Source: derivedKey(key, releaseProfileIgnored), App: t.lib.App, Name: fmt.Sprintf("Release profile %s: ignored terms", name), Pending: []PendingSpec{}}
			for _, term := range rp.Ignored {
				f.Specs = append(f.Specs, acquisitionapi.FormatSpec{Kind: acquisitionapi.SpecReleaseTitle, Name: term, Value: termRegex(term)})
			}
			formats = append(formats, f)
		}
		if len(rp.Required) > 0 {
			f := PlannedFormat{Source: derivedKey(key, releaseProfileRequired), App: t.lib.App, Name: fmt.Sprintf("Release profile %s: lacks required terms", name), Pending: []PendingSpec{}}
			for _, term := range rp.Required {
				f.Specs = append(f.Specs, acquisitionapi.FormatSpec{Kind: acquisitionapi.SpecReleaseTitle, Name: term, Value: termRegex(term), Negate: true, Required: true})
			}
			formats = append(formats, f)
		}
	}
	return formats
}

func termRegex(term string) string {
	if m := regexTerm.FindStringSubmatch(strings.TrimSpace(term)); m != nil {
		return m[1]
	}
	return regexp2.Escape(strings.TrimSpace(term))
}

// languageFormat is the "Language: not X" format a Radarr profile's language becomes; ok is false when the
// profile takes any language.
func languageFormat(profile ArrQualityProfile, source domain.SourceKey) (PlannedFormat, bool) {
	if profile.Language == nil || strings.EqualFold(profile.Language.Name, arrLanguageAny) {
		return PlannedFormat{}, false
	}
	code, ok := languageByName(profile.Language.Name)
	if !ok {
		return PlannedFormat{}, false
	}
	return PlannedFormat{
		Source: derivedKey(source, profileLanguage), App: AppRadarr, Name: "Language: not " + profile.Language.Name, Pending: []PendingSpec{},
		Specs: []acquisitionapi.FormatSpec{{Kind: acquisitionapi.SpecLanguage, Name: profile.Language.Name, Negate: true, Value: code}},
	}, true
}

// specLines describe a format's specs for the clash diff, in a stable order.
func specLines(specs []acquisitionapi.FormatSpec) []string {
	lines := make([]string, 0, len(specs))
	for _, s := range specs {
		lines = append(lines, specLine(s))
	}
	slices.Sort(lines)
	return lines
}

func specLine(s acquisitionapi.FormatSpec) string {
	value := s.Value
	if s.Kind == acquisitionapi.SpecSize || s.Kind == acquisitionapi.SpecYear {
		value = fmt.Sprintf("%g–%g", s.Min, s.Max)
	}
	line := fmt.Sprintf("%s %s", s.Kind, value)
	if s.Negate {
		line = "NOT " + line
	}
	if s.Required {
		line += " (required)"
	}
	return line
}
