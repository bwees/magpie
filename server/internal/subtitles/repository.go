package subtitles

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/platform/db"
	"magpie/internal/subtitles/api"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SubtitleID identifies one stored subtitle.
type SubtitleID string

// sessionID is the only row of the OpenSubtitles session table.
const sessionID = 1

var (
	ErrNotFound         = errors.New("subtitles: no such media file")
	ErrSubtitleNotFound = errors.New("subtitles: no such subtitle")
)

type fileRow struct {
	MediaFileID domain.MediaFileID `gorm:"primaryKey"`
	TitleID     domain.TitleID
	// EpisodeIDs are the episodes the file holds; empty for a movie.
	EpisodeIDs []domain.EpisodeID `gorm:"serializer:json"`
	// Settled is false while any wanted track is in progress; CoverageSettled flips it back.
	Settled    bool
	ImportedAt time.Time
}

func (fileRow) TableName() string { return "subtitles_files" }

type subtitleRow struct {
	ID             SubtitleID `gorm:"primaryKey"`
	MediaFileID    domain.MediaFileID
	Language       domain.Language
	Forced         bool
	SDH            bool `gorm:"column:sdh"`
	Source         api.Source
	StreamIndex    *int
	ProviderFileID *int64
	ReleaseName    string
	Score          *int
	MaxScore       *int
	Synced         bool
	SyncOffsetMs   *int64
	Path           string
	AddedAt        time.Time
	UpgradeUntil   *time.Time
	// JobID is the Whisper job that wrote a WHISPER subtitle.
	JobID *domain.JobID
}

func (subtitleRow) TableName() string { return "subtitles_subtitles" }

func (r subtitleRow) score() (Score, bool) {
	if r.Score == nil || r.MaxScore == nil {
		return Score{}, false
	}
	return Score{Points: *r.Score, Max: *r.MaxScore}, true
}

func (r subtitleRow) api() api.Subtitle {
	sub := api.Subtitle{Language: r.Language, Forced: r.Forced, SDH: r.SDH, Source: r.Source, Synced: r.Synced, Path: r.Path}
	if s, ok := r.score(); ok {
		pct := s.Percent()
		sub.Score = &pct
	}
	return sub
}

type wantedRow struct {
	MediaFileID     domain.MediaFileID `gorm:"primaryKey"`
	Language        domain.Language    `gorm:"primaryKey"`
	Forced          bool               `gorm:"primaryKey"`
	HearingImpaired bool
	Position        int
	Status          WantedStatus
	Source          *api.Source
	Reason          api.MissingReason
	Priority        Priority
	NextAttemptAt   *time.Time
	// Candidate is an accepted subtitle waiting for download quota, as JSON.
	Candidate *string
	CreatedAt time.Time
	// WhisperAttempts is how many Whisper jobs this track asked for since it was last reopened.
	WhisperAttempts int
}

func (wantedRow) TableName() string { return "subtitles_wanted" }

// pendingDownload is an accepted candidate stored on a wanted row until the quota resets.
type pendingDownload struct {
	Candidate Candidate `json:"candidate"`
	Score     Score     `json:"score"`
}

func (r *wantedRow) setCandidate(p pendingDownload) error {
	encoded, err := json.Marshal(p)
	if err != nil {
		return err
	}
	s := string(encoded)
	r.Candidate = &s
	return nil
}

func (r wantedRow) candidate() (pendingDownload, bool, error) {
	if r.Candidate == nil {
		return pendingDownload{}, false, nil
	}
	var p pendingDownload
	err := json.Unmarshal([]byte(*r.Candidate), &p)
	return p, err == nil, err
}

type blockRow struct {
	MediaFileID    domain.MediaFileID `gorm:"primaryKey"`
	ProviderFileID int64              `gorm:"primaryKey"`
}

func (blockRow) TableName() string { return "subtitles_blocklist" }

type sessionRow struct {
	ID                 int `gorm:"primaryKey"`
	DownloadsRemaining *int
	ResetAt            *time.Time
	VIP                bool `gorm:"column:vip"`
	TokenExpiresAt     *time.Time
	// PausedUntil stops OpenSubtitles calls after a rejected login.
	PausedUntil *time.Time
	Problem     string
}

func (sessionRow) TableName() string { return "subtitles_opensubtitles_session" }

// quotaLeft reports whether a download may be tried now, and if not, when it may.
func (s sessionRow) quotaLeft(now time.Time) (bool, time.Time) {
	if s.DownloadsRemaining == nil || *s.DownloadsRemaining > 0 || s.ResetAt == nil || !now.Before(*s.ResetAt) {
		return true, time.Time{}
	}
	return false, *s.ResetAt
}

// paused reports whether OpenSubtitles calls are on hold after a rejected login.
func (s sessionRow) paused(now time.Time) (bool, time.Time) {
	if s.PausedUntil == nil || !now.Before(*s.PausedUntil) {
		return false, time.Time{}
	}
	return true, *s.PausedUntil
}

