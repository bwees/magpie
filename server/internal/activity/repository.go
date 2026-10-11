package activity

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"magpie/internal/activity/api"
	"magpie/internal/domain"
	"magpie/internal/platform/db"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	movieEpisode  = ""
	emptyJSONList = "[]"
)

type refKind string

const (
	refMediaFile refKind = "MEDIA_FILE"
	refDownload  refKind = "DOWNLOAD"
	refSearch    refKind = "SEARCH"
)

var severities = []string{api.SeverityInfo, api.SeverityWarn, api.SeverityError}

type timelineRow struct {
	ID         string `gorm:"primaryKey"`
	EventID    string `gorm:"column:event_id"`
	TitleID    string `gorm:"column:title_id"`
	EpisodeIDs string `gorm:"column:episode_ids"`
	Type       string
	Severity   string
	Text       string
	Detail     string
	At         time.Time
}

func (timelineRow) TableName() string { return "activity_timeline" }

type historyRow struct {
	ID         string  `gorm:"primaryKey"`
	EventID    string  `gorm:"column:event_id"`
	Type       string  `gorm:"column:type"`
	Severity   string  `gorm:"column:severity"`
	TitleID    *string `gorm:"column:title_id"`
	TitleName  string  `gorm:"column:title_name"`
	EpisodeIDs string  `gorm:"column:episode_ids"`
	Summary    string  `gorm:"column:summary"`
	Detail     string  `gorm:"column:detail"`
	At         time.Time
}

func (historyRow) TableName() string { return "activity_history" }

type pipelineRow struct {
	TitleID       string `gorm:"column:title_id;primaryKey"`
	EpisodeID     string `gorm:"column:episode_id;primaryKey"`
	Season        int
	Number        int
	Wanted        bool
	Search        searchFacet
	Download      downloadFacet
	DownloadID    string `gorm:"column:download_id"`
	File          fileFacet
	MediaFileID   string `gorm:"column:media_file_id"`
	Jellyfin      bool
	Note          string
	Stage         api.Stage
	Failed        bool
	Detail        string
	SubsChip      *string `gorm:"column:subs_chip"`
	TranscodeChip *string `gorm:"column:transcode_chip"`
	LastEventID   string  `gorm:"column:last_event_id"`
	UpdatedAt     time.Time
}

func (pipelineRow) TableName() string { return "activity_pipeline" }

type attentionRow struct {
	ID             string `gorm:"primaryKey"`
	Kind           string
	Ref            string
	TitleID        *string `gorm:"column:title_id"`
	Text           string
	Action         string
	OpenedAt       time.Time
	OpenedEventID  string `gorm:"column:opened_event_id"`
	ClearedAt      *time.Time
	ClearedEventID *string `gorm:"column:cleared_event_id"`
}

func (attentionRow) TableName() string { return "activity_attention" }

type notificationRow struct {
	ID        string `gorm:"primaryKey"`
	UserID    string `gorm:"column:user_id"`
	EventID   string `gorm:"column:event_id"`
	Kind      string
	Title     string
	Body      string
	Link      string
	CreatedAt time.Time
	ReadAt    *time.Time
}

func (notificationRow) TableName() string { return "activity_notifications" }

type refRow struct {
	Kind       refKind `gorm:"primaryKey"`
	Ref        string  `gorm:"primaryKey"`
	TitleID    string  `gorm:"column:title_id"`
	EpisodeIDs string  `gorm:"column:episode_ids"`
}

func (refRow) TableName() string { return "activity_refs" }

type removedTitleRow struct {
	TitleID   string `gorm:"column:title_id;primaryKey"`
	RemovedAt time.Time
}

func (removedTitleRow) TableName() string { return "activity_removed_titles" }

// Repository stores Activity's projections and implements api.Reader.
type Repository struct {
	db *db.DB
}

var _ api.Reader = (*Repository)(nil)

func NewRepository(d *db.DB) *Repository {
	return &Repository{db: d}
}

func encodeJSON(v any) (string, error) {
	encoded, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("encoding %T: %w", v, err)
	}
	return string(encoded), nil
}

func encodeEpisodes(episodes []domain.EpisodeID) (string, error) {
	if len(episodes) == 0 {
		return emptyJSONList, nil
	}
	return encodeJSON(episodes)
}

func decodeEpisodes(raw string) ([]domain.EpisodeID, error) {
	episodes := []domain.EpisodeID{}
	if err := json.Unmarshal([]byte(raw), &episodes); err != nil {
		return nil, fmt.Errorf("decoding episode ids: %w", err)
	}
	return episodes, nil
}

