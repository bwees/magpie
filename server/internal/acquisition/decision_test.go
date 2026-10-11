package acquisition

import (
	"slices"
	"testing"
	"time"

	"magpie/internal/acquisition/api"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/platform/parse"
)

const (
	movieName    = "Movie Title"
	movieYear    = 2024
	movieRuntime = 120
	priorityTen  = 10
	defaultSeeds = 50
	fewSeeders   = 5
	manySeeders  = 500
	tierHigh     = 2300
	tierLow      = 300
	blockedHash  = "0a1b2c3d4e5f60718293a4b5c6d7e8f901234567"
	showName     = "Show Title"
	showRuntime  = 45
)

var decisionNow = time.Date(2026, time.October, 11, 12, 0, 0, 0, time.UTC)

func movieTitle() catalogapi.Title {
	return catalogapi.Title{
		ID: domain.NewID[domain.TitleID](), Kind: domain.MediaKindMovie, Name: movieName, Year: movieYear,
		RuntimeMinutes: movieRuntime, Monitored: true, Released: true, OriginalLanguage: "en",
	}
}

func testIndexer(name string, priority int) Indexer {
	settings := DefaultIndexerSettings()
	settings.Priority = priority
	return Indexer{ProwlarrID: len(name), Name: name, Status: IndexerOK, Settings: settings}
}

func item(name string) TorznabItem {
	return TorznabItem{GUID: name, Title: name, SizeBytes: 8 * oneGB, Seeders: defaultSeeds, DownloadVolumeFactor: 1, UploadVolumeFactor: 1}
}

// tieredProfile ranks WEB 2160p above a WEB 1080p group and scores two release groups.
func tieredProfile(t *testing.T) ScoringProfile {
	t.Helper()
	high := format("Tier High", api.FormatSpec{Kind: api.SpecReleaseGroup, Value: `^HIGH$`})
	low := format("Tier Low", api.FormatSpec{Kind: api.SpecReleaseGroup, Value: `^LOW$`})
	p := api.Profile{
		Name: "Tiered", UpgradeAllowed: true, PropersMode: api.PropersPreferAndUpgrade,
		Items: []api.ProfileItem{
			single(parse.QualityWEBDL2160p, true),
			group(1000, "WEB 1080p", parse.QualityWEBDL1080p, parse.QualityWEBRip1080p),
			single(parse.QualityWEBDL720p, false),
		},
		FormatScores:      []api.FormatScore{scoreFor(high, tierHigh), scoreFor(low, tierLow)},
		CutoffFormatScore: upgradeUntil,
	}
	return scoring(t, p, high, low)
}

func newInput(t *testing.T, title catalogapi.Title, profile ScoringProfile) *decisionInput {
	t.Helper()
	return &decisionInput{Trigger: api.TriggerInteractive, Now: decisionNow, Title: title, Profile: profile}
}

func (in *decisionInput) decided(arrival int, it TorznabItem, ix Indexer) decidedRelease {
	c := in.newCandidate(it, ix)
	return decidedRelease{ID: domain.ReleaseID(it.GUID), Arrival: arrival, Candidate: c, Rejections: decide(c, in)}
}

func rankedNames(releases ...decidedRelease) []string {
	slices.SortFunc(releases, compareReleases)
	names := make([]string, 0, len(releases))
	for _, r := range releases {
		names = append(names, r.Candidate.Item.Title)
	}
	return names
}

func reasons(rejections []api.Rejection) []api.RejectionReason {
	out := make([]api.RejectionReason, 0, len(rejections))
	for _, r := range rejections {
		out = append(out, r.Reason)
	}
	return out
}

