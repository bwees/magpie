package activity

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"time"

	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/activity/api"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	identityapi "magpie/internal/identity/api"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/config"
	"magpie/internal/platform/db"
	"magpie/internal/platform/jobs"
	"magpie/internal/platform/settings"
	requestsapi "magpie/internal/requests/api"

	"go.uber.org/fx"
	"gorm.io/gorm"
)

// ErrTitleHidden is returned when a requester asks about a title they didn't request.
var ErrTitleHidden = errors.New("activity: title not visible to this user")

// refDefiningTypes are the events that say which title and episodes a search, download or file belongs to.
var refDefiningTypes = map[string]refKind{
	acquisitionapi.TypeSearchStarted:     refSearch,
	acquisitionapi.TypeReleaseGrabbed:    refDownload,
	acquisitionapi.TypeDownloadCompleted: refDownload,
	libraryapi.TypeFileImported:          refMediaFile,
}

type ServiceParams struct {
	fx.In

	Repository *Repository
	DB         *db.DB
	Bus        *bus.Bus
	Catalog    catalogapi.Reader
	Requests   requestsapi.Reader
	Identity   identityapi.Reader
	Settings   *settings.Store
	Sender     NotificationSender
	Jobs       jobs.Enqueuer
	Env        config.Env
}

type Service struct {
	repo     *Repository
	bus      *bus.Bus
	catalog  catalogapi.Reader
	requests requestsapi.Reader
	identity identityapi.Reader
	settings *settings.Store
	sender   NotificationSender
	jobs     jobs.Enqueuer
	// baseURL makes external notification links absolute; nil leaves them out.
	baseURL *url.URL
	now     func() time.Time
}

func NewService(p ServiceParams) *Service {
	return &Service{
		repo:     p.Repository,
		bus:      p.Bus,
		catalog:  p.Catalog,
		requests: p.Requests,
		identity: p.Identity,
		settings: p.Settings,
		sender:   p.Sender,
		jobs:     p.Jobs,
		baseURL:  p.Env.BaseURL,
		now:      time.Now,
	}
}

// RecordTimeline writes the event onto its title's timeline. TitleRemoved deletes the timeline instead.
func (s *Service) RecordTimeline(_ context.Context, tx *gorm.DB, ev bus.Event) error {
	d, err := s.described(tx, ev)
	if err != nil || d.Title == nil {
		return err
	}
	title := *d.Title
	if ev.Type == catalogapi.TypeTitleRemoved {
		if err := s.repo.markRemoved(tx, title, ev.OccurredAt); err != nil {
			return err
		}
		if err := s.repo.deleteTimeline(tx, title); err != nil {
			return err
		}
		return s.announce(tx, api.ProjectionTimeline, &title)
	}
	if removed, err := s.repo.removed(tx, title); err != nil || removed {
		return err
	}
	row, err := newTimelineRow(title, entryOf(ev, d))
	if err != nil {
		return err
	}
	if err := s.repo.addTimeline(tx, row); err != nil {
		return err
	}
	return s.announce(tx, api.ProjectionTimeline, &title)
}

// RecordHistory writes every event into the global history, with the title's name as it is now.
func (s *Service) RecordHistory(ctx context.Context, tx *gorm.DB, ev bus.Event) error {
	d, err := s.described(tx, ev)
	if err != nil {
		return err
	}
	entry := entryOf(ev, d)
	entry.Title = d.Title
	if d.Title != nil {
		title, err := s.catalog.Title(ctx, *d.Title)
		switch {
		case errors.Is(err, catalogapi.ErrNotFound):
		case err != nil:
			return err
		default:
			entry.TitleName = title.Name
		}
	}
	row, err := newHistoryRow(entry)
	if err != nil {
		return err
	}
	if err := s.repo.addHistory(tx, row); err != nil {
		return err
	}
	return s.announce(tx, api.ProjectionHistory, nil)
}

