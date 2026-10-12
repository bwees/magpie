package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"magpie/internal/domain"

	"gorm.io/gorm"
)

const oldestFirst = "created_at, id"

type scanReviewRow struct {
	ID          string `gorm:"primaryKey"`
	FolderPath  string
	Kind        domain.MediaKind
	ParsedTitle string
	ParsedYear  *int
	TmdbHint    *domain.TmdbID
	Candidates  string
	State       ScanReviewState
	TitleID     *domain.TitleID
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (scanReviewRow) TableName() string { return "catalog_scan_review" }

func (row scanReviewRow) review() (ScanReview, error) {
	var candidates []ScanCandidate
	if err := json.Unmarshal([]byte(row.Candidates), &candidates); err != nil {
		return ScanReview{}, fmt.Errorf("decoding candidates of scan review %s: %w", row.ID, err)
	}
	return ScanReview{
		ID: row.ID, Folder: row.FolderPath, Kind: row.Kind, ParsedTitle: row.ParsedTitle, ParsedYear: row.ParsedYear,
		TmdbHint: row.TmdbHint, Candidates: candidates, State: row.State, Title: row.TitleID, CreatedAt: row.CreatedAt.UTC(),
	}, nil
}

var errScanReviewMissing = errors.New("catalog: no scan review for that folder")

func (r *Repository) scanReviewByPath(conn *gorm.DB, path string) (scanReviewRow, error) {
	var row scanReviewRow
	err := conn.Where("folder_path = ?", path).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return scanReviewRow{}, errScanReviewMissing
	}
	return row, err
}

func (r *Repository) scanReview(conn *gorm.DB, id string) (scanReviewRow, error) {
	var row scanReviewRow
	err := conn.Where("id = ?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return scanReviewRow{}, ErrScanReviewNotFound
	}
	return row, err
}

func (r *Repository) scanReviews(conn *gorm.DB, state ScanReviewState) ([]scanReviewRow, error) {
	var rows []scanReviewRow
	err := conn.Where("state = ?", state).Order(oldestFirst).Find(&rows).Error
	return rows, err
}

func (r *Repository) insertScanReview(tx *gorm.DB, row scanReviewRow) error {
	return tx.Create(&row).Error
}

// resolveScanReview moves a pending row to state; it reports false when the row isn't pending.
func (r *Repository) resolveScanReview(tx *gorm.DB, id string, state ScanReviewState, title *domain.TitleID, at time.Time) (bool, error) {
	result := tx.Model(&scanReviewRow{}).Where("id = ? AND state = ?", id, ScanReviewPending).
		Updates(map[string]any{"state": state, "title_id": title, "updated_at": at})
	return result.RowsAffected > 0, result.Error
}

// forgetScanMatches drops the folders matched to a removed Title, so a later scan can match them again.
func (r *Repository) forgetScanMatches(tx *gorm.DB, title domain.TitleID) error {
	return tx.Where("title_id = ?", title).Delete(&scanReviewRow{}).Error
}
