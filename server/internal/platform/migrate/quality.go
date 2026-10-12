package migrate

import (
	"magpie/internal/domain"
	"magpie/internal/platform/parse"
)

// Sonarr quality IDs that mean something else in Radarr, whose IDs Magpie uses.
const (
	sonarrBluray480p       = 13
	sonarrBluray576p       = 22
	sonarrBluray1080pRemux = 20
	sonarrBluray2160pRemux = 21
)

// sonarrQualities remaps those IDs; every other Sonarr ID is the same quality in Radarr.
var sonarrQualities = map[int]parse.QualityID{
	sonarrBluray480p:       parse.QualityBluray480p,
	sonarrBluray576p:       parse.QualityBluray576p,
	sonarrBluray1080pRemux: parse.QualityRemux1080p,
	sonarrBluray2160pRemux: parse.QualityRemux2160p,
}

// unlimitedMBPerMin is Radarr's "no maximum", which TRaSH and Magpie's presets also use.
const unlimitedMBPerMin = 2000

// magpieQualityID maps an app's quality ID onto Magpie's.
func magpieQualityID(app App, id int) parse.QualityID {
	if app == AppSonarr {
		if mapped, ok := sonarrQualities[id]; ok {
			return mapped
		}
	}
	return parse.QualityID(id)
}

// magpieQuality is the Magpie quality for an app's quality ID, with the revision; ok is false for an ID
// Magpie doesn't know.
func magpieQuality(app App, id, revision int) (domain.Quality, bool) {
	def, ok := parse.QualityByID(magpieQualityID(app, id))
	if !ok {
		return domain.Quality{}, false
	}
	return def.Quality(revision), true
}

// sizeLimits is one row of a size table, already in Magpie's 0 ≤ min ≤ preferred ≤ max form.
type sizeLimits struct {
	Min, Preferred, Max float64
}

// limitsOf reads an app's size row: no maximum means unlimited, no preferred size means the maximum.
func limitsOf(minSize float64, maxSize, preferred *float64) sizeLimits {
	limits := sizeLimits{Min: max(minSize, 0), Max: unlimitedMBPerMin}
	if maxSize != nil && *maxSize > 0 {
		limits.Max = *maxSize
	}
	limits.Preferred = limits.Max
	if preferred != nil && *preferred > 0 {
		limits.Preferred = *preferred
	}
	limits.Max = max(limits.Max, limits.Min)
	limits.Preferred = min(max(limits.Preferred, limits.Min), limits.Max)
	return limits
}

// sizeTable is an app's global size table by Magpie quality ID.
func sizeTable(lib ArrLibrary) map[parse.QualityID]sizeLimits {
	table := map[parse.QualityID]sizeLimits{}
	for _, def := range lib.QualityDefinitions {
		table[magpieQualityID(lib.App, def.QualityID)] = limitsOf(def.MinSize, def.MaxSize, def.PreferredSize)
	}
	return table
}
