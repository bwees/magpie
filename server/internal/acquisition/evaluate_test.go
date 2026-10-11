package acquisition

import (
	"bytes"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"magpie/internal/acquisition/api"
	"magpie/internal/domain"
	"magpie/internal/platform/parse"
)

const (
	webDL1080p  = "Movie.Title.2024.1080p.WEB-DL.DDP5.1.H.264-FLUX"
	webRip1080p = "Movie.Title.2024.1080p.WEBRip.x264-GRP"
	hdtv1080p   = "Movie.Title.2024.1080p.HDTV.x264-GRP"
	bluray1080p = "Movie.Title.2024.1080p.BluRay.x264-GRP"
	remux1080p  = "Movie.Title.2024.1080p.BluRay.REMUX.AVC.DTS-HD.MA.5.1-GRP"
	webDL2160p  = "Movie.Title.2024.2160p.WEB-DL.DDP5.1.H.265-GRP"
	webDL720p   = "Movie.Title.2024.720p.WEB-DL.DDP5.1.H.264-GRP"
	seasonPack  = "Show.Title.S01.1080p.WEB-DL.DDP5.1.H.264-GRP"

	tierOneScore = 1700
	tierTwoScore = 300
	blockedScore = -10000
	upgradeUntil = 2000
	oneGB        = 1 << 30

	// (a+)+b backtracks 2^n steps over a run of n a's.
	backtrackingRun = 40
)

func movie(t *testing.T, name string) ReleaseFacts {
	t.Helper()
	r, err := parse.ParseRelease(name, domain.MediaKindMovie)
	if err != nil {
		t.Fatal(err)
	}
	return ReleaseFacts{Release: r}
}

func episode(t *testing.T, name string) ReleaseFacts {
	t.Helper()
	r, err := parse.ParseRelease(name, domain.MediaKindSeries)
	if err != nil {
		t.Fatal(err)
	}
	return ReleaseFacts{Release: r}
}

func quality(id parse.QualityID) domain.Quality {
	d, _ := parse.QualityByID(id)
	return d.Quality(0)
}

func group(id int, name string, ids ...parse.QualityID) api.ProfileItem {
	item := api.ProfileItem{Group: &api.QualityGroup{ID: id, Name: name}, Allowed: true}
	for _, q := range ids {
		item.Qualities = append(item.Qualities, quality(q))
	}
	return item
}

func single(id parse.QualityID, allowed bool) api.ProfileItem {
	return api.ProfileItem{Qualities: []domain.Quality{quality(id)}, Allowed: allowed}
}

func format(name string, specs ...api.FormatSpec) CustomFormat {
	return CustomFormat{ID: domain.NewID[api.CustomFormatID](), Name: name, Specs: specs}
}

func title(regex string) api.FormatSpec {
	return api.FormatSpec{Kind: api.SpecReleaseTitle, Value: regex}
}

func scoring(t *testing.T, p api.Profile, formats ...CustomFormat) ScoringProfile {
	t.Helper()
	compiled := make([]CompiledFormat, 0, len(formats))
	for _, f := range formats {
		c, err := CompileFormat(f)
		if err != nil {
			t.Fatalf("compile %s: %v", f.Name, err)
		}
		compiled = append(compiled, c)
	}
	return NewScoringProfile(p, compiled)
}

func scoreFor(f CustomFormat, score int) api.FormatScore {
	return api.FormatScore{Format: f.ID, Name: f.Name, Score: score}
}

