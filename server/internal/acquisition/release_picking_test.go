package acquisition_test

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"magpie/internal/acquisition"
	"magpie/internal/acquisition/api"
	"magpie/internal/acquisition/fakes"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
)

// settleTasks waits for n queued automatic grabs and returns their searches.
func (f searchFixture) settleTasks(t *testing.T, n int) []domain.SearchID {
	t.Helper()
	deadline := time.Now().Add(waitForEvent)
	for time.Now().Before(deadline) {
		ids := []domain.SearchID{}
		for _, task := range f.jobs.queued() {
			if id, ok := acquisition.SettleTaskOf(task); ok {
				ids = append(ids, id)
			}
		}
		if len(ids) >= n {
			return ids
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("fewer than %d automatic grabs were queued", n)
	return nil
}

// Seed acquisition #44.
func TestTwoSearchesSettlingAtOnceGrabTheTitleOnce(t *testing.T) {
	const addLatency = 50 * time.Millisecond
	f := newSearchFixture(t)
	f.indexer(t, fastID, "Fast", idCaps, 0, release(acceptedName), release(otherName))
	f.startAuto(t, api.TriggerAuto)
	f.startAuto(t, api.TriggerUserAuto)
	f.qbit.DelayMethod(fakes.QBittorrentAddTorrent, addLatency)

	var settling sync.WaitGroup
	for _, id := range f.settleTasks(t, 2) {
		settling.Go(func() {
			if err := f.downloads.Settle(context.Background(), id); err != nil {
				t.Error(err)
			}
		})
	}
	settling.Wait()

	if grabs := f.grabbed(t); len(grabs) != 1 {
		t.Errorf("grabs = %+v, want one: the second search must see the first one's grab", grabs)
	}
}

// Seed acquisition #44, second case.
func TestAFallbackSkipsAReleaseBlocklistedSinceTheSearch(t *testing.T) {
	f := newSearchFixture(t)
	s := f.searched(t, f.torrentRelease(acceptedName, "Movie.exe"), release(otherName))
	if _, err := f.repo.AddToBlocklist(context.Background(), api.BlocklistEntry{
		Title: f.title.ID, ReleaseName: otherName, Indexer: "Fast", Reason: api.BlocklistUser, At: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	result := f.grab(t, s, acceptedName)

	if result.Status != acquisition.GrabRejected || result.Next != nil {
		t.Errorf("grab = %+v, want rejected with no fallback past the new blocklist entry", result)
	}
	if _, added := f.qbit.Torrent(magnetHash(otherName)); added {
		t.Error("the fallback grabbed a release blocklisted after the search")
	}
}

// Seed acquisition #45.
func TestAnAutomaticGrabTrustsTheSeedersMostIndexersReport(t *testing.T) {
	const inflatedID, inflated, dead, few, healthy = 14, 644, 0, 1, 40
	f := newSearchFixture(t)
	f.useTieredProfile(t)
	listed := func(name string, seeders int) acquisition.TorznabItem {
		it := release(name)
		it.Seeders = seeders
		return it
	}
	other := listed(otherName, healthy)
	other.SizeBytes = releaseSize - gigabyte
	f.indexer(t, inflatedID, "Inflated", idCaps, 0, listed(acceptedName, inflated))
	f.indexer(t, fastID, "Fast", idCaps, 0, listed(acceptedName, dead), other)
	f.indexer(t, slowID, "Slow", idCaps, 0, listed(acceptedName, few))

	f.startAuto(t, api.TriggerAuto)
	f.settle(t)

	if grabs := f.grabbed(t); len(grabs) != 1 || grabs[0].Release != otherName {
		t.Errorf("grabs = %+v, want %s: the others agree %s is dead", grabs, otherName, acceptedName)
	}
}

// Seed acquisition #49.
func TestAnEndedSeasonThatAcceptsNothingIsSearchedEpisodeByEpisode(t *testing.T) {
	s := newSeriesFixture(t)
	s.indexer(t, fastID, "Fast", tvIDCaps, 0, release("Severance.S01E01.1080p.WEB-DL.DDP5.1.H.264-GRP"))
	s.searchEpisodes(t, s.episodes...)
	s.settle(t)

	if grabs := s.grabbed(t); len(grabs) != 0 {
		t.Fatalf("grabs = %+v, want none: a season search after the season ended wants a pack", grabs)
	}
	later := s.jobs.laterTasks()
	searched := []domain.EpisodeID{}
	for i, task := range later {
		req, ok := acquisition.SearchTaskOf(task.task)
		if !ok || len(req.Episodes) != 1 || req.Trigger != api.TriggerAuto {
			t.Fatalf("queued %+v, want single-episode AUTO searches", req)
		}
		searched = append(searched, req.Episodes[0])
		if i > 0 && task.at.Sub(later[i-1].at) < acquisition.MinSweepGap {
			t.Errorf("searches %s apart, want at least %s", task.at.Sub(later[i-1].at), acquisition.MinSweepGap)
		}
	}
	want := []domain.EpisodeID{s.episodes[0].ID, s.episodes[1].ID, s.episodes[2].ID}
	if !slices.Equal(searched, want) {
		t.Errorf("searched %v, want each episode of the season", searched)
	}
}

func TestASeasonThatGrabsSomethingIsNotSearchedEpisodeByEpisode(t *testing.T) {
	s := newSeriesFixture(t)
	s.indexer(t, fastID, "Fast", tvIDCaps, 0, release("Severance.S01.1080p.WEB-DL.DDP5.1.H.264-GRP"))
	s.searchEpisodes(t, s.episodes...)
	s.settle(t)

	if len(s.grabbed(t)) != 1 || len(s.jobs.laterTasks()) != 0 {
		t.Errorf("grabs %+v, later searches %+v; want the pack and no episode searches", s.grabbed(t), s.jobs.laterTasks())
	}
}

// Seed acquisition #50.
func TestASeriesSearchQueriesEachSeason(t *testing.T) {
	s := newSeriesFixture(t)
	aired := time.Now().AddDate(-1, 0, 0)
	s.catalog.PutEpisode(catalogapi.Episode{
		ID: domain.NewID[domain.EpisodeID](), Title: s.title.ID, Season: 2, Number: 1, AiredSeason: 2, AiredNumber: 1,
		AirDate: &aired, RuntimeMinutes: 50, Monitored: true, Released: true,
	})
	s.indexer(t, fastID, "Fast", tvIDCaps, 0)

	s.startAuto(t, api.TriggerUserAuto)
	s.events.waitFor(t, api.TypeSearchResultsReceived, 1)

	seasons := []int{}
	for _, q := range s.prowlarr.Queries(fastID) {
		if q.Season == nil {
			t.Fatalf("query %+v has no season", q)
		}
		seasons = append(seasons, *q.Season)
	}
	if !slices.Equal(seasons, []int{1, 2}) {
		t.Errorf("queried seasons %v, want one query per season", seasons)
	}
}
