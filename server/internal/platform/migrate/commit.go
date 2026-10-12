package migrate

import (
	"context"
	"fmt"
	"slices"
	"strings"

	acquisitionapi "magpie/internal/acquisition/api"
	activityapi "magpie/internal/activity/api"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
)

// CommitStep is one call group of the commit, in the order they run.
type CommitStep string

const (
	StepFormats   CommitStep = "CUSTOM_FORMATS"
	StepProfiles  CommitStep = "QUALITY_PROFILES"
	StepDelays    CommitStep = "DELAY_PROFILES"
	StepTitles    CommitStep = "TITLES"
	StepRoots     CommitStep = "ROOT_FOLDERS"
	StepFiles     CommitStep = "MEDIA_FILES"
	StepBlocklist CommitStep = "BLOCKLIST"
	StepHistory   CommitStep = "HISTORY"
)

var CommitSteps = []CommitStep{StepFormats, StepProfiles, StepDelays, StepTitles, StepRoots, StepFiles, StepBlocklist, StepHistory}

// Records per Importer call; each call is one transaction in its context.
const (
	titleBatch   = 20
	fileBatch    = 25
	historyBatch = 500
)

const (
	sonarrSuffix = " (TV)"
	radarrSuffix = " (Movies)"
)

// Importers are the contexts' migration entrypoints.
type Importers struct {
	Catalog     catalogapi.Importer
	Acquisition acquisitionapi.Importer
	Library     libraryapi.Importer
	Activity    activityapi.Importer
}

// Options are the user's choices for the commit.
type Options struct {
	RenameFolders bool `json:"renameFolders"`
	ImportAliases bool `json:"importAliases"`
}

// Summary is what a commit imported and what it left for the user.
type Summary struct {
	Imported      map[CommitStep]int `json:"imported"`
	SkippedTitles []string           `json:"skippedTitles"`
	MissingFiles  []string           `json:"missingFiles"`
	UnmappedFiles []string           `json:"unmappedFiles"`
}

// committer calls each Importer in step order. Every call is idempotent on source keys, so a resumed
// commit runs every step again: finished ones change nothing and return the IDs later steps need.
type committer struct {
	importers Importers
	plan      *Plan
	options   Options
	progress  func(step CommitStep, done, total int)

	formats  map[domain.SourceKey]acquisitionapi.CustomFormatID
	profiles map[domain.SourceKey]domain.ProfileID
	titles   map[domain.SourceKey]domain.TitleID
	episodes map[domain.SourceKey]domain.EpisodeID
	summary  Summary
}

func newCommitter(importers Importers, plan *Plan, options Options, progress func(CommitStep, int, int)) *committer {
	return &committer{
		importers: importers, plan: plan, options: options, progress: progress,
		formats: map[domain.SourceKey]acquisitionapi.CustomFormatID{}, profiles: map[domain.SourceKey]domain.ProfileID{},
		titles: map[domain.SourceKey]domain.TitleID{}, episodes: map[domain.SourceKey]domain.EpisodeID{},
		summary: Summary{Imported: map[CommitStep]int{}, SkippedTitles: []string{}, MissingFiles: []string{}, UnmappedFiles: []string{}},
	}
}

func (c *committer) run(ctx context.Context) (Summary, error) {
	runs := map[CommitStep]func(context.Context) error{
		StepFormats: c.commitFormats, StepProfiles: c.commitProfiles, StepDelays: c.commitDelays, StepTitles: c.commitTitles,
		StepRoots: c.commitRoots, StepFiles: c.commitFiles, StepBlocklist: c.commitBlocklist, StepHistory: c.commitHistory,
	}
	for _, step := range CommitSteps {
		if err := ctx.Err(); err != nil {
			return Summary{}, err
		}
		if err := runs[step](ctx); err != nil {
			return Summary{}, &StepError{Step: step, Err: err}
		}
	}
	return c.summary, nil
}

// StepError says which step a commit stopped at.
type StepError struct {
	Step CommitStep
	Err  error
}

func (e *StepError) Error() string { return fmt.Sprintf("%s: %v", e.Step, e.Err) }
func (e *StepError) Unwrap() error { return e.Err }

