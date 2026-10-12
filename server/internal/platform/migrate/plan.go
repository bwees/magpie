package migrate

import (
	"slices"
	"time"

	acquisitionapi "magpie/internal/acquisition/api"
	activityapi "magpie/internal/activity/api"
	"magpie/internal/domain"
)

// Plan is what a dry run found: every record to import, in source-key terms, plus the review queues.
// Commit resolves the source keys to Magpie IDs as each Importer returns them.
type Plan struct {
	BuiltAt   time.Time                           `json:"builtAt"`
	Formats   []PlannedFormat                     `json:"formats"`
	Profiles  []PlannedProfile                    `json:"profiles"`
	Delays    []acquisitionapi.ImportDelayProfile `json:"delays"`
	Titles    []PlannedTitle                      `json:"titles"`
	Roots     []PlannedRoot                       `json:"roots"`
	Files     []PlannedFile                       `json:"files"`
	Blocklist []PlannedBlock                      `json:"blocklist"`
	History   []PlannedHistory                    `json:"history"`
	Reviews   Reviews                             `json:"reviews"`
	Warnings  []string                            `json:"warnings"`
}

type TitleStatus string

const (
	TitleMatched  TitleStatus = "MATCHED"
	TitleInReview TitleStatus = "REVIEW"
	TitleSkipped  TitleStatus = "SKIPPED"
	// TitleSplit is a Sonarr series imported as several Magpie titles, which are planned separately.
	TitleSplit TitleStatus = "SPLIT"
)

type PlannedTitle struct {
	Source            domain.SourceKey   `json:"source"`
	App               App                `json:"app"`
	ArrID             int                `json:"arrId"`
	Kind              domain.MediaKind   `json:"kind"`
	Name              string             `json:"name"`
	Year              int                `json:"year"`
	Tmdb              domain.TmdbID      `json:"tmdb"`
	External          domain.ExternalIDs `json:"external"`
	Profile           domain.SourceKey   `json:"profile"`
	Monitored         bool               `json:"monitored"`
	MonitorNewSeasons bool               `json:"monitorNewSeasons"`
	AddedAt           time.Time          `json:"addedAt"`
	Path              string             `json:"path"`
	Aliases           []string           `json:"aliases"`
	Status            TitleStatus        `json:"status"`
	EpisodeGroup      string             `json:"episodeGroup"`
	Episodes          []PlannedEpisode   `json:"episodes"`
	// Series holds what Sonarr said about a series, so its episodes can be mapped again after a review.
	Series *ArrSeries `json:"series,omitempty"`
	// SplitFrom is the Sonarr series a split title came from.
	SplitFrom domain.SourceKey `json:"splitFrom,omitempty"`
}

// PlannedEpisode is a Sonarr episode and where it lands in Magpie's numbering.
type PlannedEpisode struct {
	Source    domain.SourceKey `json:"source"`
	ArrSeason int              `json:"arrSeason"`
	ArrNumber int              `json:"arrNumber"`
	Season    int              `json:"season"`
	Number    int              `json:"number"`
	Mapped    bool             `json:"mapped"`
	Monitored bool             `json:"monitored"`
}

type PlannedProfile struct {
	Source                domain.SourceKey                   `json:"source"`
	App                   App                                `json:"app"`
	Name                  string                             `json:"name"`
	UpgradeAllowed        bool                               `json:"upgradeAllowed"`
	Items                 []acquisitionapi.ImportProfileItem `json:"items"`
	Cutoff                int                                `json:"cutoff"`
	MinFormatScore        int                                `json:"minFormatScore"`
	CutoffFormatScore     int                                `json:"cutoffFormatScore"`
	MinUpgradeFormatScore int                                `json:"minUpgradeFormatScore"`
	// Scores are keyed by the format's source key.
	Scores map[domain.SourceKey]int `json:"scores"`
}