// fileState is everything Subtitles stores about one media file.
type fileState struct {
	File      fileRow
	Subtitles []subtitleRow
	Wanted    []wantedRow
	Blocked   map[int64]bool
}

// Repository stores subtitles and implements api.Reader.
type Repository struct {
	db *db.DB
}

var _ api.Reader = (*Repository)(nil)

func NewRepository(d *db.DB) *Repository {
	return &Repository{db: d}
}

const (
	subtitleOrder = "language, forced, sdh, added_at, id"
	wantedOrder   = "position, forced"
	fileOrder     = "media_file_id"
)

func (r *Repository) ForFile(ctx context.Context, file domain.MediaFileID) ([]api.Subtitle, error) {
	rows, err := subtitlesOf(r.db.Read(ctx), file)
	if err != nil {
		return nil, err
	}
	out := make([]api.Subtitle, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.api())
	}
	return out, nil
}

func (r *Repository) Coverage(ctx context.Context, file domain.MediaFileID) ([]api.LanguageCoverage, error) {
	rows, err := wantedOf(r.db.Read(ctx), file)
	if err != nil {
		return nil, err
	}
	out := make([]api.LanguageCoverage, 0, len(rows))
	for _, row := range rows {
		out = append(out, api.LanguageCoverage{Language: row.Language, Forced: row.Forced, Status: row.Status.Coverage(), Source: row.Source})
	}
	return out, nil
}

func (r *Repository) Quota(ctx context.Context) (api.Quota, error) {
	s, err := session(r.db.Read(ctx))
	if err != nil {
		return api.Quota{}, err
	}
	q := api.Quota{}
	if s.DownloadsRemaining != nil {
		q.Remaining = *s.DownloadsRemaining
	}
	if s.ResetAt != nil {
		q.ResetAt = s.ResetAt.UTC()
	}
	return q, nil
}

func (r *Repository) state(ctx context.Context, file domain.MediaFileID) (fileState, bool, error) {
	return loadState(r.db.Read(ctx), file)
}

func loadState(conn *gorm.DB, file domain.MediaFileID) (fileState, bool, error) {
	var files []fileRow
	if err := conn.Where("media_file_id = ?", file).Limit(1).Find(&files).Error; err != nil || len(files) == 0 {
		return fileState{}, false, err
	}
	subs, err := subtitlesOf(conn, file)
	if err != nil {
		return fileState{}, false, err
	}
	wanted, err := wantedOf(conn, file)
	if err != nil {
		return fileState{}, false, err
	}
	var blocks []blockRow
	if err := conn.Where("media_file_id = ?", file).Find(&blocks).Error; err != nil {
		return fileState{}, false, err
	}
	blocked := map[int64]bool{}
	for _, b := range blocks {
		blocked[b.ProviderFileID] = true
	}
	return fileState{File: files[0], Subtitles: subs, Wanted: wanted, Blocked: blocked}, true, nil
}

func subtitlesOf(conn *gorm.DB, file domain.MediaFileID) ([]subtitleRow, error) {
	rows := []subtitleRow{}
	err := conn.Where("media_file_id = ?", file).Order(subtitleOrder).Find(&rows).Error
	return rows, err
}

func wantedOf(conn *gorm.DB, file domain.MediaFileID) ([]wantedRow, error) {
	rows := []wantedRow{}
	err := conn.Where("media_file_id = ?", file).Order(wantedOrder).Find(&rows).Error
	return rows, err
}

func subtitle(conn *gorm.DB, id SubtitleID) (subtitleRow, error) {
	var rows []subtitleRow
	if err := conn.Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
		return subtitleRow{}, err
	}
	if len(rows) == 0 {
		return subtitleRow{}, ErrSubtitleNotFound
	}
	return rows[0], nil
}

func session(conn *gorm.DB) (sessionRow, error) {
	var rows []sessionRow
	if err := conn.Where("id = ?", sessionID).Limit(1).Find(&rows).Error; err != nil || len(rows) == 0 {
		return sessionRow{ID: sessionID}, err
	}
	return rows[0], nil
}

func saveSession(tx *gorm.DB, s sessionRow) error {
	s.ID = sessionID
	return tx.Save(&s).Error
}

func insertFile(tx *gorm.DB, f fileRow) error {
	if f.EpisodeIDs == nil {
		f.EpisodeIDs = []domain.EpisodeID{}
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&f).Error
}

func saveFile(tx *gorm.DB, f fileRow) error {
	return tx.Save(&f).Error
}

func saveSubtitle(tx *gorm.DB, s subtitleRow) error {
	return tx.Save(&s).Error
}

func deleteSubtitles(tx *gorm.DB, ids []SubtitleID) error {
	if len(ids) == 0 {
		return nil
	}
	return tx.Where("id IN ?", ids).Delete(&subtitleRow{}).Error
}

