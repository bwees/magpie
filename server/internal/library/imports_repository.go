package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ImportStatus is where the import of one download stands.
type ImportStatus string

const (
	ImportPending  ImportStatus = "PENDING"
	ImportImported ImportStatus = "IMPORTED"
	// ImportFailed: the files couldn't be read or placed, even after retries.
	ImportFailed ImportStatus = "FAILED"
	// ImportManual: a video couldn't be matched to an episode, so nothing was imported.
	ImportManual ImportStatus = "MANUAL"
)

func (s ImportStatus) Valid() bool {
	switch s {
	case ImportPending, ImportImported, ImportFailed, ImportManual:
		return true
	}
	return false
}

var ErrImportNotFound = errors.New("library: import not found")

// ImportFile is one video or subtitle of a download, as the last attempt saw it.
type ImportFile struct {
	Path string   `json:"path"`
	Size int64    `json:"size"`
	Kind FileKind `json:"kind"`
	// Sample is set for a video too short for its title.
	Sample bool `json:"sample"`
	// Episodes the attempt matched the video to; empty when it couldn't.
	Episodes []domain.EpisodeRef `json:"episodes"`
}

// FileMapping imports one video of a download as the given episodes; a movie's video has none.
type FileMapping struct {
	Path     string             `json:"path"`
	Episodes []domain.EpisodeID `json:"episodes"`
}

// Import is the import of one download.
type Import struct {
	Download    domain.DownloadID
	Title       domain.TitleID
	ReleaseName string
	Status      ImportStatus
	Attempts    int
	LastError   string
	Payload     acquisitionapi.DownloadCompleted
	Files       []ImportFile
	// Mapping is the user's file → episodes choice; nil for an automatic import.
	Mapping   []FileMapping
	CreatedAt time.Time
	UpdatedAt time.Time
}

type importRow struct {
	DownloadID  domain.DownloadID `gorm:"primaryKey"`
	TitleID     domain.TitleID
	ReleaseName string
	Status      ImportStatus
	Attempts    int
	LastError   string
	Payload     string
	Files       string
	Mapping     *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (importRow) TableName() string { return "library_imports" }

func (row importRow) imp() (Import, error) {
	imp := Import{
		Download: row.DownloadID, Title: row.TitleID, ReleaseName: row.ReleaseName, Status: row.Status,
		Attempts: row.Attempts, LastError: row.LastError, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	if err := json.Unmarshal([]byte(row.Payload), &imp.Payload); err != nil {
		return Import{}, fmt.Errorf("decoding the payload of import %s: %w", row.DownloadID, err)
	}
	if err := json.Unmarshal([]byte(row.Files), &imp.Files); err != nil {
		return Import{}, fmt.Errorf("decoding the files of import %s: %w", row.DownloadID, err)
	}
	if row.Mapping != nil {
		if err := json.Unmarshal([]byte(*row.Mapping), &imp.Mapping); err != nil {
			return Import{}, fmt.Errorf("decoding the mapping of import %s: %w", row.DownloadID, err)
		}
	}
	return imp, nil
}

// insertImport stores a new pending import. It reports false when the download already has one.
func insertImport(tx *gorm.DB, completed acquisitionapi.DownloadCompleted, now time.Time) (bool, error) {
	payload, err := json.Marshal(completed)
	if err != nil {
		return false, err
	}
	row := importRow{
		DownloadID: completed.Download, TitleID: completed.Title, ReleaseName: completed.ReleaseName,
		Status: ImportPending, Payload: string(payload), Files: emptyJSONList, CreatedAt: now, UpdatedAt: now,
	}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	return result.RowsAffected > 0, result.Error
}

const emptyJSONList = "[]"

func (r *Repository) Import(ctx context.Context, id domain.DownloadID) (Import, error) {
	return importIn(r.db.Read(ctx), id)
}

func importIn(conn *gorm.DB, id domain.DownloadID) (Import, error) {
	var rows []importRow
	if err := conn.Where("download_id = ?", id).Limit(1).Find(&rows).Error; err != nil {
		return Import{}, err
	}
	if len(rows) == 0 {
		return Import{}, ErrImportNotFound
	}
	return rows[0].imp()
}

// Imports lists imports in the given statuses, most recently changed first.
func (r *Repository) Imports(ctx context.Context, statuses []ImportStatus) ([]Import, error) {
	var rows []importRow
	if err := r.db.Read(ctx).Where("status IN ?", statuses).Order("updated_at DESC, download_id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	imports := make([]Import, 0, len(rows))
	for _, row := range rows {
		imp, err := row.imp()
		if err != nil {
			return nil, err
		}
		imports = append(imports, imp)
	}
	return imports, nil
}

// moveImport changes an import only while it is in one of from, and reports whether it changed.
func moveImport(tx *gorm.DB, id domain.DownloadID, from []ImportStatus, changes map[string]any) (bool, error) {
	result := tx.Model(&importRow{}).Where("download_id = ? AND status IN ?", id, from).Updates(changes)
	return result.RowsAffected > 0, result.Error
}

func encodeFiles(files []ImportFile) (string, error) {
	encoded, err := json.Marshal(files)
	return string(encoded), err
}

func encodeMapping(mapping []FileMapping) (*string, error) {
	if mapping == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(mapping)
	text := string(encoded)
	return &text, err
}