func (c *committer) report(step CommitStep, done, total int) {
	if c.progress != nil {
		c.progress(step, done, total)
	}
}

// batches runs fn over items in slices of size n, reporting progress after each.
func batches[T any](ctx context.Context, c *committer, step CommitStep, items []T, n int, fn func([]T) error) error {
	c.report(step, 0, len(items))
	for start := 0; start < len(items); start += n {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := min(start+n, len(items))
		if err := fn(items[start:end]); err != nil {
			return err
		}
		c.report(step, end, len(items))
	}
	return nil
}

func clashChoice(plan *Plan, kind ClashKind, name string) ClashChoice {
	id := string(kind) + keySeparator + strings.ToLower(strings.TrimSpace(name))
	i := slices.IndexFunc(plan.Reviews.Clashes, func(c Clash) bool { return c.ID == id })
	if i < 0 {
		return ""
	}
	return plan.Reviews.Clashes[i].Choice
}

func suffixed(name string, app App) string {
	if app == AppSonarr {
		return name + sonarrSuffix
	}
	return name + radarrSuffix
}

// chosenApp is the app whose version a clash choice keeps, and whether the choice keeps one version.
func chosenApp(choice ClashChoice) (App, bool) {
	switch choice {
	case ClashSonarr:
		return AppSonarr, true
	case ClashRadarr:
		return AppRadarr, true
	}
	return "", false
}

// finalSpecs are a format's translated specs plus its reviewed ones.
func finalSpecs(plan *Plan, f PlannedFormat) []acquisitionapi.FormatSpec {
	specs := slices.Clone(f.Specs)
	for _, pending := range f.Pending {
		i := slices.IndexFunc(plan.Reviews.Specs, func(r SpecReview) bool { return r.ID == pending.Review })
		if i < 0 || plan.Reviews.Specs[i].Decision != SpecEdited {
			continue
		}
		spec := pending.Spec
		spec.Value = plan.Reviews.Specs[i].Value
		specs = append(specs, spec)
	}
	return specs
}

func (c *committer) commitFormats(ctx context.Context) error {
	in := make([]acquisitionapi.ImportCustomFormat, 0, len(c.plan.Formats))
	for _, f := range c.plan.Formats {
		name, specs := f.Name, finalSpecs(c.plan, f)
		choice := clashChoice(c.plan, ClashFormat, f.Name)
		if app, keepsOne := chosenApp(choice); keepsOne && app != f.App {
			kept := slices.IndexFunc(c.plan.Formats, func(o PlannedFormat) bool { return o.App == app && strings.EqualFold(o.Name, f.Name) })
			specs = finalSpecs(c.plan, c.plan.Formats[kept])
		}
		if choice == ClashKeepBoth {
			name = suffixed(name, f.App)
		}
		in = append(in, acquisitionapi.ImportCustomFormat{Source: f.Source, Name: name, IncludeWhenRenaming: f.IncludeWhenRenaming, Specs: specs})
	}
	c.report(StepFormats, 0, len(in))
	ids, err := c.importers.Acquisition.ImportCustomFormats(ctx, in)
	if err != nil {
		return err
	}
	c.formats = ids
	c.summary.Imported[StepFormats] = len(in)
	c.report(StepFormats, len(in), len(in))
	return nil
}

func (c *committer) commitProfiles(ctx context.Context) error {
	in := make([]acquisitionapi.ImportQualityProfile, 0, len(c.plan.Profiles))
	for _, p := range c.plan.Profiles {
		chosen := p
		choice := clashChoice(c.plan, ClashProfile, p.Name)
		if app, keepsOne := chosenApp(choice); keepsOne && app != p.App {
			kept := slices.IndexFunc(c.plan.Profiles, func(o PlannedProfile) bool { return o.App == app && strings.EqualFold(o.Name, p.Name) })
			chosen = c.plan.Profiles[kept]
		}
		name := p.Name
		if choice == ClashKeepBoth {
			name = suffixed(name, p.App)
		}
		scores := map[acquisitionapi.CustomFormatID]int{}
		for source, score := range chosen.Scores {
			if id, ok := c.formats[source]; ok {
				scores[id] = score
			}
		}
		in = append(in, acquisitionapi.ImportQualityProfile{
			Source: p.Source, Name: name, UpgradeAllowed: chosen.UpgradeAllowed, Items: chosen.Items, Cutoff: chosen.Cutoff,
			MinFormatScore: chosen.MinFormatScore, CutoffFormatScore: chosen.CutoffFormatScore,
			MinUpgradeFormatScore: chosen.MinUpgradeFormatScore, FormatScores: scores,
		})
	}
	c.report(StepProfiles, 0, len(in))
	ids, err := c.importers.Acquisition.ImportQualityProfiles(ctx, in)
	if err != nil {
		return err
	}
	c.profiles = ids
	c.summary.Imported[StepProfiles] = len(in)
	c.report(StepProfiles, len(in), len(in))
	return nil
}