// saveWanted upserts: gorm's Save would insert a row whose Forced key is false, the zero value.
func saveWanted(tx *gorm.DB, rows []wantedRow) error {
	for _, row := range rows {
		if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func deleteWanted(tx *gorm.DB, rows []wantedRow) error {
	for _, row := range rows {
		err := tx.Where("media_file_id = ? AND language = ? AND forced = ?", row.MediaFileID, row.Language, row.Forced).
			Delete(&wantedRow{}).Error
		if err != nil {
			return err
		}
	}
	return nil
}

func block(tx *gorm.DB, file domain.MediaFileID, providerFileID int64) error {
	return tx.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&blockRow{MediaFileID: file, ProviderFileID: providerFileID}).Error
}

// deleteFile drops everything stored for a file and returns its sidecar paths.
func deleteFile(tx *gorm.DB, file domain.MediaFileID) ([]string, error) {
	subs, err := subtitlesOf(tx, file)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, s := range subs {
		if s.Path != "" {
			paths = append(paths, s.Path)
		}
	}
	for _, model := range []any{&subtitleRow{}, &wantedRow{}, &blockRow{}, &fileRow{}} {
		if err := tx.Where("media_file_id = ?", file).Delete(model).Error; err != nil {
			return nil, err
		}
	}
	return paths, nil
}

// moveSidecars repoints a file's subtitles from each old path to its new one. Paths the file doesn't use
// are skipped, so a repeat changes nothing.
func setFileEpisodes(tx *gorm.DB, file domain.MediaFileID, episodes []domain.EpisodeID) error {
	if episodes == nil {
		episodes = []domain.EpisodeID{}
	}
	return tx.Model(&fileRow{MediaFileID: file}).Select("EpisodeIDs").Updates(fileRow{EpisodeIDs: episodes}).Error
}

// claimIssue records that a rerun for the issue ran, and reports false when one already had.
func claimIssue(tx *gorm.DB, issue domain.IssueID, at time.Time) (bool, error) {
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&issueRerunRow{IssueID: issue, HandledAt: at})
	return result.RowsAffected > 0, result.Error
}

type issueRerunRow struct {
	IssueID   domain.IssueID `gorm:"primaryKey"`
	HandledAt time.Time
}

func (issueRerunRow) TableName() string { return "subtitles_issue_reruns" }

func moveSidecars(tx *gorm.DB, file domain.MediaFileID, moves []libraryapi.SidecarMove) error {
	for _, m := range moves {
		err := tx.Model(&subtitleRow{}).Where("media_file_id = ? AND path = ?", file, m.OldPath).
			Update("path", m.NewPath).Error
		if err != nil {
			return err
		}
	}
	return nil
}

// pathInUse reports whether any stored subtitle still points at path.
func pathInUse(conn *gorm.DB, path string) (bool, error) {
	var count int64
	err := conn.Model(&subtitleRow{}).Where("path = ?", path).Count(&count).Error
	return count > 0, err
}

// filesAfter lists files in ID order after a cursor, optionally of one title.
func filesAfter(conn *gorm.DB, title *domain.TitleID, after domain.MediaFileID, limit int) ([]fileRow, error) {
	q := conn.Where("media_file_id > ?", after)
	if title != nil {
		q = q.Where("title_id = ?", *title)
	}
	var rows []fileRow
	err := q.Order(fileOrder).Limit(limit).Find(&rows).Error
	return rows, err
}

// dueFiles lists files with a wanted track to search again: missing or pending, due, not waiting on quota.
func dueFiles(conn *gorm.DB, now time.Time) ([]domain.MediaFileID, error) {
	var ids []domain.MediaFileID
	err := conn.Model(&wantedRow{}).
		Where("status IN ? AND candidate IS NULL AND (next_attempt_at IS NULL OR next_attempt_at <= ?)",
			[]WantedStatus{StatusPending, StatusMissing}, now).
		Distinct().Order(fileOrder).Pluck("media_file_id", &ids).Error
	return ids, err
}

// waitingDownloads lists accepted candidates whose quota wait is over, in quota order.
func waitingDownloads(conn *gorm.DB, now time.Time) ([]wantedRow, error) {
	var rows []wantedRow
	if err := conn.Where("candidate IS NOT NULL AND next_attempt_at <= ?", now).Order("created_at").Find(&rows).Error; err != nil {
		return nil, err
	}
	slices.SortStableFunc(rows, func(a, b wantedRow) int { return cmp.Compare(a.Priority.rank(), b.Priority.rank()) })
	return rows, nil
}

// upgradable lists provider subtitles still inside their upgrade window.
func upgradable(conn *gorm.DB, now time.Time) ([]subtitleRow, error) {
	var rows []subtitleRow
	err := conn.Where("source = ? AND upgrade_until > ?", api.SourceProvider, now).Order("added_at, id").Find(&rows).Error
	return rows, err
}
