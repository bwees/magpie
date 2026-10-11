package acquisition

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"magpie/internal/acquisition/api"
	"magpie/internal/domain"
	"magpie/internal/platform/parse"
	"magpie/internal/platform/settings"
)

const webTierTrashScore = 1700

func newQualityService(t *testing.T) *QualityService {
	t.Helper()
	d := openQualityDB(t)
	return NewQualityService(d, NewQualityRepository(d), settings.NewStore(d))
}

func names(summaries []api.ProfileSummary) []string {
	out := make([]string, 0, len(summaries))
	for _, s := range summaries {
		out = append(out, s.Name)
	}
	return out
}

func TestFirstRunCreatesThePresetsWith1080pAsDefault(t *testing.T) {
	ctx := context.Background()
	s := newQualityService(t)

	if err := s.SeedPresets(ctx); err != nil {
		t.Fatal(err)
	}
	profiles, err := s.Profiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(profiles); !slices.Equal(got, []string{"1080p", "2160p", "720p"}) {
		t.Fatalf("profiles = %v", got)
	}
	cfg, err := s.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if profiles[0].Preset != api.Preset1080p || cfg.DefaultProfile != profiles[0].ID {
		t.Errorf("default = %s, want the 1080p preset %s", cfg.DefaultProfile, profiles[0].ID)
	}

	p, err := s.Profile(ctx, profiles[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Items) != 1 || p.Cutoff != 0 || !p.Items[0].Allowed || p.CutoffFormatScore != 0 {
		t.Errorf("1080p preset = %+v, want one allowed group that is the cutoff, upgrade-until 0", p)
	}
	var sources []string
	for _, q := range p.Items[0].Qualities {
		sources = append(sources, q.Name)
	}
	if want := []string{"WEBDL-1080p", "Bluray-1080p", "HDTV-1080p", "WEBRip-1080p", "Remux-1080p"}; !slices.Equal(sources, want) {
		t.Errorf("1080p group = %v, want %v", sources, want)
	}

	if err := s.DeleteProfile(ctx, profiles[1].ID); err != nil {
		t.Fatal(err)
	}
	if err := s.SeedPresets(ctx); err != nil {
		t.Fatal(err)
	}
	if again, _ := s.Profiles(ctx); len(again) != 2 {
		t.Errorf("second start recreated presets: %v", names(again))
	}
}

func TestFromPresetAttachesTrashScores(t *testing.T) {
	ctx := context.Background()
	s := newQualityService(t)
	tier, err := s.CreateFormat(ctx, CustomFormat{
		Name:        "WEB Tier 01",
		TrashID:     "c20f169ef63c5f40c2def54abaf4438e",
		TrashScores: map[string]int{trashDefaultScoreSet: webTierTrashScore},
		Specs:       []api.FormatSpec{{Kind: api.SpecReleaseGroup, Value: `^FLUX$`}},
	})
	if err != nil {
		t.Fatal(err)
	}

	p, err := s.CreateFromPreset(ctx, api.Preset2160p)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "2160p" || p.Preset != api.Preset2160p {
		t.Errorf("profile = %s %s", p.Name, p.Preset)
	}
	want := []api.FormatScore{{Format: tier.ID, Name: tier.Name, Score: webTierTrashScore}}
	if !slices.Equal(p.FormatScores, want) {
		t.Errorf("scores = %+v, want %+v", p.FormatScores, want)
	}
	if _, err := s.CreateFromPreset(ctx, api.Preset2160p); !errors.Is(err, ErrNameTaken) {
		t.Errorf("second 2160p err = %v, want ErrNameTaken", err)
	}
	if _, err := s.CreateFromPreset(ctx, "480P"); !errors.Is(err, ErrInvalidProfile) {
		t.Errorf("unknown preset err = %v, want ErrInvalidProfile", err)
	}
}

func validProfile(name string) api.Profile {
	return api.Profile{
		Name:           name,
		UpgradeAllowed: true,
		Items: []api.ProfileItem{
			group(firstGroupID, "WEB 1080p", parse.QualityWEBDL1080p, parse.QualityWEBRip1080p),
			single(parse.QualityBluray1080p, false),
		},
		PropersMode: api.PropersPreferAndUpgrade,
	}
}

func TestProfileValidation(t *testing.T) {
	ctx := context.Background()
	s := newQualityService(t)
	if _, err := s.CreateProfile(ctx, validProfile("Mine")); err != nil {
		t.Fatal(err)
	}

	cases := map[string]struct {
		edit func(*api.Profile)
		want error
	}{
		"name taken, any case": {func(p *api.Profile) { p.Name = "MINE" }, ErrNameTaken},
		"empty name":           {func(p *api.Profile) { p.Name = " " }, ErrInvalidProfile},
		"no items":             {func(p *api.Profile) { p.Items = nil }, ErrInvalidProfile},
		"cutoff not allowed":   {func(p *api.Profile) { p.Cutoff = 1 }, ErrInvalidProfile},
		"cutoff out of range":  {func(p *api.Profile) { p.Cutoff = 2 }, ErrInvalidProfile},
		"min above preferred": {func(p *api.Profile) {
			p.Items[0].Sizes = api.SizeLimits{MinMBPerMin: 20, PreferredMBPerMin: 10, MaxMBPerMin: 30}
		}, ErrInvalidProfile},
		"preferred above max": {func(p *api.Profile) {
			p.Items[0].Sizes = api.SizeLimits{MinMBPerMin: 1, PreferredMBPerMin: 40, MaxMBPerMin: 30}
		}, ErrInvalidProfile},
		"quality twice": {func(p *api.Profile) {
			p.Items = append(p.Items, single(parse.QualityWEBDL1080p, false))
		}, ErrInvalidProfile},
		"unknown quality": {func(p *api.Profile) {
			p.Items = append(p.Items, api.ProfileItem{Qualities: []domain.Quality{{ID: 13}}})
		}, ErrInvalidProfile},
		"single item with two qualities": {func(p *api.Profile) {
			p.Items[1].Qualities = append(p.Items[1].Qualities, quality(parse.QualityHDTV1080p))
		}, ErrInvalidProfile},
		"unknown propers mode": {func(p *api.Profile) { p.PropersMode = "SOMETIMES" }, ErrInvalidProfile},
		"unknown format": {func(p *api.Profile) {
			p.FormatScores = []api.FormatScore{{Format: "nope", Score: 1}}
		}, ErrInvalidProfile},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p := validProfile("Other")
			tc.edit(&p)
			if _, err := s.CreateProfile(ctx, p); !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestEditingATrashScoreMarksItOverridden(t *testing.T) {
	ctx := context.Background()
	s := newQualityService(t)
	tier, err := s.CreateFormat(ctx, CustomFormat{
		Name:        "WEB Tier 01",
		TrashID:     "c20f169ef63c5f40c2def54abaf4438e",
		TrashScores: map[string]int{trashDefaultScoreSet: webTierTrashScore},
		Specs:       []api.FormatSpec{{Kind: api.SpecReleaseGroup, Value: `^FLUX$`}},
	})
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateFromPreset(ctx, api.Preset1080p)
	if err != nil {
		t.Fatal(err)
	}

	p.FormatScores = []api.FormatScore{{Format: tier.ID, Score: webTierTrashScore + 1}}
	if p, err = s.UpdateProfile(ctx, p); err != nil {
		t.Fatal(err)
	}
	if !p.FormatScores[0].UserOverride {
		t.Error("changed score not marked as a user override")
	}

	p.FormatScores = []api.FormatScore{{Format: tier.ID, Score: webTierTrashScore}}
	if p, err = s.UpdateProfile(ctx, p); err != nil {
		t.Fatal(err)
	}
	if p.FormatScores[0].UserOverride {
		t.Error("score set back to TRaSH's value still marked as an override")
	}
}

func TestCustomFormatValidation(t *testing.T) {
	ctx := context.Background()
	s := newQualityService(t)
	f, err := s.CreateFormat(ctx, CustomFormat{Name: "x265", Specs: []api.FormatSpec{title(`x265`)}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateFormat(ctx, CustomFormat{Name: "X265", Specs: []api.FormatSpec{title(`x`)}}); !errors.Is(err, ErrNameTaken) {
		t.Errorf("duplicate name err = %v", err)
	}
	if _, err := s.CreateFormat(ctx, CustomFormat{Name: "Broken", Specs: []api.FormatSpec{title(`(`)}}); !errors.Is(err, ErrInvalidFormat) {
		t.Errorf("bad regex err = %v", err)
	}
	if _, err := s.CreateFormat(ctx, CustomFormat{Name: ""}); !errors.Is(err, ErrInvalidFormat) {
		t.Errorf("empty name err = %v", err)
	}
	f.Specs = []api.FormatSpec{title(`[xh]\.?265`)}
	if _, err := s.UpdateFormat(ctx, f); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteFormat(ctx, f.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteFormat(ctx, f.ID); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("second delete err = %v", err)
	}
}

func TestBlockTermScoresMinus10000InEveryProfile(t *testing.T) {
	ctx := context.Background()
	s := newQualityService(t)
	if err := s.SeedPresets(ctx); err != nil {
		t.Fatal(err)
	}

	f, err := s.BlockTerm(ctx, "HC.Subs")
	if err != nil {
		t.Fatal(err)
	}
	profiles, _ := s.Profiles(ctx)
	for _, summary := range profiles {
		sp, err := s.ScoringProfile(ctx, summary.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got := scoreOf(sp.Profile(), f.ID); got != BlockTermScore {
			t.Errorf("%s score = %d, want %d", summary.Name, got, BlockTermScore)
		}
		blocked := Evaluate(movie(t, "Movie.Title.2024.1080p.WEB-DL.HC.Subs.H.264-GRP"), sp)
		clean := Evaluate(movie(t, "Movie.Title.2024.1080p.WEB-DL.HCxSubs.H.264-GRP"), sp)
		if blocked.Score != BlockTermScore || clean.Score != 0 {
			t.Errorf("%s: blocked %d, clean %d; the term must match literally", summary.Name, blocked.Score, clean.Score)
		}
	}
	if _, err := s.BlockTerm(ctx, "  "); !errors.Is(err, ErrInvalidFormat) {
		t.Errorf("blank term err = %v", err)
	}
}

func scoreOf(p api.Profile, id api.CustomFormatID) int {
	for _, s := range p.FormatScores {
		if s.Format == id {
			return s.Score
		}
	}
	return 0
}

func TestQualitySettings(t *testing.T) {
	ctx := context.Background()
	s := newQualityService(t)

	cfg, err := s.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultDelay != 0 || cfg.SettleWindow != DefaultSettleWindow || !cfg.TrashSyncEnabled {
		t.Errorf("defaults = %+v", cfg)
	}

	p, err := s.CreateProfile(ctx, validProfile("Mine"))
	if err != nil {
		t.Fatal(err)
	}
	valid := QualitySettings{DefaultProfile: p.ID, DefaultDelay: 2 * time.Hour, SettleWindow: time.Minute}
	if _, err := s.UpdateSettings(ctx, valid); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Settings(ctx); got != valid {
		t.Errorf("settings = %+v, want %+v", got, valid)
	}

	invalid := map[string]QualitySettings{
		"missing profile":  {DefaultProfile: "nope", SettleWindow: time.Minute},
		"delay too long":   {DefaultProfile: p.ID, DefaultDelay: 25 * time.Hour, SettleWindow: time.Minute},
		"negative delay":   {DefaultProfile: p.ID, DefaultDelay: -time.Minute, SettleWindow: time.Minute},
		"settle too short": {DefaultProfile: p.ID, SettleWindow: time.Second},
		"settle too long":  {DefaultProfile: p.ID, SettleWindow: 3 * time.Minute},
	}
	for name, cfg := range invalid {
		if _, err := s.UpdateSettings(ctx, cfg); !errors.Is(err, ErrInvalidSettings) {
			t.Errorf("%s: err = %v, want ErrInvalidSettings", name, err)
		}
	}

	if err := s.DeleteProfile(ctx, p.ID); !errors.Is(err, ErrDefaultProfile) {
		t.Errorf("deleting the default err = %v, want ErrDefaultProfile", err)
	}
}