func (c *committer) commitDelays(ctx context.Context) error {
	c.report(StepDelays, 0, len(c.plan.Delays))
	if err := c.importers.Acquisition.ImportDelayProfiles(ctx, c.plan.Delays); err != nil {
		return err
	}
	c.summary.Imported[StepDelays] = len(c.plan.Delays)
	c.report(StepDelays, len(c.plan.Delays), len(c.plan.Delays))
	return nil
}

// importable reports whether a title is committed: matched, and not a daily or anime series the user skipped.
func (c *committer) importable(t PlannedTitle) bool {
	if t.Status != TitleMatched {
		return false
	}
	review := cmpSource(t.SplitFrom, t.Source)
	i := slices.IndexFunc(c.plan.Reviews.SeriesTypes, func(r SeriesTypeReview) bool { return r.ID == string(review) })
	return i < 0 || c.plan.Reviews.SeriesTypes[i].Decision != SeriesTypeSkip
}

func cmpSource(a, b domain.SourceKey) domain.SourceKey {
	if a != "" {
		return a
	}
	return b
}

func (c *committer) commitTitles(ctx context.Context) error {
	var in []catalogapi.ImportTitle
	for _, t := range c.plan.Titles {
		if !c.importable(t) {
			if t.Status != TitleSplit {
				c.summary.SkippedTitles = append(c.summary.SkippedTitles, t.Name)
			}
			continue
		}
		title := catalogapi.ImportTitle{
			Source: t.Source, Kind: t.Kind, Tmdb: t.Tmdb, External: t.External, Profile: c.profiles[t.Profile],
			Monitored: t.Monitored, MonitorNewSeasons: t.MonitorNewSeasons, AddedAt: t.AddedAt, Episodes: []catalogapi.ImportEpisode{},
		}
		if t.EpisodeGroup != "" {
			group := t.EpisodeGroup
			title.EpisodeGroup = &group
		}
		if c.options.ImportAliases {
			title.Aliases = t.Aliases
		}
		for _, e := range t.Episodes {
			if e.Mapped {
				title.Episodes = append(title.Episodes, catalogapi.ImportEpisode{Source: e.Source, Season: e.Season, Number: e.Number, Monitored: e.Monitored})
			}
		}
		in = append(in, title)
	}
	err := batches(ctx, c, StepTitles, in, titleBatch, func(batch []catalogapi.ImportTitle) error {
		got, err := c.importers.Catalog.ImportTitles(ctx, batch)
		for source, id := range got.Titles {
			c.titles[source] = id
		}
		for source, id := range got.Episodes {
			c.episodes[source] = id
		}
		return err
	})
	c.summary.Imported[StepTitles] = len(c.titles)
	return err
}

// titleOf resolves a record's title: the title holding its first imported episode, else the title itself,
// else the first part a split series became.
func (c *committer) titleOf(title domain.SourceKey, episodes []domain.SourceKey) (domain.TitleID, bool) {
	for _, t := range c.plan.Titles {
		if t.Source != title && t.SplitFrom != title {
			continue
		}
		for _, e := range t.Episodes {
			if e.Mapped && slices.Contains(episodes, e.Source) {
				if id, ok := c.titles[t.Source]; ok {
					return id, true
				}
			}
		}
	}
	if id, ok := c.titles[title]; ok {
		return id, true
	}
	for _, t := range c.plan.Titles {
		if t.SplitFrom == title {
			if id, ok := c.titles[t.Source]; ok {
				return id, true
			}
		}
	}
	return "", false
}

