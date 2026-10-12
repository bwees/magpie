package migrate

import (
	"context"
	"errors"
	"fmt"
	"slices"

	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/domain"
	"magpie/internal/platform/parse"
)

var (
	ErrUnknownReview   = errors.New("migrate: no such review item")
	ErrInvalidDecision = errors.New("migrate: invalid decision")
)

// Decision settles one review item. Only the fields of its queue are read.
type Decision struct {
	Queue      ReviewQueue        `json:"queue"`
	ID         string             `json:"id"`
	Title      TitleDecision      `json:"title,omitempty"`
	Tmdb       []domain.TmdbID    `json:"tmdb,omitempty"`
	Numbering  NumberingDecision  `json:"numbering,omitempty"`
	Group      string             `json:"group,omitempty"`
	Clash      ClashChoice        `json:"clash,omitempty"`
	Spec       SpecDecision       `json:"spec,omitempty"`
	Value      string             `json:"value,omitempty"`
	SeriesType SeriesTypeDecision `json:"seriesType,omitempty"`
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidDecision, fmt.Sprintf(format, args...))
}

func find[T any](items []T, id func(T) string, want string) (*T, error) {
	i := slices.IndexFunc(items, func(x T) bool { return id(x) == want })
	if i < 0 {
		return nil, fmt.Errorf("%w: %s", ErrUnknownReview, want)
	}
	return &items[i], nil
}

// decide applies a decision to the plan. Matching a title or changing a series' numbering reads TMDB.
func decide(ctx context.Context, tmdb TMDB, plan *Plan, d Decision) error {
	switch d.Queue {
	case QueueTitles:
		return decideTitle(ctx, tmdb, plan, d)
	case QueueNumbering:
		return decideNumbering(ctx, tmdb, plan, d)
	case QueueClashes:
		clash, err := find(plan.Reviews.Clashes, func(c Clash) string { return c.ID }, d.ID)
		if err != nil {
			return err
		}
		if !slices.Contains([]ClashChoice{ClashSonarr, ClashRadarr, ClashKeepBoth}, d.Clash) {
			return invalid("unknown choice %q", d.Clash)
		}
		clash.Choice = d.Clash
		return nil
	case QueueSpecs:
		return decideSpec(plan, d)
	case QueueSeriesTypes:
		review, err := find(plan.Reviews.SeriesTypes, func(r SeriesTypeReview) string { return r.ID }, d.ID)
		if err != nil {
			return err
		}
		if d.SeriesType != SeriesTypeImport && d.SeriesType != SeriesTypeSkip {
			return invalid("unknown decision %q", d.SeriesType)
		}
		review.Decision = d.SeriesType
		return nil
	}
	return invalid("unknown queue %q", d.Queue)
}

func decideTitle(ctx context.Context, tmdb TMDB, plan *Plan, d Decision) error {
	review, err := find(plan.Reviews.Titles, func(r TitleReview) string { return r.ID }, d.ID)
	if err != nil {
		return err
	}
	title := plan.title(domain.SourceKey(d.ID))
	if title == nil {
		return fmt.Errorf("%w: %s", ErrUnknownReview, d.ID)
	}
	plan.dropSplit(title.Source)
	switch {
	case d.Title == TitleDecideSkip:
		title.Status = TitleSkipped
	case d.Title == TitleDecideMatch && len(d.Tmdb) == 1:
		if err := matchTitle(ctx, tmdb, plan, title, d.Tmdb[0]); err != nil {
			return err
		}
	case d.Title == TitleDecideSplit && len(d.Tmdb) > 1 && title.Kind == domain.MediaKindSeries:
		if err := splitTitle(ctx, tmdb, plan, title, d.Tmdb); err != nil {
			return err
		}
	default:
		return invalid("%s needs one TMDB ID to match, or several to split a series", d.Title)
	}
	review.Decision, review.Tmdb = d.Title, d.Tmdb
	if review.Tmdb == nil {
		review.Tmdb = []domain.TmdbID{}
	}
	return nil
}

// matchTitle points a title at a TMDB ID; a series' episodes are mapped again and may need a numbering review.
func matchTitle(ctx context.Context, tmdb TMDB, plan *Plan, title *PlannedTitle, id domain.TmdbID) error {
	if title.Kind == domain.MediaKindMovie {
		if _, err := tmdb.Movie(ctx, id); err != nil {
			return err
		}
		title.Tmdb, title.External.Tmdb, title.Status = id, id, TitleMatched
		return nil
	}
	show, err := fetchShow(ctx, tmdb, id)
	if err != nil {
		return err
	}
	title.Tmdb, title.External.Tmdb, title.Status = id, id, TitleMatched
	review, err := planNumbering(ctx, tmdb, title, show)
	if err != nil {
		return err
	}
	plan.Reviews.Numbering = slices.DeleteFunc(plan.Reviews.Numbering, func(r NumberingReview) bool { return r.ID == string(title.Source) })
	if review != nil {
		plan.Reviews.Numbering = append(plan.Reviews.Numbering, *review)
	}
	seriesType := title.Series.SeriesType
	listed := slices.ContainsFunc(plan.Reviews.SeriesTypes, func(r SeriesTypeReview) bool { return r.ID == string(title.Source) })
	if (seriesType == SeriesTypeDaily || seriesType == SeriesTypeAnime) && !listed {
		plan.Reviews.SeriesTypes = append(plan.Reviews.SeriesTypes, SeriesTypeReview{ID: string(title.Source), Name: title.Name, Type: seriesType, Decision: SeriesTypePending})
	}
	return nil
}

