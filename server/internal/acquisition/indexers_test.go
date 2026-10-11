package acquisition_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"magpie/internal/acquisition"
	"magpie/internal/acquisition/api"
	"magpie/internal/acquisition/fakes"
	"magpie/internal/platform/db/dbtest"
	"magpie/internal/platform/settings"
)

const (
	nyaa   = 1
	beyond = 2
)

var (
	movieCaps = acquisition.TorznabCaps{
		MovieSearch: true,
		MovieParams: []acquisition.TorznabParam{acquisition.ParamQuery, acquisition.ParamIMDb},
		Categories:  []int{2000},
	}
	tvCaps = acquisition.TorznabCaps{
		TVSearch:   true,
		TVParams:   []acquisition.TorznabParam{acquisition.ParamQuery, acquisition.ParamTVDB},
		Categories: []int{5000},
	}
)

type fixture struct {
	service  *acquisition.Service
	repo     *acquisition.Repository
	prowlarr *fakes.Prowlarr
	store    *settings.Store
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	d := dbtest.Open(t, acquisition.Migrations())
	repo := acquisition.NewRepository(d)
	store := settings.NewStore(d)
	prowlarr := fakes.NewProwlarr()
	return fixture{service: acquisition.NewService(repo, prowlarr, store), repo: repo, prowlarr: prowlarr, store: store}
}

