package migrate

import (
	"fmt"
	"slices"
	"strings"

	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/domain"
	"magpie/internal/platform/parse"
)

// profiles translates an app's quality profiles: qualities remapped onto Magpie's IDs, the app's size table
// copied in, most preferred first, and format scores keyed by format source. Every profile also scores the
// app's release-profile formats, and a Radarr profile language becomes a format of its own.
func profiles(lib ArrLibrary, releaseFormats []PlannedFormat) ([]PlannedProfile, []PlannedFormat) {
	sizes := sizeTable(lib)
	var out []PlannedProfile
	var languageFormats []PlannedFormat
	for _, ap := range lib.QualityProfiles {
		source := recordKey(lib.App, recordQualityProfile, ap.ID)
		p := PlannedProfile{
			Source: source, App: lib.App, Name: strings.TrimSpace(ap.Name), UpgradeAllowed: ap.UpgradeAllowed,
			MinFormatScore: ap.MinFormatScore, CutoffFormatScore: ap.CutoffFormatScore, MinUpgradeFormatScore: ap.MinUpgradeFormatScore,
			Items: []acquisitionapi.ImportProfileItem{}, Scores: map[domain.SourceKey]int{},
		}
		cutoff := -1
		for i := len(ap.Items) - 1; i >= 0; i-- {
			item, ok := profileItem(lib.App, ap.Items[i], sizes)
			if !ok {
				continue
			}
			if isCutoff(ap.Items[i], ap.Cutoff) {
				cutoff = len(p.Items)
			}
			p.Items = append(p.Items, item)
		}
		if cutoff < 0 {
			cutoff = max(slices.IndexFunc(p.Items, func(i acquisitionapi.ImportProfileItem) bool { return i.Allowed }), 0)
		}
		p.Cutoff = cutoff
		for _, fi := range ap.FormatItems {
			if fi.Score != 0 {
				p.Scores[recordKey(lib.App, recordCustomFormat, fi.Format)] = fi.Score
			}
		}
		for _, f := range releaseFormats {
			p.Scores[f.Source] = blockScore
		}
		if f, ok := languageFormat(ap, source); ok {
			languageFormats = append(languageFormats, f)
			p.Scores[f.Source] = blockScore
		}
		out = append(out, p)
	}
	return out, languageFormats
}

func isCutoff(item ArrProfileItem, cutoff int) bool {
	if len(item.Items) > 0 {
		return item.GroupID == cutoff
	}
	return item.QualityID == cutoff
}

// profileItem translates one item; a group's size row spans its members'. ok is false when Magpie knows
// none of its qualities.
func profileItem(app App, item ArrProfileItem, table map[parse.QualityID]sizeLimits) (acquisitionapi.ImportProfileItem, bool) {
	leaves := item.Items
	if len(leaves) == 0 {
		leaves = []ArrProfileItem{item}
	}
	out := acquisitionapi.ImportProfileItem{Allowed: item.Allowed, Qualities: []domain.Quality{}}
	if len(item.Items) > 0 {
		out.Group = strings.TrimSpace(item.Name)
	}
	var limits *sizeLimits
	for _, leaf := range leaves {
		q, ok := magpieQuality(app, leaf.QualityID, 0)
		if !ok {
			continue
		}
		out.Qualities = append(out.Qualities, q)
		row := table[parse.QualityID(q.ID)]
		if leaf.MaxSize != nil || leaf.MinSize != nil {
			row = limitsOf(deref(leaf.MinSize), leaf.MaxSize, leaf.PreferredSize)
		}
		if limits == nil {
			limits = &row
			continue
		}
		limits.Min = min(limits.Min, row.Min)
		limits.Max = max(limits.Max, row.Max)
		limits.Preferred = max(limits.Preferred, row.Preferred)
	}
	if len(out.Qualities) == 0 {
		return out, false
	}
	if limits == nil {
		limits = &sizeLimits{Max: unlimitedMBPerMin, Preferred: unlimitedMBPerMin}
	}
	if limits.Max == 0 {
		limits.Max, limits.Preferred = unlimitedMBPerMin, unlimitedMBPerMin
	}
	out.Sizes = acquisitionapi.SizeLimits{MinMBPerMin: limits.Min, PreferredMBPerMin: limits.Preferred, MaxMBPerMin: limits.Max}
	return out, true
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

// profileLines describe a profile for the clash diff: its settings, allowed qualities in order, and the
// scores it gives by format name. Qualities a profile doesn't allow don't count, so Radarr's movie-only
// qualities don't make every shared profile clash.
func profileLines(p PlannedProfile, formatNames map[domain.SourceKey]string) []string {
	lines := []string{
		fmt.Sprintf("Upgrades allowed: %t", p.UpgradeAllowed),
		fmt.Sprintf("Minimum score %d, upgrade until %d, increment %d", p.MinFormatScore, p.CutoffFormatScore, p.MinUpgradeFormatScore),
	}
	for i, item := range p.Items {
		if !item.Allowed {
			continue
		}
		names := make([]string, 0, len(item.Qualities))
		for _, q := range item.Qualities {
			names = append(names, q.Name)
		}
		line := strings.Join(names, ", ")
		if item.Group != "" {
			line = fmt.Sprintf("%s (%s)", item.Group, line)
		}
		if i == p.Cutoff {
			line += " ← cutoff"
		}
		lines = append(lines, line)
	}
	var scores []string
	for source, score := range p.Scores {
		scores = append(scores, fmt.Sprintf("%s: %d", formatNames[source], score))
	}
	slices.Sort(scores)
	return append(lines, scores...)
}
