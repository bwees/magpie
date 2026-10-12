package library

import (
	"context"
	"errors"
	"time"

	"magpie/internal/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ScanFolderStatus is where a top-level folder a scan found stands.
type ScanFolderStatus string

const (
	// ScanFolderFound: reported to Catalog, waiting for a match.
	ScanFolderFound ScanFolderStatus = "FOUND"
	// ScanFolderLinked: its videos are media files of the matched title.
	ScanFolderLinked ScanFolderStatus = "LINKED"
	// ScanFolderIgnored: matched, but it holds no playable video.
	ScanFolderIgnored ScanFolderStatus = "IGNORED"
)

var errScanFolderMissing = errors.New("library: no scan folder at that path")

type scanFolderRow struct {
	Path         string `gorm:"primaryKey"`
	RootFolderID string
	Kind         domain.MediaKind
	Status       ScanFolderStatus
	TitleID      *domain.TitleID
	FoundAt      time.Time
	UpdatedAt    time.Time
}

func (scanFolderRow) TableName() string { return "library_scan_folders" }

func scanFolderIn(conn *gorm.DB, path string) (scanFolderRow, error) {
	var row scanFolderRow
	err := conn.Where("path = ?", path).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return scanFolderRow{}, errScanFolderMissing
	}
	return row, err
}

func (r *Repository) scanFolders(ctx context.Context) ([]scanFolderRow, error) {
	var rows []scanFolderRow
	err := r.db.Read(ctx).Order("path").Find(&rows).Error
	return rows, err
}

// saveScanFolder inserts the row, or updates its status and title while keeping when it was first found.
func saveScanFolder(tx *gorm.DB, row scanFolderRow) error {
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "path"}},
		DoUpdates: clause.AssignmentColumns([]string{"root_folder_id", "kind", "status", "title_id", "updated_at"}),
	}).Create(&row).Error
}

func setScanFolderStatus(tx *gorm.DB, path string, status ScanFolderStatus, at time.Time) error {
	return tx.Model(&scanFolderRow{}).Where("path = ?", path).Updates(map[string]any{"status": status, "updated_at": at}).Error
}

// ScanCounts are the scan folders by status.
type ScanCounts struct {
	Found, Linked, Ignored int
}

func (r *Repository) scanCounts(ctx context.Context) (ScanCounts, error) {
	var rows []struct {
		Status ScanFolderStatus
		Count  int
	}
	err := r.db.Read(ctx).Model(&scanFolderRow{}).Select("status, COUNT(*) AS count").Group("status").Scan(&rows).Error
	var counts ScanCounts
	for _, row := range rows {
		counts.Found += row.Count
		switch row.Status {
		case ScanFolderLinked:
			counts.Linked = row.Count
		case ScanFolderIgnored:
			counts.Ignored = row.Count
		}
	}
	return counts, err
}

// mediaFilePaths are the paths of every media file, for telling which folders already hold the library's files.
func (r *Repository) mediaFilePaths(ctx context.Context) ([]string, error) {
	var paths []string
	err := r.db.Read(ctx).Model(&mediaFileRow{}).Pluck("path", &paths).Error
	return paths, err
}

// RenameProblems are the reasons a title's files kept their old names after a renumbering, by file.
func (r *Repository) RenameProblems(ctx context.Context, title domain.TitleID) (map[domain.MediaFileID]string, error) {
	var rows []struct {
		ID            domain.MediaFileID
		RenameProblem string
	}
	err := r.db.Read(ctx).Model(&mediaFileRow{}).Select("id, rename_problem").
		Where("title_id = ? AND rename_problem <> ''", title).Scan(&rows).Error
	problems := make(map[domain.MediaFileID]string, len(rows))
	for _, row := range rows {
		problems[row.ID] = row.RenameProblem
	}
	return problems, err
}

func flagRenameProblem(tx *gorm.DB, id domain.MediaFileID, problem string) error {
	return tx.Model(&mediaFileRow{}).Where("id = ?", id).Update("rename_problem", problem).Error
}

// moveMediaFile records a renamed file's new path and episode numbers and clears any earlier rename problem.
func moveMediaFile(tx *gorm.DB, id domain.MediaFileID, path, etag string, episodes []domain.EpisodeRef) error {
	err := tx.Model(&mediaFileRow{}).Where("id = ?", id).
		Updates(map[string]any{"path": path, "etag": etag, "rename_problem": ""}).Error
	if err != nil {
		return err
	}
	if err := tx.Where("media_file_id = ?", id).Delete(&mediaFileEpisodeRow{}).Error; err != nil {
		return err
	}
	for _, ref := range episodes {
		row := mediaFileEpisodeRow{MediaFileID: id, EpisodeID: ref.Episode, Season: ref.Season, Number: ref.Number}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}
