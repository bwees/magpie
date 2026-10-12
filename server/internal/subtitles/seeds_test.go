package subtitles_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	librarytest "magpie/internal/library/api/contracttest"
	requestsapi "magpie/internal/requests/api"
	"magpie/internal/subtitles"
	"magpie/internal/subtitles/api"
	"magpie/internal/subtitles/fakes"
)

const (
	embeddedEnglishIndex = 2
	embeddedSpanishIndex = 3
	quotaWait            = 3 * time.Hour
)

func englishText() libraryapi.Stream {
	return libraryapi.Stream{Index: embeddedEnglishIndex, Kind: libraryapi.StreamSubtitle, Codec: "subrip", Language: english}
}

func spanishImage() libraryapi.Stream {
	return libraryapi.Stream{Index: embeddedSpanishIndex, Kind: libraryapi.StreamSubtitle, Codec: "hdmv_pgs_subtitle", Language: spanish, Image: true}
}

// Seed 1.
func TestEmbeddedTrackSatisfiesAWantedLanguage(t *testing.T) {
	h := newHarness(t, english)
	file := h.episodeFile(englishText())
	h.imported(file)

	subs := h.view(file.ID).Subtitles
	if len(subs) != 1 || subs[0].Source != api.SourceEmbedded || subs[0].Language != english || subs[0].Path != "" {
		t.Fatalf("subtitles = %+v, want one embedded English row", subs)
	}
	if got := h.coverage(file.ID); got[string(english)] != api.CoverageSatisfied {
		t.Errorf("coverage = %v, want English satisfied", got)
	}
	if n := h.searchesQueued(file.ID); n != 0 {
		t.Errorf("%d searches queued, want none", n)
	}
	h.runDue()
	if n := h.os.Called(fakes.OpenSubtitlesSearch); n != 0 {
		t.Errorf("%d searches ran, want none", n)
	}
	if events := h.events.all(); len(events) != 0 {
		t.Errorf("published %v, want nothing", events)
	}
}

// Seed 2.
func TestHashMatchIsDownloadedSyncedAndWritten(t *testing.T) {
	h := newHarness(t, english)
	file := h.episodeFile()
	h.os.AddCandidates(h.episodeQuery(english), perfectEpisode(11, english))
	h.os.SetContent(11, srt)
	h.imported(file)
	h.runDue()

	const want = "/tv/Show (2011)/Season 01/Show S01E02.en.srt"
	if content, ok := h.sidecars.Read(want); !ok || string(content) != string(srt) {
		t.Fatalf("sidecar %s = %q, %v", want, content, ok)
	}
	added := ofType[api.SubtitleAdded](t, h.events, api.TypeSubtitleAdded)
	wantAdded := api.SubtitleAdded{
		MediaFile: file.ID, Title: h.show.ID, Episodes: []domain.EpisodeID{h.episode.ID},
		Language: english, Source: api.SourceProvider, Synced: true,
	}
	if len(added) != 1 || !reflect.DeepEqual(added[0], wantAdded) {
		t.Fatalf("SubtitleAdded = %+v, want %+v", added, wantAdded)
	}
	subs := h.view(file.ID).Subtitles
	if len(subs) != 1 || subs[0].SyncOffsetMs == nil || *subs[0].SyncOffsetMs != syncOffset.Milliseconds() || subs[0].UpgradeUntil != nil {
		t.Errorf("stored = %+v, want a synced, perfect provider subtitle", subs)
	}
	if got := h.coverage(file.ID); got[string(english)] != api.CoverageSatisfied {
		t.Errorf("coverage = %v", got)
	}
}

// Seed 4, with Whisper off so the next step is MISSING; seed 5 covers Whisper.
func TestCandidateBelowTheThresholdIsNotDownloaded(t *testing.T) {
	h := newHarness(t, english)
	h.setWhisper(false)
	file := h.episodeFile()
	// Series, year, season and episode match but nothing from the release: 86%.
	weak := subtitles.Candidate{FileID: 12, Language: english, ReleaseName: "Show.S01E02.720p.HDTV.x264-OTHER", Tmdb: showTmdb, Season: 1, Episode: 2}
	h.os.AddCandidates(h.episodeQuery(english), weak)
	h.os.SetContent(12, srt)
	h.imported(file)
	h.runDue()

	if n := h.os.Called(fakes.OpenSubtitlesDownload); n != 0 {
		t.Errorf("%d downloads, want none", n)
	}
	missing := ofType[api.SubtitleMissing](t, h.events, api.TypeSubtitleMissing)
	if len(missing) != 1 || missing[0].Reason != api.MissingNoMatch {
		t.Errorf("SubtitleMissing = %+v, want one NO_MATCH", missing)
	}
	if got := h.coverage(file.ID); got[string(english)] != api.CoverageMissing {
		t.Errorf("coverage = %v, want English missing", got)
	}
}