func (c *committer) episodeIDs(sources []domain.SourceKey) []domain.EpisodeID {
	ids := []domain.EpisodeID{}
	for _, source := range sources {
		if id, ok := c.episodes[source]; ok {
			ids = append(ids, id)
		}
	}
	return ids
}

// commitRoots imports the root folders that hold an imported title; unused ones aren't needed.
func (c *committer) commitRoots(ctx context.Context) error {
	var in []libraryapi.ImportRootFolder
	for _, r := range c.plan.Roots {
		used := slices.ContainsFunc(c.plan.Titles, func(t PlannedTitle) bool {
			_, imported := c.titles[t.Source]
			return imported && t.App == r.App && hasPathPrefix(t.Path, r.Path)
		})
		if used {
			in = append(in, libraryapi.ImportRootFolder{Source: r.Source, Kind: r.Kind, Path: r.Path})
		}
	}
	c.report(StepRoots, 0, len(in))
	if err := c.importers.Library.ImportRootFolders(ctx, in); err != nil {
		return err
	}
	c.summary.Imported[StepRoots] = len(in)
	c.report(StepRoots, len(in), len(in))
	return nil
}

func (c *committer) commitFiles(ctx context.Context) error {
	var in []libraryapi.ImportMediaFile
	for _, f := range c.plan.Files {
		title, ok := c.titleOf(f.Title, f.Episodes)
		episodes := c.episodeIDs(f.Episodes)
		switch {
		case !ok:
			continue
		case f.Missing:
			c.summary.MissingFiles = append(c.summary.MissingFiles, f.Path)
			continue
		case len(f.Episodes) > 0 && len(episodes) == 0:
			c.summary.UnmappedFiles = append(c.summary.UnmappedFiles, f.Path)
			continue
		}
		in = append(in, libraryapi.ImportMediaFile{
			Source: f.Source, Title: title, Episodes: episodes, Path: f.Path, Size: f.Size, Quality: f.Quality,
			ReleaseName: f.ReleaseName, ReleaseGroup: f.ReleaseGroup, Languages: f.Languages, AddedAt: f.AddedAt,
			Rename: c.options.RenameFolders,
		})
	}
	imported := 0
	err := batches(ctx, c, StepFiles, in, fileBatch, func(batch []libraryapi.ImportMediaFile) error {
		got, err := c.importers.Library.ImportMediaFiles(ctx, batch)
		imported += len(got)
		return err
	})
	c.summary.Imported[StepFiles] = imported
	return err
}

func (c *committer) commitBlocklist(ctx context.Context) error {
	var in []acquisitionapi.ImportBlockedRelease
	for _, b := range c.plan.Blocklist {
		title, ok := c.titleOf(b.Title, b.Episodes)
		if !ok {
			continue
		}
		in = append(in, acquisitionapi.ImportBlockedRelease{Source: b.Source, Title: title, InfoHash: b.InfoHash, ReleaseName: b.ReleaseName, Indexer: b.Indexer, At: b.At})
	}
	c.summary.Imported[StepBlocklist] = len(in)
	return batches(ctx, c, StepBlocklist, in, historyBatch, func(batch []acquisitionapi.ImportBlockedRelease) error {
		return c.importers.Acquisition.ImportBlocklist(ctx, batch)
	})
}

func (c *committer) commitHistory(ctx context.Context) error {
	var in []activityapi.ImportTimelineEvent
	for _, h := range c.plan.History {
		title, ok := c.titleOf(h.Title, h.Episodes)
		if !ok {
			continue
		}
		in = append(in, activityapi.ImportTimelineEvent{
			Source: h.Source, Title: title, Episodes: c.episodeIDs(h.Episodes), Kind: h.Kind, ReleaseName: h.ReleaseName, Reason: h.Reason, At: h.At,
		})
	}
	c.summary.Imported[StepHistory] = len(in)
	return batches(ctx, c, StepHistory, in, historyBatch, func(batch []activityapi.ImportTimelineEvent) error {
		return c.importers.Activity.ImportTimeline(ctx, batch)
	})
}
