package acquisition

import (
	"cmp"
	"math"
	"slices"

	"magpie/internal/acquisition/api"
	"magpie/internal/domain"
)

// SizeBucket rounds the distance to the preferred size, so near-equal sizes fall back to arrival order.
const SizeBucket = 200 * bytesPerMB

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

// compareReleases orders accepted releases best first: quality rank, CF score, episode count, indexer
// priority, seeders and size. Arrival breaks the remaining ties, so the order never changes as more arrive.
func compareReleases(a, b decidedRelease) int {
	ac, bc := a.Candidate, b.Candidate
	return cmp.Or(
		cmp.Compare(bc.Eval.QualityRank, ac.Eval.QualityRank),
		cmp.Compare(bc.Eval.Score, ac.Eval.Score),
		compareEpisodeCount(ac, bc),
		cmp.Compare(ac.Indexer.Settings.Priority, bc.Indexer.Settings.Priority),
		cmp.Compare(seederBucket(bc.Item.Seeders), seederBucket(ac.Item.Seeders)),
		compareSize(ac, bc),
		cmp.Compare(a.Arrival, b.Arrival),
	)
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

// ranks numbers the accepted releases from 1, best first.
func ranks(releases []decidedRelease) map[domain.ReleaseID]int {
	accepted := make([]decidedRelease, 0, len(releases))
	for _, r := range releases {
		if r.accepted() {
			accepted = append(accepted, r)
		}
	}
	slices.SortFunc(accepted, compareReleases)
	byID := make(map[domain.ReleaseID]int, len(accepted))
	for i, r := range accepted {
		byID[r.ID] = i + 1
	}
	return byID
}
