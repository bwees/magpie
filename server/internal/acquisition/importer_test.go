package acquisition

import (
	"context"
	"testing"
	"time"

	"magpie/internal/acquisition/api"
	"magpie/internal/acquisition/api/contracttest"
	"magpie/internal/domain"
	"magpie/internal/platform/db/dbtest"
	"magpie/internal/platform/settings"

	"gorm.io/gorm"
)

const (
	syncedFormatName = "x265 (HD)"
	syncedTrashID    = "trash-x265"
	migratedDelay    = 2 * time.Hour
	shorterDelay     = 15 * time.Minute
)

func TestImporterPassesTheContract(t *testing.T) {
	contracttest.RunImporter(t, func(t *testing.T) api.Importer {
		return NewImporter(dbtest.Open(t, Migrations()))
	})
}

func TestImportedFormatNamedLikeASyncedOneKeepsTheSyncedOne(t *testing.T) {
	d := dbtest.Open(t, Migrations())
	ctx := context.Background()
	synced := CustomFormat{
		ID: domain.NewID[api.CustomFormatID](), Name: syncedFormatName, TrashID: syncedTrashID,
		Specs: []api.FormatSpec{{Kind: api.SpecReleaseTitle, Name: "x265", Value: "x265"}},
	}
	repo := NewQualityRepository(d)
	if err := d.Write(ctx, func(tx *gorm.DB) error { return saveFormat(tx, synced, time.Now()) }); err != nil {
		t.Fatal(err)
	}

	got, err := NewImporter(d).ImportCustomFormats(ctx, []api.ImportCustomFormat{{
		Source: "radarr:customformat:9", Name: "X265 (hd)",
		Specs: []api.FormatSpec{{Kind: api.SpecReleaseTitle, Name: "other", Value: "hevc"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	formats, err := repo.Formats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got["radarr:customformat:9"] != synced.ID || len(formats) != 1 || formats[0].TrashID != syncedTrashID || formats[0].Specs[0].Value != "x265" {
		t.Fatalf("got %v and formats %+v, want the synced format kept and reused", got, formats)
	}
}

func TestImportedProfileKeepsItsItemsCutoffAndScores(t *testing.T) {
	d := dbtest.Open(t, Migrations())
	ctx := context.Background()
	importer := NewImporter(d)
	formats, err := importer.ImportCustomFormats(ctx, []api.ImportCustomFormat{{
		Source: "sonarr:customformat:1", Name: "Remux",
		Specs: []api.FormatSpec{{Kind: api.SpecQualityModifier, Name: "Remux", Value: "REMUX"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	remux := formats["sonarr:customformat:1"]
	sizes := api.SizeLimits{MinMBPerMin: 1, PreferredMBPerMin: 100, MaxMBPerMin: 200}
	profiles, err := importer.ImportQualityProfiles(ctx, []api.ImportQualityProfile{{
		Source: "sonarr:qualityprofile:6", Name: "Ultra-HD", UpgradeAllowed: true,
		Items: []api.ImportProfileItem{
			{Group: "WEB 2160p", Qualities: []domain.Quality{{ID: 18}, {ID: 17}}, Allowed: true, Sizes: sizes},
			{Qualities: []domain.Quality{{ID: 31}}, Allowed: true, Sizes: sizes},
			{Qualities: []domain.Quality{{ID: 16}}, Allowed: false, Sizes: sizes},
		},
		Cutoff: 1, CutoffFormatScore: 500, FormatScores: map[api.CustomFormatID]int{remux: 300},
	}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewQualityRepository(d).Profile(ctx, profiles["sonarr:qualityprofile:6"])
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Items) != 3 || p.Items[0].Group == nil || p.Items[0].Group.Name != "WEB 2160p" || p.Cutoff != 1 || p.Items[1].Qualities[0].ID != 31 {
		t.Fatalf("profile = %+v, want the group first and Remux-2160p as cutoff", p)
	}
	if p.Items[2].Allowed || p.Items[1].Sizes != sizes || p.CutoffFormatScore != 500 {
		t.Fatalf("profile = %+v, want HDTV-2160p not allowed and sizes copied", p)
	}
	if len(p.FormatScores) != 1 || p.FormatScores[0].Format != remux || p.FormatScores[0].Score != 300 {
		t.Fatalf("scores = %+v, want Remux at 300", p.FormatScores)
	}
}

func TestImportedDelayIsTheShortestUntaggedOne(t *testing.T) {
	d := dbtest.Open(t, Migrations())
	ctx := context.Background()
	err := NewImporter(d).ImportDelayProfiles(ctx, []api.ImportDelayProfile{
		{Source: "sonarr:delayprofile:1", Delay: migratedDelay},
		{Source: "radarr:delayprofile:1", Delay: shorterDelay + time.Minute},
		{Source: "radarr:delayprofile:2", Delay: shorterDelay - time.Minute, Tags: []string{"anime"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := DefaultDelayKey.Get(ctx, settings.NewStore(d))
	if err != nil {
		t.Fatal(err)
	}
	if got.Std() != shorterDelay+time.Minute {
		t.Fatalf("default delay = %v, want %v", got.Std(), shorterDelay+time.Minute)
	}
}

func TestImportedBlocklistRowsAreStoredOnceAsPermanentMigratedEntries(t *testing.T) {
	d := dbtest.Open(t, Migrations())
	ctx := context.Background()
	title := domain.NewID[domain.TitleID]()
	rows := []api.ImportBlockedRelease{
		{Source: "sonarr:blocklist:1", Title: title, InfoHash: "ABCDEF", ReleaseName: "Show.S01E01-GRP", Indexer: "Tracker", At: time.Now()},
		{Source: "sonarr:blocklist:2", Title: title, ReleaseName: "Show.S01E02-GRP", Indexer: "Tracker", At: time.Now()},
	}
	importer := NewImporter(d)
	for range 2 {
		if err := importer.ImportBlocklist(ctx, rows); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := NewRepository(d).Blocklist(ctx, title)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %+v, want 2", entries)
	}
	for _, e := range entries {
		if e.Reason != api.BlocklistMigrated || e.ExpiresAt != nil {
			t.Fatalf("entry = %+v, want a permanent MIGRATED entry", e)
		}
		if e.ReleaseName == rows[0].ReleaseName && e.InfoHash != "abcdef" {
			t.Fatalf("hash = %q, want it lower-cased", e.InfoHash)
		}
	}
}
