package acquisition

import (
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"time"

	"magpie/internal/acquisition/api"
	"magpie/internal/domain"
	"magpie/internal/platform/parse"

	"github.com/dlclark/regexp2/v2"
)

// FormatMatchTimeout bounds one custom-format regex match. A timeout counts as no match.
const FormatMatchTimeout = 100 * time.Millisecond

const bytesPerGB = 1 << 30

// Sonarr splits Bluray and TV into encodes and raw sources (Remux, Raw-HD). These two values keep that
// split for its formats: a Remux release has both BLURAY and BLURAY_RAW.
const (
	SourceBlurayRaw parse.QualitySource = "BLURAY_RAW"
	SourceTVRaw     parse.QualitySource = "TV_RAW"
)

// LanguageOriginal in a LANGUAGE spec matches the title's original language.
const LanguageOriginal = "ORIGINAL"

type IndexerFlag string

const (
	FlagFreeleech IndexerFlag = "FREELEECH"
	FlagInternal  IndexerFlag = "INTERNAL"
)

type ReleaseType string

const (
	ReleaseTypeSingleEpisode ReleaseType = "SINGLE_EPISODE"
	ReleaseTypeMultiEpisode  ReleaseType = "MULTI_EPISODE"
	ReleaseTypeSeasonPack    ReleaseType = "SEASON_PACK"
)

var ErrInvalidFormat = errors.New("acquisition: invalid custom format")

// CustomFormat is a stored custom format. TrashScores holds TRaSH's score per score set for a format
// linked to TRaSH by TrashID.
type CustomFormat struct {
	ID                  api.CustomFormatID
	Name                string
	TrashID             string
	TrashScores         map[string]int
	IncludeWhenRenaming bool
	Specs               []api.FormatSpec
}

// ReleaseFacts is what a custom format can look at.
type ReleaseFacts struct {
	Release      parse.Release
	SizeBytes    int64
	IndexerFlags []IndexerFlag
	// OriginalLanguage is the title's original language. A release that names no language is taken
	// to be in it, as Sonarr and Radarr do.
	OriginalLanguage domain.Language
}

// CompiledFormat is a custom format with its regexes compiled.
type CompiledFormat struct {
	Format CustomFormat
	specs  []compiledSpec
}

type compiledSpec struct {
	api.FormatSpec
	re *regexp2.Regexp
}

var (
	formatSources = []parse.QualitySource{
		parse.SourceUnknown, parse.SourceCam, parse.SourceTelesync, parse.SourceTelecine, parse.SourceWorkprint,
		parse.SourceDVD, parse.SourceTV, parse.SourceWebDL, parse.SourceWebRip, parse.SourceBluray,
		SourceBlurayRaw, SourceTVRaw,
	}
	formatResolutions = []parse.Resolution{
		parse.Resolution360p, parse.Resolution480p, parse.Resolution540p, parse.Resolution576p,
		parse.Resolution720p, parse.Resolution1080p, parse.Resolution2160p,
	}
	formatModifiers = []parse.QualityModifier{
		parse.ModifierNone, parse.ModifierRegional, parse.ModifierScreener, parse.ModifierRawHD, parse.ModifierBRDisk, parse.ModifierRemux,
	}
	formatFlags        = []IndexerFlag{FlagFreeleech, FlagInternal}
	formatReleaseTypes = []ReleaseType{ReleaseTypeSingleEpisode, ReleaseTypeMultiEpisode, ReleaseTypeSeasonPack}
)

// CompileFormat checks every spec and compiles its regexes with regexp2, the .NET-compatible engine
// Sonarr, Radarr and TRaSH write them for.
func CompileFormat(f CustomFormat) (CompiledFormat, error) {
	compiled := CompiledFormat{Format: f, specs: make([]compiledSpec, 0, len(f.Specs))}
	for _, spec := range f.Specs {
		c, err := compileSpec(spec)
		if err != nil {
			return CompiledFormat{}, fmt.Errorf("%w: %s: spec %q: %w", ErrInvalidFormat, f.Name, spec.Name, err)
		}
		compiled.specs = append(compiled.specs, c)
	}
	return compiled, nil
}

