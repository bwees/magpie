package catalog

import (
	"slices"
	"time"

	"magpie/internal/catalog/api"
)

// theatricalHoldback is how long after the theatrical date a movie with no digital or physical date
// counts as released.
const (
	theatricalHoldback = 90 * 24 * time.Hour
	anyRegion          = ""
)

var (
	homeReleaseTypes = []TmdbReleaseType{TmdbReleaseDigital, TmdbReleasePhysical}
	theatricalTypes  = []TmdbReleaseType{TmdbReleaseTheatricalLimited, TmdbReleaseTheatrical}
	digitalTypes     = []TmdbReleaseType{TmdbReleaseDigital}
	physicalTypes    = []TmdbReleaseType{TmdbReleasePhysical}
)

// resolveReleaseDates picks the earliest date per kind. Digital and physical come from the region when it
// has either of them, else from any region, so they follow the released rule. Theatrical comes from the
// region when it has one, else from any region.
func resolveReleaseDates(dates []TmdbReleaseDate, region string) api.ReleaseDates {
	homeRegion := anyRegion
	if earliest(dates, region, homeReleaseTypes) != nil {
		homeRegion = region
	}
	theatricalRegion := anyRegion
	if earliest(dates, region, theatricalTypes) != nil {
		theatricalRegion = region
	}
	return api.ReleaseDates{
		Theatrical: earliest(dates, theatricalRegion, theatricalTypes),
		Digital:    earliest(dates, homeRegion, digitalTypes),
		Physical:   earliest(dates, homeRegion, physicalTypes),
	}
}

// releasePoint is when a movie counts as released: its earliest digital or physical date, else its
// theatrical date plus the holdback. Nil means never.
func releasePoint(d api.ReleaseDates) *time.Time {
	if home := earliestOf(d.Digital, d.Physical); home != nil {
		return home
	}
	if d.Theatrical != nil {
		point := d.Theatrical.Add(theatricalHoldback)
		return &point
	}
	return nil
}

// releasedBy reports whether the release point has been reached at now.
func releasedBy(d api.ReleaseDates, now time.Time) bool {
	point := releasePoint(d)
	return point != nil && !now.Before(*point)
}

func earliest(dates []TmdbReleaseDate, region string, types []TmdbReleaseType) *time.Time {
	var found *time.Time
	for _, d := range dates {
		if (region != anyRegion && d.Region != region) || !slices.Contains(types, d.Type) {
			continue
		}
		if found == nil || d.Date.Before(*found) {
			found = ptr(d.Date.UTC())
		}
	}
	return found
}

func earliestOf(times ...*time.Time) *time.Time {
	var found *time.Time
	for _, t := range times {
		if t != nil && (found == nil || t.Before(*found)) {
			found = t
		}
	}
	return found
}

func ptr[T any](v T) *T {
	return &v
}