func (f fixture) sync(t *testing.T) {
	t.Helper()
	if err := f.service.SyncIndexers(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func (f fixture) stored(t *testing.T, id int) acquisition.Indexer {
	t.Helper()
	ix, err := f.repo.Indexer(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return ix
}

func healthy(id int, name string, caps acquisition.TorznabCaps) acquisition.ProwlarrIndexer {
	return acquisition.ProwlarrIndexer{ID: id, Name: name, Privacy: acquisition.PrivacyPublic, Caps: &caps,
		Health: acquisition.ProwlarrHealth{AverageResponse: 420 * time.Millisecond}}
}

func TestSyncAddsNewIndexersWithDefaults(t *testing.T) {
	f := newFixture(t)
	f.prowlarr.AddIndexer(healthy(nyaa, "Nyaa", tvCaps))

	f.sync(t)

	ix := f.stored(t, nyaa)
	if ix.Settings != acquisition.DefaultIndexerSettings() {
		t.Errorf("settings = %+v, want defaults", ix.Settings)
	}
	if ix.Settings.Priority != 25 || ix.Settings.SeedingOverride != acquisition.OverrideUseGlobal {
		t.Errorf("priority %d override %s, want 25 USE_GLOBAL", ix.Settings.Priority, ix.Settings.SeedingOverride)
	}
	if ix.Status != acquisition.IndexerOK || ix.LastOKAt == nil || ix.Name != "Nyaa" {
		t.Errorf("indexer = %+v", ix)
	}
	if !ix.Caps.TVSearch || !slices.Equal(ix.Caps.TVParams, tvCaps.TVParams) || ix.AverageResponse != 420*time.Millisecond {
		t.Errorf("caps or response not stored: %+v", ix)
	}
}

func TestSyncUpdatesCapsAndKeepsSettings(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.prowlarr.AddIndexer(healthy(beyond, "BHD", movieCaps))
	f.sync(t)
	ratio := 1.5
	custom := acquisition.IndexerSettings{Priority: 5, MinSeeders: 3, SeedingOverride: acquisition.OverrideSeedUntilGoals, SeedRatio: &ratio}
	if _, err := f.service.UpdateIndexer(ctx, beyond, custom); err != nil {
		t.Fatal(err)
	}

	both := movieCaps
	both.TVSearch, both.TVParams = tvCaps.TVSearch, tvCaps.TVParams
	f.prowlarr.AddIndexer(healthy(beyond, "BeyondHD", both))
	f.sync(t)

	ix := f.stored(t, beyond)
	if ix.Name != "BeyondHD" || !ix.Caps.TVSearch || !ix.Caps.MovieSearch {
		t.Errorf("indexer not refreshed: %+v", ix)
	}
	if ix.Settings.Priority != 5 || ix.Settings.MinSeeders != 3 || *ix.Settings.SeedRatio != ratio ||
		ix.Settings.SeedingOverride != acquisition.OverrideSeedUntilGoals {
		t.Errorf("settings = %+v, want the user's", ix.Settings)
	}
}

func TestSyncMarksRemovedIndexersGoneAndKeepsTheRow(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.prowlarr.AddIndexer(healthy(nyaa, "Nyaa", tvCaps))
	f.prowlarr.AddIndexer(healthy(beyond, "BeyondHD", movieCaps))
	f.sync(t)
	if _, err := f.service.UpdateIndexer(ctx, nyaa, acquisition.IndexerSettings{Priority: 7, SeedingOverride: acquisition.OverrideDontSeed}); err != nil {
		t.Fatal(err)
	}

	f.prowlarr.RemoveIndexer(nyaa)
	f.sync(t)

	gone := f.stored(t, nyaa)
	if gone.Status != acquisition.IndexerGone || gone.Settings.Priority != 7 || gone.Name != "Nyaa" {
		t.Errorf("removed indexer = %+v, want GONE with its settings", gone)
	}
	list, err := f.service.Indexers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Indexers) != 1 || list.Indexers[0].ProwlarrID != beyond {
		t.Errorf("listed %+v, want only BeyondHD", list.Indexers)
	}

	f.prowlarr.AddIndexer(healthy(nyaa, "Nyaa", tvCaps))
	f.sync(t)
	back := f.stored(t, nyaa)
	if back.Status != acquisition.IndexerOK || back.Settings.Priority != 7 {
		t.Errorf("returning indexer = %+v, want OK with its old settings", back)
	}
}

func TestSyncShowsRepeatedFailuresWithTheLastError(t *testing.T) {
	f := newFixture(t)
	f.prowlarr.AddIndexer(healthy(nyaa, "Nyaa", tvCaps))
	f.sync(t)

	since := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	f.prowlarr.AddIndexer(acquisition.ProwlarrIndexer{ID: nyaa, Name: "Nyaa", Privacy: acquisition.PrivacyPublic,
		Health: acquisition.ProwlarrHealth{Error: "blocked by CloudFlare Protection", FailingSince: &since}})
	f.sync(t)

	failing := f.stored(t, nyaa)
	if failing.Status != acquisition.IndexerFailing || failing.LastError != "blocked by CloudFlare Protection" {
		t.Errorf("indexer = %+v, want FAILING with the error", failing)
	}
	if failing.FailingSince == nil || !failing.FailingSince.Equal(since) {
		t.Errorf("failing since = %v, want %v", failing.FailingSince, since)
	}
	if !failing.Caps.TVSearch {
		t.Error("caps were lost while the caps call failed")
	}
	if failing.LastOKAt == nil {
		t.Error("last OK was cleared")
	}

	f.prowlarr.AddIndexer(healthy(nyaa, "Nyaa", tvCaps))
	f.sync(t)
	recovered := f.stored(t, nyaa)
	if recovered.Status != acquisition.IndexerOK || recovered.LastError != "" || recovered.FailingSince != nil {
		t.Errorf("recovered indexer = %+v", recovered)
	}
}

func TestSyncFailureChangesNothing(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.prowlarr.AddIndexer(healthy(nyaa, "Nyaa", tvCaps))
	f.sync(t)

	f.prowlarr.FailNext(fakes.ProwlarrIndexers, acquisition.ErrUnavailable)
	f.prowlarr.RemoveIndexer(nyaa)
	if err := f.service.SyncIndexers(ctx); !errors.Is(err, acquisition.ErrUnavailable) {
		t.Fatalf("sync error = %v, want ErrUnavailable", err)
	}

	if ix := f.stored(t, nyaa); ix.Status != acquisition.IndexerOK {
		t.Errorf("status = %s after a failed sync, want OK", ix.Status)
	}
	list, err := f.service.Indexers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(list.LastSync.Err, acquisition.ErrUnavailable) || list.LastSync.At.IsZero() {
		t.Errorf("last sync = %+v, want the failure", list.LastSync)
	}
}

func TestUpdateIndexerValidates(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.prowlarr.AddIndexer(healthy(nyaa, "Nyaa", tvCaps))
	f.sync(t)
	ratio, tooMuchRatio := 2.0, 100.5
	days, tooManyDays := 14, 366

	valid := []acquisition.IndexerSettings{
		{Priority: 1, MinSeeders: 0, SeedingOverride: acquisition.OverrideUseGlobal},
		{Priority: 50, MinSeeders: 100, SeedingOverride: acquisition.OverrideSeedUntilGoals, SeedRatio: &ratio, SeedDays: &days},
	}
	for _, s := range valid {
		if _, err := f.service.UpdateIndexer(ctx, nyaa, s); err != nil {
			t.Errorf("UpdateIndexer(%+v) = %v", s, err)
		}
	}

	invalid := []acquisition.IndexerSettings{
		{Priority: 0, SeedingOverride: acquisition.OverrideUseGlobal},
		{Priority: 51, SeedingOverride: acquisition.OverrideUseGlobal},
		{Priority: 25, MinSeeders: -1, SeedingOverride: acquisition.OverrideUseGlobal},
		{Priority: 25, SeedingOverride: "SOMETIMES"},
		{Priority: 25, SeedingOverride: acquisition.OverrideUseGlobal, SeedRatio: &tooMuchRatio},
		{Priority: 25, SeedingOverride: acquisition.OverrideUseGlobal, SeedDays: &tooManyDays},
	}
	for _, s := range invalid {
		if _, err := f.service.UpdateIndexer(ctx, nyaa, s); !errors.Is(err, acquisition.ErrInvalidIndexerSettings) {
			t.Errorf("UpdateIndexer(%+v) = %v, want ErrInvalidIndexerSettings", s, err)
		}
	}

	if _, err := f.service.UpdateIndexer(ctx, 99, acquisition.DefaultIndexerSettings()); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("unknown indexer error = %v, want ErrNotFound", err)
	}
}

func TestIndexerOptionsValidate(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	options, err := f.service.IndexerOptions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if options.SyncInterval.Std() != time.Hour || options.SearchTimeout.Std() != 60*time.Second || options.ResultCache.Std() != 30*time.Minute {
		t.Errorf("defaults = %+v", options)
	}

	options.ResultCache = 0
	if err := f.service.SetIndexerOptions(ctx, options); err != nil {
		t.Errorf("turning the cache off: %v", err)
	}

	invalid := []acquisition.IndexerOptions{
		{SyncInterval: settings.Duration(time.Hour), SearchTimeout: settings.Duration(9 * time.Second)},
		{SyncInterval: settings.Duration(time.Hour), SearchTimeout: settings.Duration(121 * time.Second)},
		{SyncInterval: settings.Duration(time.Hour), SearchTimeout: settings.Duration(time.Minute), ResultCache: settings.Duration(121 * time.Minute)},
		{SyncInterval: settings.Duration(time.Minute), SearchTimeout: settings.Duration(time.Minute)},
	}
	for _, o := range invalid {
		if err := f.service.SetIndexerOptions(ctx, o); !errors.Is(err, acquisition.ErrInvalidIndexerOptions) {
			t.Errorf("SetIndexerOptions(%+v) = %v, want ErrInvalidIndexerOptions", o, err)
		}
	}
}