// UpdatePipeline applies the event's facet changes to the rows it targets and recomputes their stage.
func (s *Service) UpdatePipeline(ctx context.Context, tx *gorm.DB, ev bus.Event) error {
	if err := s.rememberRefs(tx, ev); err != nil {
		return err
	}
	switch ev.Type {
	case catalogapi.TypeTitleRemoved:
		return s.removePipeline(tx, ev)
	case catalogapi.TypeEpisodeGroupChanged:
		return s.renumber(tx, ev)
	}
	changesOf, ok := pipelineChanges[ev.Type]
	if !ok {
		return nil
	}
	changes, err := changesOf(ev)
	if err != nil {
		return err
	}
	var touched []domain.TitleID
	for _, change := range changes {
		rows, err := s.targetRows(ctx, tx, change)
		if err != nil {
			return err
		}
		for i := range rows {
			applied, err := applyChange(&rows[i], change, ev)
			if err != nil {
				return err
			}
			if !applied {
				continue
			}
			if err := s.repo.savePipelineRow(tx, &rows[i]); err != nil {
				return err
			}
			if title := domain.TitleID(rows[i].TitleID); !slices.Contains(touched, title) {
				touched = append(touched, title)
			}
		}
	}
	for _, title := range touched {
		failed, err := s.repo.titleFailed(tx, title)
		if err != nil {
			return err
		}
		if err := s.applyAttention(tx, ev, pipelineAttention(title, failed)); err != nil {
			return err
		}
		if err := s.announce(tx, api.ProjectionPipeline, &title); err != nil {
			return err
		}
	}
	return nil
}

// UpdateAttention opens and clears attention items. PIPELINE_FAILED items follow the pipeline instead.
func (s *Service) UpdateAttention(_ context.Context, tx *gorm.DB, ev bus.Event) error {
	if err := s.rememberRefs(tx, ev); err != nil {
		return err
	}
	changesOf, ok := attentionChanges[ev.Type]
	if !ok {
		return nil
	}
	changes, err := changesOf(ev)
	if err != nil {
		return err
	}
	for _, change := range changes {
		if err := s.applyAttention(tx, ev, change); err != nil {
			return err
		}
	}
	return nil
}

// PruneHistory deletes history older than the retention setting.
func (s *Service) PruneHistory(ctx context.Context) error {
	days, err := historyRetentionDays.Get(ctx, s.settings)
	if err != nil {
		return err
	}
	return s.repo.pruneHistory(ctx, s.now().AddDate(0, 0, -days))
}

func entryOf(ev bus.Event, d description) api.TimelineEntry {
	return api.TimelineEntry{
		EventID: ev.ID, Type: ev.Type, Severity: d.Severity, Episodes: d.Episodes,
		Text: d.Text, Detail: d.Detail, At: ev.OccurredAt,
	}
}

// described renders ev and fills in its title and episodes from what earlier events said about its
// file, download or search.
func (s *Service) described(tx *gorm.DB, ev bus.Event) (description, error) {
	if err := s.rememberRefs(tx, ev); err != nil {
		return description{}, err
	}
	d := describe(ev)
	if d.Title != nil && len(d.Episodes) > 0 {
		return d, nil
	}
	for _, ref := range []struct {
		kind refKind
		id   string
	}{
		{refMediaFile, ptrValue(d.MediaFile)},
		{refDownload, ptrValue(d.Download)},
		{refSearch, ptrValue(d.Search)},
	} {
		if ref.id == "" {
			continue
		}
		title, episodes, found, err := s.repo.lookup(tx, ref.kind, ref.id)
		if err != nil {
			return d, err
		}
		if !found || (d.Title != nil && *d.Title != title) {
			continue
		}
		d.Title, d.Episodes = &title, episodes
		return d, nil
	}
	return d, nil
}

func ptrValue[T ~string](p *T) string {
	if p == nil {
		return ""
	}
	return string(*p)
}

// rememberRefs records which title and episodes a new search, download or file belongs to.
// Every handler records them, so none depends on another handler's progress.
func (s *Service) rememberRefs(tx *gorm.DB, ev bus.Event) error {
	kind, ok := refDefiningTypes[ev.Type]
	if !ok {
		return nil
	}
	d := describe(ev)
	if d.Title == nil {
		return nil
	}
	ref := map[refKind]string{
		refSearch:    ptrValue(d.Search),
		refDownload:  ptrValue(d.Download),
		refMediaFile: ptrValue(d.MediaFile),
	}[kind]
	if ref == "" {
		return nil
	}
	return s.repo.remember(tx, kind, ref, *d.Title, d.Episodes)
}

func (s *Service) removePipeline(tx *gorm.DB, ev bus.Event) error {
	removed, err := bus.Decode[catalogapi.TitleRemoved](ev)
	if err != nil {
		return err
	}
	if err := s.repo.markRemoved(tx, removed.Title, ev.OccurredAt); err != nil {
		return err
	}
	if err := s.repo.deletePipeline(tx, removed.Title); err != nil {
		return err
	}
	if err := s.applyAttention(tx, ev, pipelineAttention(removed.Title, nil)); err != nil {
		return err
	}
	return s.announce(tx, api.ProjectionPipeline, &removed.Title)
}