// Seed acquisition #1.
func TestRankingPutsQualityBeforeScore(t *testing.T) {
	in := newInput(t, movieTitle(), tieredProfile(t))
	ix := testIndexer("Nyaa", DefaultIndexerPriority)
	a := in.decided(0, item("Movie.Title.2024.2160p.WEB-DL.DDP5.1.H.265-LOW"), ix)
	b := in.decided(1, item("Movie.Title.2024.1080p.WEB-DL.DDP5.1.H.264-HIGH"), ix)

	if !a.accepted() || !b.accepted() {
		t.Fatalf("rejections: A %+v, B %+v", a.Rejections, b.Rejections)
	}
	if got := rankedNames(b, a); got[0] != a.Candidate.Item.Title {
		t.Errorf("ranked %v, want the 2160p release first despite its lower score", got)
	}
}

// Seed acquisition #1, second case.
func TestRankingBreaksAGroupTieOnScore(t *testing.T) {
	in := newInput(t, movieTitle(), tieredProfile(t))
	ix := testIndexer("Nyaa", DefaultIndexerPriority)
	low := in.decided(0, item("Movie.Title.2024.1080p.WEB-DL.DDP5.1.H.264-LOW"), ix)
	high := in.decided(1, item("Movie.Title.2024.1080p.WEBRip.x264-HIGH"), ix)

	if got := rankedNames(low, high); got[0] != high.Candidate.Item.Title {
		t.Errorf("ranked %v, want the 2300 release first inside the WEB 1080p group", got)
	}
}

// Seed acquisition #2.
func TestRankingTiebreaksOnIndexerPriorityThenSeeders(t *testing.T) {
	in := newInput(t, movieTitle(), tieredProfile(t))
	name := "Movie.Title.2024.1080p.WEB-DL.DDP5.1.H.264-HIGH"

	fromDefault := in.decided(0, item(name), testIndexer("Default", DefaultIndexerPriority))
	fromPreferred := in.decided(1, item(name), testIndexer("Preferred", priorityTen))
	ordered := []decidedRelease{fromDefault, fromPreferred}
	slices.SortFunc(ordered, compareReleases)
	if ordered[0].Candidate.Indexer.Name != "Preferred" {
		t.Errorf("first is from %s, want the priority-10 indexer", ordered[0].Candidate.Indexer.Name)
	}

	few, many := item(name), item(name)
	few.Seeders, many.Seeders = fewSeeders, manySeeders
	ix := testIndexer("Nyaa", DefaultIndexerPriority)
	ordered = []decidedRelease{in.decided(0, few, ix), in.decided(1, many, ix)}
	slices.SortFunc(ordered, compareReleases)
	if ordered[0].Candidate.Item.Seeders != manySeeders {
		t.Errorf("first has %d seeders, want the one with %d", ordered[0].Candidate.Item.Seeders, manySeeders)
	}
}

func TestRanksNumberAcceptedReleasesBestFirst(t *testing.T) {
	in := newInput(t, movieTitle(), tieredProfile(t))
	ix := testIndexer("Nyaa", DefaultIndexerPriority)
	low := in.decided(0, item("Movie.Title.2024.1080p.WEB-DL.DDP5.1.H.264-LOW"), ix)
	rejected := in.decided(1, item("Movie.Title.2024.720p.WEB-DL.DDP5.1.H.264-HIGH"), ix)
	best := in.decided(2, item("Movie.Title.2024.2160p.WEB-DL.DDP5.1.H.265-HIGH"), ix)

	got := ranks([]decidedRelease{low, rejected, best})
	if got[best.ID] != 1 || got[low.ID] != 2 {
		t.Errorf("ranks = %v, want best 1 and low 2", got)
	}
	if _, ok := got[rejected.ID]; ok {
		t.Errorf("a rejected release got rank %d", got[rejected.ID])
	}
}

// Seed acquisition #11.
func TestBlocklistMatchesByHashForTheSameTitleOnly(t *testing.T) {
	title := movieTitle()
	in := newInput(t, title, tieredProfile(t))
	in.Blocklist = []api.BlocklistEntry{{Title: title.ID, InfoHash: blockedHash, Reason: api.BlocklistBlockedFile, At: decisionNow}}
	it := item("Movie.Title.2024.1080p.WEB-DL.DDP5.1.H.264-HIGH")
	it.InfoHash = blockedHash

	got := in.decided(0, it, testIndexer("Nyaa", DefaultIndexerPriority))
	if !slices.Contains(reasons(got.Rejections), api.RejectBlocklisted) {
		t.Errorf("rejections = %+v, want BLOCKLISTED", got.Rejections)
	}

	other := newInput(t, movieTitle(), tieredProfile(t))
	if got := other.decided(0, it, testIndexer("Nyaa", DefaultIndexerPriority)); !got.accepted() {
		t.Errorf("another title's release was rejected: %+v", got.Rejections)
	}
}