func newTimelineRow(title domain.TitleID, e api.TimelineEntry) (timelineRow, error) {
	episodes, err := encodeEpisodes(e.Episodes)
	return timelineRow{
		ID:         e.EventID,
		EventID:    e.EventID,
		TitleID:    string(title),
		EpisodeIDs: episodes,
		Type:       e.Type,
		Severity:   e.Severity,
		Text:       e.Text,
		Detail:     e.Detail,
		At:         e.At.UTC(),
	}, err
}

func (r timelineRow) entry() (api.TimelineEntry, error) {
	episodes, err := decodeEpisodes(r.EpisodeIDs)
	title := domain.TitleID(r.TitleID)
	return api.TimelineEntry{
		EventID: r.EventID, Type: r.Type, Severity: r.Severity, Title: &title,
		Episodes: episodes, Text: r.Text, Detail: r.Detail, At: r.At,
	}, err
}

func newHistoryRow(e api.TimelineEntry) (historyRow, error) {
	episodes, err := encodeEpisodes(e.Episodes)
	row := historyRow{
		ID:         e.EventID,
		EventID:    e.EventID,
		Type:       e.Type,
		Severity:   e.Severity,
		TitleName:  e.TitleName,
		EpisodeIDs: episodes,
		Summary:    e.Text,
		Detail:     e.Detail,
		At:         e.At.UTC(),
	}
	if e.Title != nil {
		title := string(*e.Title)
		row.TitleID = &title
	}
	return row, err
}

func (r historyRow) entry() (api.TimelineEntry, error) {
	episodes, err := decodeEpisodes(r.EpisodeIDs)
	e := api.TimelineEntry{
		EventID: r.EventID, Type: r.Type, Severity: r.Severity, TitleName: r.TitleName,
		Episodes: episodes, Text: r.Summary, Detail: r.Detail, At: r.At,
	}
	if r.TitleID != nil {
		title := domain.TitleID(*r.TitleID)
		e.Title = &title
	}
	return e, err
}

func (r pipelineRow) facets() (facets, error) {
	f := facets{
		Wanted: r.Wanted, Search: r.Search, Download: r.Download, DownloadID: domain.DownloadID(r.DownloadID), File: r.File,
		MediaFile: domain.MediaFileID(r.MediaFileID), Jellyfin: r.Jellyfin, Note: r.Note,
	}
	var err error
	if r.SubsChip != nil {
		f.Subs = &api.SubsChip{}
		err = json.Unmarshal([]byte(*r.SubsChip), f.Subs)
	}
	if r.TranscodeChip != nil && err == nil {
		f.Transcode = &api.TranscodeChip{}
		err = json.Unmarshal([]byte(*r.TranscodeChip), f.Transcode)
	}
	return f, err
}

// setFacets stores f and the stage derived from it.
func (r *pipelineRow) setFacets(f facets) error {
	r.Wanted, r.Search, r.Download, r.File = f.Wanted, f.Search, f.Download, f.File
	r.DownloadID = ""
	if f.Download == downloadActive {
		r.DownloadID = string(f.DownloadID)
	}
	r.MediaFileID, r.Jellyfin, r.Note = string(f.MediaFile), f.Jellyfin, f.Note
	stage := derive(f)
	r.Stage, r.Failed, r.Detail = stage.Stage, stage.Failed, stage.Detail
	r.SubsChip, r.TranscodeChip = nil, nil
	if f.Subs != nil {
		chip, err := encodeJSON(f.Subs)
		if err != nil {
			return err
		}
		r.SubsChip = &chip
	}
	if f.Transcode != nil {
		chip, err := encodeJSON(f.Transcode)
		if err != nil {
			return err
		}
		r.TranscodeChip = &chip
	}
	return nil
}

func (r pipelineRow) row() (api.PipelineRow, error) {
	f, err := r.facets()
	row := api.PipelineRow{
		Title: domain.TitleID(r.TitleID), Stage: r.Stage, Failed: r.Failed, Detail: r.Detail,
		Subs: f.Subs, Transcode: f.Transcode, Updated: r.UpdatedAt,
	}
	if r.EpisodeID != movieEpisode {
		episode := domain.EpisodeID(r.EpisodeID)
		row.Episode = &episode
	}
	if r.DownloadID != "" {
		download := domain.DownloadID(r.DownloadID)
		row.Download = &download
	}
	return row, err
}

func (r attentionRow) item() api.AttentionItem {
	item := api.AttentionItem{ID: r.ID, Kind: r.Kind, Ref: r.Ref, Text: r.Text, Action: r.Action, OpenedAt: r.OpenedAt}
	if r.TitleID != nil {
		title := domain.TitleID(*r.TitleID)
		item.Title = &title
	}
	return item
}

func (r notificationRow) notification() api.Notification {
	return api.Notification{
		ID: r.ID, Kind: r.Kind, Title: r.Title, Body: r.Body, Link: r.Link,
		CreatedAt: r.CreatedAt, Read: r.ReadAt != nil,
	}
}