func TestEvaluateRanksQualityByProfileOrderWithGroupsEqual(t *testing.T) {
	p := api.Profile{
		Items: []api.ProfileItem{
			group(1000, "WEB 2160p", parse.QualityWEBDL2160p, parse.QualityWEBRip2160p),
			group(1001, "WEB 1080p", parse.QualityWEBDL1080p, parse.QualityWEBRip1080p),
			single(parse.QualityBluray1080p, true),
			single(parse.QualityHDTV1080p, false),
		},
	}
	sp := scoring(t, p)

	rank := func(name string) Evaluation { return Evaluate(movie(t, name), sp) }
	web2160, web1080, rip1080, bluray, hdtv := rank(webDL2160p), rank(webDL1080p), rank(webRip1080p), rank(bluray1080p), rank(hdtv1080p)

	if web2160.QualityRank <= web1080.QualityRank || web1080.QualityRank <= bluray.QualityRank || bluray.QualityRank <= hdtv.QualityRank {
		t.Errorf("ranks 2160p=%d web1080p=%d bluray=%d hdtv=%d, want profile order", web2160.QualityRank, web1080.QualityRank, bluray.QualityRank, hdtv.QualityRank)
	}
	if web1080.QualityRank != rip1080.QualityRank {
		t.Errorf("group members rank %d and %d, want equal", web1080.QualityRank, rip1080.QualityRank)
	}
	if !web1080.Allowed || hdtv.Allowed {
		t.Errorf("allowed: web=%v hdtv=%v, want true and false", web1080.Allowed, hdtv.Allowed)
	}
	if web1080.Quality.ID != int(parse.QualityWEBDL1080p) {
		t.Errorf("quality = %+v", web1080.Quality)
	}
	if got := rank(webDL720p); got.QualityRank != 0 || got.Allowed {
		t.Errorf("quality outside the profile = %+v, want rank 0 and not allowed", got)
	}
}

func TestEvaluateResolutionPresets(t *testing.T) {
	cases := []struct {
		preset  api.QualityPreset
		allowed []string
		refused []string
	}{
		{api.Preset720p, []string{webDL720p, "Movie.Title.2024.720p.BluRay.x264-GRP", "Movie.Title.2024.720p.HDTV.x264-GRP", "Movie.Title.2024.720p.WEBRip.x264-GRP"}, []string{webDL1080p, webDL2160p}},
		{api.Preset1080p, []string{webDL1080p, webRip1080p, hdtv1080p, bluray1080p, remux1080p}, []string{webDL720p, webDL2160p}},
		{api.Preset2160p, []string{webDL2160p, "Movie.Title.2024.2160p.WEBRip.x265-GRP", "Movie.Title.2024.2160p.BluRay.x265-GRP", "Movie.Title.2024.2160p.BluRay.REMUX.HEVC.DTS-HD.MA.5.1-GRP", "Movie.Title.2024.2160p.HDTV.x265-GRP"}, []string{webDL1080p, remux1080p}},
	}
	for _, tc := range cases {
		t.Run(string(tc.preset), func(t *testing.T) {
			sp := scoring(t, PresetProfile(tc.preset))
			for _, name := range tc.allowed {
				got := Evaluate(movie(t, name), sp)
				if !got.Allowed || got.QualityRank != 1 {
					t.Errorf("%s: %+v, want allowed in the preset's one group", name, got)
				}
				if !got.CutoffMet {
					t.Errorf("%s: cutoff not met, want every source at the resolution to meet it", name)
				}
			}
			for _, name := range tc.refused {
				if got := Evaluate(movie(t, name), sp); got.Allowed {
					t.Errorf("%s allowed by %s preset", name, tc.preset)
				}
			}
		})
	}
}

func TestEvaluateAddsFormatScores(t *testing.T) {
	webTier := format("WEB Tier 01", api.FormatSpec{Kind: api.SpecReleaseGroup, Value: `^(FLUX|NTb)$`})
	ddp := format("DD+", title(`\bDDP|DD\+`))
	x265 := format("x265 (HD)", title(`[xh][ ._-]?265|\bHEVC(\b|\d)`))
	unscored := format("Unscored", title(`WEB`))

	p := PresetProfile(api.Preset1080p)
	p.FormatScores = []api.FormatScore{scoreFor(webTier, tierOneScore), scoreFor(ddp, tierTwoScore), scoreFor(x265, blockedScore)}
	sp := scoring(t, p, webTier, ddp, x265, unscored)

	got := Evaluate(movie(t, webDL1080p), sp)
	if got.Score != tierOneScore+tierTwoScore {
		t.Errorf("score = %d, want %d", got.Score, tierOneScore+tierTwoScore)
	}
	if want := []string{"DD+", "Unscored", "WEB Tier 01"}; !slices.Equal(got.Formats, want) {
		t.Errorf("formats = %v, want %v", got.Formats, want)
	}

	hevc := Evaluate(movie(t, "Movie.Title.2024.1080p.WEB-DL.DDP5.1.H.265-GRP"), sp)
	if hevc.Score != tierTwoScore+blockedScore {
		t.Errorf("x265 score = %d, want %d", hevc.Score, tierTwoScore+blockedScore)
	}
}

