package acquisition

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"

	"magpie/internal/acquisition/api"
	"magpie/internal/domain"
)

// SizeBucket rounds the distance to the preferred size, so near-equal sizes fall back to arrival order.
const SizeBucket = 200 * bytesPerMB

// WellSeededSeeders is where an automatic pick's upper seeder tier starts.
const WellSeededSeeders = 10

// seederTier groups seeders coarsely for automatic picks: below MinAutomaticSeeders, weak, well seeded.
type seederTier int

const (
	seederTierDead seederTier = iota
	seederTierWeak
	seederTierWell
)

func tierOf(seeders int) seederTier {
	switch {
	case seeders >= WellSeededSeeders:
		return seederTierWell
	case seeders >= MinAutomaticSeeders:
		return seederTierWeak
	}
	return seederTierDead
}

// decidedRelease is a release with the engine's verdict. Arrival is its position in the search.
type decidedRelease struct {
	ID         domain.ReleaseID
	Arrival    int
	Candidate  candidate
	Rejections []api.Rejection
}

func (d decidedRelease) accepted() bool {
	return len(d.Rejections) == 0
}

// ranking orders accepted releases best first. seeders is what a release's seeders count as.
type ranking struct {
	seeders func(decidedRelease) int
	// tiered ranks a better-seeded release ahead of one covering more episodes.
	tiered bool
}

// interactiveRanking reads each release's own seeders, so the order never changes as more answers arrive.
var interactiveRanking = ranking{seeders: func(d decidedRelease) int { return d.Candidate.Item.Seeders }}

// automaticRanking is what a search's own grab picks by: the seeders the indexers agree on, ranked by tier
// ahead of the episode count.
func automaticRanking(releases []decidedRelease) ranking {
	seeds := seederConsensus(releases)
	return ranking{seeders: func(d decidedRelease) int { return seeds[d.ID] }, tiered: true}
}

// compare orders by quality rank, CF score, seeder tier (automatic only), episode count, indexer priority,
// seeders and size. Arrival breaks the remaining ties.
func (r ranking) compare(a, b decidedRelease) int {
	ac, bc := a.Candidate, b.Candidate
	as, bs := r.seeders(a), r.seeders(b)
	tier := 0
	if r.tiered {
		tier = cmp.Compare(tierOf(bs), tierOf(as))
	}
	return cmp.Or(
		cmp.Compare(bc.Eval.QualityRank, ac.Eval.QualityRank),
		cmp.Compare(bc.Eval.Score, ac.Eval.Score),
		tier,
		compareEpisodeCount(ac, bc),
		cmp.Compare(ac.Indexer.Settings.Priority, bc.Indexer.Settings.Priority),
		cmp.Compare(seederBucket(bs), seederBucket(as)),
		compareSize(ac, bc),
		cmp.Compare(a.Arrival, b.Arrival),
	)
}

// compareReleases is the interactive order.
func compareReleases(a, b decidedRelease) int {
	return interactiveRanking.compare(a, b)
}

// seederConsensus is the seeders of each release as the indexers listing it agree: the lower median over
// every copy with the same info hash, name, or size and quality. One indexer inflating its count can't lift
// a release the others list as dead.
func seederConsensus(releases []decidedRelease) map[domain.ReleaseID]int {
	listings := map[string][]int{}
	for i, d := range releases {
		for _, key := range copyKeys(d.Candidate) {
			listings[key] = append(listings[key], i)
		}
	}
	seeds := make(map[domain.ReleaseID]int, len(releases))
	for _, d := range releases {
		copies := map[int]bool{}
		for _, key := range copyKeys(d.Candidate) {
			for _, i := range listings[key] {
				copies[i] = true
			}
		}
		counts := make([]int, 0, len(copies))
		for i := range copies {
			counts = append(counts, releases[i].Candidate.Item.Seeders)
		}
		seeds[d.ID] = lowerMedian(counts)
	}
	return seeds
}

// copyKeys identify one release across indexers.
func copyKeys(c candidate) []string {
	keys := []string{"name:" + cleanTitle(c.Item.Title)}
	if c.Item.InfoHash != "" {
		keys = append(keys, "hash:"+strings.ToLower(c.Item.InfoHash))
	}
	if c.Item.SizeBytes > 0 {
		keys = append(keys, fmt.Sprintf("size:%d:%d", c.Item.SizeBytes, c.Eval.Quality.ID))
	}
	return keys
}

// lowerMedian takes the lower of the two middle values, so with two copies the smaller count wins.
func lowerMedian(counts []int) int {
	slices.Sort(counts)
	return counts[(len(counts)-1)/2]
}

// compareEpisodeCount puts releases covering more episodes first, and a season pack before a release of
// the same count.
func compareEpisodeCount(a, b candidate) int {
	return cmp.Or(
		cmp.Compare(len(b.Episodes), len(a.Episodes)),
		compareTrueFirst(a.Release.FullSeason, b.Release.FullSeason),
	)
}

func compareTrueFirst(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return -1
	}
	return 1
}

// seederBucket groups seeders by order of magnitude, as Sonarr does: 1, 10, 100 and so on.
func seederBucket(seeders int) int {
	if seeders <= 0 {
		return 0
	}
	return int(math.Round(math.Log10(float64(seeders)))) + 1
}

// compareSize prefers the size closest to the profile's preferred size, else the larger release.
func compareSize(a, b candidate) int {
	if a.PreferredBytes > 0 && b.PreferredBytes > 0 {
		return cmp.Compare(sizeDistance(a), sizeDistance(b))
	}
	return cmp.Compare(sizeBuckets(b.Item.SizeBytes), sizeBuckets(a.Item.SizeBytes))
}

func sizeDistance(c candidate) int64 {
	return sizeBuckets(c.Item.SizeBytes - c.PreferredBytes)
}

func sizeBuckets(bytes int64) int64 {
	return int64(math.Round(math.Abs(float64(bytes)) / SizeBucket))
}

// ranks numbers the accepted releases from 1, best first in order.
func ranks(releases []decidedRelease, order ranking) map[domain.ReleaseID]int {
	accepted := make([]decidedRelease, 0, len(releases))
	for _, r := range releases {
		if r.accepted() {
			accepted = append(accepted, r)
		}
	}
	slices.SortFunc(accepted, order.compare)
	byID := make(map[domain.ReleaseID]int, len(accepted))
	for i, r := range accepted {
		byID[r.ID] = i + 1
	}
	return byID
}
