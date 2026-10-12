package acquisition

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"magpie/internal/acquisition/api"
	"magpie/internal/domain"

	"gorm.io/gorm"
)

// download is a stored Download plus what Acquisition needs to manage it in qBittorrent.
type download struct {
	api.Download
	IndexerID int
	// QbitHash is the torrent's ID in qBittorrent.
	QbitHash  string
	Release   domain.ReleaseID
	SizeBytes int64
	Trigger   api.SearchTrigger
	Override  bool
	// Failure says why the download was dropped.
	Failure string
}

type downloadRow struct {
	ID          domain.DownloadID `gorm:"primaryKey"`
	TitleID     domain.TitleID
	EpisodeIDs  string
	SearchID    *domain.SearchID
	ReleaseID   domain.ReleaseID
	ReleaseName string
	IndexerID   int
	Indexer     string
	InfoHash    string
	QbitHash    string
	Quality     string
	CFScore     int `gorm:"column:cf_score"`
	SizeBytes   int64
	State       api.DownloadState
	Progress    float64
	SeedingMode api.SeedingMode
	Trigger     api.SearchTrigger
	Override    bool
	Failure     string
	GrabbedAt   time.Time
	CompletedAt *time.Time
	RemovedAt   *time.Time
}

func (downloadRow) TableName() string { return "acquisition_downloads" }

// ErrDownloadNotFound is an unknown download ID.
var ErrDownloadNotFound = errors.New("acquisition: download not found")

func insertDownload(tx *gorm.DB, d download) error {
	row, err := toDownloadRow(d)
	if err != nil {
		return err
	}
	return tx.Create(&row).Error
}

func toDownloadRow(d download) (downloadRow, error) {
	episodes, err := json.Marshal(nonNil(d.Episodes))
	if err != nil {
		return downloadRow{}, err
	}
	quality, err := json.Marshal(d.Quality)
	if err != nil {
		return downloadRow{}, err
	}
	return downloadRow{
		ID: d.ID, TitleID: d.Title, EpisodeIDs: string(episodes), SearchID: d.Search, ReleaseID: d.Release,
		ReleaseName: d.ReleaseName, IndexerID: d.IndexerID, Indexer: d.Indexer, InfoHash: d.InfoHash,
		QbitHash: d.QbitHash, Quality: string(quality), CFScore: d.CFScore, SizeBytes: d.SizeBytes, State: d.State,
		Progress: d.Progress, SeedingMode: d.SeedingMode, Trigger: d.Trigger, Override: d.Override,
		Failure: d.Failure, GrabbedAt: d.GrabbedAt, CompletedAt: d.CompletedAt, RemovedAt: d.RemovedAt,
	}, nil
}

func (row downloadRow) download() (download, error) {
	var episodes []domain.EpisodeID
	if err := json.Unmarshal([]byte(row.EpisodeIDs), &episodes); err != nil {
		return download{}, fmt.Errorf("decoding episodes of download %s: %w", row.ID, err)
	}
	var quality domain.Quality
	if err := json.Unmarshal([]byte(row.Quality), &quality); err != nil {
		return download{}, fmt.Errorf("decoding quality of download %s: %w", row.ID, err)
	}
	return download{
		Download: api.Download{
			ID: row.ID, Title: row.TitleID, Episodes: episodes, Search: row.SearchID, ReleaseName: row.ReleaseName,
			Indexer: row.Indexer, InfoHash: row.InfoHash, Quality: quality, CFScore: row.CFScore, State: row.State,
			Progress: row.Progress, SeedingMode: row.SeedingMode, GrabbedAt: row.GrabbedAt,
			CompletedAt: row.CompletedAt, RemovedAt: row.RemovedAt,
		},
		IndexerID: row.IndexerID, QbitHash: row.QbitHash, Release: row.ReleaseID, SizeBytes: row.SizeBytes,
		Trigger: row.Trigger, Override: row.Override, Failure: row.Failure,
	}, nil
}

func downloadsFrom(rows []downloadRow) ([]download, error) {
	list := make([]download, 0, len(rows))
	for _, row := range rows {
		d, err := row.download()
		if err != nil {
			return nil, err
		}
		list = append(list, d)
	}
	return list, nil
}

// Download returns one download. ErrDownloadNotFound for an unknown ID.
func (r *Repository) Download(ctx context.Context, id domain.DownloadID) (download, error) {
	var rows []downloadRow
	if err := r.db.Read(ctx).Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
		return download{}, err
	}
	if len(rows) == 0 {
		return download{}, ErrDownloadNotFound
	}
	return rows[0].download()
}

// activeDownloads lists downloads still in qBittorrent, newest first.
func (r *Repository) activeDownloads(ctx context.Context) ([]download, error) {
	var rows []downloadRow
	if err := r.db.Read(ctx).Where("removed_at IS NULL").Order("grabbed_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return downloadsFrom(rows)
}

// titleDownloads lists every download of a title, removed ones included, newest first.
func (r *Repository) titleDownloads(ctx context.Context, title domain.TitleID) ([]download, error) {
	var rows []downloadRow
	if err := r.db.Read(ctx).Where("title_id = ?", title).Order("grabbed_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return downloadsFrom(rows)
}

func (r *Repository) hasActiveDownloads(ctx context.Context) (bool, error) {
	var found int64
	err := r.db.Read(ctx).Model(&downloadRow{}).Where("removed_at IS NULL AND state NOT IN ?", finishedStates).Limit(1).Count(&found).Error
	return found > 0, err
}

// moveDownload changes a download's state only while it is still in one of from, so a repeated
// check can't apply the same change twice. It reports whether the row changed.
func moveDownload(tx *gorm.DB, id domain.DownloadID, from []api.DownloadState, changes map[string]any) (bool, error) {
	result := tx.Model(&downloadRow{}).Where("id = ? AND removed_at IS NULL AND state IN ?", id, from).Updates(changes)
	return result.RowsAffected > 0, result.Error
}

// ActiveDownloads lists downloads not yet removed from qBittorrent, newest first.
func (r *Repository) ActiveDownloads(ctx context.Context) ([]api.Download, error) {
	list, err := r.activeDownloads(ctx)
	return publicDownloads(list), err
}

// DownloadsForTitle lists every download of a title, including removed ones, newest first.
func (r *Repository) DownloadsForTitle(ctx context.Context, title domain.TitleID) ([]api.Download, error) {
	list, err := r.titleDownloads(ctx, title)
	return publicDownloads(list), err
}

func publicDownloads(list []download) []api.Download {
	out := make([]api.Download, 0, len(list))
	for _, d := range list {
		out = append(out, d.Download)
	}
	return out
}
