package migrate_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	acquisitionapi "magpie/internal/acquisition/api"
	acquisitiontest "magpie/internal/acquisition/api/contracttest"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	librarytest "magpie/internal/library/api/contracttest"
	"magpie/internal/platform/migrate"
	"magpie/internal/platform/migrate/fakes"
)

// Migration seed 1.
func TestARadarrMovieWithATmdbIDMatchesWithoutFind(t *testing.T) {
	h := newHarness(t)
	h.arr.Put(migrate.ArrLibrary{App: migrate.AppSonarr, Version: sonarrVersion})

	plan := h.dryRun()

	movie := titleOf(plan, "radarr:40")
	if movie.Status != migrate.TitleMatched || movie.Tmdb != conclaveTmdb {
		t.Fatalf("movie = %+v, want matched to %d", movie, conclaveTmdb)
	}
	if n := h.tmdb.Called(fakes.TMDBFind); n != 0 {
		t.Fatalf("TMDB /find called %d times, want none", n)
	}
}

// Migration seed 2.
func TestASonarrSeriesWithoutTmdbIDMatchesThroughTvdbFind(t *testing.T) {
	h := newHarness(t)
	plan := h.dryRun()

	series := titleOf(plan, "sonarr:1")
	if series.Status != migrate.TitleMatched || series.Tmdb != blueyTmdb || series.External.Tvdb == nil || *series.External.Tvdb != "371980" {
		t.Fatalf("series = %+v, want matched to %d with TVDB 371980", series, blueyTmdb)
	}
	h.clearReviews()
	h.commit()
	created := h.catalog.Created()
	i := slices.IndexFunc(created, func(c catalogapi.ImportTitle) bool { return c.Source == "sonarr:1" })
	if i < 0 || created[i].External.Tvdb == nil || *created[i].External.Tvdb != "371980" || created[i].Tmdb != blueyTmdb {
		t.Fatalf("imported titles = %+v, want Bluey with External.Tvdb 371980", created)
	}
}

// Migration seed 3.
func TestASeasonCountMismatchPreselectsTheMatchingDVDGroup(t *testing.T) {
	h := newHarness(t)
	plan := h.dryRun()

	i := slices.IndexFunc(plan.Reviews.Numbering, func(r migrate.NumberingReview) bool { return r.ID == "sonarr:2" })
	if i < 0 {
		t.Fatalf("numbering reviews = %+v, want Firefly listed", plan.Reviews.Numbering)
	}
	review := plan.Reviews.Numbering[i]
	if review.Suggested != fireflyDVD || review.Decision != migrate.NumberingPending || review.Groups[0].Score != 1 {
		t.Fatalf("review = %+v, want the DVD group preselected with a full score, still pending", review)
	}
	firefly := titleOf(plan, "sonarr:2")
	if firefly.EpisodeGroup != fireflyDVD || !slices.ContainsFunc(firefly.Episodes, func(e migrate.PlannedEpisode) bool {
		return e.ArrSeason == 2 && e.ArrNumber == 1 && e.Mapped && e.Season == 2 && e.Number == 1
	}) {
		t.Fatalf("Firefly = %+v, want S02E01 on the DVD group's S02E01", firefly.Episodes)
	}
}

// Migration seed 4.
func TestSonarrQuality20IsImportedAsRemux1080p(t *testing.T) {
	h := newHarness(t)
	h.dryRun()
	h.clearReviews()
	if s := h.commit(); s.Phase != migrate.PhaseDone {
		t.Fatalf("commit ended in %s: %s", s.Phase, s.Error)
	}

	files := h.library.Files()
	i := slices.IndexFunc(files, func(f libraryapi.ImportMediaFile) bool { return f.Source == "sonarr:episodefile:11" })
	if i < 0 || files[i].Quality.ID != 30 || files[i].Quality.Name != "Remux-1080p" {
		t.Fatalf("files = %+v, want Bluey's file as quality 30 Remux-1080p", files)
	}
	if files[i].ReleaseName != "Bluey.2018.S01E01.1080p.BluRay.REMUX-GRP" || files[i].Path != blueyVideoTo {
		t.Fatalf("file = %+v, want the scene name kept and the path rewritten", files[i])
	}
}

// Migration seed 5.
func TestABlurayRawSourceAloneBecomesBlurayAndRemux(t *testing.T) {
	h := newHarness(t)
	plan := h.dryRun()

	i := slices.IndexFunc(plan.Formats, func(f migrate.PlannedFormat) bool { return f.Source == "sonarr:customformat:3" })
	if i < 0 {
		t.Fatalf("formats = %+v, want Sonarr's Remux", plan.Formats)
	}
	got := plan.Formats[i]
	want := []acquisitionapi.FormatSpec{
		{Kind: acquisitionapi.SpecSource, Name: "BlurayRaw", Value: "BLURAY"},
		{Kind: acquisitionapi.SpecQualityModifier, Name: "BlurayRaw", Value: "REMUX"},
	}
	if !slices.Equal(got.Specs, want) || len(got.Pending) != 0 {
		t.Fatalf("specs = %+v pending %+v, want %+v and nothing to review", got.Specs, got.Pending, want)
	}
	if slices.ContainsFunc(plan.Reviews.Specs, func(r migrate.SpecReview) bool { return r.Format == got.Source }) {
		t.Fatalf("spec reviews = %+v, want none for Remux", plan.Reviews.Specs)
	}
}

