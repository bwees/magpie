package trash

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"magpie/internal/acquisition"
	"magpie/internal/acquisition/api"
	"magpie/internal/platform/parse"
)

const (
	contentsPrefix = "/contents"
	rawPrefix      = "/raw"
	fixtureDir     = "testdata"
	jsonSuffix     = ".json"
)

// recorded serves TRaSH's GitHub directory listings and raw files captured in testdata.
func recorded(t *testing.T) *Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(contentsPrefix+"/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get(refParam) != guideBranch {
			http.Error(w, "missing ref", http.StatusBadRequest)
			return
		}
		serveFixture(t, w, r, filepath.Join(fixtureDir, r.URL.Path)+jsonSuffix)
	})
	mux.HandleFunc(rawPrefix+"/", func(w http.ResponseWriter, r *http.Request) {
		serveFixture(t, w, r, filepath.Join(fixtureDir, r.URL.Path))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return NewClient(server.URL+contentsPrefix, server.URL+rawPrefix, server.Client())
}

func serveFixture(t *testing.T, w http.ResponseWriter, r *http.Request, path string) {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		t.Errorf("fixture %s: %v", path, err)
		return
	}
	_, _ = w.Write(body)
}

func byName[T any](items []T, name func(T) string) map[string]T {
	out := map[string]T{}
	for _, item := range items {
		out[name(item)] = item
	}
	return out
}

func TestCustomFormatsMapRadarrSpecs(t *testing.T) {
	formats, err := recorded(t).CustomFormats(context.Background(), acquisition.TrashRadarr)
	if err != nil {
		t.Fatal(err)
	}
	got := byName(formats, func(f acquisition.TrashCustomFormat) string { return f.Name })
	if len(got) != 6 {
		t.Fatalf("formats = %v, want the 6 supported ones (README, drafts dir and the exceptLanguage format skipped)", slices.Collect(maps.Keys(got)))
	}

	web := got["WEB Tier 01"]
	if web.TrashID != "c20f169ef63c5f40c2def54abaf4438e" || web.Scores["default"] != 1700 || web.Scores["sqp-4-ma-hybrid"] != 30 {
		t.Errorf("WEB Tier 01 = %+v", web)
	}
	if web.Specs[0] != (api.FormatSpec{Kind: api.SpecReleaseGroup, Name: "ABBIE", Value: "^(ABBIE)$"}) {
		t.Errorf("first spec = %+v", web.Specs[0])
	}

	cases := map[string]api.FormatSpec{
		"1080p":                  {Kind: api.SpecResolution, Name: "1080p", Value: "1080", Required: true},
		"FreeLeech":              {Kind: api.SpecIndexerFlag, Name: "FreeLeech", Value: string(acquisition.FlagFreeleech)},
		"Language: Not Original": {Kind: api.SpecLanguage, Name: "Not Original Language", Value: acquisition.LanguageOriginal, Negate: true},
	}
	for name, want := range cases {
		if specs := got[name].Specs; len(specs) != 1 || specs[0] != want {
			t.Errorf("%s specs = %+v, want %+v", name, specs, want)
		}
	}
	remux := got["Remux Tier 01"].Specs
	if !slices.Contains(remux, api.FormatSpec{Kind: api.SpecQualityModifier, Name: "Remux", Value: string(parse.ModifierRemux), Required: true}) {
		t.Errorf("remux tier specs lack the REMUX modifier: %+v", remux)
	}
	for _, f := range formats {
		if _, err := acquisition.CompileFormat(acquisition.CustomFormat{Name: f.Name, Specs: f.Specs}); err != nil {
			t.Errorf("%s doesn't compile: %v", f.Name, err)
		}
	}
}