// Seed 6.
func TestNoMatchForAnotherLanguageIsNotOriginalLanguage(t *testing.T) {
	h := newHarness(t, spanish)
	h.setWhisper(true)
	file := h.episodeFile()
	h.imported(file)
	h.runDue()

	missing := ofType[api.SubtitleMissing](t, h.events, api.TypeSubtitleMissing)
	want := api.SubtitleMissing{MediaFile: file.ID, Language: spanish, Reason: api.MissingNotOriginalLanguage}
	if len(missing) != 1 || missing[0] != want {
		t.Fatalf("SubtitleMissing = %+v, want %+v", missing, want)
	}

	h.advance(7 * time.Hour)
	h.sweep()
	if got := ofType[api.SubtitleMissing](t, h.events, api.TypeSubtitleMissing); len(got) != 1 {
		t.Errorf("SubtitleMissing published %d times after a retry, want once", len(got))
	}
	if n := h.os.Called(fakes.OpenSubtitlesSearch); n != 2 {
		t.Errorf("%d searches, want the import search and one sweep retry", n)
	}
}

// Seed 8.
func TestNoQuotaMeansNoDownloadUntilTheReset(t *testing.T) {
	h := newHarness(t, english)
	resetAt := h.now.Add(quotaWait)
	if err := subtitles.SeedQuota(h.ctx, h.db, 0, resetAt); err != nil {
		t.Fatal(err)
	}
	file := h.episodeFile()
	h.os.AddCandidates(h.episodeQuery(english), perfectEpisode(13, english))
	h.os.SetContent(13, srt)
	h.imported(file)
	h.runDue()

	if n := h.os.Called(fakes.OpenSubtitlesDownload); n != 0 {
		t.Fatalf("%d downloads with no quota, want none", n)
	}
	cov := h.view(file.ID).Wanted
	if len(cov) != 1 || cov[0].Status != subtitles.StatusPending || cov[0].NextAttemptAt == nil || !cov[0].NextAttemptAt.Equal(resetAt) {
		t.Fatalf("wanted = %+v, want PENDING until %v", cov, resetAt)
	}

	h.advance(quotaWait)
	h.runDue()
	if _, ok := h.sidecars.Read("/tv/Show (2011)/Season 01/Show S01E02.en.srt"); !ok {
		t.Fatal("no sidecar after the quota reset")
	}
	if got := h.coverage(file.ID); got[string(english)] != api.CoverageSatisfied {
		t.Errorf("coverage = %v, want English satisfied after the reset", got)
	}
}

// Seed 9.
func TestNoSpeechKeepsTheSubtitleUnsynced(t *testing.T) {
	h := newHarness(t, english)
	h.syncer.SetNoSpeech(episodeVideo)
	file := h.episodeFile()
	h.os.AddCandidates(h.episodeQuery(english), perfectEpisode(14, english))
	h.os.SetContent(14, srt)
	h.imported(file)
	h.runDue()

	added := ofType[api.SubtitleAdded](t, h.events, api.TypeSubtitleAdded)
	if len(added) != 1 || added[0].Synced {
		t.Fatalf("SubtitleAdded = %+v, want one unsynced", added)
	}
	if content, _ := h.sidecars.Read("/tv/Show (2011)/Season 01/Show S01E02.en.srt"); string(content) != string(srt) {
		t.Errorf("sidecar = %q, want the downloaded subtitle as is", content)
	}
}

// movieCandidate scores 133/180 (73%): title, year, source, video codec, resolution, HI.
func movieCandidate(fileID int64, release string) subtitles.Candidate {
	return subtitles.Candidate{FileID: fileID, Language: english, ReleaseName: release, Tmdb: duneTmdb, Year: duneYear}
}