// splitTitle imports one Sonarr series as several TMDB series. Each episode goes to the first part the
// crosswalk finds it in; an episode no part has stays unmapped.
func splitTitle(ctx context.Context, tmdb TMDB, plan *Plan, title *PlannedTitle, ids []domain.TmdbID) error {
	claimed := map[domain.SourceKey]bool{}
	var parts []PlannedTitle
	for _, id := range ids {
		show, err := fetchShow(ctx, tmdb, id)
		if err != nil {
			return err
		}
		episodes, err := episodeMapper{tmdb: tmdb, series: *title.Series, show: show, split: true}.mapEpisodes(ctx, nil)
		if err != nil {
			return err
		}
		part := *title
		part.Source, part.SplitFrom, part.Status = splitKey(title.Source, id), title.Source, TitleMatched
		part.Tmdb, part.External.Tmdb, part.Name, part.Year, part.EpisodeGroup = id, id, show.series.Name, show.series.Year, ""
		part.Series, part.Episodes = nil, []PlannedEpisode{}
		for _, e := range episodes {
			if e.Mapped && !claimed[e.Source] {
				claimed[e.Source] = true
				part.Episodes = append(part.Episodes, e)
			}
		}
		parts = append(parts, part)
	}
	title.Status = TitleSplit
	plan.Titles = append(plan.Titles, parts...)
	return nil
}

// dropSplit removes the parts a series was split into before.
func (p *Plan) dropSplit(series domain.SourceKey) {
	p.Titles = slices.DeleteFunc(p.Titles, func(t PlannedTitle) bool { return t.SplitFrom == series })
}

func decideNumbering(ctx context.Context, tmdb TMDB, plan *Plan, d Decision) error {
	review, err := find(plan.Reviews.Numbering, func(r NumberingReview) string { return r.ID }, d.ID)
	if err != nil {
		return err
	}
	title := plan.title(domain.SourceKey(d.ID))
	if title == nil || title.Series == nil {
		return fmt.Errorf("%w: %s", ErrUnknownReview, d.ID)
	}
	var group *TmdbGroup
	switch d.Numbering {
	case NumberingAired:
	case NumberingGroup:
		if !slices.ContainsFunc(review.Groups, func(g GroupOption) bool { return g.ID == d.Group }) {
			return invalid("TMDB lists no group %q for this series", d.Group)
		}
		g, err := tmdb.EpisodeGroup(ctx, d.Group)
		if err != nil {
			return err
		}
		group = &g
	default:
		return invalid("unknown decision %q", d.Numbering)
	}
	show, err := fetchShow(ctx, tmdb, title.Tmdb)
	if err != nil {
		return err
	}
	episodes, err := episodeMapper{tmdb: tmdb, series: *title.Series, show: show}.mapEpisodes(ctx, group)
	if err != nil {
		return err
	}
	title.Episodes = episodes
	title.EpisodeGroup = ""
	if group != nil {
		title.EpisodeGroup = group.ID
	}
	review.Decision, review.Group = d.Numbering, title.EpisodeGroup
	return nil
}

func decideSpec(plan *Plan, d Decision) error {
	review, err := find(plan.Reviews.Specs, func(r SpecReview) string { return r.ID }, d.ID)
	if err != nil {
		return err
	}
	switch d.Spec {
	case SpecDropped:
		review.Decision, review.Value = SpecDropped, ""
		return nil
	case SpecEdited:
	default:
		return invalid("unknown decision %q", d.Spec)
	}
	if !validSpecValue(review.Kind, review.Options, d.Value) {
		return invalid("%q isn't a value a %s condition takes", d.Value, review.Kind)
	}
	review.Decision, review.Value = SpecEdited, d.Value
	return nil
}

func validSpecValue(kind acquisitionapi.FormatSpecKind, options []string, value string) bool {
	switch {
	case kind == "" || value == "":
		return false
	case len(options) > 0:
		return slices.Contains(options, value)
	case kind == acquisitionapi.SpecLanguage:
		_, ok := parse.LanguageCode(value)
		return ok || value == languageOrigin
	}
	return true
}

// skipQueue settles every open item of a queue with its safe default: unmatched titles and daily or anime
// series are skipped, the suggested numbering is kept, clashing items are kept under both names, and
// untranslated specs are dropped.
func skipQueue(plan *Plan, queue ReviewQueue) error {
	r := &plan.Reviews
	switch queue {
	case QueueTitles:
		for i := range r.Titles {
			if r.Titles[i].Decision == TitlePending {
				r.Titles[i].Decision = TitleDecideSkip
				if title := plan.title(domain.SourceKey(r.Titles[i].ID)); title != nil {
					title.Status = TitleSkipped
				}
			}
		}
	case QueueNumbering:
		for i := range r.Numbering {
			if r.Numbering[i].Decision == NumberingPending {
				r.Numbering[i].Decision, r.Numbering[i].Group = NumberingAired, r.Numbering[i].Suggested
				if r.Numbering[i].Suggested != "" {
					r.Numbering[i].Decision = NumberingGroup
				}
			}
		}
	case QueueClashes:
		for i := range r.Clashes {
			if r.Clashes[i].Choice == ClashPending {
				r.Clashes[i].Choice = ClashKeepBoth
			}
		}
	case QueueSpecs:
		for i := range r.Specs {
			if r.Specs[i].Decision == SpecPending {
				r.Specs[i].Decision = SpecDropped
			}
		}
	case QueueSeriesTypes:
		for i := range r.SeriesTypes {
			if r.SeriesTypes[i].Decision == SeriesTypePending {
				r.SeriesTypes[i].Decision = SeriesTypeSkip
			}
		}
	default:
		return invalid("unknown queue %q", queue)
	}
	return nil
}
