package subtitles

import (
	"cmp"
	"regexp"
	"slices"
	"strings"

	"magpie/internal/domain"
	"magpie/internal/platform/parse"
)

const (
	percent = 100
	// upgradeMargin: a provider subtitle scoring below max − upgradeMargin is still re-searched for a better one.
	upgradeMargin = 3
	// extraPoint is the weight of each minor attribute: audio and video codec, resolution, HI and streaming service.
	extraPoint = 1
	imdbPrefix = "tt"
)

// weights are Bazarr's score weights. Hash is always worth the sum of the rest minus one.
type weights struct {
	hash, title, year, season, episode, source, edition, releaseGroup int
}

var (
	episodeWeights = weights{hash: 359, title: 160, year: 90, season: 30, episode: 30, source: 25, releaseGroup: 20}
	movieWeights   = weights{hash: 179, title: 60, year: 40, source: 30, edition: 30, releaseGroup: 15}
)

func weightsFor(kind domain.MediaKind) weights {
	if kind == domain.MediaKindSeries {
		return episodeWeights
	}
	return movieWeights
}

// extraAttributes are the 1-point attributes: audio codec, video codec, resolution, HI, streaming service.
const extraAttributes = 5

func (w weights) max() int {
	return w.title + w.year + w.season + w.episode + w.source + w.edition + w.releaseGroup + extraAttributes*extraPoint
}

// Score is a candidate's points out of the most its kind can reach.
type Score struct {
	Points int
	Max    int
}

func (s Score) Percent() int {
	if s.Max == 0 {
		return 0
	}
	return s.Points * percent / s.Max
}

// Meets reports whether the score reaches minPercent of the maximum.
func (s Score) Meets(minPercent int) bool {
	return s.Points*percent >= minPercent*s.Max
}

// Upgradable reports whether a better subtitle is still worth looking for.
func (s Score) Upgradable() bool {
	return s.Points < s.Max-upgradeMargin
}

// video is what scoring knows about the file a subtitle is for. Season and Episode are TMDB aired numbers.
type video struct {
	Kind    domain.MediaKind
	Tmdb    domain.TmdbID
	Imdb    string
	Year    int
	Season  int
	Episode int
	Release releaseTraits
}

// releaseTraits are the release attributes scoring compares, read from a release name.
type releaseTraits struct {
	Source       string
	Resolution   int
	Edition      string
	ReleaseGroup string
	VideoCodec   string
	AudioCodec   string
	Streaming    string
}

type namedPattern struct {
	name    string
	pattern *regexp.Regexp
}

// First match wins, so more specific patterns come first.
var (
	videoCodecs = []namedPattern{
		{"H265", regexp.MustCompile(`(?i)\b(?:[xh]\.?265|hevc)\b`)},
		{"H264", regexp.MustCompile(`(?i)\b(?:[xh]\.?264|avc)\b`)},
		{"AV1", regexp.MustCompile(`(?i)\bav1\b`)},
	}
	audioCodecs = []namedPattern{
		{"TRUEHD", regexp.MustCompile(`(?i)\btrue-?hd\b`)},
		{"EAC3", regexp.MustCompile(`(?i)\b(?:ddp|dd\+|e-?ac-?3)`)},
		{"DTS", regexp.MustCompile(`(?i)\bdts`)},
		{"AC3", regexp.MustCompile(`(?i)\b(?:dd|ac-?3)(?:[ .]?[1-7]\.[01])?\b`)},
		{"AAC", regexp.MustCompile(`(?i)\baac`)},
		{"FLAC", regexp.MustCompile(`(?i)\bflac\b`)},
		{"OPUS", regexp.MustCompile(`(?i)\bopus\b`)},
	}
	streamingServices = []namedPattern{
		{"AMZN", regexp.MustCompile(`(?i)\bamzn\b`)},
		{"NF", regexp.MustCompile(`\bNF\b`)},
		{"DSNP", regexp.MustCompile(`(?i)\bdsnp\b`)},
		{"HMAX", regexp.MustCompile(`(?i)\bhmax\b`)},
		{"ATVP", regexp.MustCompile(`(?i)\batvp\b`)},
		{"HULU", regexp.MustCompile(`(?i)\bhulu\b`)},
		{"PCOK", regexp.MustCompile(`(?i)\bpcok\b`)},
		{"MAX", regexp.MustCompile(`\bMAX\b`)},
	}
)

func firstMatch(patterns []namedPattern, name string) string {
	for _, p := range patterns {
		if p.pattern.MatchString(name) {
			return p.name
		}
	}
	return ""
}