func compileSpec(spec api.FormatSpec) (compiledSpec, error) {
	c := compiledSpec{FormatSpec: spec}
	switch spec.Kind {
	case api.SpecReleaseTitle, api.SpecReleaseGroup, api.SpecEdition:
		re, err := regexp2.Compile(spec.Value, regexp2.IgnoreCase)
		if err != nil {
			return c, err
		}
		re.MatchTimeout = FormatMatchTimeout
		c.re = re
	case api.SpecSource:
		return c, oneOf(parse.QualitySource(spec.Value), formatSources)
	case api.SpecResolution:
		lines, err := strconv.Atoi(spec.Value)
		if err != nil {
			return c, err
		}
		return c, oneOf(parse.Resolution(lines), formatResolutions)
	case api.SpecQualityModifier:
		return c, oneOf(parse.QualityModifier(spec.Value), formatModifiers)
	case api.SpecIndexerFlag:
		return c, oneOf(IndexerFlag(spec.Value), formatFlags)
	case api.SpecReleaseType:
		return c, oneOf(ReleaseType(spec.Value), formatReleaseTypes)
	case api.SpecLanguage:
		if spec.Value == "" {
			return c, errors.New("language is empty")
		}
	case api.SpecSize, api.SpecYear:
		if spec.Min < 0 || spec.Min > spec.Max {
			return c, fmt.Errorf("range %v-%v is not 0 ≤ min ≤ max", spec.Min, spec.Max)
		}
	default:
		return c, fmt.Errorf("unknown kind %q", spec.Kind)
	}
	return c, nil
}

func oneOf[T comparable](v T, known []T) error {
	if !slices.Contains(known, v) {
		return fmt.Errorf("unknown value %v", v)
	}
	return nil
}

// Matches applies Sonarr's rule: specs of one kind are OR'd unless one is required, which then must
// match; kinds are AND'd. A format with no specs never matches.
func (f CompiledFormat) Matches(r ReleaseFacts) bool {
	if len(f.specs) == 0 {
		return false
	}
	type kindResult struct{ anyMatched, requiredFailed bool }
	results := map[api.FormatSpecKind]kindResult{}
	for _, spec := range f.specs {
		matched := spec.matches(f.Format.Name, r) != spec.Negate
		result := results[spec.Kind]
		result.anyMatched = result.anyMatched || matched
		result.requiredFailed = result.requiredFailed || spec.Required && !matched
		results[spec.Kind] = result
	}
	for _, result := range results {
		if result.requiredFailed || !result.anyMatched {
			return false
		}
	}
	return true
}

func (s compiledSpec) matches(format string, r ReleaseFacts) bool {
	release := r.Release
	switch s.Kind {
	case api.SpecReleaseTitle:
		return s.matchRegex(format, release.Name)
	case api.SpecReleaseGroup:
		return s.matchRegex(format, release.Group)
	case api.SpecEdition:
		return s.matchRegex(format, release.Edition)
	case api.SpecSource:
		return slices.Contains(releaseSources(release.Quality), parse.QualitySource(s.Value))
	case api.SpecResolution:
		return strconv.Itoa(release.Quality.Resolution) == s.Value
	case api.SpecQualityModifier:
		return qualityModifier(release.Quality) == parse.QualityModifier(s.Value)
	case api.SpecIndexerFlag:
		return slices.Contains(r.IndexerFlags, IndexerFlag(s.Value))
	case api.SpecReleaseType:
		return releaseType(release) == ReleaseType(s.Value)
	case api.SpecLanguage:
		return slices.Contains(releaseLanguages(r), s.language(r))
	case api.SpecSize:
		gb := float64(r.SizeBytes) / bytesPerGB
		return gb > s.Min && gb <= s.Max
	case api.SpecYear:
		year := float64(release.Year)
		return year >= s.Min && year <= s.Max
	}
	return false
}

func (s compiledSpec) matchRegex(format, text string) bool {
	matched, err := s.re.MatchString(text)
	if err != nil {
		slog.Warn("custom format regex failed; counted as no match", "format", format, "spec", s.Name, "text", text, "error", err)
		return false
	}
	return matched
}

func (s compiledSpec) language(r ReleaseFacts) domain.Language {
	if s.Value == LanguageOriginal {
		return r.OriginalLanguage
	}
	return domain.Language(s.Value)
}

func releaseLanguages(r ReleaseFacts) []domain.Language {
	if len(r.Release.Languages) == 0 && r.OriginalLanguage != "" {
		return []domain.Language{r.OriginalLanguage}
	}
	return r.Release.Languages
}

func releaseSources(q domain.Quality) []parse.QualitySource {
	sources := []parse.QualitySource{parse.QualitySource(q.Source)}
	switch qualityModifier(q) {
	case parse.ModifierRemux:
		sources = append(sources, SourceBlurayRaw)
	case parse.ModifierRawHD:
		sources = append(sources, SourceTVRaw)
	}
	return sources
}

func qualityModifier(q domain.Quality) parse.QualityModifier {
	d, ok := parse.QualityByID(parse.QualityID(q.ID))
	if !ok {
		return parse.ModifierNone
	}
	return d.Modifier
}

func releaseType(r parse.Release) ReleaseType {
	switch {
	case r.Kind != domain.MediaKindSeries:
		return ""
	case r.FullSeason:
		return ReleaseTypeSeasonPack
	case len(r.Episodes) > 1:
		return ReleaseTypeMultiEpisode
	case len(r.Episodes) == 1:
		return ReleaseTypeSingleEpisode
	}
	return ""
}
