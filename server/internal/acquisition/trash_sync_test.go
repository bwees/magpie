package acquisition

import (
	"context"
	"errors"
	"slices"
	"testing"

	"magpie/internal/acquisition/api"
	"magpie/internal/platform/settings"
)

const (
	radarrWebTier   = "c20f169ef63c5f40c2def54abaf4438e"
	sonarrWebTier   = "e6258996055b9fbab7e9cb2f75819294"
	radarrBRDisk    = "ed38b889b31be83fda192888e2286d83"
	sonarrSeason    = "3bc5f395426614e155e585a2f056cdf1"
	guideRemuxWeb   = "9ca12ea80aa55ef916e3751f4b874151"
	brDiskScore     = -10000
	seasonPackScore = 10
	userScore       = 1234
	newTrashScore   = 1800
)

// stubGuides stands in for fakes.TrashGuides, which this package's tests can't import (it imports
// the package).
type stubGuides struct {
	formats  map[TrashApp][]TrashCustomFormat
	profiles map[TrashApp][]TrashQualityProfile
	err      error
}

func (g *stubGuides) SetCustomFormats(app TrashApp, formats ...TrashCustomFormat) {
	g.formats[app] = formats
}

func (g *stubGuides) SetQualityProfiles(app TrashApp, profiles ...TrashQualityProfile) {
	g.profiles[app] = profiles
}

func (g *stubGuides) CustomFormats(_ context.Context, app TrashApp) ([]TrashCustomFormat, error) {
	return g.formats[app], g.err
}

func (g *stubGuides) QualityProfiles(_ context.Context, app TrashApp) ([]TrashQualityProfile, error) {
	return g.profiles[app], g.err
}

func (g *stubGuides) QualitySizes(context.Context, TrashApp) ([]TrashQualitySize, error) {
	return nil, g.err
}

type trashFixture struct {
	guides  *stubGuides
	quality *QualityService
	sync    *TrashSyncService
}

func newTrashFixture(t *testing.T) trashFixture {
	t.Helper()
	d := openQualityDB(t)
	repo := NewQualityRepository(d)
	store := settings.NewStore(d)
	guides := &stubGuides{formats: map[TrashApp][]TrashCustomFormat{}, profiles: map[TrashApp][]TrashQualityProfile{}}
	guides.SetCustomFormats(TrashRadarr,
		trashFormat(radarrWebTier, "WEB Tier 01", webTierTrashScore, `^(FLUX)$`),
		trashFormat(radarrBRDisk, "BR-DISK", brDiskScore, `BR-DISK`),
	)
	guides.SetCustomFormats(TrashSonarr,
		trashFormat(sonarrWebTier, "WEB Tier 01", webTierTrashScore, `^(NTb)$`),
		TrashCustomFormat{
			TrashID: sonarrSeason, Name: "Season Pack", Scores: map[string]int{trashDefaultScoreSet: seasonPackScore},
			Specs: []api.FormatSpec{{Kind: api.SpecReleaseType, Value: string(ReleaseTypeSeasonPack)}},
		},
	)
	return trashFixture{
		guides:  guides,
		quality: NewQualityService(d, repo, store, &queuedTasks{}),
		sync:    NewTrashSyncService(d, repo, store, guides, &queuedTasks{}),
	}
}

func trashFormat(id, name string, score int, groupRegex string) TrashCustomFormat {
	return TrashCustomFormat{
		TrashID: id,
		Name:    name,
		Scores:  map[string]int{trashDefaultScoreSet: score},
		Specs:   []api.FormatSpec{{Kind: api.SpecReleaseGroup, Value: groupRegex}},
	}
}