func (s *Service) renumber(tx *gorm.DB, ev bus.Event) error {
	changed, err := bus.Decode[catalogapi.EpisodeGroupChanged](ev)
	if err != nil {
		return err
	}
	for _, move := range changed.Moves {
		err := tx.Model(&pipelineRow{}).
			Where("title_id = ? AND episode_id = ?", changed.Title, move.Episode).
			Updates(map[string]any{"season": move.ToSeason, "number": move.ToNo}).Error
		if err != nil {
			return err
		}
	}
	return s.announce(tx, api.ProjectionPipeline, &changed.Title)
}

// targetRows loads the rows a change selects, creating missing ones when the change creates rows.
func (s *Service) targetRows(ctx context.Context, tx *gorm.DB, change pipelineChange) ([]pipelineRow, error) {
	target := change.Target
	if target.File != "" {
		return s.repo.pipelineRowsOfFile(tx, target.File)
	}
	for _, ref := range []struct {
		kind refKind
		id   string
	}{{refDownload, string(target.Download)}, {refSearch, string(target.Search)}} {
		if ref.id == "" {
			continue
		}
		title, episodes, found, err := s.repo.lookup(tx, ref.kind, ref.id)
		if err != nil {
			return nil, err
		}
		if found {
			target.Title, target.Episodes = title, episodes
		}
	}
	if target.Title == "" {
		return nil, nil
	}
	if removed, err := s.repo.removed(tx, target.Title); err != nil || removed {
		return nil, err
	}

	keys := episodeKeys(target)
	rows, err := s.repo.pipelineRows(tx, target.Title, keys)
	if err != nil || !change.Create {
		return rows, err
	}
	missing := slices.DeleteFunc(slices.Clone(keys), func(key string) bool {
		return slices.ContainsFunc(rows, func(r pipelineRow) bool { return r.EpisodeID == key })
	})
	if len(missing) == 0 {
		return rows, nil
	}
	numbering, err := s.numbering(ctx, target.Title, missing)
	if err != nil {
		return nil, err
	}
	for _, key := range missing {
		n := numbering[key]
		rows = append(rows, pipelineRow{TitleID: string(target.Title), EpisodeID: key, Season: n.season, Number: n.number, File: fileNone})
	}
	return rows, nil
}

// episodeKeys are the episode_id values a target selects; nil selects every row of the title.
func episodeKeys(target rowTarget) []string {
	if target.Movie {
		return []string{movieEpisode}
	}
	if len(target.Episodes) == 0 {
		return nil
	}
	keys := make([]string, 0, len(target.Episodes))
	for _, episode := range target.Episodes {
		keys = append(keys, string(episode))
	}
	return keys
}

type episodeNumber struct{ season, number int }

// numbering finds where new episode rows sort. Episodes the catalog doesn't know sort first.
func (s *Service) numbering(ctx context.Context, title domain.TitleID, keys []string) (map[string]episodeNumber, error) {
	numbers := map[string]episodeNumber{}
	if slices.Equal(keys, []string{movieEpisode}) {
		return numbers, nil
	}
	episodes, err := s.catalog.Episodes(ctx, title)
	if errors.Is(err, catalogapi.ErrNotFound) {
		return numbers, nil
	}
	if err != nil {
		return nil, err
	}
	for _, episode := range episodes {
		numbers[string(episode.ID)] = episodeNumber{season: episode.Season, number: episode.Number}
	}
	return numbers, nil
}

// applyChange applies change to row unless the row has already seen a newer event (or this one).
func applyChange(row *pipelineRow, change pipelineChange, ev bus.Event) (bool, error) {
	if ev.ID <= row.LastEventID {
		return false, nil
	}
	f, err := row.facets()
	if err != nil {
		return false, err
	}
	change.Apply(&f)
	if err := row.setFacets(f); err != nil {
		return false, err
	}
	row.LastEventID, row.UpdatedAt = ev.ID, ev.OccurredAt.UTC()
	return true, nil
}

type attentionID string

// applyAttention opens or clears one item and publishes AttentionChanged. An item of that kind already
// opened or cleared by this event, an open item opened again, and a clear with nothing open change nothing.
func (s *Service) applyAttention(tx *gorm.DB, ev bus.Event, change attentionChange) error {
	seen, err := s.repo.attentionSeen(tx, change.Kind, ev.ID)
	if err != nil || seen {
		return err
	}
	open, err := s.repo.openAttention(tx, change.Kind, change.Ref)
	if err != nil {
		return err
	}

	var item *attentionRow
	switch {
	case change.Open && open == nil:
		item = &attentionRow{
			ID: string(domain.NewID[attentionID]()), Kind: change.Kind, Ref: change.Ref,
			Text: change.Text, Action: change.Action, OpenedAt: ev.OccurredAt.UTC(), OpenedEventID: ev.ID,
		}
		if err := s.attentionTitle(tx, change, item); err != nil {
			return err
		}
		if err := s.repo.addAttention(tx, item); err != nil {
			return err
		}
	case !change.Open && open != nil && ev.ID > open.OpenedEventID:
		item = open
		if err := s.repo.clearAttention(tx, item, ev.ID, ev.OccurredAt); err != nil {
			return err
		}
	default:
		return nil
	}

	count, err := s.repo.openAttentionCount(tx)
	if err != nil {
		return err
	}
	return s.bus.Publish(tx, bus.EventOf(api.AttentionChanged{
		Item: item.ID, Kind: item.Kind, Ref: item.Ref, Open: change.Open, Count: count,
	}))
}