func TestEvaluateCutoffNeedsQualityAndUpgradeUntilScore(t *testing.T) {
	tier := format("WEB Tier 01", api.FormatSpec{Kind: api.SpecReleaseGroup, Value: `^FLUX$`})
	p := api.Profile{
		Items: []api.ProfileItem{
			group(1000, "WEB 2160p", parse.QualityWEBDL2160p),
			group(1001, "WEB 1080p", parse.QualityWEBDL1080p),
		},
		Cutoff:            1,
		CutoffFormatScore: upgradeUntil,
		FormatScores:      []api.FormatScore{scoreFor(tier, tierOneScore)},
	}
	sp := scoring(t, p, tier)

	if got := Evaluate(movie(t, webDL1080p), sp); got.CutoffMet {
		t.Errorf("score %d below upgrade-until met the cutoff", got.Score)
	}
	p.CutoffFormatScore = tierOneScore
	sp = scoring(t, p, tier)
	if got := Evaluate(movie(t, webDL1080p), sp); !got.CutoffMet {
		t.Errorf("WEB 1080p with score %d did not meet cutoff WEB 1080p / %d", got.Score, tierOneScore)
	}
	if got := Evaluate(movie(t, webDL2160p), sp); got.CutoffMet {
		t.Error("score 0 met the cutoff")
	}
}

func TestFormatSpecSemantics(t *testing.T) {
	remuxOnly := api.FormatSpec{Kind: api.SpecSource, Value: string(SourceBlurayRaw), Required: true}
	cases := []struct {
		name    string
		release ReleaseFacts
		specs   []api.FormatSpec
		want    bool
	}{
		{"same kind is OR", movie(t, webDL1080p), []api.FormatSpec{title(`nomatch`), title(`FLUX`)}, true},
		{"different kinds are AND", movie(t, webDL1080p), []api.FormatSpec{title(`FLUX`), {Kind: api.SpecResolution, Value: "2160"}}, false},
		{"a failed required spec fails its kind", movie(t, webDL1080p), []api.FormatSpec{{Kind: api.SpecReleaseTitle, Value: `nomatch`, Required: true}, title(`FLUX`)}, false},
		{"negate inverts", movie(t, webDL1080p), []api.FormatSpec{{Kind: api.SpecReleaseTitle, Value: `x265`, Negate: true}}, true},
		{"regex ignores case", movie(t, webDL1080p), []api.FormatSpec{title(`flux`)}, true},
		{"group", movie(t, webDL1080p), []api.FormatSpec{{Kind: api.SpecReleaseGroup, Value: `^FLUX$`}}, true},
		{"source", movie(t, webRip1080p), []api.FormatSpec{{Kind: api.SpecSource, Value: string(parse.SourceWebRip)}}, true},
		{"bluray source covers remux", movie(t, remux1080p), []api.FormatSpec{{Kind: api.SpecSource, Value: string(parse.SourceBluray)}}, true},
		{"raw bluray source is remux only", movie(t, bluray1080p), []api.FormatSpec{remuxOnly}, false},
		{"raw bluray source matches remux", movie(t, remux1080p), []api.FormatSpec{remuxOnly}, true},
		{"modifier", movie(t, remux1080p), []api.FormatSpec{{Kind: api.SpecQualityModifier, Value: string(parse.ModifierRemux)}}, true},
		{"resolution", movie(t, webDL2160p), []api.FormatSpec{{Kind: api.SpecResolution, Value: "2160"}}, true},
		{"season pack", episode(t, seasonPack), []api.FormatSpec{{Kind: api.SpecReleaseType, Value: string(ReleaseTypeSeasonPack)}}, true},
		{"single episode is not a pack", episode(t, "Show.Title.S01E01.1080p.WEB-DL-GRP"), []api.FormatSpec{{Kind: api.SpecReleaseType, Value: string(ReleaseTypeSeasonPack)}}, false},
		{"size in GB, min exclusive", withSize(movie(t, webDL1080p), 2*oneGB), []api.FormatSpec{{Kind: api.SpecSize, Min: 1, Max: 2}}, true},
		{"size above max", withSize(movie(t, webDL1080p), 3*oneGB), []api.FormatSpec{{Kind: api.SpecSize, Min: 1, Max: 2}}, false},
		{"year", movie(t, webDL1080p), []api.FormatSpec{{Kind: api.SpecYear, Min: 2020, Max: 2024}}, true},
		{"indexer flag", withFlags(movie(t, webDL1080p), FlagFreeleech), []api.FormatSpec{{Kind: api.SpecIndexerFlag, Value: string(FlagFreeleech)}}, true},
		{"language", movie(t, "Movie.Title.2024.GERMAN.1080p.WEB-DL.H.264-GRP"), []api.FormatSpec{{Kind: api.SpecLanguage, Value: "de"}}, true},
		{"unnamed language is the original", withOriginal(movie(t, webDL1080p), "en"), []api.FormatSpec{{Kind: api.SpecLanguage, Value: LanguageOriginal}}, true},
		{"edition", movie(t, "Movie.Title.2024.Directors.Cut.1080p.BluRay.x264-GRP"), []api.FormatSpec{{Kind: api.SpecEdition, Value: `Director`}}, true},
		{"no specs never match", movie(t, webDL1080p), nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := format(tc.name, tc.specs...)
			sp := scoring(t, PresetProfile(api.Preset1080p), f)
			got := slices.Contains(Evaluate(tc.release, sp).Formats, tc.name)
			if got != tc.want {
				t.Errorf("matched = %v, want %v", got, tc.want)
			}
		})
	}
}