func formatNamed(t *testing.T, s *QualityService, name string) CustomFormat {
	t.Helper()
	formats, err := s.Formats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range formats {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("no format %q", name)
	return CustomFormat{}
}

func TestTrashSyncAddsFormatsFirstAppWinsByName(t *testing.T) {
	ctx := context.Background()
	f := newTrashFixture(t)
	if err := f.sync.Sync(ctx); err != nil {
		t.Fatal(err)
	}

	formats, err := f.quality.Formats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, cf := range formats {
		got = append(got, cf.Name+"="+cf.TrashID)
	}
	want := []string{"BR-DISK=" + radarrBRDisk, "Season Pack=" + sonarrSeason, "WEB Tier 01=" + radarrWebTier}
	if !slices.Equal(got, want) {
		t.Errorf("formats = %v, want %v", got, want)
	}

	state, err := f.sync.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.LastSyncedAt == nil || state.LastError != "" {
		t.Errorf("state = %+v", state)
	}
}

func TestTrashSyncLinksAnUntaggedFormatByName(t *testing.T) {
	ctx := context.Background()
	f := newTrashFixture(t)
	migrated, err := f.quality.CreateFormat(ctx, CustomFormat{Name: "web tier 01", Specs: []api.FormatSpec{title(`old`)}})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.sync.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	linked := formatNamed(t, f.quality, "WEB Tier 01")
	if linked.ID != migrated.ID || linked.TrashID != radarrWebTier || linked.Specs[0].Value != `^(FLUX)$` {
		t.Errorf("format = %+v, want the migrated one linked to TRaSH and refreshed", linked)
	}
}

func TestTrashSyncAttachesPresetFormatsAndKeepsUserOverrides(t *testing.T) {
	ctx := context.Background()
	f := newTrashFixture(t)
	if err := f.quality.SeedPresets(ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.sync.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	cfg, _ := f.quality.Settings(ctx)
	p, err := f.quality.Profile(ctx, cfg.DefaultProfile)
	if err != nil {
		t.Fatal(err)
	}
	webTier, brDisk := formatNamed(t, f.quality, "WEB Tier 01"), formatNamed(t, f.quality, "BR-DISK")
	if scoreOf(p, webTier.ID) != webTierTrashScore || scoreOf(p, brDisk.ID) != brDiskScore {
		t.Fatalf("preset scores = %+v, want TRaSH's", p.FormatScores)
	}
	if slices.ContainsFunc(p.FormatScores, func(s api.FormatScore) bool { return s.Name == "Season Pack" }) {
		t.Error("format the preset doesn't name was attached")
	}

	for i := range p.FormatScores {
		if p.FormatScores[i].Format == webTier.ID {
			p.FormatScores[i].Score = userScore
		}
	}
	if _, err := f.quality.UpdateProfile(ctx, p); err != nil {
		t.Fatal(err)
	}

	f.guides.SetCustomFormats(TrashRadarr,
		trashFormat(radarrWebTier, "WEB Tier 01", newTrashScore, `^(FLUX)$`),
		trashFormat(radarrBRDisk, "BR-DISK", newTrashScore, `BR-DISK`),
	)
	if err := f.sync.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	p, _ = f.quality.Profile(ctx, cfg.DefaultProfile)
	if got := scoreOf(p, webTier.ID); got != userScore {
		t.Errorf("user override = %d after re-sync, want %d", got, userScore)
	}
	if got := scoreOf(p, brDisk.ID); got != newTrashScore {
		t.Errorf("untouched score = %d after re-sync, want TRaSH's new %d", got, newTrashScore)
	}
}

func TestTrashSyncMatchesGuideProfilesByIDElseNameWithRemuxAliases(t *testing.T) {
	ctx := context.Background()
	f := newTrashFixture(t)
	f.guides.SetQualityProfiles(TrashSonarr, TrashQualityProfile{
		TrashID:  guideRemuxWeb,
		Name:     "Bluray-1080p Remux + WEB",
		ScoreSet: trashDefaultScoreSet,
		Cutoff:   "Bluray-1080p Remux",
		Formats:  []string{sonarrSeason, radarrBRDisk},
	})
	mine, err := f.quality.CreateProfile(ctx, validProfile("Remux-1080p + WEB"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.sync.Sync(ctx); err != nil {
		t.Fatal(err)
	}

	p, _ := f.quality.Profile(ctx, mine.ID)
	if p.TrashProfileID != guideRemuxWeb {
		t.Fatalf("profile not linked by aliased name: %+v", p)
	}
	season, brDisk := formatNamed(t, f.quality, "Season Pack"), formatNamed(t, f.quality, "BR-DISK")
	if scoreOf(p, season.ID) != seasonPackScore || scoreOf(p, brDisk.ID) != brDiskScore {
		t.Errorf("guide formats not scored: %+v", p.FormatScores)
	}

	p.Name = "Renamed"
	if _, err := f.quality.UpdateProfile(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := f.sync.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if p, _ = f.quality.Profile(ctx, mine.ID); p.TrashProfileID != guideRemuxWeb {
		t.Error("renamed profile lost its TRaSH link")
	}

	guide, err := f.sync.GuideProfiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(guide) != 1 || guide[0].App != TrashSonarr || guide[0].Profile.Name != "Bluray-1080p Remux + WEB" {
		t.Errorf("guide profiles = %+v", guide)
	}
}

func TestTrashSyncFailureChangesNothing(t *testing.T) {
	ctx := context.Background()
	f := newTrashFixture(t)
	f.guides.err = ErrUnavailable

	if err := f.sync.Sync(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
	if formats, _ := f.quality.Formats(ctx); len(formats) != 0 {
		t.Errorf("formats written by a failed sync: %v", formats)
	}
	state, _ := f.sync.Status(ctx)
	if state.LastError == "" || state.LastSyncedAt != nil || state.LastAttemptAt == nil {
		t.Errorf("state = %+v, want the failure recorded", state)
	}
}