type PlannedRoot struct {
	Source domain.SourceKey `json:"source"`
	App    App              `json:"app"`
	Kind   domain.MediaKind `json:"kind"`
	From   string           `json:"from"`
	Path   string           `json:"path"`
}

type PlannedFile struct {
	Source       domain.SourceKey   `json:"source"`
	App          App                `json:"app"`
	Title        domain.SourceKey   `json:"title"`
	Episodes     []domain.SourceKey `json:"episodes"`
	Path         string             `json:"path"`
	Size         int64              `json:"size"`
	Quality      domain.Quality     `json:"quality"`
	ReleaseName  string             `json:"releaseName"`
	ReleaseGroup string             `json:"releaseGroup"`
	Languages    []domain.Language  `json:"languages"`
	AddedAt      time.Time          `json:"addedAt"`
	// Missing is set when the server can't see the file at its rewritten path.
	Missing bool `json:"missing"`
}

type PlannedBlock struct {
	Source      domain.SourceKey   `json:"source"`
	Title       domain.SourceKey   `json:"title"`
	Episodes    []domain.SourceKey `json:"episodes"`
	ReleaseName string             `json:"releaseName"`
	Indexer     string             `json:"indexer"`
	InfoHash    string             `json:"infoHash"`
	At          time.Time          `json:"at"`
}

type PlannedHistory struct {
	Source      domain.SourceKey       `json:"source"`
	Title       domain.SourceKey       `json:"title"`
	Episodes    []domain.SourceKey     `json:"episodes"`
	Kind        activityapi.ImportKind `json:"kind"`
	ReleaseName string                 `json:"releaseName"`
	Reason      string                 `json:"reason"`
	At          time.Time              `json:"at"`
}

// ReviewQueue names one of the dry run's review queues.
type ReviewQueue string

const (
	QueueTitles      ReviewQueue = "TITLES"
	QueueNumbering   ReviewQueue = "NUMBERING"
	QueueClashes     ReviewQueue = "CLASHES"
	QueueSpecs       ReviewQueue = "SPECS"
	QueueSeriesTypes ReviewQueue = "SERIES_TYPES"
)

var ReviewQueues = []ReviewQueue{QueueTitles, QueueNumbering, QueueClashes, QueueSpecs, QueueSeriesTypes}

type Reviews struct {
	Titles      []TitleReview      `json:"titles"`
	Numbering   []NumberingReview  `json:"numbering"`
	Clashes     []Clash            `json:"clashes"`
	Specs       []SpecReview       `json:"specs"`
	SeriesTypes []SeriesTypeReview `json:"seriesTypes"`
}

type TitleDecision string

const (
	TitlePending     TitleDecision = "PENDING"
	TitleDecideMatch TitleDecision = "MATCH"
	TitleDecideSplit TitleDecision = "SPLIT"
	TitleDecideSkip  TitleDecision = "SKIP"
)

// TitleReview is a title that didn't match TMDB by ID.
type TitleReview struct {
	ID         string           `json:"id"` // the title's source key
	App        App              `json:"app"`
	Kind       domain.MediaKind `json:"kind"`
	Name       string           `json:"name"`
	Year       int              `json:"year"`
	Candidates []TmdbCandidate  `json:"candidates"`
	Decision   TitleDecision    `json:"decision"`
	Tmdb       []domain.TmdbID  `json:"tmdb"`
}

type NumberingDecision string

const (
	NumberingPending NumberingDecision = "PENDING"
	NumberingGroup   NumberingDecision = "GROUP"
	NumberingAired   NumberingDecision = "AIRED"
)

// GroupOption is a TMDB episode group scored against Sonarr's season structure (1 = every season matches).
type GroupOption struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Type  int     `json:"type"`
	Score float64 `json:"score"`
}

