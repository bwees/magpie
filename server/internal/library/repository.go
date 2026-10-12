package library

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"magpie/internal/domain"
	"magpie/internal/library/api"
	"magpie/internal/platform/db"

	"gorm.io/gorm"
)

const (
	// StagingDirName is the folder under each root that holds transcoded files until Library swaps them.
	StagingDirName = ".magpie-staging"

	newestFirst   = "imported_at DESC, id DESC"
	episodeOrder  = "season, number"
	rootFolderAge = "created_at, id"
)

type rootFolderRow struct {
	ID        string `gorm:"primaryKey"`
	Kind      domain.MediaKind
	Path      string
	DeviceID  uint64
	CreatedAt time.Time
}

func (rootFolderRow) TableName() string { return "library_root_folders" }

type mediaFileRow struct {
	ID             domain.MediaFileID `gorm:"primaryKey"`
	TitleID        domain.TitleID
	RootFolderID   string
	Path           string
	Size           int64
	Quality        string
	ReleaseName    string
	OSHash         string `gorm:"column:os_hash"`
	MediaInfo      string
	ETag           string `gorm:"column:etag"`
	Origin         domain.Origin
	ImportedAt     time.Time
	DownloadID     *domain.DownloadID
	InfoHash       string
	Transcoded     bool
	TranscodedAt   *time.Time
	OriginalSize   *int64
	JellyfinItemID *string
	AvailableAt    *time.Time
}

func (mediaFileRow) TableName() string { return "library_media_files" }

type mediaFileEpisodeRow struct {
	MediaFileID domain.MediaFileID `gorm:"primaryKey"`
	EpisodeID   domain.EpisodeID   `gorm:"primaryKey"`
	Season      int
	Number      int
}

func (mediaFileEpisodeRow) TableName() string { return "library_media_file_episodes" }

// Repository is Library's store and its api.Reader.
type Repository struct {
	db *db.DB
}

var _ api.Reader = (*Repository)(nil)

func NewRepository(d *db.DB) *Repository {
	return &Repository{db: d}
}

// storedFile is a media file as Library keeps it: the Reader's view plus its root folder's ID and its ETag.
type storedFile struct {
	api.MediaFile
	RootFolderID string
	ETag         string
}

func insertMediaFile(tx *gorm.DB, f storedFile) error {
	quality, err := json.Marshal(f.Quality)
	if err != nil {
		return err
	}
	info, err := json.Marshal(f.MediaInfo)
	if err != nil {
		return err
	}
	row := mediaFileRow{
		ID: f.ID, TitleID: f.Title, RootFolderID: f.RootFolderID, Path: f.Path, Size: f.Size,
		Quality: string(quality), ReleaseName: f.ReleaseName, OSHash: f.OSHash, MediaInfo: string(info),
		ETag: f.ETag, Origin: f.Origin, ImportedAt: f.ImportedAt, DownloadID: f.Download, InfoHash: f.InfoHash,
		Transcoded: f.Transcoded,
	}
	if f.Available {
		row.AvailableAt = &row.ImportedAt
	}
	if err := tx.Create(&row).Error; err != nil {
		return err
	}
	for _, ref := range f.Episodes {
		episode := mediaFileEpisodeRow{MediaFileID: f.ID, EpisodeID: ref.Episode, Season: ref.Season, Number: ref.Number}
		if err := tx.Create(&episode).Error; err != nil {
			return err
		}
	}
	return nil
}

// fileRecord is one media file row with its root folder's path.
type fileRecord struct {
	mediaFileRow
	RootPath string
}

func (r *Repository) records(ctx context.Context, where string, args ...any) ([]fileRecord, error) {
	var rows []mediaFileRow
	if err := r.db.Read(ctx).Where(where, args...).Order(newestFirst).Find(&rows).Error; err != nil {
		return nil, err
	}
	var roots []rootFolderRow
	if err := r.db.Read(ctx).Find(&roots).Error; err != nil {
		return nil, err
	}
	paths := map[string]string{}
	for _, root := range roots {
		paths[root.ID] = root.Path
	}
	records := make([]fileRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, fileRecord{mediaFileRow: row, RootPath: paths[row.RootFolderID]})
	}
	return records, nil
}