// Seed 10.
func TestUpgradeNeedsAtLeastOneMorePoint(t *testing.T) {
	h := newHarness(t, english)
	file := h.movieFile()
	h.os.AddCandidates(h.movieQuery(english), movieCandidate(20, "Dune.2021.1080p.WEB-DL.x264-OTHER"))
	h.os.SetContent(20, srt)
	h.imported(file)
	h.runDue()
	if subs := h.view(file.ID).Subtitles; len(subs) != 1 || subs[0].UpgradeUntil == nil {
		t.Fatalf("subtitles = %+v, want one provider subtitle in its upgrade window", subs)
	}
	downloads := h.os.Called(fakes.OpenSubtitlesDownload)

	h.advance(2 * 24 * time.Hour)
	h.os.AddCandidates(h.movieQuery(english), movieCandidate(21, "Dune.2021.1080p.WEB-DL.x264-THIRD"))
	h.os.SetContent(21, betterSRT)
	h.sweep()
	if n := h.os.Called(fakes.OpenSubtitlesDownload); n != downloads {
		t.Fatalf("%d more downloads, want an equal score ignored", n-downloads)
	}

	h.os.AddCandidates(h.movieQuery(english), movieCandidate(22, "Dune.2021.1080p.WEB-DL.DDP5.1.x264-OTHER"))
	h.os.SetContent(22, betterSRT)
	h.sweep()
	added := ofType[api.SubtitleAdded](t, h.events, api.TypeSubtitleAdded)
	if len(added) != 2 || !added[1].Replaces {
		t.Fatalf("SubtitleAdded = %+v, want a second one replacing the first", added)
	}
	if content, _ := h.sidecars.Read("/movies/Dune (2021)/Dune (2021).en.srt"); string(content) != string(betterSRT) {
		t.Errorf("sidecar = %q, want the upgrade", content)
	}
	if subs := h.view(file.ID).Subtitles; len(subs) != 1 || *subs[0].ProviderFileID != 22 {
		t.Errorf("subtitles = %+v, want only the upgrade", subs)
	}
}

// Seed 11.
func TestUpgradeWindowEnds(t *testing.T) {
	h := newHarness(t, english)
	file := h.movieFile()
	h.os.AddCandidates(h.movieQuery(english), movieCandidate(20, "Dune.2021.1080p.WEB-DL.x264-OTHER"))
	h.os.SetContent(20, srt)
	h.imported(file)
	h.runDue()
	searches := h.os.Called(fakes.OpenSubtitlesSearch)

	h.advance(8 * 24 * time.Hour)
	h.sweep()
	if n := h.os.Called(fakes.OpenSubtitlesSearch); n != searches {
		t.Errorf("%d searches after the window, want none", n-searches)
	}
}

// Seed 12.
func TestTranscodeThatDroppedATrackReopensTheLanguage(t *testing.T) {
	h := newHarness(t, spanish)
	file := h.episodeFile(englishText(), spanishImage())
	h.imported(file)
	if got := h.coverage(file.ID); got[string(spanish)] != api.CoverageSatisfied {
		t.Fatalf("coverage = %v, want Spanish satisfied by the image track", got)
	}

	file.MediaInfo.Streams = []libraryapi.Stream{englishText()}
	h.library.Put(libraryTestFile(file))
	h.handle(subtitles.NewLibraryHandler(h.service), libraryapi.FileTranscoded{MediaFile: file.ID, Path: file.Path})

	for _, sub := range h.view(file.ID).Subtitles {
		if sub.Language == spanish {
			t.Errorf("Spanish embedded row kept: %+v", sub)
		}
	}
	if got := h.coverage(file.ID); got[string(spanish)] != api.CoveragePending {
		t.Errorf("coverage = %v, want Spanish pending", got)
	}
	if n := h.searchesQueued(file.ID); n != 1 {
		t.Errorf("%d searches queued, want one", n)
	}
}