// NumberingReview is a series whose seasons differ from TMDB's aired order.
type NumberingReview struct {
	ID            string            `json:"id"` // the series' source key
	Name          string            `json:"name"`
	SonarrSeasons map[int]int       `json:"sonarrSeasons"`
	TmdbSeasons   map[int]int       `json:"tmdbSeasons"`
	Groups        []GroupOption     `json:"groups"`
	Suggested     string            `json:"suggested"`
	Decision      NumberingDecision `json:"decision"`
	Group         string            `json:"group"`
}

type ClashKind string

const (
	ClashFormat  ClashKind = "FORMAT"
	ClashProfile ClashKind = "PROFILE"
)

type ClashChoice string

const (
	ClashPending  ClashChoice = "PENDING"
	ClashSonarr   ClashChoice = "SONARR"
	ClashRadarr   ClashChoice = "RADARR"
	ClashKeepBoth ClashChoice = "KEEP_BOTH"
)

// Clash is a format or profile both apps have under one name with different contents.
type Clash struct {
	ID     string      `json:"id"`
	Kind   ClashKind   `json:"kind"`
	Name   string      `json:"name"`
	Sonarr []string    `json:"sonarr"`
	Radarr []string    `json:"radarr"`
	Choice ClashChoice `json:"choice"`
}

type SpecDecision string

const (
	SpecPending SpecDecision = "PENDING"
	SpecDropped SpecDecision = "DROP"
	SpecEdited  SpecDecision = "EDIT"
)

// SpecReview is a custom-format condition that didn't translate.
type SpecReview struct {
	ID             string                        `json:"id"`
	Format         domain.SourceKey              `json:"format"`
	FormatName     string                        `json:"formatName"`
	App            App                           `json:"app"`
	SpecName       string                        `json:"specName"`
	Implementation string                        `json:"implementation"`
	Kind           acquisitionapi.FormatSpecKind `json:"kind"`
	RawValue       string                        `json:"rawValue"`
	Reason         string                        `json:"reason"`
	Options        []string                      `json:"options"`
	Decision       SpecDecision                  `json:"decision"`
	Value          string                        `json:"value"`
}

type SeriesTypeDecision string

const (
	SeriesTypePending SeriesTypeDecision = "PENDING"
	SeriesTypeImport  SeriesTypeDecision = "IMPORT"
	SeriesTypeSkip    SeriesTypeDecision = "SKIP"
)

// SeriesTypeReview is a daily or anime series, which Magpie imports only as a standard series.
type SeriesTypeReview struct {
	ID       string             `json:"id"` // the series' source key
	Name     string             `json:"name"`
	Type     string             `json:"type"`
	Decision SeriesTypeDecision `json:"decision"`
}

// Pending counts the open items per queue.
func (r Reviews) Pending() map[ReviewQueue]int {
	return map[ReviewQueue]int{
		QueueTitles:      countWhere(r.Titles, func(x TitleReview) bool { return x.Decision == TitlePending }),
		QueueNumbering:   countWhere(r.Numbering, func(x NumberingReview) bool { return x.Decision == NumberingPending }),
		QueueClashes:     countWhere(r.Clashes, func(x Clash) bool { return x.Choice == ClashPending }),
		QueueSpecs:       countWhere(r.Specs, func(x SpecReview) bool { return x.Decision == SpecPending }),
		QueueSeriesTypes: countWhere(r.SeriesTypes, func(x SeriesTypeReview) bool { return x.Decision == SeriesTypePending }),
	}
}

// Cleared reports whether every queue is empty or decided.
func (r Reviews) Cleared() bool {
	for _, n := range r.Pending() {
		if n > 0 {
			return false
		}
	}
	return true
}

func countWhere[T any](items []T, keep func(T) bool) int {
	return len(slices.DeleteFunc(slices.Clone(items), func(x T) bool { return !keep(x) }))
}

func (p *Plan) title(source domain.SourceKey) *PlannedTitle {
	i := slices.IndexFunc(p.Titles, func(t PlannedTitle) bool { return t.Source == source })
	if i < 0 {
		return nil
	}
	return &p.Titles[i]
}
