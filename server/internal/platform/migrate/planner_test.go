package migrate_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/adapters/arr"
	"magpie/internal/domain"
	"magpie/internal/platform/migrate"
)

// recordings are the Sonarr and Radarr answers the arr adapter is tested against.
const recordings = "../../adapters/arr/testdata"

const (
	dailyTmdb    domain.TmdbID = 2224
	homeMovies                 = "Home Movies 1999"
	homeTmdb     domain.TmdbID = 555
	duneTwoTmdb  domain.TmdbID = 693134
	recordingKey               = "recorded"
)

// recorded serves one app's recordings the way the arr adapter's own test does.
func recorded(t *testing.T, app string) migrate.Connection {
	t.Helper()
	dir := filepath.Join(recordings, app)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.ReplaceAll(strings.TrimPrefix(r.URL.Path, "/api/v3/"), "/", "_")
		if r.URL.RawQuery != "" {
			name += "_" + r.URL.RawQuery
		}
		body, err := os.ReadFile(filepath.Join(dir, name+".json"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	return migrate.Connection{URL: server.URL, APIKey: recordingKey}
}

// recordedHarness runs the migration over the recorded libraries, with TMDB knowing their titles.
func recordedHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	client := arr.New()
	for app, dir := range map[migrate.App]string{migrate.AppSonarr: "sonarr", migrate.AppRadarr: "radarr"} {
		lib, err := client.Library(context.Background(), app, recorded(t, dir), nil)
		if err != nil {
			t.Fatal(err)
		}
		h.arr.Put(lib)
	}
	dailyTvdb := "71256"
	h.tmdb.PutSeries(migrate.TmdbSeries{TmdbCandidate: migrate.TmdbCandidate{Tmdb: dailyTmdb, Kind: domain.MediaKindSeries, Name: "The Daily Show"}, Tvdb: &dailyTvdb},
		migrate.TmdbSeason{Number: 2024, Episodes: []migrate.TmdbEpisode{{ID: 3001, Season: 2024, Number: 1}, {ID: 3002, Season: 2024, Number: 2}}})
	h.tmdb.PutFind(migrate.FindTvdb, "297993", migrate.TmdbFound{Episodes: []migrate.TmdbFoundEpisode{{ID: 2005, Series: fireflyTmdb, Season: 1, Number: 5}}})
	h.tmdb.SetSearch(migrate.TmdbCandidate{Tmdb: homeTmdb, Kind: domain.MediaKindMovie, Name: homeMovies, Year: 1999})
	h.tmdb.PutMovie(migrate.TmdbCandidate{Tmdb: homeTmdb, Kind: domain.MediaKindMovie, Name: homeMovies, Year: 1999})
	for _, path := range []string{"/tv", "/movies"} {
		h.disk.AddFolder(path)
	}
	for _, file := range []string{
		"/tv/Bluey/Season 01/Bluey - S01E01 - Magic Xylophone Bluray-1080p Remux.mkv",
		"/tv/Firefly/Season 01/Firefly - S01E01 - Serenity WEBDL-1080p.mkv",
		"/movies/Conclave (2024)/Conclave (2024) Bluray-1080p.mkv",
	} {
		h.disk.AddFile(file)
	}
	return h
}

func (h *harness) recordedDryRun() *migrate.Plan {
	h.t.Helper()
	ctx := context.Background()
	for _, app := range migrate.Apps {
		if _, err := h.service.Connect(ctx, migrate.ConnectRequest{App: app, Connection: migrate.Connection{URL: "http://arr"}}); err != nil {
			h.t.Fatal(err)
		}
	}
	if _, err := h.service.StartDryRun(ctx); err != nil {
		h.t.Fatal(err)
	}
	h.run()
	s := h.state()
	if s.Plan == nil {
		h.t.Fatalf("dry run ended in %s: %s", s.Phase, s.Error)
	}
	return s.Plan
}

func format(t *testing.T, plan *migrate.Plan, source domain.SourceKey) migrate.PlannedFormat {
	t.Helper()
	i := slices.IndexFunc(plan.Formats, func(f migrate.PlannedFormat) bool { return f.Source == source })
	if i < 0 {
		t.Fatalf("no format %s in %+v", source, plan.Formats)
	}
	return plan.Formats[i]
}

func TestCustomFormatConditionsTranslateByMeaning(t *testing.T) {
	plan := recordedHarness(t).recordedDryRun()

	if got := format(t, plan, "sonarr:customformat:3").Specs; len(got) != 1 || got[0].Kind != acquisitionapi.SpecLanguage || got[0].Value != "en" || !got[0].Negate {
		t.Errorf("Not English = %+v, want NOT LANGUAGE en", got)
	}
	scene := format(t, plan, "sonarr:customformat:4")
	if len(scene.Specs) != 1 || scene.Specs[0].Kind != acquisitionapi.SpecSize || scene.Specs[0].Max != 1.5 {
		t.Errorf("Scene = %+v, want only the size condition kept", scene.Specs)
	}
	if !slices.ContainsFunc(plan.Warnings, func(w string) bool { return strings.Contains(w, `"Scene"`) }) {
		t.Errorf("warnings = %v, want the dropped Scene flag", plan.Warnings)
	}
	golden := format(t, plan, "radarr:customformat:3")
	if len(golden.Specs) != 1 || golden.Specs[0].Value != "FREELEECH" {
		t.Errorf("PTP Golden = %+v, want PTP_Golden dropped and G_Freeleech as FREELEECH", golden.Specs)
	}
	tier := format(t, plan, "radarr:customformat:2")
	want := []acquisitionapi.FormatSpec{
		{Kind: acquisitionapi.SpecReleaseGroup, Name: "FGT", Value: "^(FGT|FraMeSToR)$"},
		{Kind: acquisitionapi.SpecQualityModifier, Name: "Remux", Required: true, Value: "REMUX"},
		{Kind: acquisitionapi.SpecSource, Name: "Bluray", Required: true, Value: "BLURAY"},
	}
	if !slices.Equal(tier.Specs, want) {
		t.Errorf("Remux Tier 01 = %+v, want %+v", tier.Specs, want)
	}

	rawTV := format(t, plan, "sonarr:customformat:5")
	if len(rawTV.Specs) != 1 || rawTV.Specs[0].Value != "TV" || len(rawTV.Pending) != 1 {
		t.Fatalf("Raw TV = %+v pending %+v, want Television kept and TelevisionRaw for review", rawTV.Specs, rawTV.Pending)
	}
	review := plan.Reviews.Specs[slices.IndexFunc(plan.Reviews.Specs, func(r migrate.SpecReview) bool { return r.ID == rawTV.Pending[0].Review })]
	if review.Kind != acquisitionapi.SpecSource || !slices.Contains(review.Options, "TV_RAW") {
		t.Errorf("review = %+v, want a Source edit offering TV_RAW", review)
	}
}

func TestReleaseProfilesAndProfileLanguagesBecomeBlockingFormats(t *testing.T) {
	plan := recordedHarness(t).recordedDryRun()

	ignored := format(t, plan, "sonarr:releaseprofile:1:ignored")
	if len(ignored.Specs) != 2 || ignored.Specs[0].Value != "YIFY" || ignored.Specs[1].Value != `\bRARBG\b` {
		t.Errorf("ignored terms = %+v, want YIFY literal and the RARBG regex", ignored.Specs)
	}
	if slices.ContainsFunc(plan.Formats, func(f migrate.PlannedFormat) bool {
		return strings.HasPrefix(string(f.Source), "sonarr:releaseprofile:2")
	}) {
		t.Error("the disabled release profile became a format")
	}
	if !slices.ContainsFunc(plan.Warnings, func(w string) bool { return strings.Contains(w, "Bad groups") }) {
		t.Errorf("warnings = %v, want Bad groups' lost tag scope", plan.Warnings)
	}
	language := format(t, plan, "radarr:qualityprofile:4:language")
	if language.Name != "Language: not English" || !language.Specs[0].Negate || language.Specs[0].Value != "en" {
		t.Errorf("language format = %+v", language)
	}
	for _, p := range plan.Profiles {
		score, scored := p.Scores["radarr:qualityprofile:4:language"]
		if (p.Source == "radarr:qualityprofile:4") != (scored && score == -10000) {
			t.Errorf("profile %s scores the language format %d, want -10000 only in Radarr's HD-1080p", p.Source, score)
		}
		if p.App == migrate.AppSonarr && p.Scores["sonarr:releaseprofile:1:ignored"] != -10000 {
			t.Errorf("profile %s doesn't block the release profile's terms", p.Source)
		}
	}
}

func TestProfilesKeepTheirOrderGroupsCutoffAndSizes(t *testing.T) {
	plan := recordedHarness(t).recordedDryRun()

	i := slices.IndexFunc(plan.Profiles, func(p migrate.PlannedProfile) bool { return p.Source == "sonarr:qualityprofile:4" })
	hd := plan.Profiles[i]
	names := func(item acquisitionapi.ImportProfileItem) []string {
		var out []string
		for _, q := range item.Qualities {
			out = append(out, q.Name)
		}
		return out
	}
	if first := names(hd.Items[0]); !slices.Equal(first, []string{"Remux-1080p"}) || hd.Items[0].Allowed {
		t.Errorf("first item = %v, want Sonarr's remux, as Remux-1080p and not allowed", first)
	}
	if web := hd.Items[2]; web.Group != "WEB 1080p" || !slices.Equal(names(web), []string{"WEBRip-1080p", "WEBDL-1080p"}) || hd.Cutoff != 2 {
		t.Errorf("profile = %+v, want the WEB 1080p group as cutoff", hd)
	}
	if web := hd.Items[2].Sizes; web.MinMBPerMin != 4 || web.MaxMBPerMin != 137.3 || web.PreferredMBPerMin != 117 {
		t.Errorf("group sizes = %+v, want Sonarr's WEB 1080p row", web)
	}
	if remux := hd.Items[0].Sizes; remux.MaxMBPerMin != 2000 || remux.PreferredMBPerMin != 2000 {
		t.Errorf("remux sizes = %+v, want no maximum as 2000", remux)
	}
	if !slices.ContainsFunc(plan.Reviews.Clashes, func(c migrate.Clash) bool { return c.Kind == migrate.ClashFormat && c.Name == "x265 (HD)" }) {
		t.Errorf("clashes = %+v, want x265 (HD), whose Sonarr version also excludes 2160p", plan.Reviews.Clashes)
	}
	if !slices.ContainsFunc(plan.Delays, func(d acquisitionapi.ImportDelayProfile) bool {
		return d.Source == "radarr:delayprofile:1" && d.Delay.Minutes() == 30
	}) {
		t.Errorf("delays = %+v, want Radarr's 30 minutes", plan.Delays)
	}
}

func TestRecordedTitlesFilesBlocklistAndHistory(t *testing.T) {
	plan := recordedHarness(t).recordedDryRun()

	if home := titleOf(plan, "radarr:42"); home.Status != migrate.TitleInReview {
		t.Errorf("Home Movies = %+v, want it in review", home)
	}
	i := slices.IndexFunc(plan.Reviews.Titles, func(r migrate.TitleReview) bool { return r.ID == "radarr:42" })
	if i < 0 || len(plan.Reviews.Titles[i].Candidates) != 1 || plan.Reviews.Titles[i].Candidates[0].Tmdb != homeTmdb {
		t.Errorf("title reviews = %+v, want the search result offered", plan.Reviews.Titles)
	}
	if !slices.ContainsFunc(plan.Reviews.SeriesTypes, func(r migrate.SeriesTypeReview) bool { return r.ID == "sonarr:3" && r.Type == migrate.SeriesTypeDaily }) {
		t.Errorf("series types = %+v, want The Daily Show", plan.Reviews.SeriesTypes)
	}
	dune := slices.IndexFunc(plan.Files, func(f migrate.PlannedFile) bool { return f.Source == "radarr:moviefile:42" })
	if dune < 0 || plan.Files[dune].Quality.ID != 31 || !plan.Files[dune].Missing {
		t.Errorf("files = %+v, want Dune as Remux-2160p and missing on this disk", plan.Files)
	}
	var hashes []string
	for _, b := range plan.Blocklist {
		hashes = append(hashes, b.InfoHash)
	}
	want := []string{"0a1b2c3d4e5f60718293a4b5c6d7e8f901234567", "1111222233334444555566667777888899990000"}
	if !slices.Equal(hashes, want) {
		t.Errorf("blocklist hashes = %v, want %v (the usenet row left out)", hashes, want)
	}
	kinds := map[string]int{}
	for _, h := range plan.History {
		kinds[string(h.Kind)]++
	}
	if len(plan.History) != 6 || kinds["GRABBED"] != 2 || kinds["IMPORTED"] != 2 || kinds["FILE_DELETED"] != 1 {
		t.Errorf("history = %v, want 6 rows without the rename and the usenet grab", kinds)
	}
}

func TestReviewDecisionsUpdateThePlan(t *testing.T) {
	h := recordedHarness(t)
	ctx := context.Background()
	h.recordedDryRun()

	s, err := h.service.Decide(ctx, migrate.Decision{Queue: migrate.QueueTitles, ID: "radarr:42", Title: migrate.TitleDecideMatch, Tmdb: []domain.TmdbID{homeTmdb}})
	if err != nil {
		t.Fatal(err)
	}
	if home := titleOf(s.Plan, "radarr:42"); home.Status != migrate.TitleMatched || home.Tmdb != homeTmdb {
		t.Errorf("Home Movies = %+v, want matched", home)
	}
	s, err = h.service.Decide(ctx, migrate.Decision{Queue: migrate.QueueNumbering, ID: "sonarr:2", Numbering: migrate.NumberingAired})
	if err != nil {
		t.Fatal(err)
	}
	firefly := titleOf(s.Plan, "sonarr:2")
	if firefly.EpisodeGroup != "" || !slices.ContainsFunc(firefly.Episodes, func(e migrate.PlannedEpisode) bool {
		return e.ArrSeason == 2 && e.ArrNumber == 1 && e.Mapped && e.Season == 1 && e.Number == 5
	}) {
		t.Errorf("Firefly in aired order = %+v, want Safe found by its TVDB ID as aired S01E05", firefly.Episodes)
	}

	review := s.Plan.Reviews.Specs[0]
	if _, err := h.service.Decide(ctx, migrate.Decision{Queue: migrate.QueueSpecs, ID: review.ID, Spec: migrate.SpecEdited, Value: "NOT_A_SOURCE"}); err == nil {
		t.Error("an unknown source value was accepted")
	}
	s, err = h.service.Decide(ctx, migrate.Decision{Queue: migrate.QueueSpecs, ID: review.ID, Spec: migrate.SpecEdited, Value: "TV_RAW"})
	if err != nil || s.Plan.Reviews.Specs[0].Decision != migrate.SpecEdited {
		t.Errorf("editing the spec: %v, %+v", err, s.Plan.Reviews.Specs[0])
	}
}
