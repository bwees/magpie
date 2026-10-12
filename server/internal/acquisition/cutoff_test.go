package acquisition_test

import (
	"context"
	"errors"
	"testing"

	"magpie/internal/acquisition"
	"magpie/internal/acquisition/api"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/parse"
)

const (
	upgradeUntil  = 2000
	highTierScore = 2300
	lowTierScore  = 1700

	fileSize      = 8 << 30
	cutoffAt2160p = 0
	cutoffAt1080p = 1

	high2160p   = "Movie.Title.2024.2160p.WEB-DL.DDP5.1.H.265-HIGH"
	low2160p    = "Movie.Title.2024.2160p.WEB-DL.DDP5.1.H.265-LOW"
	high1080p   = "Movie.Title.2024.1080p.WEB-DL.DDP5.1.H.264-HIGH"
	low1080p    = "Movie.Title.2024.1080p.WEB-DL.DDP5.1.H.264-LOW"
	bluray1080p = "Movie.Title.2024.1080p.BluRay.x264-GRP"
)

// useScoredProfile gives the title a profile ordering WEB 2160p above WEB 1080p with upgrade-until 2000.
// Releases from HIGH score 2300 and from LOW 1700.
func (f searchFixture) useScoredProfile(t *testing.T, cutoff int) api.Profile {
	t.Helper()
	ctx := context.Background()
	var scores []api.FormatScore
	for name, score := range map[string]int{"HIGH": highTierScore, "LOW": lowTierScore} {
		format, err := f.quality.CreateFormat(ctx, acquisition.CustomFormat{
			Name: "Tier " + name, Specs: []api.FormatSpec{{Kind: api.SpecReleaseGroup, Value: "^" + name + "$"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		scores = append(scores, api.FormatScore{Format: format.ID, Name: format.Name, Score: score})
	}
	profile, err := f.quality.CreateProfile(ctx, api.Profile{
		Name: "Scored", UpgradeAllowed: true, PropersMode: api.PropersPreferAndUpgrade,
		Items: []api.ProfileItem{
			{Qualities: []domain.Quality{profileQuality(parse.QualityWEBDL2160p)}, Allowed: true},
			{Qualities: []domain.Quality{profileQuality(parse.QualityWEBDL1080p)}, Allowed: true},
		},
		Cutoff: cutoff, CutoffFormatScore: upgradeUntil, MinUpgradeFormatScore: 1, FormatScores: scores,
	})
	if err != nil {
		t.Fatal(err)
	}
	f.title.Profile = profile.ID
	f.catalog.PutTitle(f.title)
	return profile
}

func (f searchFixture) presetProfile(t *testing.T, preset api.QualityPreset) domain.ProfileID {
	t.Helper()
	profiles, err := f.quality.Profiles(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range profiles {
		if p.Preset == preset {
			return p.ID
		}
	}
	t.Fatalf("no %s preset", preset)
	return ""
}

func (f searchFixture) fileImported(name string, id parse.QualityID, origin domain.Origin) libraryapi.FileImported {
	return libraryapi.FileImported{
		MediaFile: domain.NewID[domain.MediaFileID](), Title: f.title.ID, Path: "/movies/Movie Title (2024)/Movie Title (2024).mkv",
		Quality: profileQuality(id), ReleaseName: name, Size: fileSize, Origin: origin, Episodes: []domain.EpisodeRef{},
	}
}

func (f searchFixture) deliverScores(t *testing.T, m domain.Message) {
	t.Helper()
	f.deliver(t, acquisition.NewFileScoresHandler(f.cutoff), bus.EventOf(m))
}

func (f searchFixture) cutoffMet(t *testing.T) []api.CutoffMet {
	t.Helper()
	var met []api.CutoffMet
	for _, ev := range f.events.ofType(api.TypeCutoffMet) {
		met = append(met, payload[api.CutoffMet](t, ev))
	}
	return met
}

func (f searchFixture) cutoffStatus(t *testing.T, file domain.MediaFileID) (api.CutoffStatus, error) {
	t.Helper()
	return acquisition.NewReader(acquisition.NewQualityRepository(f.db), f.repo).CutoffStatus(context.Background(), file)
}

// Seed acquisition #15.
func TestAFileMeetingQualityAndUpgradeUntilMeetsTheCutoff(t *testing.T) {
	f := newSearchFixture(t)
	f.useScoredProfile(t, cutoffAt2160p)
	file := f.fileImported(high2160p, parse.QualityWEBDL2160p, domain.OriginDownload)

	f.deliverScores(t, file)

	met := f.cutoffMet(t)
	want := api.CutoffMet{File: file.MediaFile, Title: f.title.ID, Episodes: []domain.EpisodeID{},
		Quality: profileQuality(parse.QualityWEBDL2160p), CFScore: highTierScore, Origin: domain.OriginDownload}
	if len(met) != 1 || met[0].File != want.File || met[0].CFScore != want.CFScore || met[0].Origin != want.Origin || met[0].Quality.ID != want.Quality.ID {
		t.Fatalf("CutoffMet = %+v, want %+v", met, want)
	}
	if status, err := f.cutoffStatus(t, file.MediaFile); err != nil || !status.CutoffMet || !status.UpgradesAllowed {
		t.Errorf("status = %+v, %v; want cutoff met, upgrades still allowed", status, err)
	}
}

// Seed acquisition #15.
func TestAFileBelowUpgradeUntilDoesNotMeetTheCutoff(t *testing.T) {
	f := newSearchFixture(t)
	f.useScoredProfile(t, cutoffAt2160p)
	file := f.fileImported(low2160p, parse.QualityWEBDL2160p, domain.OriginDownload)

	f.deliverScores(t, file)

	if met := f.cutoffMet(t); len(met) != 0 {
		t.Fatalf("CutoffMet = %+v, want none for CF 1700", met)
	}
	status, err := f.cutoffStatus(t, file.MediaFile)
	if err != nil || status.CutoffMet || status.CFScore != lowTierScore || !status.UpgradesAllowed {
		t.Errorf("status = %+v, %v; want CF 1700 below the cutoff with upgrades allowed", status, err)
	}
}

// Seed acquisition #14.
func TestARedeliveredFileImportedPublishesCutoffMetOnce(t *testing.T) {
	f := newSearchFixture(t)
	f.useScoredProfile(t, cutoffAt2160p)
	file := f.fileImported(high2160p, parse.QualityWEBDL2160p, domain.OriginDownload)

	f.deliverScores(t, file)
	f.deliverScores(t, file)

	if met := f.cutoffMet(t); len(met) != 1 {
		t.Errorf("CutoffMet published %d times, want once", len(met))
	}
}

// Seed acquisition #31.
func TestABlurayFileMeetsThe1080pPresetCutoffWithNoScore(t *testing.T) {
	f := newSearchFixture(t)
	f.title.Profile = f.presetProfile(t, api.Preset1080p)
	f.catalog.PutTitle(f.title)
	file := f.fileImported(bluray1080p, parse.QualityBluray1080p, domain.OriginDownload)

	f.deliverScores(t, file)

	if met := f.cutoffMet(t); len(met) != 1 || met[0].CFScore != 0 {
		t.Errorf("CutoffMet = %+v, want one with CF 0", met)
	}
}

// Seed acquisition #33.
func TestMovingATitleToALowerPresetMeetsTheCutoffOnce(t *testing.T) {
	f := newSearchFixture(t)
	uhd, hd := f.presetProfile(t, api.Preset2160p), f.presetProfile(t, api.Preset1080p)
	f.title.Profile = uhd
	f.catalog.PutTitle(f.title)
	file := f.fileImported(acceptedName, parse.QualityWEBDL1080p, domain.OriginDownload)
	f.deliverScores(t, file)
	if met := f.cutoffMet(t); len(met) != 0 {
		t.Fatalf("CutoffMet = %+v under 2160p, want none", met)
	}

	f.title.Profile = hd
	f.catalog.PutTitle(f.title)
	changed := catalogapi.ProfileChanged{Title: f.title.ID, From: uhd, To: hd}
	f.deliverScores(t, changed)
	f.deliverScores(t, changed)

	if met := f.cutoffMet(t); len(met) != 1 || met[0].File != file.MediaFile {
		t.Errorf("CutoffMet = %+v, want the file once", met)
	}
}

// Seed acquisition #29.
func TestAScannedFileMeetingTheCutoffCarriesItsOrigin(t *testing.T) {
	f := newSearchFixture(t)
	f.useScoredProfile(t, cutoffAt2160p)
	file := f.fileImported(high2160p, parse.QualityWEBDL2160p, domain.OriginLibraryScan)

	f.deliverScores(t, file)

	if met := f.cutoffMet(t); len(met) != 1 || met[0].Origin != domain.OriginLibraryScan {
		t.Errorf("CutoffMet = %+v, want Origin LIBRARY_SCAN", met)
	}
}

// Seed acquisition #30.
func TestAnUpgradeLeavesOnlyTheNewFileScored(t *testing.T) {
	f := newSearchFixture(t)
	f.useScoredProfile(t, cutoffAt2160p)
	old := f.fileImported(low2160p, parse.QualityWEBDL2160p, domain.OriginDownload)
	f.deliverScores(t, old)

	upgrade := f.fileImported(high2160p, parse.QualityWEBDL2160p, domain.OriginDownload)
	upgrade.Replaces = &old.MediaFile
	deleted := libraryapi.FileDeleted{MediaFile: old.MediaFile, Title: f.title.ID, Path: old.Path, Reason: libraryapi.DeleteUpgraded}
	f.deliverScores(t, upgrade)
	f.deliverScores(t, deleted)
	f.deliverScores(t, deleted)

	if _, err := f.cutoffStatus(t, old.MediaFile); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("old file status err = %v, want ErrNotFound", err)
	}
	if status, err := f.cutoffStatus(t, upgrade.MediaFile); err != nil || !status.CutoffMet {
		t.Errorf("new file status = %+v, %v; want it scored and meeting the cutoff", status, err)
	}
}

func TestATranscodedFileStopsUpgradingAndKeepsItsScore(t *testing.T) {
	f := newSearchFixture(t)
	f.useScoredProfile(t, cutoffAt2160p)
	file := f.fileImported(high2160p, parse.QualityWEBDL2160p, domain.OriginDownload)
	f.deliverScores(t, file)

	f.deliverScores(t, libraryapi.FileTranscoded{MediaFile: file.MediaFile, Path: "/movies/Movie Title (2024)/Movie Title (2024).mkv"})

	status, err := f.cutoffStatus(t, file.MediaFile)
	if err != nil || status.UpgradesAllowed || status.CFScore != highTierScore {
		t.Errorf("status = %+v, %v; want upgrades stopped and CF 2300 kept", status, err)
	}
}
