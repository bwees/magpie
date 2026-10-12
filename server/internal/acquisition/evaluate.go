package acquisition

import (
	"slices"

	"magpie/internal/acquisition/api"
	"magpie/internal/domain"
)

// unrankedQuality is the rank of a quality the profile doesn't list.
const unrankedQuality = 0

// ScoringProfile is a quality profile with the custom formats to evaluate against it.
type ScoringProfile struct {
	profile api.Profile
	formats []CompiledFormat
	scores  map[api.CustomFormatID]int
}

func NewScoringProfile(p api.Profile, formats []CompiledFormat) ScoringProfile {
	scores := make(map[api.CustomFormatID]int, len(p.FormatScores))
	for _, s := range p.FormatScores {
		scores[s.Format] = s.Score
	}
	return ScoringProfile{profile: p, formats: formats, scores: scores}
}

func (p ScoringProfile) Profile() api.Profile {
	return p.profile
}

// MaxAttainableScore sums the profile's positive format scores. No release can score more, so a release
// at the cutoff with this score can be grabbed without waiting for others.
func (p ScoringProfile) MaxAttainableScore() int {
	total := 0
	for _, s := range p.profile.FormatScores {
		total += max(s.Score, 0)
	}
	return total
}

// Evaluation is what the quality model says about one release under one profile.
type Evaluation struct {
	Quality domain.Quality
	// QualityRank is higher for more preferred items; members of a group share one rank.
	// It is 0 when the profile doesn't list the quality.
	QualityRank int
	// Formats are the names of every matching custom format, sorted.
	Formats []string
	// Score sums the profile's scores of the matching formats.
	Score int
	// Allowed reports whether the profile allows the quality. The minimum format score is a
	// separate decision.
	Allowed bool
	// CutoffMet is quality at or above the cutoff and Score at or above upgrade-until.
	CutoffMet bool
}

// Evaluate is pure: it reads only the release and the profile.
func Evaluate(r ReleaseFacts, p ScoringProfile) Evaluation {
	q := r.Release.Quality
	ev := Evaluation{Quality: q, Formats: []string{}}

	if index, ok := itemIndex(p.profile.Items, q.ID); ok {
		ev.QualityRank = itemRank(p.profile.Items, index)
		ev.Allowed = p.profile.Items[index].Allowed
	}
	for _, f := range p.formats {
		if f.Matches(r) {
			ev.Formats = append(ev.Formats, f.Format.Name)
			ev.Score += p.scores[f.Format.ID]
		}
	}
	slices.Sort(ev.Formats)

	ev.CutoffMet = ev.QualityRank != unrankedQuality &&
		ev.QualityRank >= itemRank(p.profile.Items, p.profile.Cutoff) &&
		ev.Score >= p.profile.CutoffFormatScore
	return ev
}

func itemIndex(items []api.ProfileItem, qualityID int) (int, bool) {
	for i, item := range items {
		if slices.ContainsFunc(item.Qualities, func(q domain.Quality) bool { return q.ID == qualityID }) {
			return i, true
		}
	}
	return 0, false
}

// itemRank turns "most preferred first" into "higher is better".
func itemRank(items []api.ProfileItem, index int) int {
	return len(items) - index
}
