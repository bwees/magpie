package acquisition

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"magpie/internal/acquisition/api"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/platform/parse"
)

// Thresholds Sonarr's specs use.
const (
	bytesPerMB = 1 << 20
	// SampleMaxSize: a release named "sample" smaller than this is a sample.
	SampleMaxSize = 70 * bytesPerMB
	// MinFreeSpaceAfterGrab is what the download disk must keep free once a release is downloaded.
	MinFreeSpaceAfterGrab = 100 * bytesPerMB
	// RecentlyAiredGrace lets an episode that aired within it through the size check while its runtime is unknown.
	RecentlyAiredGrace = 24 * time.Hour
	// ProperWindowDays is how long after import RSS may still grab a proper for a file.
	ProperWindowDays = 7
	ProperWindow     = ProperWindowDays * 24 * time.Hour
	// yearTolerance allows a movie release to name the year before or after the title's.
	yearTolerance = 1
)

var rawDiskPattern = regexp.MustCompile(`(?i)\b(?:BDMV|VIDEO_TS)\b`)

// Articles and conjunctions Sonarr drops before comparing titles.
var ignoredTitleWords = []string{"a", "an", "the", "and", "or", "of"}

type triggerSet []api.SearchTrigger

func (s triggerSet) has(t api.SearchTrigger) bool { return slices.Contains(s, t) }

// The "applies to" column of the decision table.
var (
	allTriggers       = triggerSet{api.TriggerRSS, api.TriggerAuto, api.TriggerUserAuto, api.TriggerInteractive, api.TriggerRegrab}
	rssTriggers       = triggerSet{api.TriggerRSS}
	searchTriggers    = triggerSet{api.TriggerAuto, api.TriggerUserAuto, api.TriggerInteractive, api.TriggerRegrab}
	automaticTriggers = triggerSet{api.TriggerRSS, api.TriggerAuto, api.TriggerRegrab}
	nonInteractive    = triggerSet{api.TriggerRSS, api.TriggerAuto, api.TriggerUserAuto, api.TriggerRegrab}
	// A REGRAB replaces a file someone reported as broken, however well it scores, so it skips the upgrade specs.
	upgradeTriggers = triggerSet{api.TriggerRSS, api.TriggerAuto, api.TriggerUserAuto, api.TriggerInteractive}
)

// candidate is one indexer result, parsed and scored, on its way through the decision engine.
type candidate struct {
	Item    TorznabItem
	Indexer Indexer
	Release parse.Release
	// ParseErr is set when the name couldn't be parsed; no other spec runs then.
	ParseErr error
	Eval     Evaluation
	// Episodes are the catalog episodes a series release covers.
	Episodes []catalogapi.Episode
	// RuntimeMinutes is what the release covers: the movie, or the sum of its episodes.
	RuntimeMinutes int
	// PreferredBytes is the profile's preferred size for its quality and runtime; 0 when unknown.
	PreferredBytes int64
}

func (c candidate) freeleech() bool {
	return c.Item.DownloadVolumeFactor == 0
}

// existingFile is a library file the release would replace, scored against the title's profile.
type existingFile struct {
	File  libraryapi.MediaFile
	Eval  Evaluation
	Group string
}

// activeGrab is a download in the queue, a recent grab, or a pending release covering some items.
type activeGrab struct {
	Episodes    []domain.EpisodeID // empty for a movie
	QualityRank int
	Score       int
	CutoffMet   bool
}

// queuedStates are downloads still on their way to the library.
var queuedStates = []api.DownloadState{api.StateAwaitingMetadata, api.StateDownloading, api.StateCompleted, api.StateImportPendingUser}