// Seed 13.
func TestFileDeletedRemovesSidecarsNotUsedByTheUpgrade(t *testing.T) {
	h := newHarness(t, english, spanish)
	const (
		englishSidecar = "/tv/Show (2011)/Season 01/Show S01E02.en.srt"
		spanishSidecar = "/tv/Show (2011)/Season 01/Show S01E02.es.srt"
	)
	h.sidecars.Put(englishSidecar, srt)
	h.sidecars.Put(spanishSidecar, srt)
	old := h.episodeFile()
	h.imported(old,
		libraryapi.Sidecar{Path: englishSidecar, Language: english},
		libraryapi.Sidecar{Path: spanishSidecar, Language: spanish})

	upgrade := h.episodeFile()
	h.library.Remove(old.ID)
	h.imported(upgrade, libraryapi.Sidecar{Path: englishSidecar, Language: english})
	h.runDue()
	if got := h.sidecars.Paths(); len(got) != 2 {
		t.Fatalf("FileImported alone left %v, want both sidecars", got)
	}
	if subs := h.view(old.ID).Subtitles; len(subs) != 2 {
		t.Fatalf("FileImported alone left %d rows for the old file, want 2", len(subs))
	}

	h.handle(subtitles.NewLibraryHandler(h.service), libraryapi.FileDeleted{MediaFile: old.ID, Title: old.Title, Path: old.Path, Reason: libraryapi.DeleteUpgraded})
	h.runDue()
	if got := h.sidecars.Paths(); !slices.Equal(got, []string{englishSidecar}) {
		t.Errorf("sidecars = %v, want only the one the upgrade uses", got)
	}
	if v := h.view(old.ID); len(v.Subtitles) != 0 || len(v.Wanted) != 0 {
		t.Errorf("old file still has %+v", v)
	}
}