// Migration seed 6.
func TestSameNamedProfilesWithDifferentItemsBlockCommitUntilOneIsPicked(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	plan := h.dryRun()

	i := slices.IndexFunc(plan.Reviews.Clashes, func(c migrate.Clash) bool { return c.Kind == migrate.ClashProfile && c.Name == hdProfileName })
	if i < 0 {
		t.Fatalf("clashes = %+v, want HD-1080p listed", plan.Reviews.Clashes)
	}
	clash := plan.Reviews.Clashes[i]
	for _, q := range []migrate.ReviewQueue{migrate.QueueTitles, migrate.QueueNumbering, migrate.QueueSpecs, migrate.QueueSeriesTypes} {
		if _, err := h.service.SkipQueue(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := h.service.SetCutover(ctx, true); err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.StartCommit(ctx); !errors.Is(err, migrate.ErrNotReady) {
		t.Fatalf("commit with the clash open = %v, want ErrNotReady", err)
	}

	s, err := h.service.Decide(ctx, migrate.Decision{Queue: migrate.QueueClashes, ID: clash.ID, Clash: migrate.ClashRadarr})
	if err != nil {
		t.Fatal(err)
	}
	if !s.CanCommit() {
		t.Fatalf("pending = %v, want commit allowed once Radarr's version is picked", s.Plan.Reviews.Pending())
	}
	if s := h.commit(); s.Phase != migrate.PhaseDone {
		t.Fatalf("commit ended in %s: %s", s.Phase, s.Error)
	}
	ids := map[domain.ProfileID]bool{}
	for _, created := range h.catalog.Created() {
		ids[created.Profile] = true
	}
	if len(ids) != 1 {
		t.Fatalf("titles use profiles %v, want both apps' titles on the one merged HD-1080p", ids)
	}
	for id := range ids {
		profile, ok := h.acquisition.Profile(id)
		if !ok || len(profile.Items) != 2 {
			t.Fatalf("profile = %+v, want Radarr's two qualities", profile)
		}
	}
}

// Migration seed 7.
func TestABlocklistRowTakesTheHashOfTheMatchingGrab(t *testing.T) {
	h := newHarness(t)
	h.dryRun()
	h.clearReviews()
	h.commit()

	blocked := h.acquisition.Blocklist()
	if len(blocked) != 1 || blocked[0].InfoHash != "abcdef0123456789abcdef0123456789abcdef01" || blocked[0].Indexer != "Tracker" || blocked[0].ReleaseName != blockedRelease {
		t.Fatalf("blocklist = %+v, want the grab's hash and the indexer without its Prowlarr suffix", blocked)
	}
	if rows := h.activity.Rows(); len(rows) != 2 {
		t.Fatalf("history = %+v, want the grab and the failure", rows)
	}
}

// Migration seed 9.
func TestCommitIsDisabledUntilTheCutoverIsConfirmed(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.dryRun()
	for _, q := range migrate.ReviewQueues {
		if _, err := h.service.SkipQueue(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if s := h.state(); s.CanCommit() {
		t.Fatal("commit is allowed with the cutover checkbox unticked")
	}
	if _, err := h.service.StartCommit(ctx); !errors.Is(err, migrate.ErrNotReady) {
		t.Fatalf("commit = %v, want ErrNotReady", err)
	}
	s, err := h.service.SetCutover(ctx, true)
	if err != nil || !s.CanCommit() {
		t.Fatalf("after ticking the checkbox: %v, can commit %t", err, s.CanCommit())
	}
}

// Migration seed 10.
func TestACommitThatFailsInLibraryResumesWithoutRedoingEarlierSteps(t *testing.T) {
	h := newHarness(t)
	h.dryRun()
	h.clearReviews()
	h.library.FailNext(librarytest.ImporterImportMediaFiles, errors.New("disk unplugged"))

	failed := h.commit()
	if failed.Phase != migrate.PhaseCommitFailed || failed.FailedStep != migrate.StepFiles || !failed.CanCommit() {
		t.Fatalf("commit = %s at %s (%s), want a resumable failure at MEDIA_FILES", failed.Phase, failed.FailedStep, failed.Error)
	}
	titles, formats := len(h.catalog.Created()), h.acquisition.Called(acquisitiontest.ImporterImportCustomFormats)

	resumed := h.commit()
	if resumed.Phase != migrate.PhaseDone {
		t.Fatalf("resumed commit ended in %s: %s", resumed.Phase, resumed.Error)
	}
	if got := len(h.catalog.Created()); got != titles {
		t.Fatalf("resume created %d more titles, want none", got-titles)
	}
	if h.acquisition.Called(acquisitiontest.ImporterImportCustomFormats) != formats+1 {
		t.Fatal("resume didn't ask Acquisition for the format IDs again")
	}
	if files := h.library.Files(); len(files) != 3 {
		t.Fatalf("library files = %+v, want all three linked after the resume", files)
	}
	if blocked := h.acquisition.Blocklist(); len(blocked) != 1 {
		t.Fatalf("blocklist = %+v, want one row", blocked)
	}
}