// grabsOf turns a title's downloads into the Queue (still in qBittorrent) and the History (grabbed within
// RecentGrabWindow) the specs read, each scored against the search's profile. Failed downloads count for neither.
func grabsOf(downloads []download, title catalogapi.Title, p ScoringProfile, now time.Time) (queue, recent []activeGrab) {
	queue, recent = []activeGrab{}, []activeGrab{}
	for _, d := range downloads {
		if d.State == api.StateFailed {
			continue
		}
		g := grabOf(d, title, p)
		if d.RemovedAt == nil && slices.Contains(queuedStates, d.State) {
			queue = append(queue, g)
		}
		if now.Sub(d.GrabbedAt) < RecentGrabWindow {
			recent = append(recent, g)
		}
	}
	return queue, recent
}

func grabOf(d download, title catalogapi.Title, p ScoringProfile) activeGrab {
	release, err := parse.ParseRelease(d.ReleaseName, title.Kind)
	if err != nil {
		release = parse.Release{Name: d.ReleaseName, Kind: title.Kind}
	}
	release.Quality = d.Quality
	eval := Evaluate(ReleaseFacts{Release: release, SizeBytes: d.SizeBytes, OriginalLanguage: title.OriginalLanguage}, p)
	return activeGrab{Episodes: d.Episodes, QualityRank: eval.QualityRank, Score: eval.Score, CutoffMet: eval.CutoffMet}
}

// pendingGrabs scores the title's held releases against the search's profile.
func pendingGrabs(held []PendingRelease, title catalogapi.Title, p ScoringProfile) []activeGrab {
	grabs := make([]activeGrab, 0, len(held))
	for _, r := range held {
		release, err := parse.ParseRelease(r.Release.Title, title.Kind)
		if err != nil {
			release = parse.Release{Name: r.Release.Title, Kind: title.Kind}
		}
		eval := Evaluate(ReleaseFacts{Release: release, SizeBytes: r.Release.SizeBytes, OriginalLanguage: title.OriginalLanguage}, p)
		grabs = append(grabs, activeGrab{Episodes: r.Episodes, QualityRank: eval.QualityRank, Score: eval.Score, CutoffMet: eval.CutoffMet})
	}
	return grabs
}

// decisionInput is everything the specs read besides the release itself. It is built once per search.
type decisionInput struct {
	Trigger api.SearchTrigger
	Now     time.Time
	Title   catalogapi.Title
	// Episodes are every episode of a series.
	Episodes []catalogapi.Episode
	// Searched are the episodes asked for; empty when the whole title was searched.
	Searched  []catalogapi.Episode
	Profile   ScoringProfile
	Files     []existingFile
	Blocklist []api.BlocklistEntry
	Queue     []activeGrab
	History   []activeGrab
	Pending   []activeGrab
	Delay     time.Duration
	// MaxScore is the profile's maximum attainable CF score: the early-grab and delay-bypass threshold.
	MaxScore int
	// MaximumSize is the global size cap in bytes; 0 means none.
	MaximumSize int64
	// FreeSpace reports the download disk's free bytes, or false when it is unknown.
	FreeSpace func() (int64, bool)
}

type specification struct {
	triggers triggerSet
	check    func(c candidate, in *decisionInput) (api.Rejection, bool)
}

// tierZero runs in full for every release; every rejection is collected.
var tierZero = []specification{
	{allTriggers, unknownTitle},
	{allTriggers, unknownEpisode},
	{allTriggers, qualityAllowedByProfile},
	{allTriggers, customFormatAllowedByProfile},
	{allTriggers, acceptableSize},
	{allTriggers, maximumSize},
	{allTriggers, notSample},
	{allTriggers, rawDisk},
	{allTriggers, splitEpisode},
	{allTriggers, blocklisted},
	{allTriggers, alreadyImported},
	{allTriggers, torrentSeeding},
	{allTriggers, blockedIndexer},
	{upgradeTriggers, upgradable},
	{upgradeTriggers, repack},
	{rssTriggers, proper},
	{allTriggers, sameEpisodesGrab},
	{allTriggers, queued},
	{rssTriggers, history},
	{allTriggers, fullSeason},
	{allTriggers, multiSeason},
	{searchTriggers, seasonPackOnly},
	{nonInteractive, monitored},
	{searchTriggers, searchedItems},
	{automaticTriggers, released},
	{automaticTriggers, delayed},
	{automaticTriggers, pending},
}