func (s *Service) attentionTitle(tx *gorm.DB, change attentionChange, item *attentionRow) error {
	title := change.Title
	if title == nil && change.File != "" {
		found, _, ok, err := s.repo.lookup(tx, refMediaFile, string(change.File))
		if err != nil {
			return err
		}
		if ok {
			title = &found
		}
	}
	if title != nil {
		id := string(*title)
		item.TitleID = &id
	}
	return nil
}

// announce tells live views, once tx commits, that a projection changed.
func (s *Service) announce(tx *gorm.DB, projection string, title *domain.TitleID) error {
	return db.AfterCommit(tx, func() {
		s.bus.Emit(bus.EventOf(api.ProjectionUpdated{Projection: projection, Title: title}))
	})
}

// mayView reports whether viewer may see a title's activity: admins see all, requesters the titles they
// requested, or every requested title with ViewOthersRequests.
func (s *Service) mayView(ctx context.Context, viewer domain.Principal, title domain.TitleID) (bool, error) {
	if viewer.Admin {
		return true, nil
	}
	requesters, err := s.requests.Requesters(ctx, title)
	if err != nil {
		return false, err
	}
	return slices.Contains(requesters, viewer.User) || (viewer.Permissions.ViewOthersRequests && len(requesters) > 0), nil
}

// Timeline returns a title's timeline. Requesters don't see details such as release names.
func (s *Service) Timeline(ctx context.Context, viewer domain.Principal, title domain.TitleID, p api.Page) ([]api.TimelineEntry, error) {
	allowed, err := s.mayView(ctx, viewer, title)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrTitleHidden
	}
	entries, err := s.repo.Timeline(ctx, title, p)
	if err != nil || viewer.Admin {
		return entries, err
	}
	for i := range entries {
		entries[i].Detail = ""
	}
	return entries, nil
}

func (s *Service) Pipeline(ctx context.Context, viewer domain.Principal, title domain.TitleID) ([]api.PipelineRow, error) {
	allowed, err := s.mayView(ctx, viewer, title)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrTitleHidden
	}
	return s.repo.Pipeline(ctx, title)
}

// TitleStages returns the summary row of each title the viewer may see.
func (s *Service) TitleStages(ctx context.Context, viewer domain.Principal, titles []domain.TitleID) (map[domain.TitleID]api.PipelineRow, error) {
	visible, err := s.visible(ctx, viewer, titles)
	if err != nil {
		return nil, err
	}
	return s.repo.TitleStages(ctx, visible)
}

// ActivePipeline returns the rows in "Live now" that the viewer may see.
func (s *Service) ActivePipeline(ctx context.Context, viewer domain.Principal) ([]api.PipelineRow, error) {
	rows, err := s.repo.ActivePipeline(ctx, api.MaxPageLimit)
	if err != nil || viewer.Admin {
		return rows, err
	}
	var titles []domain.TitleID
	for _, row := range rows {
		if !slices.Contains(titles, row.Title) {
			titles = append(titles, row.Title)
		}
	}
	visible, err := s.visible(ctx, viewer, titles)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(rows, func(row api.PipelineRow) bool { return !slices.Contains(visible, row.Title) }), nil
}

func (s *Service) visible(ctx context.Context, viewer domain.Principal, titles []domain.TitleID) ([]domain.TitleID, error) {
	visible := make([]domain.TitleID, 0, len(titles))
	for _, title := range titles {
		allowed, err := s.mayView(ctx, viewer, title)
		if err != nil {
			return nil, err
		}
		if allowed {
			visible = append(visible, title)
		}
	}
	return visible, nil
}

func (s *Service) History(ctx context.Context, minSeverity string, p api.Page) ([]api.TimelineEntry, error) {
	return s.repo.History(ctx, minSeverity, p)
}

func (s *Service) Attention(ctx context.Context) ([]api.AttentionItem, error) {
	return s.repo.Attention(ctx)
}
