package acquisition

import (
	"fmt"
	"slices"
	"testing"

	"magpie/internal/acquisition/api"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
)

const (
	inflatedSeeders = 644
	weakSeeders     = 4
	healthySeeders  = 200
)

// Seed acquisition #48.
func TestReleaseLanguageWantsTheOriginalOrAWantedLanguage(t *testing.T) {
	in := newInput(t, movieTitle(), scoring(t, PresetProfile(api.Preset1080p)))
	ix := testIndexer("Nyaa", DefaultIndexerPriority)
	italian := item("Movie.Title.2024.iTALiAN.1080p.WEB-DL.H.264-GRP")
	for _, name := range []string{
		"Movie.Title.2024.1080p.WEB-DL.H.264-GRP",
		"Movie.Title.2024.ITA.ENG.1080p.WEB-DL.H.264-GRP",
		"Movie.Title.2024.GERMAN.DL.1080p.WEB-DL.H.264-GRP",
	} {
		if got := in.decided(0, item(name), ix); !got.accepted() {
			t.Errorf("%s: %+v, want accepted", name, got.Rejections)
		}
	}

	for _, trigger := range []api.SearchTrigger{api.TriggerInteractive, api.TriggerAuto} {
		in.Trigger = trigger
		got := in.decided(0, italian, ix)
		want := []api.Rejection{{Reason: api.RejectLanguageNotWanted, Message: "Audio in Italian, not English"}}
		if !slices.Equal(got.Rejections, want) {
			t.Errorf("%s search: %+v, want %+v", trigger, got.Rejections, want)
		}
	}

	in.Languages = []domain.Language{"it"}
	if got := in.decided(0, italian, ix); !got.accepted() {
		t.Errorf("Italian wanted: %+v, want accepted", got.Rejections)
	}
	in.Title.OriginalLanguage, in.Languages = "", nil
	if got := in.decided(0, italian, ix); !got.accepted() {
		t.Errorf("no language known for the title: %+v, want accepted", got.Rejections)
	}
}

// Seed acquisition #46.
func TestAGrabbingSearchNeedsMinAutomaticSeeders(t *testing.T) {
	in := newInput(t, movieTitle(), tieredProfile(t))
	ix := testIndexer("Nyaa", DefaultIndexerPriority)
	few := item("Movie.Title.2024.1080p.WEB-DL.DDP5.1.H.264-HIGH")
	few.Seeders = MinAutomaticSeeders - 1

	for _, trigger := range allTriggers {
		in.Trigger = trigger
		rejected := slices.Contains(reasons(in.decided(0, few, ix).Rejections), api.RejectNotEnoughSeeders)
		if want := grabbingSearches.has(trigger); rejected != want {
			t.Errorf("%s search with %d seeders: rejected %v, want %v", trigger, few.Seeders, rejected, want)
		}
	}
}

// copiesOf lists one release on several indexers, each reporting its own seeders.
func copiesOf(in *decisionInput, name string, seeders ...int) []decidedRelease {
	copies := []decidedRelease{}
	for i, n := range seeders {
		it := item(name)
		it.GUID, it.Seeders = fmt.Sprintf("%s#%d", name, i), n
		copies = append(copies, in.decided(i, it, testIndexer(fmt.Sprintf("Indexer %d", i), DefaultIndexerPriority)))
	}
	return copies
}

func TestSeederConsensusIgnoresOneIndexersInflatedCount(t *testing.T) {
	in := newInput(t, movieTitle(), tieredProfile(t))
	name := "Movie.Title.2024.1080p.WEB-DL.DDP5.1.H.264-HIGH"
	for _, tc := range []struct {
		seeders []int
		want    int
	}{
		{[]int{inflatedSeeders, 0, 1}, 1},
		{[]int{inflatedSeeders, 1}, 1},
		{[]int{inflatedSeeders}, inflatedSeeders},
	} {
		copies := copiesOf(in, name, tc.seeders...)
		if got := seederConsensus(copies)[copies[0].ID]; got != tc.want {
			t.Errorf("copies with %v seeders count as %d, want %d", tc.seeders, got, tc.want)
		}
	}

	bySize := copiesOf(in, name, inflatedSeeders)
	renamed := item("Movie.Title.2024.1080p.WEB-DL.DDP5.1.H.264-HIGH.mkv")
	renamed.Seeders = 0
	bySize = append(bySize, in.decided(1, renamed, testIndexer("Other", DefaultIndexerPriority)))
	if got := seederConsensus(bySize)[bySize[0].ID]; got != 0 {
		t.Errorf("a copy of the same size and quality under another name: %d, want 0", got)
	}
}

// Seed acquisition #47.
func TestAutomaticRankingPutsAWellSeededReleaseBeforeAWeakPack(t *testing.T) {
	const episodesPerSeason = 4
	title := seriesTitle()
	in := newInput(t, title, scoring(t, PresetProfile(api.Preset1080p)))
	in.Episodes = seasons(title.ID, map[int]bool{1: true, 2: true, 3: true}, episodesPerSeason)
	ix := testIndexer("Nyaa", DefaultIndexerPriority)
	pack := item("Show.Title.S01.1080p.WEB-DL.DDP5.1.H.264-GRP")
	pack.SizeBytes, pack.Seeders = episodesPerSeason*oneGB, weakSeeders
	single := item("Show.Title.S01E02.1080p.WEB-DL.DDP5.1.H.264-GRP")
	single.SizeBytes, single.Seeders = oneGB, healthySeeders
	releases := []decidedRelease{in.decided(0, pack, ix), in.decided(1, single, ix)}

	if got := rankedNames(slices.Clone(releases)...); got[0] != pack.Title {
		t.Errorf("interactive order %v, want the pack first", got)
	}
	automatic := slices.Clone(releases)
	slices.SortFunc(automatic, automaticRanking(releases).compare)
	if automatic[0].Candidate.Item.Title != single.Title {
		t.Errorf("automatic order starts with %s, want the well-seeded single", automatic[0].Candidate.Item.Title)
	}
}

func TestSeasonTargetsSplitASeriesSearchBySeason(t *testing.T) {
	title := seriesTitle()
	all := seasons(title.ID, map[int]bool{1: true, 2: true, 3: true}, 2)
	seasonOf := func(targets []searchTarget) []int {
		out := []int{}
		for _, target := range targets {
			out = append(out, target.Episodes[0].Season)
		}
		return out
	}

	if got := seasonOf(seasonTargets(searchTarget{Title: title}, all)); !slices.Equal(got, []int{1, 2, 3}) {
		t.Errorf("whole series: seasons %v, want one target per season", got)
	}
	picked := []catalogapi.Episode{all[0], all[5]}
	if got := seasonOf(seasonTargets(searchTarget{Title: title, Episodes: picked}, all)); !slices.Equal(got, []int{1, 3}) {
		t.Errorf("episodes of seasons 1 and 3: seasons %v, want those two", got)
	}
	movie := searchTarget{Title: movieTitle()}
	if got := seasonTargets(movie, nil); len(got) != 1 || len(got[0].Episodes) != 0 {
		t.Errorf("movie: %+v, want the target as is", got)
	}
}