// tierOne runs only when tier zero accepted the release.
var tierOne = []specification{
	{allTriggers, freeSpace},
}

// decide runs the decision table on one release. An empty result means accepted.
func decide(c candidate, in *decisionInput) []api.Rejection {
	if c.ParseErr != nil {
		return []api.Rejection{permanent(api.RejectUnableToParse, "Magpie couldn't read this release name")}
	}
	rejections := runTier(tierZero, c, in)
	if len(rejections) > 0 {
		return rejections
	}
	return runTier(tierOne, c, in)
}

func runTier(specs []specification, c candidate, in *decisionInput) []api.Rejection {
	rejections := []api.Rejection{}
	for _, spec := range specs {
		if !spec.triggers.has(in.Trigger) {
			continue
		}
		if rejection, rejected := spec.check(c, in); rejected {
			rejections = append(rejections, rejection)
		}
	}
	return rejections
}

func permanent(reason api.RejectionReason, format string, args ...any) api.Rejection {
	return api.Rejection{Reason: reason, Message: fmt.Sprintf(format, args...)}
}

func temporary(reason api.RejectionReason, format string, args ...any) api.Rejection {
	return api.Rejection{Reason: reason, Message: fmt.Sprintf(format, args...), Temporary: true}
}

var accept = api.Rejection{}

// newCandidate parses and scores one indexer result against the search's title and profile.
func (in *decisionInput) newCandidate(item TorznabItem, ix Indexer) candidate {
	c := candidate{Item: item, Indexer: ix}
	c.Release, c.ParseErr = parse.ParseRelease(item.Title, in.Title.Kind)
	if c.ParseErr != nil {
		unknown, _ := parse.QualityByID(parse.QualityUnknown)
		c.Eval = Evaluation{Quality: unknown.Quality(0), Formats: []string{}}
		return c
	}
	facts := ReleaseFacts{Release: c.Release, SizeBytes: item.SizeBytes, OriginalLanguage: in.Title.OriginalLanguage}
	if c.freeleech() {
		facts.IndexerFlags = []IndexerFlag{FlagFreeleech}
	}
	c.Eval = Evaluate(facts, in.Profile)
	c.Episodes = coveredEpisodes(c.Release, in.Episodes)
	c.RuntimeMinutes = in.runtime(c.Episodes)
	if limits, ok := in.sizeLimits(c.Eval.Quality); ok {
		c.PreferredBytes = megabytes(limits.PreferredMBPerMin * float64(c.RuntimeMinutes))
	}
	return c
}

func (in *decisionInput) runtime(episodes []catalogapi.Episode) int {
	if in.Title.Kind != domain.MediaKindSeries {
		return in.Title.RuntimeMinutes
	}
	total := 0
	for _, ep := range episodes {
		total += cmp.Or(ep.RuntimeMinutes, in.Title.RuntimeMinutes)
	}
	return total
}

// coveredEpisodes maps a series release to catalog episodes: by air date for daily releases, every
// episode of the season(s) for packs, else the listed episode numbers.
func coveredEpisodes(r parse.Release, all []catalogapi.Episode) []catalogapi.Episode {
	if r.Kind != domain.MediaKindSeries {
		return nil
	}
	seasons := r.Seasons
	if len(seasons) == 0 {
		seasons = []int{r.Season}
	}
	covered := []catalogapi.Episode{}
	for _, ep := range all {
		switch {
		case r.AirDate != "":
			if ep.AirDate != nil && ep.AirDate.Format(parse.AirDateLayout) == r.AirDate {
				covered = append(covered, ep)
			}
		case r.FullSeason:
			if slices.Contains(seasons, ep.Season) {
				covered = append(covered, ep)
			}
		case ep.Season == r.Season && slices.Contains(r.Episodes, ep.Number):
			covered = append(covered, ep)
		}
	}
	return covered
}