func convert[R any, T any](rows []R, fn func(R) (T, error)) ([]T, error) {
	out := make([]T, 0, len(rows))
	for _, row := range rows {
		item, err := fn(row)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

// paged applies a Page to a query ordered newest first on column.
func paged(q *gorm.DB, column string, p api.Page) *gorm.DB {
	if p.Before != nil {
		q = q.Where(column+" < ?", p.Before.UTC())
	}
	return q.Order(clause.OrderBy{Columns: []clause.OrderByColumn{
		{Column: clause.Column{Name: column}, Desc: true},
		{Column: clause.Column{Name: "id"}, Desc: true},
	}}).Limit(p.EffectiveLimit())
}

func (r *Repository) Timeline(ctx context.Context, title domain.TitleID, p api.Page) ([]api.TimelineEntry, error) {
	var rows []timelineRow
	if err := paged(r.db.Read(ctx).Where("title_id = ?", title), "at", p).Find(&rows).Error; err != nil {
		return nil, err
	}
	return convert(rows, timelineRow.entry)
}

func (r *Repository) History(ctx context.Context, minSeverity string, p api.Page) ([]api.TimelineEntry, error) {
	q := r.db.Read(ctx)
	if least := slices.Index(severities, minSeverity); least > 0 {
		q = q.Where("severity IN ?", severities[least:])
	}
	var rows []historyRow
	if err := paged(q, "at", p).Find(&rows).Error; err != nil {
		return nil, err
	}
	return convert(rows, historyRow.entry)
}

func (r *Repository) Pipeline(ctx context.Context, title domain.TitleID) ([]api.PipelineRow, error) {
	var rows []pipelineRow
	if err := r.db.Read(ctx).Where("title_id = ?", title).Order("season, number, episode_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	return convert(rows, pipelineRow.row)
}

// TitleStages picks each title's least-advanced row. Rows the user stopped monitoring count only when
// no row of the title is monitored.
func (r *Repository) TitleStages(ctx context.Context, titles []domain.TitleID) (map[domain.TitleID]api.PipelineRow, error) {
	stages := map[domain.TitleID]api.PipelineRow{}
	if len(titles) == 0 {
		return stages, nil
	}
	var rows []pipelineRow
	if err := r.db.Read(ctx).Where("title_id IN ?", titles).Order("title_id, season, number, episode_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	byTitle := map[string][]pipelineRow{}
	for _, row := range rows {
		byTitle[row.TitleID] = append(byTitle[row.TitleID], row)
	}
	for title, rows := range byTitle {
		if wanted := slices.DeleteFunc(slices.Clone(rows), func(r pipelineRow) bool { return !r.Wanted }); len(wanted) > 0 {
			rows = wanted
		}
		least := slices.MinFunc(rows, func(a, b pipelineRow) int {
			return cmp.Compare(slices.Index(api.Stages(), a.Stage), slices.Index(api.Stages(), b.Stage))
		})
		row, err := least.row()
		if err != nil {
			return nil, err
		}
		stages[domain.TitleID(title)] = row
	}
	return stages, nil
}

// ActivePipeline returns rows moving through the main stages or transcoding, most recently changed first.
func (r *Repository) ActivePipeline(ctx context.Context, limit int) ([]api.PipelineRow, error) {
	var rows []pipelineRow
	err := r.db.Read(ctx).
		Where("(failed = ? AND stage IN ?) OR json_extract(transcode_chip, '$.State') = ?",
			false, []api.Stage{api.StageSearching, api.StageDownloading, api.StageImporting}, api.TranscodeRunning).
		Order("updated_at DESC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return convert(rows, pipelineRow.row)
}

func (r *Repository) Notifications(ctx context.Context, user domain.UserID, unreadOnly bool, p api.Page) ([]api.Notification, error) {
	q := r.db.Read(ctx).Where("user_id = ?", user)
	if unreadOnly {
		q = q.Where("read_at IS NULL")
	}
	var rows []notificationRow
	if err := paged(q, "created_at", p).Find(&rows).Error; err != nil {
		return nil, err
	}
	return convert(rows, func(n notificationRow) (api.Notification, error) { return n.notification(), nil })
}

func (r *Repository) UnreadCount(ctx context.Context, user domain.UserID) (int, error) {
	var count int64
	err := r.db.Read(ctx).Model(&notificationRow{}).Where("user_id = ? AND read_at IS NULL", user).Count(&count).Error
	return int(count), err
}

func (r *Repository) Attention(ctx context.Context) ([]api.AttentionItem, error) {
	var rows []attentionRow
	if err := r.db.Read(ctx).Where("cleared_at IS NULL").Order("opened_at, id").Find(&rows).Error; err != nil {
		return nil, err
	}
	return convert(rows, func(a attentionRow) (api.AttentionItem, error) { return a.item(), nil })
}

func insertOnce(tx *gorm.DB, row any) error {
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(row).Error
}

func (r *Repository) addTimeline(tx *gorm.DB, row timelineRow) error { return insertOnce(tx, &row) }

func (r *Repository) addHistory(tx *gorm.DB, row historyRow) error { return insertOnce(tx, &row) }

func (r *Repository) deleteTimeline(tx *gorm.DB, title domain.TitleID) error {
	return tx.Where("title_id = ?", title).Delete(&timelineRow{}).Error
}

func (r *Repository) deletePipeline(tx *gorm.DB, title domain.TitleID) error {
	return tx.Where("title_id = ?", title).Delete(&pipelineRow{}).Error
}

func (r *Repository) pipelineRows(tx *gorm.DB, title domain.TitleID, episodes []string) ([]pipelineRow, error) {
	q := tx.Where("title_id = ?", title)
	if episodes != nil {
		q = q.Where("episode_id IN ?", episodes)
	}
	var rows []pipelineRow
	return rows, q.Order("season, number, episode_id").Find(&rows).Error
}

func (r *Repository) pipelineRowsOfFile(tx *gorm.DB, file domain.MediaFileID) ([]pipelineRow, error) {
	var rows []pipelineRow
	return rows, tx.Where("media_file_id = ?", file).Order("season, number, episode_id").Find(&rows).Error
}

// savePipelineRow upserts: a movie's episode key is "", which gorm's Save would take for an unset key.
func (r *Repository) savePipelineRow(tx *gorm.DB, row *pipelineRow) error {
	return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(row).Error
}

func (r *Repository) titleFailed(tx *gorm.DB, title domain.TitleID) (*pipelineRow, error) {
	var rows []pipelineRow
	err := tx.Where("title_id = ? AND failed = ?", title, true).Order("season, number, episode_id").Limit(1).Find(&rows).Error
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return &rows[0], nil
}

func (r *Repository) remember(tx *gorm.DB, kind refKind, ref string, title domain.TitleID, episodes []domain.EpisodeID) error {
	encoded, err := encodeEpisodes(episodes)
	if err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{UpdateAll: true}).
		Create(&refRow{Kind: kind, Ref: ref, TitleID: string(title), EpisodeIDs: encoded}).Error
}

// lookup returns what a media file, download or search belongs to.
func (r *Repository) lookup(tx *gorm.DB, kind refKind, ref string) (domain.TitleID, []domain.EpisodeID, bool, error) {
	var row refRow
	err := tx.Where("kind = ? AND ref = ?", kind, ref).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil, false, nil
	}
	if err != nil {
		return "", nil, false, err
	}
	episodes, err := decodeEpisodes(row.EpisodeIDs)
	return domain.TitleID(row.TitleID), episodes, err == nil, err
}

func (r *Repository) markRemoved(tx *gorm.DB, title domain.TitleID, at time.Time) error {
	return insertOnce(tx, &removedTitleRow{TitleID: string(title), RemovedAt: at.UTC()})
}

func (r *Repository) removed(tx *gorm.DB, title domain.TitleID) (bool, error) {
	var count int64
	err := tx.Model(&removedTitleRow{}).Where("title_id = ?", title).Count(&count).Error
	return count > 0, err
}

func (r *Repository) openAttention(tx *gorm.DB, kind, ref string) (*attentionRow, error) {
	var rows []attentionRow
	err := tx.Where("kind = ? AND ref = ? AND cleared_at IS NULL", kind, ref).Limit(1).Find(&rows).Error
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return &rows[0], nil
}

// attentionSeen reports whether ev already opened or cleared an item of this kind.
func (r *Repository) attentionSeen(tx *gorm.DB, kind, eventID string) (bool, error) {
	var count int64
	err := tx.Model(&attentionRow{}).
		Where("kind = ? AND (opened_event_id = ? OR cleared_event_id = ?)", kind, eventID, eventID).
		Count(&count).Error
	return count > 0, err
}

func (r *Repository) addAttention(tx *gorm.DB, row *attentionRow) error {
	return tx.Create(row).Error
}

func (r *Repository) clearAttention(tx *gorm.DB, row *attentionRow, eventID string, at time.Time) error {
	at = at.UTC()
	row.ClearedAt, row.ClearedEventID = &at, &eventID
	return tx.Save(row).Error
}

func (r *Repository) openAttentionCount(tx *gorm.DB) (int, error) {
	var count int64
	err := tx.Model(&attentionRow{}).Where("cleared_at IS NULL").Count(&count).Error
	return int(count), err
}

// pruneHistory deletes history rows recorded before cutoff.
func (r *Repository) pruneHistory(ctx context.Context, cutoff time.Time) error {
	return r.db.Write(ctx, func(tx *gorm.DB) error {
		return tx.Where("at < ?", cutoff.UTC()).Delete(&historyRow{}).Error
	})
}
