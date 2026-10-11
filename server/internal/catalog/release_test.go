package catalog

import (
	"testing"
	"time"

	"magpie/internal/catalog/api"
)

const (
	regionUS = "US"
	regionGB = "GB"
)

var ruleNow = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

func daysAgo(days int) time.Time {
	return ruleNow.AddDate(0, 0, -days)
}

func released(dates []TmdbReleaseDate, region string, now time.Time) bool {
	point := releasePoint(resolveReleaseDates(dates, region))
	return point != nil && !now.Before(*point)
}

func TestReleasedRule(t *testing.T) {
	cases := []struct {
		name  string
		dates []TmdbReleaseDate
		want  bool
	}{
		{"digital in the region yesterday", []TmdbReleaseDate{{regionUS, TmdbReleaseDigital, daysAgo(1)}}, true},
		{"physical in the region yesterday", []TmdbReleaseDate{{regionUS, TmdbReleasePhysical, daysAgo(1)}}, true},
		{"digital in the region tomorrow", []TmdbReleaseDate{{regionUS, TmdbReleaseDigital, daysAgo(-1)}}, false},
		{
			"the region's future digital date wins over another region's past one",
			[]TmdbReleaseDate{{regionUS, TmdbReleaseDigital, daysAgo(-10)}, {regionGB, TmdbReleaseDigital, daysAgo(5)}},
			false,
		},
		{"only another region has a digital date", []TmdbReleaseDate{{regionGB, TmdbReleaseDigital, daysAgo(1)}}, true},
		{"theatrical 89 days ago", []TmdbReleaseDate{{regionUS, TmdbReleaseTheatrical, daysAgo(89)}}, false},
		{"theatrical 90 days ago", []TmdbReleaseDate{{regionUS, TmdbReleaseTheatrical, daysAgo(90)}}, true},
		{"limited theatrical counts as theatrical", []TmdbReleaseDate{{regionGB, TmdbReleaseTheatricalLimited, daysAgo(120)}}, true},
		{
			"a future digital date overrides an old theatrical date",
			[]TmdbReleaseDate{{regionUS, TmdbReleaseTheatrical, daysAgo(200)}, {regionUS, TmdbReleaseDigital, daysAgo(-3)}},
			false,
		},
		{"a premiere alone is not a release", []TmdbReleaseDate{{regionUS, TmdbReleasePremiere, daysAgo(400)}}, false},
		{"no dates at all", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := released(tc.dates, regionUS, ruleNow); got != tc.want {
				t.Errorf("released = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestResolveReleaseDatesPrefersTheRegionPerKind(t *testing.T) {
	dates := []TmdbReleaseDate{
		{regionGB, TmdbReleaseTheatrical, daysAgo(40)},
		{regionUS, TmdbReleaseTheatrical, daysAgo(30)},
		{regionUS, TmdbReleaseTheatrical, daysAgo(20)},
		{regionGB, TmdbReleaseDigital, daysAgo(10)},
		{regionGB, TmdbReleasePhysical, daysAgo(5)},
	}
	got := resolveReleaseDates(dates, regionUS)
	want := api.ReleaseDates{Theatrical: ptr(daysAgo(30)), Digital: ptr(daysAgo(10)), Physical: ptr(daysAgo(5))}
	if !sameTime(got.Theatrical, want.Theatrical) || !sameTime(got.Digital, want.Digital) || !sameTime(got.Physical, want.Physical) {
		t.Errorf("dates = %+v, want %+v", got, want)
	}
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}