func (r *Repository) files(ctx context.Context, where string, args ...any) ([]storedFile, error) {
	rows, err := r.records(ctx, where, args...)
	if err != nil || len(rows) == 0 {
		return []storedFile{}, err
	}
	ids := make([]domain.MediaFileID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	var episodes []mediaFileEpisodeRow
	if err := r.db.Read(ctx).Where("media_file_id IN ?", ids).Order(episodeOrder).Find(&episodes).Error; err != nil {
		return nil, err
	}
	byFile := map[domain.MediaFileID][]domain.EpisodeRef{}
	for _, e := range episodes {
		byFile[e.MediaFileID] = append(byFile[e.MediaFileID], domain.EpisodeRef{Episode: e.EpisodeID, Season: e.Season, Number: e.Number})
	}
	files := make([]storedFile, 0, len(rows))
	for _, row := range rows {
		episodes := byFile[row.ID]
		if episodes == nil {
			episodes = []domain.EpisodeRef{}
		}
		f, err := row.stored(episodes)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, nil
}

func (row fileRecord) stored(episodes []domain.EpisodeRef) (storedFile, error) {
	var quality domain.Quality
	if err := json.Unmarshal([]byte(row.Quality), &quality); err != nil {
		return storedFile{}, fmt.Errorf("decoding quality of media file %s: %w", row.ID, err)
	}
	var info api.MediaInfo
	if err := json.Unmarshal([]byte(row.MediaInfo), &info); err != nil {
		return storedFile{}, fmt.Errorf("decoding media info of media file %s: %w", row.ID, err)
	}
	return storedFile{
		MediaFile: api.MediaFile{
			ID: row.ID, Title: row.TitleID, Episodes: episodes, RootFolder: row.RootPath, Path: row.Path,
			Size: row.Size, Quality: quality, ReleaseName: row.ReleaseName, OSHash: row.OSHash, MediaInfo: info,
			Origin: row.Origin, Download: row.DownloadID, InfoHash: row.InfoHash, Transcoded: row.Transcoded,
			ImportedAt: row.ImportedAt, Available: row.AvailableAt != nil,
		},
		RootFolderID: row.RootFolderID,
		ETag:         row.ETag,
	}, nil
}

func (r *Repository) file(ctx context.Context, id domain.MediaFileID) (storedFile, error) {
	files, err := r.files(ctx, "id = ?", id)
	if err != nil {
		return storedFile{}, err
	}
	if len(files) == 0 {
		return storedFile{}, api.ErrNotFound
	}
	return files[0], nil
}

func publicFiles(files []storedFile) []api.MediaFile {
	out := make([]api.MediaFile, 0, len(files))
	for _, f := range files {
		out = append(out, f.MediaFile)
	}
	return out
}

func (r *Repository) FilesForTitle(ctx context.Context, title domain.TitleID) ([]api.MediaFile, error) {
	files, err := r.files(ctx, "title_id = ?", title)
	return publicFiles(files), err
}

func (r *Repository) FilesForTitles(ctx context.Context, titles []domain.TitleID, f api.FileFilter) ([]api.MediaFile, error) {
	if len(titles) == 0 {
		return []api.MediaFile{}, nil
	}
	where, args := "title_id IN ?", []any{titles}
	if f.Transcoded != nil {
		where, args = where+" AND transcoded = ?", append(args, *f.Transcoded)
	}
	files, err := r.files(ctx, where, args...)
	if err != nil {
		return nil, err
	}
	position := make(map[domain.TitleID]int, len(titles))
	for i, title := range titles {
		position[title] = i
	}
	slices.SortStableFunc(files, func(a, b storedFile) int {
		return cmp.Compare(position[a.Title], position[b.Title])
	})
	return publicFiles(files), nil
}

func (r *Repository) FileForEpisode(ctx context.Context, ep domain.EpisodeID) (api.MediaFile, error) {
	files, err := r.files(ctx, "id IN (?)", r.db.Read(ctx).Model(&mediaFileEpisodeRow{}).Select("media_file_id").Where("episode_id = ?", ep))
	if err != nil {
		return api.MediaFile{}, err
	}
	if len(files) == 0 {
		return api.MediaFile{}, api.ErrNotFound
	}
	return files[0].MediaFile, nil
}

func (r *Repository) File(ctx context.Context, id domain.MediaFileID) (api.MediaFile, error) {
	f, err := r.file(ctx, id)
	return f.MediaFile, err
}

func (r *Repository) SourceForProcessing(ctx context.Context, id domain.MediaFileID) (api.ProcessingSource, error) {
	f, err := r.file(ctx, id)
	if err != nil {
		return api.ProcessingSource{}, err
	}
	return api.ProcessingSource{
		Path:       f.Path,
		ETag:       f.ETag,
		Size:       f.Size,
		StagingDir: filepath.Join(f.RootFolder, StagingDirName),
		MediaInfo:  f.MediaInfo,
	}, nil
}

func (r *Repository) AvailableEpisodes(ctx context.Context, title domain.TitleID) ([]domain.EpisodeID, error) {
	var refs []mediaFileEpisodeRow
	err := r.db.Read(ctx).Table("library_media_file_episodes AS e").
		Select("DISTINCT e.episode_id, e.season, e.number").
		Joins("JOIN library_media_files AS f ON f.id = e.media_file_id").
		Where("f.title_id = ? AND f.available_at IS NOT NULL", title).
		Order("e.season, e.number").Find(&refs).Error
	if err != nil {
		return nil, err
	}
	episodes := make([]domain.EpisodeID, 0, len(refs))
	for _, ref := range refs {
		if !slices.Contains(episodes, ref.EpisodeID) {
			episodes = append(episodes, ref.EpisodeID)
		}
	}
	return episodes, nil
}

func (r *Repository) Coverage(ctx context.Context, titles []domain.TitleID) ([]api.TitleCoverage, error) {
	coverage := make([]api.TitleCoverage, 0, len(titles))
	if len(titles) == 0 {
		return coverage, nil
	}
	var sizes []struct {
		TitleID domain.TitleID
		Size    int64
	}
	err := r.db.Read(ctx).Model(&mediaFileRow{}).Select("title_id, SUM(size) AS size").
		Where("title_id IN ?", titles).Group("title_id").Scan(&sizes).Error
	if err != nil {
		return nil, err
	}
	var episodes []struct {
		TitleID   domain.TitleID
		WithFile  int
		Available int
	}
	err = r.db.Read(ctx).Table("library_media_file_episodes AS e").
		Joins("JOIN library_media_files AS f ON f.id = e.media_file_id").
		Select("f.title_id AS title_id, COUNT(DISTINCT e.episode_id) AS with_file, "+
			"COUNT(DISTINCT CASE WHEN f.available_at IS NOT NULL THEN e.episode_id END) AS available").
		Where("f.title_id IN ?", titles).Group("f.title_id").Scan(&episodes).Error
	if err != nil {
		return nil, err
	}
	byTitle := make(map[domain.TitleID]api.TitleCoverage, len(titles))
	for _, s := range sizes {
		c := byTitle[s.TitleID]
		c.SizeBytes = s.Size
		byTitle[s.TitleID] = c
	}
	for _, e := range episodes {
		c := byTitle[e.TitleID]
		c.EpisodesWithFile, c.EpisodesAvailable = e.WithFile, e.Available
		byTitle[e.TitleID] = c
	}
	for _, title := range titles {
		c := byTitle[title]
		c.Title = title
		coverage = append(coverage, c)
	}
	return coverage, nil
}

// RootFolder is a library folder Library imports into.
type RootFolder struct {
	ID     string
	Kind   domain.MediaKind
	Path   string
	Device uint64
}

var (
	ErrRootFolderNotFound = errors.New("library: root folder not found")
	ErrNoRootFolder       = errors.New("library: no root folder for this kind of title")
)

func (row rootFolderRow) folder() RootFolder {
	return RootFolder{ID: row.ID, Kind: row.Kind, Path: row.Path, Device: row.DeviceID}
}

// RootFolders lists every root folder, oldest first.
func (r *Repository) RootFolders(ctx context.Context) ([]RootFolder, error) {
	var rows []rootFolderRow
	if err := r.db.Read(ctx).Order(rootFolderAge).Find(&rows).Error; err != nil {
		return nil, err
	}
	folders := make([]RootFolder, 0, len(rows))
	for _, row := range rows {
		folders = append(folders, row.folder())
	}
	return folders, nil
}

// rootFolderFor is the oldest root folder of a kind; imports go there.
func (r *Repository) rootFolderFor(ctx context.Context, kind domain.MediaKind) (RootFolder, error) {
	var rows []rootFolderRow
	if err := r.db.Read(ctx).Where("kind = ?", kind).Order(rootFolderAge).Limit(1).Find(&rows).Error; err != nil {
		return RootFolder{}, err
	}
	if len(rows) == 0 {
		return RootFolder{}, ErrNoRootFolder
	}
	return rows[0].folder(), nil
}

func insertRootFolder(tx *gorm.DB, f RootFolder, at time.Time) error {
	return tx.Create(&rootFolderRow{ID: f.ID, Kind: f.Kind, Path: f.Path, DeviceID: f.Device, CreatedAt: at}).Error
}