func TestBlocklistByNameAndIndexerAndExpiry(t *testing.T) {
	title := movieTitle()
	in := newInput(t, title, tieredProfile(t))
	name := "Movie.Title.2024.1080p.WEB-DL.DDP5.1.H.264-HIGH"
	expired := decisionNow.Add(-time.Minute)
	in.Blocklist = []api.BlocklistEntry{
		{Title: title.ID, ReleaseName: name, Indexer: "Nyaa", Reason: api.BlocklistUser},
		{Title: title.ID, InfoHash: blockedHash, Reason: api.BlocklistStalled, ExpiresAt: &expired},
	}
	it := item(name)

	if got := in.decided(0, it, testIndexer("Nyaa", DefaultIndexerPriority)); !slices.Contains(reasons(got.Rejections), api.RejectBlocklisted) {
		t.Errorf("same name and indexer: %+v, want BLOCKLISTED", got.Rejections)
	}
	if got := in.decided(0, it, testIndexer("Other", DefaultIndexerPriority)); !got.accepted() {
		t.Errorf("same name from another indexer: %+v, want accepted", got.Rejections)
	}
	hashed := item("Movie.Title.2024.2160p.WEB-DL.DDP5.1.H.265-HIGH")
	hashed.InfoHash = blockedHash
	if got := in.decided(0, hashed, testIndexer("Nyaa", DefaultIndexerPriority)); !got.accepted() {
		t.Errorf("expired entry still matched: %+v", got.Rejections)
	}
}

func TestTierZeroCollectsEveryRejection(t *testing.T) {
	in := newInput(t, movieTitle(), tieredProfile(t))
	it := item("Movie.Title.2024.720p.WEB-DL.DDP5.1.H.264-GRP")
	it.Seeders = 0
	ix := testIndexer("Nyaa", DefaultIndexerPriority)
	ix.Status = IndexerFailing

	got := in.decided(0, it, ix)
	want := []api.RejectionReason{api.RejectQualityNotAllowed, api.RejectNotEnoughSeeders, api.RejectIndexerBackoff}
	if !slices.Equal(reasons(got.Rejections), want) {
		t.Fatalf("reasons = %v, want %v", reasons(got.Rejections), want)
	}
	if got.Rejections[0].Temporary || !got.Rejections[2].Temporary {
		t.Errorf("temporary flags = %+v, want only the backoff temporary", got.Rejections)
	}
}

func TestFreeSpaceRunsOnlyAfterTierZeroPassed(t *testing.T) {
	in := newInput(t, movieTitle(), tieredProfile(t))
	asked := false
	in.FreeSpace = func() (int64, bool) {
		asked = true
		return oneGB, true
	}
	ix := testIndexer("Nyaa", DefaultIndexerPriority)

	if got := in.decided(0, item("Movie.Title.2024.720p.WEB-DL.DDP5.1.H.264-GRP"), ix); slices.Contains(reasons(got.Rejections), api.RejectNotEnoughSpace) || asked {
		t.Errorf("tier one ran on a rejected release: %+v", got.Rejections)
	}
	got := in.decided(0, item("Movie.Title.2024.1080p.WEB-DL.DDP5.1.H.264-HIGH"), ix)
	if !slices.Equal(reasons(got.Rejections), []api.RejectionReason{api.RejectNotEnoughSpace}) {
		t.Errorf("rejections = %+v, want NOT_ENOUGH_SPACE", got.Rejections)
	}
}