// Seed 14.
func TestRejectedLoginPausesWithoutHammering(t *testing.T) {
	h := newHarness(t, english)
	h.os.FailAlways(fakes.OpenSubtitlesDownload, subtitles.ErrAuth)
	file := h.episodeFile()
	h.os.AddCandidates(h.episodeQuery(english), perfectEpisode(15, english))
	h.imported(file)
	h.runDue()

	if logins, downloads := h.os.Called(fakes.OpenSubtitlesLogin), h.os.Called(fakes.OpenSubtitlesDownload); logins != 1 || downloads != 2 {
		t.Fatalf("logins = %d, downloads = %d; want one re-login between two downloads", logins, downloads)
	}
	quota, err := h.service.Quota(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if quota.Problem == "" || quota.PausedUntil == nil {
		t.Fatalf("quota = %+v, want the paused login surfaced", quota)
	}

	calls := len(h.os.Calls())
	h.advance(time.Hour)
	h.sweep()
	if n := len(h.os.Calls()); n != calls {
		t.Errorf("%d more OpenSubtitles calls while paused, want none", n-calls)
	}
	if got := h.coverage(file.ID); got[string(english)] != api.CoveragePending {
		t.Errorf("coverage = %v, want English still pending", got)
	}
}

// Seed 15.
func TestCoverageSettlesOncePerRound(t *testing.T) {
	h := newHarness(t, english, spanish)
	file := h.episodeFile()
	h.os.AddCandidates(h.episodeQuery(english), perfectEpisode(16, english))
	h.os.SetContent(16, srt)
	h.imported(file)
	h.runDue()

	settled := ofType[api.CoverageSettled](t, h.events, api.TypeCoverageSettled)
	want := api.CoverageSettled{MediaFile: file.ID, Title: h.show.ID, Satisfied: []domain.Language{english}, Missing: []domain.Language{spanish}}
	if len(settled) != 1 || !slices.Equal(settled[0].Satisfied, want.Satisfied) || !slices.Equal(settled[0].Missing, want.Missing) || settled[0].Title != want.Title {
		t.Fatalf("CoverageSettled = %+v, want once %+v", settled, want)
	}

	h.catalog.SetGlobalLanguages(languages(english, spanish, japanese))
	h.handle(subtitles.NewLanguagesChangedHandler(h.service), catalogapi.LanguagesChanged{Languages: languages(english, spanish, japanese)})
	recompute(t, h)
	if n := len(ofType[api.CoverageSettled](t, h.events, api.TypeCoverageSettled)); n != 1 {
		t.Fatalf("CoverageSettled published %d times before the new search ended, want 1", n)
	}
	h.runDue()
	if n := len(ofType[api.CoverageSettled](t, h.events, api.TypeCoverageSettled)); n != 2 {
		t.Errorf("CoverageSettled published %d times, want again once the new search ended", n)
	}
}

// Seed 16.
func TestTitleLanguagesChangeQueuesOnlyThatTitle(t *testing.T) {
	h := newHarness(t, english)
	episode := h.episodeFile(englishText())
	movie := h.movieFile(englishText())
	h.imported(episode)
	h.imported(movie)

	override := languages(english, japanese)
	h.catalog.SetLanguageOverride(h.show.ID, override)
	h.handle(subtitles.NewLanguagesChangedHandler(h.service), catalogapi.LanguagesChanged{Title: &h.show.ID, Languages: override})
	recompute(t, h)

	if got := h.coverage(episode.ID); got[string(japanese)] != api.CoveragePending {
		t.Errorf("episode coverage = %v, want Japanese pending", got)
	}
	if n := h.searchesQueued(episode.ID); n != 1 {
		t.Errorf("%d searches queued for the episode, want one", n)
	}
	if got := h.coverage(movie.ID); len(got) != 1 {
		t.Errorf("movie coverage = %v, want it untouched", got)
	}
}

func TestRerunSubtitlesBlocklistsAndSearchesAgain(t *testing.T) {
	h := newHarness(t, english)
	file := h.episodeFile()
	h.os.AddCandidates(h.episodeQuery(english), perfectEpisode(17, english), perfectEpisode(18, english))
	h.os.SetContent(17, srt)
	h.os.SetContent(18, betterSRT)
	h.imported(file)
	h.runDue()

	issue := requestsapi.IssueActionRequested{
		Issue: domain.NewID[domain.IssueID](), Title: h.show.ID, Episode: &h.episode.ID, Action: requestsapi.IssueActionRerunSubtitles,
	}
	h.library.Put(libraryTestFile(file))
	h.handle(subtitles.NewIssueActionHandler(h.service), issue)
	h.runDue()

	subs := h.view(file.ID).Subtitles
	if len(subs) != 1 || *subs[0].ProviderFileID != 18 {
		t.Fatalf("subtitles = %+v, want the other candidate", subs)
	}
	if content, _ := h.sidecars.Read("/tv/Show (2011)/Season 01/Show S01E02.en.srt"); string(content) != string(betterSRT) {
		t.Errorf("sidecar = %q, want the new download", content)
	}
	added := ofType[api.SubtitleAdded](t, h.events, api.TypeSubtitleAdded)
	if last := added[len(added)-1]; last.Title != h.show.ID || !slices.Equal(last.Episodes, []domain.EpisodeID{h.episode.ID}) {
		t.Errorf("SubtitleAdded = %+v, want the show's episode so Requests can resolve the issue", last)
	}

	h.handle(subtitles.NewIssueActionHandler(h.service), issue)
	h.runDue()

	if subs := h.view(file.ID).Subtitles; len(subs) != 1 || *subs[0].ProviderFileID != 18 {
		t.Errorf("subtitles after redelivery = %+v, want the issue's rerun to run once", subs)
	}
}

func TestManualDownloadAndDelete(t *testing.T) {
	h := newHarness(t, english)
	h.setWhisper(false)
	file := h.movieFile()
	h.os.AddCandidates(h.movieQuery(english), movieCandidate(30, "Dune.2021.720p.HDTV-NOPE"))
	h.os.SetContent(30, srt)
	h.imported(file)
	h.runDue()
	if got := h.coverage(file.ID); got[string(english)] != api.CoverageMissing {
		t.Fatalf("coverage = %v, want English missing", got)
	}

	cands, err := h.service.Candidates(h.ctx, file.ID, english, false)
	if err != nil || len(cands) != 1 || cands[0].Acceptable {
		t.Fatalf("Candidates = %+v, %v; want one below the threshold", cands, err)
	}
	view, err := h.service.Download(h.ctx, file.ID, english, false, 30)
	if err != nil || len(view.Subtitles) != 1 {
		t.Fatalf("Download = %+v, %v", view, err)
	}
	if got := h.coverage(file.ID); got[string(english)] != api.CoverageSatisfied {
		t.Errorf("coverage = %v, want English satisfied", got)
	}

	if err := h.service.DeleteSubtitle(h.ctx, view.Subtitles[0].ID); err != nil {
		t.Fatal(err)
	}
	if paths := h.sidecars.Paths(); len(paths) != 0 {
		t.Errorf("sidecars = %v, want the file removed", paths)
	}
	if got := h.coverage(file.ID); got[string(english)] != api.CoveragePending {
		t.Errorf("coverage = %v, want English searched again", got)
	}
}

// Seed 18.
func TestFileRenamedMovesStoredSidecarPaths(t *testing.T) {
	h := newHarness(t, english, spanish)
	const (
		oldEnglish = "/tv/Show (2011)/Season 01/Show S01E02.en.srt"
		oldSpanish = "/tv/Show (2011)/Season 01/Show S01E02.es.srt"
		newVideo   = "/tv/Show (2011)/Season 02/Show S02E01.mkv"
		newEnglish = "/tv/Show (2011)/Season 02/Show S02E01.en.srt"
		newSpanish = "/tv/Show (2011)/Season 02/Show S02E01.es.srt"
	)
	file := h.episodeFile()
	h.os.AddCandidates(h.episodeQuery(spanish), perfectEpisode(11, spanish))
	h.os.SetContent(11, srt)
	h.imported(file, libraryapi.Sidecar{Path: oldEnglish, Language: english})
	h.runDue()
	sidecarPaths := func() []string {
		var paths []string
		for _, sub := range h.view(file.ID).Subtitles {
			paths = append(paths, sub.Path)
		}
		slices.Sort(paths)
		return paths
	}
	if got, want := sidecarPaths(), []string{oldEnglish, oldSpanish}; !slices.Equal(got, want) {
		t.Fatalf("sidecar paths before the rename = %v, want %v", got, want)
	}
	renamed := libraryapi.FileRenamed{
		MediaFile: file.ID, Title: file.Title, OldPath: file.Path, NewPath: newVideo,
		Episodes: []domain.EpisodeRef{{Episode: h.episode.ID, Season: 2, Number: 1}},
		Sidecars: []libraryapi.SidecarMove{
			{OldPath: oldEnglish, NewPath: newEnglish},
			{OldPath: oldSpanish, NewPath: newSpanish},
			{OldPath: "/tv/Show (2011)/Season 01/Show S01E02.nfo", NewPath: "/tv/Show (2011)/Season 02/Show S02E01.nfo"},
		},
	}

	h.handle(subtitles.NewLibraryHandler(h.service), renamed)
	h.handle(subtitles.NewLibraryHandler(h.service), renamed)

	if got, want := sidecarPaths(), []string{newEnglish, newSpanish}; !slices.Equal(got, want) {
		t.Errorf("sidecar paths = %v, want %v", got, want)
	}
}

func TestRedeliveredImportChangesNothing(t *testing.T) {
	h := newHarness(t, english)
	file := h.episodeFile(englishText())
	h.imported(file)
	h.imported(file)
	if subs := h.view(file.ID).Subtitles; len(subs) != 1 {
		t.Errorf("subtitles = %+v, want one row", subs)
	}
}

func TestSearchStopsWhenTheJobIsCancelled(t *testing.T) {
	h := newHarness(t, english)
	h.os.DelayMethod(fakes.OpenSubtitlesSearch, time.Hour)
	file := h.episodeFile()
	h.imported(file)

	ctx, cancel := context.WithCancel(h.ctx)
	done := make(chan error, 1)
	go func() {
		for _, task := range h.queue.takeDue(h.now) {
			_, err := subtitles.RunTask(ctx, h.service, task)
			done <- err
		}
	}()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("search = %v, want context.Canceled for backlite to retry", err)
		}
	case <-time.After(time.Minute):
		t.Fatal("search kept running after its job was cancelled")
	}
	if got := h.coverage(file.ID); got[string(english)] != api.CoveragePending {
		t.Errorf("coverage = %v, want English still pending", got)
	}
}

func languages(langs ...domain.Language) []catalogapi.WantedLanguage {
	out := make([]catalogapi.WantedLanguage, 0, len(langs))
	for _, l := range langs {
		out = append(out, catalogapi.WantedLanguage{Language: l})
	}
	return out
}

// recompute runs the queued recompute jobs and leaves every other task queued.
func recompute(t *testing.T, h *harness) {
	t.Helper()
	for _, task := range h.queue.takeDue(h.now) {
		if !subtitles.IsRecompute(task) {
			h.queue.add(task, time.Time{})
			continue
		}
		if _, err := subtitles.RunTask(h.ctx, h.service, task); err != nil {
			t.Fatal(err)
		}
	}
}

func libraryTestFile(f libraryapi.MediaFile) librarytest.StoredFile {
	return librarytest.StoredFile{File: f}
}