// traitsOf parses a release name. A name the parser can't read still yields its codecs and service.
func traitsOf(name string, kind domain.MediaKind) releaseTraits {
	traits := releaseTraits{
		VideoCodec: firstMatch(videoCodecs, name),
		AudioCodec: firstMatch(audioCodecs, name),
		Streaming:  firstMatch(streamingServices, name),
	}
	release, err := parse.ParseRelease(name, kind)
	if err != nil {
		return traits
	}
	if release.Quality.Source != string(parse.SourceUnknown) {
		traits.Source = release.Quality.Source
	}
	traits.Resolution = release.Quality.Resolution
	traits.Edition = strings.ToLower(release.Edition)
	traits.ReleaseGroup = strings.ToLower(release.Group)
	return traits
}

// sameKnown reports whether a and b are equal and known.
func sameKnown[T comparable](a, b T) bool {
	var zero T
	return a != zero && a == b
}

func normaliseImdb(id string) string {
	return strings.TrimLeft(strings.TrimPrefix(strings.ToLower(id), imdbPrefix), "0")
}

// score rates a candidate Bazarr-style. A hash counts only when the identity fields also match: the
// movie, or the series, season and episode. Then the hash stands in for every other attribute.
func score(v video, c Candidate, hearingImpairedOK bool) Score {
	w := weightsFor(v.Kind)
	result := Score{Max: w.max()}

	sameTitle := sameKnown(c.Tmdb, v.Tmdb) || sameKnown(normaliseImdb(c.Imdb), normaliseImdb(v.Imdb))
	sameSeason := sameKnown(c.Season, v.Season)
	sameEpisode := sameKnown(c.Episode, v.Episode)
	identity := sameTitle
	if v.Kind == domain.MediaKindSeries {
		identity = sameTitle && sameSeason && sameEpisode
	}
	hiPoint := 0
	if !c.HearingImpaired || hearingImpairedOK {
		hiPoint = extraPoint
	}
	if c.HashMatch && identity {
		result.Points = w.hash + hiPoint
		return result
	}

	theirs := traitsOf(c.ReleaseName, v.Kind)
	ours := v.Release
	for _, match := range []struct {
		matched bool
		weight  int
	}{
		{sameTitle, w.title},
		// A matching TMDB or IMDb ID implies the year, as in Bazarr.
		{sameTitle || sameKnown(c.Year, v.Year), w.year},
		{sameSeason, w.season},
		{sameEpisode, w.episode},
		{sameKnown(theirs.Source, ours.Source), w.source},
		{sameKnown(theirs.Edition, ours.Edition), w.edition},
		{sameKnown(theirs.ReleaseGroup, ours.ReleaseGroup), w.releaseGroup},
		{sameKnown(theirs.AudioCodec, ours.AudioCodec), extraPoint},
		{sameKnown(theirs.VideoCodec, ours.VideoCodec), extraPoint},
		{sameKnown(theirs.Resolution, ours.Resolution), extraPoint},
		{sameKnown(theirs.Streaming, ours.Streaming), extraPoint},
	} {
		if match.matched {
			result.Points += match.weight
		}
	}
	result.Points += hiPoint
	return result
}

// scored is a candidate with its score.
type scored struct {
	Candidate
	Score Score
}

// wantedTrack is one wanted (language, forced) and whether hearing-impaired subtitles are fine for it.
type wantedTrack struct {
	Language          domain.Language
	Forced            bool
	HearingImpairedOK bool
}

// rank scores the usable candidates for a wanted track, best first. Machine-translated and blocklisted
// candidates and those of the wrong forced kind are dropped. Ties go to non-HI unless HI is fine.
func rank(v video, want wantedTrack, cands []Candidate, blocked map[int64]bool) []scored {
	out := make([]scored, 0, len(cands))
	for _, c := range cands {
		if c.MachineTranslated || blocked[c.FileID] || c.Forced != want.Forced || c.Language != want.Language {
			continue
		}
		out = append(out, scored{Candidate: c, Score: score(v, c, want.HearingImpairedOK)})
	}
	slices.SortStableFunc(out, func(a, b scored) int {
		if by := cmp.Compare(b.Score.Points, a.Score.Points); by != 0 || want.HearingImpairedOK {
			return by
		}
		return compareBool(a.HearingImpaired, b.HearingImpaired)
	})
	return out
}

// compareBool orders false before true.
func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case b:
		return -1
	default:
		return 1
	}
}