func TestUnknownTitleAndSize(t *testing.T) {
	in := newInput(t, movieTitle(), tieredProfile(t))
	in.Profile.profile.Items[1].Sizes = api.SizeLimits{MinMBPerMin: 10, PreferredMBPerMin: 50, MaxMBPerMin: 60}
	ix := testIndexer("Nyaa", DefaultIndexerPriority)

	if got := in.decided(0, item("Other.Film.2024.1080p.WEB-DL.DDP5.1.H.264-HIGH"), ix); !slices.Contains(reasons(got.Rejections), api.RejectUnknownTitle) {
		t.Errorf("other title: %+v, want UNKNOWN_TITLE", got.Rejections)
	}
	if got := in.decided(0, item("Movie.Title.2019.1080p.WEB-DL.DDP5.1.H.264-HIGH"), ix); !slices.Contains(reasons(got.Rejections), api.RejectUnknownTitle) {
		t.Errorf("wrong year: %+v, want UNKNOWN_TITLE", got.Rejections)
	}
	small := item("Movie.Title.2024.1080p.WEB-DL.DDP5.1.H.264-HIGH")
	small.SizeBytes = oneGB
	if got := in.decided(0, small, ix); !slices.Equal(reasons(got.Rejections), []api.RejectionReason{api.RejectSizeTooSmall}) {
		t.Errorf("1 GB for 120 min at 10 MB/min: %+v, want SIZE_TOO_SMALL", got.Rejections)
	}
	in.Title.RuntimeMinutes = 0
	if got := in.decided(0, small, ix); !slices.Equal(reasons(got.Rejections), []api.RejectionReason{api.RejectUnknownRuntime}) {
		t.Errorf("no runtime: %+v, want UNKNOWN_RUNTIME", got.Rejections)
	}
}

func TestMonitoredAndReleasedApplyOnlyToTheirTriggers(t *testing.T) {
	title := movieTitle()
	title.Monitored, title.Released = false, false
	in := newInput(t, title, tieredProfile(t))
	name := "Movie.Title.2024.1080p.WEB-DL.DDP5.1.H.264-HIGH"
	ix := testIndexer("Nyaa", DefaultIndexerPriority)

	if got := in.decided(0, item(name), ix); !got.accepted() {
		t.Errorf("interactive: %+v, want accepted", got.Rejections)
	}
	in.Trigger = api.TriggerAuto
	want := []api.RejectionReason{api.RejectNotMonitored, api.RejectNotReleased}
	if got := in.decided(0, item(name), ix); !slices.Equal(reasons(got.Rejections), want) {
		t.Errorf("auto: %v, want %v", reasons(got.Rejections), want)
	}
}

func TestUpgradeRuleAgainstTheExistingFile(t *testing.T) {
	title := movieTitle()
	in := newInput(t, title, tieredProfile(t))
	p := in.Profile.Profile()
	p.Cutoff = 1
	p.MinUpgradeFormatScore = 100
	in.Profile.profile = p
	scored := func(name string) Evaluation { return Evaluate(movie(t, name), in.Profile) }

	existing := scored("Movie.Title.2024.1080p.WEB-DL.DDP5.1.H.264-GRP")
	existing.Score = tierOneScore
	in.Files = []existingFile{{File: libraryapi.MediaFile{Title: title.ID, Quality: existing.Quality}, Eval: existing, Group: "GRP"}}

	release := func(score int) Evaluation {
		ev := scored("Movie.Title.2024.1080p.WEB-DL.DDP5.1.H.264-NEW")
		ev.Score = score
		return ev
	}
	if _, rejected := upgradeRejection(p, release(1750), in.Files[0]); !rejected {
		t.Error("CF 1750 over 1700 with increment 100 was accepted")
	}
	if r, rejected := upgradeRejection(p, release(1800), in.Files[0]); rejected {
		t.Errorf("CF 1800 rejected: %+v", r)
	}
	transcoded := in.Files[0]
	transcoded.File.Transcoded = true
	if r, _ := upgradeRejection(p, release(1800), transcoded); r.Reason != api.RejectUpgradeNotAllowed {
		t.Errorf("transcoded file: %+v, want UPGRADE_NOT_ALLOWED", r)
	}

	repackName := "Movie.Title.2024.1080p.WEB-DL.REPACK.DDP5.1.H.264-OTHER"
	if got := in.decided(0, item(repackName), testIndexer("Nyaa", DefaultIndexerPriority)); !slices.Contains(reasons(got.Rejections), api.RejectRepackDifferentGroup) {
		t.Errorf("repack from another group: %+v, want REPACK_DIFFERENT_GROUP", got.Rejections)
	}
}