// Seed acquisition #26: a regex past the match timeout doesn't match and the decision still completes.
func TestFormatRegexTimeoutCountsAsNoMatch(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	catastrophic := format("Catastrophic", title(`(a+)+b`))
	fast := format("Fast", title(`WEB`))
	p := PresetProfile(api.Preset1080p)
	p.FormatScores = []api.FormatScore{scoreFor(catastrophic, tierOneScore), scoreFor(fast, tierTwoScore)}
	sp := scoring(t, p, catastrophic, fast)

	release := movie(t, "Movie.Title.2024.1080p.WEB-DL."+strings.Repeat("a", backtrackingRun)+".H.264-GRP")
	began := time.Now()
	got := Evaluate(release, sp)

	if slices.Contains(got.Formats, catastrophic.Name) {
		t.Error("format that timed out matched")
	}
	if got.Score != tierTwoScore || !got.Allowed {
		t.Errorf("evaluation = %+v, want the other format scored and the release allowed", got)
	}
	if elapsed := time.Since(began); elapsed > 10*FormatMatchTimeout {
		t.Errorf("evaluation took %v", elapsed)
	}
	if !strings.Contains(logs.String(), "custom format regex failed") || !strings.Contains(logs.String(), catastrophic.Name) {
		t.Errorf("no warning logged, got %q", logs.String())
	}
}

func TestCompileFormatRejectsBadSpecs(t *testing.T) {
	cases := map[string]api.FormatSpec{
		"bad regex":         title(`(unclosed`),
		"unknown kind":      {Kind: "COLOUR", Value: "x"},
		"unknown source":    {Kind: api.SpecSource, Value: "VHS"},
		"bad resolution":    {Kind: api.SpecResolution, Value: "1080p"},
		"unknown flag":      {Kind: api.SpecIndexerFlag, Value: "GOLDEN"},
		"unknown type":      {Kind: api.SpecReleaseType, Value: "TRILOGY"},
		"size min over max": {Kind: api.SpecSize, Min: 3, Max: 2},
		"empty language":    {Kind: api.SpecLanguage},
		"unknown modifier":  {Kind: api.SpecQualityModifier, Value: "IMAX"},
	}
	for name, spec := range cases {
		if _, err := CompileFormat(format(name, spec)); err == nil {
			t.Errorf("%s compiled", name)
		}
	}
	if _, err := CompileFormat(format("lookaround", title(`(?<!repac)[^0-9]2[ .]0\b`))); err != nil {
		t.Errorf(".NET lookbehind rejected: %v", err)
	}
}

func withSize(r ReleaseFacts, size int64) ReleaseFacts {
	r.SizeBytes = size
	return r
}

func withFlags(r ReleaseFacts, flags ...IndexerFlag) ReleaseFacts {
	r.IndexerFlags = flags
	return r
}

func withOriginal(r ReleaseFacts, language domain.Language) ReleaseFacts {
	r.OriginalLanguage = language
	return r
}