func TestCustomFormatsMapSonarrSourcesAndReleaseTypes(t *testing.T) {
	formats, err := recorded(t).CustomFormats(context.Background(), acquisition.TrashSonarr)
	if err != nil {
		t.Fatal(err)
	}
	got := byName(formats, func(f acquisition.TrashCustomFormat) string { return f.Name })

	sources := func(name string) []api.FormatSpec {
		var out []api.FormatSpec
		for _, s := range got[name].Specs {
			if s.Kind == api.SpecSource {
				out = append(out, api.FormatSpec{Kind: s.Kind, Value: s.Value, Negate: s.Negate, Required: s.Required})
			}
		}
		return out
	}
	wantBluray := []api.FormatSpec{
		{Kind: api.SpecSource, Value: string(parse.SourceBluray), Required: true},
		{Kind: api.SpecSource, Value: string(acquisition.SourceBlurayRaw), Negate: true, Required: true},
	}
	if got := sources("HD Bluray Tier 01"); !slices.Equal(got, wantBluray) {
		t.Errorf("HD Bluray sources = %+v, want %+v", got, wantBluray)
	}
	if got := sources("Remux Tier 01"); !slices.Equal(got, []api.FormatSpec{{Kind: api.SpecSource, Value: string(acquisition.SourceBlurayRaw), Required: true}}) {
		t.Errorf("Remux sources = %+v", got)
	}
	season := got["Season Pack"]
	if len(season.Specs) != 1 || season.Specs[0].Value != string(acquisition.ReleaseTypeSeasonPack) || season.Scores["default"] != 10 {
		t.Errorf("Season Pack = %+v", season)
	}
}

func TestQualityProfilesKeepItemsScoreSetAndFormats(t *testing.T) {
	ctx := context.Background()
	c := recorded(t)
	radarr, err := c.QualityProfiles(ctx, acquisition.TrashRadarr)
	if err != nil {
		t.Fatal(err)
	}
	got := byName(radarr, func(p acquisition.TrashQualityProfile) string { return p.Name })

	web := got["WEB 1080p"]
	if web.TrashID != "e8c5acb741363a0dbda67d3978f4912f" || web.ScoreSet != "default" || web.Cutoff != "WEB 1080p" || !web.UpgradeAllowed {
		t.Errorf("WEB 1080p = %+v", web)
	}
	if web.CutoffFormatScore != 10000 || web.MinUpgradeFormatScore != 1 {
		t.Errorf("WEB 1080p scores = %+v", web)
	}
	first := web.Items[0]
	if first.Name != "WEB 1080p" || !first.Allowed || !slices.Equal(first.Qualities, []string{"WEBRip-1080p", "WEBDL-1080p"}) {
		t.Errorf("first item = %+v", first)
	}
	if !slices.Contains(web.Formats, "c20f169ef63c5f40c2def54abaf4438e") || len(web.Formats) != 6 || !slices.IsSorted(web.Formats) {
		t.Errorf("formats = %v, want the 6 trash_ids sorted", web.Formats)
	}
	if sqp := got["[SQP] SQP-1 (1080p)"]; sqp.ScoreSet != "sqp-1-1080p" || sqp.MinFormatScore != 1000 {
		t.Errorf("SQP-1 = %+v", sqp)
	}

	sonarr, err := c.QualityProfiles(ctx, acquisition.TrashSonarr)
	if err != nil {
		t.Fatal(err)
	}
	if len(sonarr) != 1 || sonarr[0].Cutoff != "Bluray-1080p Remux" {
		t.Errorf("sonarr profiles = %+v", sonarr)
	}
}

func TestQualitySizesReadTheDefaultTablePerApp(t *testing.T) {
	ctx := context.Background()
	c := recorded(t)
	movie, err := c.QualitySizes(ctx, acquisition.TrashRadarr)
	if err != nil {
		t.Fatal(err)
	}
	sizes := byName(movie, func(s acquisition.TrashQualitySize) string { return s.Quality })
	if got := sizes["WEBDL-1080p"].Sizes; got != (api.SizeLimits{MinMBPerMin: 12.5, PreferredMBPerMin: 1999, MaxMBPerMin: 2000}) {
		t.Errorf("WEBDL-1080p = %+v", got)
	}
	series, err := c.QualitySizes(ctx, acquisition.TrashSonarr)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(series, func(s acquisition.TrashQualitySize) bool { return s.Quality == "Bluray-1080p Remux" }) {
		t.Errorf("series sizes = %+v", series)
	}
}

func TestFailuresAreUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	t.Cleanup(server.Close)
	c := NewClient(server.URL, server.URL, server.Client())

	_, err := c.CustomFormats(context.Background(), acquisition.TrashRadarr)
	if !errors.Is(err, acquisition.ErrUnavailable) || !strings.Contains(err.Error(), "403") {
		t.Errorf("err = %v, want ErrUnavailable with the status", err)
	}
}