func seriesTitle() catalogapi.Title {
	return catalogapi.Title{ID: domain.NewID[domain.TitleID](), Kind: domain.MediaKindSeries, Name: showName,
		RuntimeMinutes: showRuntime, Monitored: true, Released: true}
}

func seasons(title domain.TitleID, monitored map[int]bool, perSeason int) []catalogapi.Episode {
	aired := decisionNow.AddDate(-1, 0, 0)
	var eps []catalogapi.Episode
	for _, season := range []int{1, 2, 3} {
		for number := 1; number <= perSeason; number++ {
			eps = append(eps, catalogapi.Episode{ID: domain.NewID[domain.EpisodeID](), Title: title, Season: season, Number: number,
				AiredSeason: season, AiredNumber: number, AirDate: &aired, RuntimeMinutes: showRuntime, Monitored: monitored[season], Released: true})
		}
	}
	return eps
}

// Seed acquisition #25.
func TestMultiSeasonPackNeedsEverySeasonWanted(t *testing.T) {
	const episodesPerSeason = 8
	title := seriesTitle()
	profile := scoring(t, PresetProfile(api.Preset1080p))
	in := newInput(t, title, profile)
	pack := item("Show.Title.S01-S03.1080p.WEB-DL.DDP5.1.H.264-GRP")
	pack.SizeBytes = 3 * episodesPerSeason * oneGB
	ix := testIndexer("Nyaa", DefaultIndexerPriority)

	in.Episodes = seasons(title.ID, map[int]bool{1: true, 2: true}, episodesPerSeason)
	if got := in.decided(0, pack, ix); !slices.Contains(reasons(got.Rejections), api.RejectSeasonNotWanted) {
		t.Errorf("season 3 unmonitored: %+v, want SEASON_NOT_WANTED", got.Rejections)
	}

	in.Episodes = seasons(title.ID, map[int]bool{1: true, 2: true, 3: true}, episodesPerSeason)
	if got := in.decided(0, pack, ix); !got.accepted() {
		t.Errorf("every season wanted: %+v, want accepted", got.Rejections)
	}
}

func TestSeriesEpisodeSpecs(t *testing.T) {
	const episodesPerSeason = 4
	title := seriesTitle()
	in := newInput(t, title, scoring(t, PresetProfile(api.Preset1080p)))
	in.Episodes = seasons(title.ID, map[int]bool{1: true, 2: true, 3: true}, episodesPerSeason)
	ix := testIndexer("Nyaa", DefaultIndexerPriority)
	single := item("Show.Title.S02E03.1080p.WEB-DL.DDP5.1.H.264-GRP")
	single.SizeBytes = oneGB

	if got := in.decided(0, item("Show.Title.S09E01.1080p.WEB-DL.DDP5.1.H.264-GRP"), ix); !slices.Contains(reasons(got.Rejections), api.RejectUnknownEpisode) {
		t.Errorf("unknown episode: %+v, want UNKNOWN_EPISODE", got.Rejections)
	}

	in.Searched = slices.DeleteFunc(slices.Clone(in.Episodes), func(ep catalogapi.Episode) bool { return ep.Season != 1 })
	got := in.decided(0, single, ix)
	if want := []api.RejectionReason{api.RejectSeasonPackOnly, api.RejectSearchMismatch}; !slices.Equal(reasons(got.Rejections), want) {
		t.Errorf("S02E03 in an ended season-1 search: %v, want %v", reasons(got.Rejections), want)
	}
}