func unknownTitle(c candidate, in *decisionInput) (api.Rejection, bool) {
	if !titleMatches(c.Release, in.Title) {
		return permanent(api.RejectUnknownTitle, "%q isn't %s", c.Release.Title, in.Title.Name), true
	}
	if in.Title.Kind == domain.MediaKindMovie && c.Release.Year != 0 && in.Title.Year != 0 &&
		abs(c.Release.Year-in.Title.Year) > yearTolerance {
		return permanent(api.RejectUnknownTitle, "Year %d doesn't match %d", c.Release.Year, in.Title.Year), true
	}
	return accept, false
}

func titleMatches(r parse.Release, t catalogapi.Title) bool {
	known := map[string]bool{}
	for _, name := range append([]string{t.Name, t.OriginalName}, t.Aliases...) {
		if clean := cleanTitle(name); clean != "" {
			known[clean] = true
		}
	}
	for _, name := range append([]string{r.Title}, r.Aliases...) {
		if known[cleanTitle(name)] {
			return true
		}
	}
	return false
}

// cleanTitle lowercases a title and drops punctuation and filler words, as Sonarr's CleanSeriesTitle does.
func cleanTitle(title string) string {
	words := strings.FieldsFunc(strings.ToLower(strings.ReplaceAll(title, "&", " and ")), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	kept := make([]string, 0, len(words))
	for _, w := range words {
		if len(words) > 1 && slices.Contains(ignoredTitleWords, w) {
			continue
		}
		kept = append(kept, w)
	}
	return strings.Join(kept, "")
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func unknownEpisode(c candidate, in *decisionInput) (api.Rejection, bool) {
	if in.Title.Kind == domain.MediaKindSeries && len(c.Episodes) == 0 {
		return permanent(api.RejectUnknownEpisode, "No known episode matches %s", episodeLabel(c.Release)), true
	}
	return accept, false
}

func episodeLabel(r parse.Release) string {
	switch {
	case r.AirDate != "":
		return r.AirDate
	case r.FullSeason:
		return fmt.Sprintf("season %d", r.Season)
	case len(r.Episodes) > 0:
		return fmt.Sprintf("S%02dE%02d", r.Season, r.Episodes[0])
	}
	return "the release"
}

func qualityAllowedByProfile(c candidate, in *decisionInput) (api.Rejection, bool) {
	if !c.Eval.Allowed {
		return permanent(api.RejectQualityNotAllowed, "%s isn't wanted by %s", c.Eval.Quality.Name, in.Profile.Profile().Name), true
	}
	return accept, false
}

func customFormatAllowedByProfile(c candidate, in *decisionInput) (api.Rejection, bool) {
	if minimum := in.Profile.Profile().MinFormatScore; c.Eval.Score < minimum {
		return permanent(api.RejectFormatScoreTooLow, "Score %d is below the minimum %d", c.Eval.Score, minimum), true
	}
	return accept, false
}

// sizeLimits returns the profile item's limits for the release's quality.
func (in *decisionInput) sizeLimits(q domain.Quality) (api.SizeLimits, bool) {
	items := in.Profile.Profile().Items
	index, ok := itemIndex(items, q.ID)
	if !ok {
		return api.SizeLimits{}, false
	}
	return items[index].Sizes, true
}

func acceptableSize(c candidate, in *decisionInput) (api.Rejection, bool) {
	limits, ok := in.sizeLimits(c.Eval.Quality)
	if !ok {
		return accept, false
	}
	if c.RuntimeMinutes == 0 {
		if in.airedRecently(c.Episodes) {
			return accept, false
		}
		return permanent(api.RejectUnknownRuntime, "The runtime is unknown, so the size can't be checked"), true
	}
	size := c.Item.SizeBytes
	if minimum := megabytes(limits.MinMBPerMin * float64(c.RuntimeMinutes)); size < minimum {
		return permanent(api.RejectSizeTooSmall, "%s is smaller than the minimum %s", formatBytes(size), formatBytes(minimum)), true
	}
	if limits.MaxMBPerMin > 0 {
		if maximum := megabytes(limits.MaxMBPerMin * float64(c.RuntimeMinutes)); size > maximum {
			return permanent(api.RejectSizeTooLarge, "%s is larger than the maximum %s", formatBytes(size), formatBytes(maximum)), true
		}
	}
	return accept, false
}

func (in *decisionInput) airedRecently(episodes []catalogapi.Episode) bool {
	return slices.ContainsFunc(episodes, func(ep catalogapi.Episode) bool {
		return ep.AirDate != nil && in.Now.Sub(*ep.AirDate) < RecentlyAiredGrace
	})
}

func megabytes(mb float64) int64 {
	return int64(mb * bytesPerMB)
}

func formatBytes(size int64) string {
	if size >= bytesPerGB {
		return fmt.Sprintf("%.1f GB", float64(size)/bytesPerGB)
	}
	return fmt.Sprintf("%d MB", size/bytesPerMB)
}

func maximumSize(c candidate, in *decisionInput) (api.Rejection, bool) {
	if in.MaximumSize > 0 && c.Item.SizeBytes > in.MaximumSize {
		return permanent(api.RejectSizeTooLarge, "%s is over the %s limit", formatBytes(c.Item.SizeBytes), formatBytes(in.MaximumSize)), true
	}
	return accept, false
}

func notSample(c candidate, _ *decisionInput) (api.Rejection, bool) {
	if c.Release.Sample && c.Item.SizeBytes < SampleMaxSize {
		return permanent(api.RejectSample, "A sample, not the full release"), true
	}
	return accept, false
}

func rawDisk(c candidate, _ *decisionInput) (api.Rejection, bool) {
	if rawDiskPattern.MatchString(c.Item.Title) {
		return permanent(api.RejectRawDisk, "A raw disc folder (BDMV or VIDEO_TS)"), true
	}
	return accept, false
}

func splitEpisode(c candidate, _ *decisionInput) (api.Rejection, bool) {
	if c.Release.SplitEpisode {
		return permanent(api.RejectSplitEpisode, "Split episodes aren't supported"), true
	}
	return accept, false
}

// blocklisted matches by info hash, else by release name and indexer. Expired temporary entries don't match.
func blocklisted(c candidate, in *decisionInput) (api.Rejection, bool) {
	for _, entry := range in.Blocklist {
		if entry.ExpiresAt != nil && !in.Now.Before(*entry.ExpiresAt) {
			continue
		}
		byHash := entry.InfoHash != "" && c.Item.InfoHash != "" && strings.EqualFold(entry.InfoHash, c.Item.InfoHash)
		byName := (entry.InfoHash == "" || c.Item.InfoHash == "") &&
			strings.EqualFold(entry.ReleaseName, c.Item.Title) && entry.Indexer == c.Indexer.Name
		if byHash || byName {
			return permanent(api.RejectBlocklisted, "Blocklisted: %s", strings.ToLower(strings.ReplaceAll(string(entry.Reason), "_", " "))), true
		}
	}
	return accept, false
}

func alreadyImported(c candidate, in *decisionInput) (api.Rejection, bool) {
	if c.Item.InfoHash == "" {
		return accept, false
	}
	for _, f := range in.Files {
		if strings.EqualFold(f.File.InfoHash, c.Item.InfoHash) {
			return permanent(api.RejectAlreadyImported, "This release is already in the library"), true
		}
	}
	return accept, false
}

func torrentSeeding(c candidate, _ *decisionInput) (api.Rejection, bool) {
	if minimum := c.Indexer.Settings.MinSeeders; c.Item.Seeders < minimum {
		return permanent(api.RejectNotEnoughSeeders, "%d seeders, the indexer needs %d", c.Item.Seeders, minimum), true
	}
	return accept, false
}

func blockedIndexer(c candidate, _ *decisionInput) (api.Rejection, bool) {
	if c.Indexer.Status == IndexerFailing {
		return temporary(api.RejectIndexerBackoff, "%s is failing and backed off", c.Indexer.Name), true
	}
	return accept, false
}

// replacedFiles are the existing files the release would replace.
func (in *decisionInput) replacedFiles(c candidate) []existingFile {
	if in.Title.Kind != domain.MediaKindSeries {
		return in.Files
	}
	covered := map[domain.EpisodeID]bool{}
	for _, ep := range c.Episodes {
		covered[ep.ID] = true
	}
	files := []existingFile{}
	for _, f := range in.Files {
		if slices.ContainsFunc(f.File.Episodes, func(ref domain.EpisodeRef) bool { return covered[ref.Episode] }) {
			files = append(files, f)
		}
	}
	return files
}

func upgradable(c candidate, in *decisionInput) (api.Rejection, bool) {
	for _, f := range in.replacedFiles(c) {
		if rejection, rejected := upgradeRejection(in.Profile.Profile(), c.Eval, f); rejected {
			return rejection, true
		}
	}
	return accept, false
}

// upgradeRejection applies the upgrade rule of the acquisition spec, in order.
func upgradeRejection(p api.Profile, release Evaluation, file existingFile) (api.Rejection, bool) {
	current := file.Eval
	cutoff := itemRank(p.Items, p.Cutoff)
	revision := release.Quality.Revision
	switch {
	case file.File.Transcoded:
		return permanent(api.RejectUpgradeNotAllowed, "The existing file was transcoded"), true
	case release.QualityRank > current.QualityRank && current.QualityRank < cutoff:
		return accept, false
	case release.QualityRank < current.QualityRank:
		return permanent(api.RejectNotAnUpgrade, "%s is worse than the existing %s", release.Quality.Name, current.Quality.Name), true
	case release.QualityRank == current.QualityRank && revision > current.Quality.Revision && p.PropersMode != api.PropersDoNotPrefer:
		return accept, false
	case !p.UpgradeAllowed:
		return permanent(api.RejectUpgradeNotAllowed, "%s doesn't allow upgrades", p.Name), true
	case release.QualityRank == current.QualityRank && revision < current.Quality.Revision:
		return permanent(api.RejectNotAnUpgrade, "An older revision than the existing file"), true
	case release.QualityRank > current.QualityRank:
		return permanent(api.RejectNotAnUpgrade, "The existing file already meets the cutoff"), true
	case release.Score <= current.Score:
		return permanent(api.RejectNotAnUpgrade, "Score %d isn't above the existing file's %d", release.Score, current.Score), true
	case current.Score >= p.CutoffFormatScore:
		return permanent(api.RejectNotAnUpgrade, "The existing file already reaches the upgrade-until score %d", p.CutoffFormatScore), true
	case release.Score < current.Score+p.MinUpgradeFormatScore:
		return permanent(api.RejectNotAnUpgrade, "Score %d is less than %d above the existing file's %d", release.Score, p.MinUpgradeFormatScore, current.Score), true
	}
	return accept, false
}

func repack(c candidate, in *decisionInput) (api.Rejection, bool) {
	if !c.Release.Revision.Repack {
		return accept, false
	}
	for _, f := range in.replacedFiles(c) {
		if f.Eval.QualityRank == c.Eval.QualityRank && f.Group != "" && !strings.EqualFold(f.Group, c.Release.Group) {
			return permanent(api.RejectRepackDifferentGroup, "A repack from %s, but the existing file is from %s", c.Release.Group, f.Group), true
		}
	}
	return accept, false
}

func proper(c candidate, in *decisionInput) (api.Rejection, bool) {
	for _, f := range in.replacedFiles(c) {
		isProper := f.Eval.QualityRank == c.Eval.QualityRank && c.Eval.Quality.Revision > f.Eval.Quality.Revision
		if isProper && in.Now.Sub(f.File.ImportedAt) > ProperWindow {
			return permanent(api.RejectProperNotWanted, "A proper for a file imported more than %d days ago", ProperWindowDays), true
		}
	}
	return accept, false
}

func sameEpisodesGrab(c candidate, in *decisionInput) (api.Rejection, bool) {
	if in.Title.Kind != domain.MediaKindSeries {
		return accept, false
	}
	for _, f := range in.replacedFiles(c) {
		if len(f.File.Episodes) > len(c.Episodes) {
			return permanent(api.RejectSameEpisodesGrabbed, "The existing file holds more episodes than this release"), true
		}
	}
	return accept, false
}

// overlapping are the grabs that cover any of the release's items; for a movie, all of them.
func (in *decisionInput) overlapping(c candidate, grabs []activeGrab) []activeGrab {
	if in.Title.Kind != domain.MediaKindSeries {
		return grabs
	}
	found := []activeGrab{}
	for _, g := range grabs {
		if slices.ContainsFunc(c.Episodes, func(ep catalogapi.Episode) bool { return slices.Contains(g.Episodes, ep.ID) }) {
			found = append(found, g)
		}
	}
	return found
}

func coveredBy(c candidate, grabs []activeGrab) bool {
	return slices.ContainsFunc(grabs, func(g activeGrab) bool {
		return g.CutoffMet || g.QualityRank > c.Eval.QualityRank ||
			(g.QualityRank == c.Eval.QualityRank && g.Score >= c.Eval.Score)
	})
}

func queued(c candidate, in *decisionInput) (api.Rejection, bool) {
	if coveredBy(c, in.overlapping(c, in.Queue)) {
		return permanent(api.RejectAlreadyQueued, "A download in the queue is as good or meets the cutoff"), true
	}
	return accept, false
}

func history(c candidate, in *decisionInput) (api.Rejection, bool) {
	if coveredBy(c, in.overlapping(c, in.History)) {
		return permanent(api.RejectInHistory, "A recent grab is as good or meets the cutoff"), true
	}
	return accept, false
}

// pending holds a release with the pending ones covering its items, so the best of them is grabbed when the
// delay passes. A release that skips the delay isn't held.
func pending(c candidate, in *decisionInput) (api.Rejection, bool) {
	if in.bypassesDelay(c) {
		return accept, false
	}
	if len(in.overlapping(c, in.Pending)) > 0 {
		return temporary(api.RejectPending, "A pending release already covers these items"), true
	}
	return accept, false
}

func fullSeason(c candidate, in *decisionInput) (api.Rejection, bool) {
	if !c.Release.FullSeason {
		return accept, false
	}
	for _, ep := range c.Episodes {
		if ep.AirDate == nil || ep.AirDate.After(in.Now) {
			return permanent(api.RejectSeasonNotFullyAired, "Season %d hasn't finished airing", ep.Season), true
		}
	}
	return accept, false
}

// multiSeason accepts a pack of several seasons only when every one of them has a monitored episode.
func multiSeason(c candidate, in *decisionInput) (api.Rejection, bool) {
	if !c.Release.MultiSeason {
		return accept, false
	}
	for _, season := range c.Release.Seasons {
		wanted := slices.ContainsFunc(in.Episodes, func(ep catalogapi.Episode) bool { return ep.Season == season && ep.Monitored })
		if !wanted {
			return permanent(api.RejectSeasonNotWanted, "Season %d isn't wanted", season), true
		}
	}
	return accept, false
}

// seasonPackOnly asks for a pack when the search covers a whole season that has finished airing.
func seasonPackOnly(c candidate, in *decisionInput) (api.Rejection, bool) {
	season, ok := in.searchedSeason()
	if !ok || c.Release.FullSeason || !in.seasonAired(season) {
		return accept, false
	}
	return permanent(api.RejectSeasonPackOnly, "Season %d has ended, so only a season pack is wanted", season), true
}

// searchedSeason is the season a search asked for in full, if it did.
func (in *decisionInput) searchedSeason() (int, bool) {
	if len(in.Searched) == 0 {
		return 0, false
	}
	season := in.Searched[0].Season
	inSeason := 0
	for _, ep := range in.Episodes {
		if ep.Season == season {
			inSeason++
		}
	}
	sameSeason := !slices.ContainsFunc(in.Searched, func(ep catalogapi.Episode) bool { return ep.Season != season })
	return season, sameSeason && len(in.Searched) == inSeason
}

func (in *decisionInput) seasonAired(season int) bool {
	return !slices.ContainsFunc(in.Episodes, func(ep catalogapi.Episode) bool {
		return ep.Season == season && (ep.AirDate == nil || ep.AirDate.After(in.Now))
	})
}

func monitored(c candidate, in *decisionInput) (api.Rejection, bool) {
	if !in.Title.Monitored {
		return permanent(api.RejectNotMonitored, "%s isn't monitored", in.Title.Name), true
	}
	for _, ep := range c.Episodes {
		if !ep.Monitored {
			return permanent(api.RejectNotMonitored, "S%02dE%02d isn't monitored", ep.Season, ep.Number), true
		}
	}
	return accept, false
}

// searchedItems rejects a series release that covers none of the searched episodes.
func searchedItems(c candidate, in *decisionInput) (api.Rejection, bool) {
	if in.Title.Kind != domain.MediaKindSeries || len(in.Searched) == 0 {
		return accept, false
	}
	for _, ep := range c.Episodes {
		if slices.ContainsFunc(in.Searched, func(s catalogapi.Episode) bool { return s.ID == ep.ID }) {
			return accept, false
		}
	}
	return permanent(api.RejectSearchMismatch, "%s wasn't searched for", episodeLabel(c.Release)), true
}

func released(c candidate, in *decisionInput) (api.Rejection, bool) {
	if in.Title.Kind != domain.MediaKindSeries {
		if !in.Title.Released {
			return permanent(api.RejectNotReleased, "%s isn't released yet", in.Title.Name), true
		}
		return accept, false
	}
	for _, ep := range c.Episodes {
		if !ep.Released {
			return permanent(api.RejectNotReleased, "S%02dE%02d hasn't aired yet", ep.Season, ep.Number), true
		}
	}
	return accept, false
}

// delayed holds a release until the delay has passed since the indexer published it.
func delayed(c candidate, in *decisionInput) (api.Rejection, bool) {
	if in.Delay == 0 || in.bypassesDelay(c) {
		return accept, false
	}
	if wait := c.Item.PublishedAt.Add(in.Delay).Sub(in.Now); wait > 0 {
		return temporary(api.RejectDelayed, "Waiting %s for a better release", wait.Round(time.Minute)), true
	}
	return accept, false
}

// bypassesDelay is a release that meets the cutoff, or reaches the profile's maximum attainable score.
func (in *decisionInput) bypassesDelay(c candidate) bool {
	return c.Eval.CutoffMet || (in.MaxScore > 0 && c.Eval.Score >= in.MaxScore)
}

// grabsEarly is a release nothing can beat: at the cutoff with the maximum attainable score.
func (in *decisionInput) grabsEarly(c candidate) bool {
	return c.Eval.CutoffMet && c.Eval.Score >= in.MaxScore
}

func freeSpace(c candidate, in *decisionInput) (api.Rejection, bool) {
	if in.FreeSpace == nil {
		return accept, false
	}
	free, known := in.FreeSpace()
	if !known {
		return accept, false
	}
	if c.Item.SizeBytes+MinFreeSpaceAfterGrab > free {
		return permanent(api.RejectNotEnoughSpace, "Only %s free for %s", formatBytes(free), formatBytes(c.Item.SizeBytes)), true
	}
	return accept, false
}
