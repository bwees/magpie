// Package contracttest holds the acquisition.Reader contract suite and the in-memory Fake that passes it.
// The real repository runs the same suite in its own tests.
package contracttest

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"magpie/internal/acquisition/api"
	"magpie/internal/domain"
)

// FileScore is one file_scores row: Acquisition's own view of a library file.
type FileScore struct {
	File       domain.MediaFileID
	Title      domain.TitleID
	Profile    domain.ProfileID // the Title's profile when the file was last evaluated
	Quality    domain.Quality
	CFScore    int
	CutoffMet  bool
	Transcoded bool
	Origin     domain.Origin
}

// Fixture is the stored state a Reader starts with.
type Fixture struct {
	Profiles   []api.Profile
	Downloads  []api.Download
	Blocklist  []api.BlocklistEntry
	FileScores []FileScore
}

// Factory returns a Reader holding exactly the fixture's state.
type Factory func(t *testing.T, f Fixture) api.Reader

const (
	blocklistExpiry = 24 * time.Hour
	storedCFScore   = 1700
)

var (
	start    = time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)
	web1080p = domain.Quality{ID: 3, Name: "WEBDL-1080p", Source: "WEB", Resolution: 1080}
)

// Run checks a Reader against the acquisition contract.
func Run(t *testing.T, newReader Factory) {
	ctx := context.Background()

	p1080 := profile("1080p", true)
	p2160 := profile("2160p", true)
	locked := profile("archive", false)

	movie, series := domain.NewID[domain.TitleID](), domain.NewID[domain.TitleID]()
	oldGrab := download(movie, start, nil)
	removedAt := start.Add(time.Hour)
	removed := download(movie, start.Add(time.Minute), &removedAt)
	newGrab := download(series, start.Add(2*time.Minute), nil)

	expires := start.Add(blocklistExpiry)
	olderBlock := blocked(movie, start, api.BlocklistBlockedFile, nil)
	newerBlock := blocked(movie, start.Add(time.Hour), api.BlocklistMetadataTimeout, &expires)
	seriesBlock := blocked(series, start.Add(2*time.Hour), api.BlocklistUser, nil)

	upgradable := fileScore(movie, p1080.ID, false, false)
	transcoded := fileScore(movie, p1080.ID, true, true)
	noUpgrades := fileScore(series, locked.ID, true, false)

	fixture := Fixture{
		Profiles:   []api.Profile{p2160, locked, p1080},
		Downloads:  []api.Download{oldGrab, removed, newGrab},
		Blocklist:  []api.BlocklistEntry{olderBlock, seriesBlock, newerBlock},
		FileScores: []FileScore{upgradable, transcoded, noUpgrades},
	}

	t.Run("Profile returns the profile and ErrNotFound for an unknown ID", func(t *testing.T) {
		r := newReader(t, fixture)
		got, err := r.Profile(ctx, p1080.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != p1080.Name || len(got.Items) != len(p1080.Items) || got.CutoffFormatScore != p1080.CutoffFormatScore {
			t.Errorf("Profile = %+v, want %+v", got, p1080)
		}
		if _, err := r.Profile(ctx, domain.NewID[domain.ProfileID]()); !errors.Is(err, api.ErrNotFound) {
			t.Errorf("unknown profile err = %v, want ErrNotFound", err)
		}
	})

	t.Run("Profiles lists every profile ordered by name", func(t *testing.T) {
		r := newReader(t, fixture)
		got, err := r.Profiles(ctx)
		if err != nil {
			t.Fatal(err)
		}
		names := make([]string, 0, len(got))
		for _, p := range got {
			names = append(names, p.Name)
		}
		if want := []string{p1080.Name, p2160.Name, locked.Name}; !slices.Equal(names, want) {
			t.Errorf("Profiles = %v, want %v", names, want)
		}
	})

	t.Run("ActiveDownloads excludes removed downloads, newest first", func(t *testing.T) {
		r := newReader(t, fixture)
		got, err := r.ActiveDownloads(ctx)
		if err != nil {
			t.Fatal(err)
		}
		assertDownloads(t, got, newGrab.ID, oldGrab.ID)
	})

	t.Run("DownloadsForTitle includes removed downloads, newest first", func(t *testing.T) {
		r := newReader(t, fixture)
		got, err := r.DownloadsForTitle(ctx, movie)
		if err != nil {
			t.Fatal(err)
		}
		assertDownloads(t, got, removed.ID, oldGrab.ID)
	})

	t.Run("Blocklist filters by title, lists all for an empty title, newest first", func(t *testing.T) {
		r := newReader(t, fixture)
		got, err := r.Blocklist(ctx, movie)
		if err != nil {
			t.Fatal(err)
		}
		assertEntries(t, got, newerBlock.ID, olderBlock.ID)

		got, err = r.Blocklist(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		assertEntries(t, got, seriesBlock.ID, newerBlock.ID, olderBlock.ID)
	})

	t.Run("CutoffStatus reports the stored score and when upgrades stop", func(t *testing.T) {
		r := newReader(t, fixture)
		cases := []struct {
			file           FileScore
			wantUpgradesOK bool
		}{
			{upgradable, true},
			{transcoded, false},
			{noUpgrades, false},
		}
		for _, tc := range cases {
			got, err := r.CutoffStatus(ctx, tc.file.File)
			if err != nil {
				t.Fatal(err)
			}
			want := api.CutoffStatus{File: tc.file.File, Quality: tc.file.Quality, CFScore: tc.file.CFScore, CutoffMet: tc.file.CutoffMet, UpgradesAllowed: tc.wantUpgradesOK}
			if got != want {
				t.Errorf("CutoffStatus = %+v, want %+v", got, want)
			}
		}
		if _, err := r.CutoffStatus(ctx, domain.NewID[domain.MediaFileID]()); !errors.Is(err, api.ErrNotFound) {
			t.Errorf("unknown file err = %v, want ErrNotFound", err)
		}
	})
}

func profile(name string, upgradeAllowed bool) api.Profile {
	return api.Profile{
		ID:             domain.NewID[domain.ProfileID](),
		Name:           name,
		UpgradeAllowed: upgradeAllowed,
		Items:          []api.ProfileItem{{Qualities: []domain.Quality{web1080p}, Allowed: true}},
		PropersMode:    api.PropersPreferAndUpgrade,
	}
}

func download(title domain.TitleID, grabbedAt time.Time, removedAt *time.Time) api.Download {
	return api.Download{
		ID:          domain.NewID[domain.DownloadID](),
		Title:       title,
		Episodes:    []domain.EpisodeID{},
		ReleaseName: "Release.1080p.WEB-DL",
		Quality:     web1080p,
		State:       api.StateDownloading,
		SeedingMode: api.SeedUntilGoals,
		GrabbedAt:   grabbedAt,
		RemovedAt:   removedAt,
	}
}

type blocklistID string

func blocked(title domain.TitleID, at time.Time, reason api.BlocklistReason, expires *time.Time) api.BlocklistEntry {
	return api.BlocklistEntry{
		ID:          string(domain.NewID[blocklistID]()),
		Title:       title,
		InfoHash:    string(domain.NewID[blocklistID]()),
		ReleaseName: "Blocked.Release",
		Reason:      reason,
		At:          at,
		ExpiresAt:   expires,
	}
}

func fileScore(title domain.TitleID, profile domain.ProfileID, cutoffMet, isTranscoded bool) FileScore {
	return FileScore{
		File:       domain.NewID[domain.MediaFileID](),
		Title:      title,
		Profile:    profile,
		Quality:    web1080p,
		CFScore:    storedCFScore,
		CutoffMet:  cutoffMet,
		Transcoded: isTranscoded,
		Origin:     domain.OriginDownload,
	}
}

func assertDownloads(t *testing.T, got []api.Download, want ...domain.DownloadID) {
	t.Helper()
	ids := make([]domain.DownloadID, 0, len(got))
	for _, d := range got {
		ids = append(ids, d.ID)
	}
	if !slices.Equal(ids, want) {
		t.Errorf("downloads = %v, want %v", ids, want)
	}
}

func assertEntries(t *testing.T, got []api.BlocklistEntry, want ...string) {
	t.Helper()
	ids := make([]string, 0, len(got))
	for _, e := range got {
		ids = append(ids, e.ID)
	}
	if !slices.Equal(ids, want) {
		t.Errorf("blocklist = %v, want %v", ids, want)
	}
}
