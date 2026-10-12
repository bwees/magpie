package acquisition

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"magpie/internal/acquisition/api"
	"magpie/internal/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// FileScore is Acquisition's own view of one library file: what it was scored from, its CF score under the
// title's profile, and whether that meets the cutoff.
type FileScore struct {
	File     domain.MediaFileID
	Title    domain.TitleID
	Episodes []domain.EpisodeID
	Quality  domain.Quality
	// ReleaseName and Path are what the score is computed from: the release name, else the file's path.
	ReleaseName string
	Path        string
	SizeBytes   int64
	CFScore     int
	// Profile is the title's profile at the last evaluation.
	Profile    domain.ProfileID
	CutoffMet  bool
	Transcoded bool
	Origin     domain.Origin
}

type fileScoreRow struct {
	MediaFileID domain.MediaFileID `gorm:"primaryKey"`
	TitleID     domain.TitleID
	EpisodeIDs  string
	Quality     string
	ReleaseName string
	Path        string
	SizeBytes   int64
	CFScore     int `gorm:"column:cf_score"`
	ProfileID   domain.ProfileID
	CutoffMet   bool
	Transcoded  bool
	Origin      domain.Origin
	EvaluatedAt time.Time
}

func (fileScoreRow) TableName() string { return "acquisition_file_scores" }

func toFileScoreRow(f FileScore, at time.Time) (fileScoreRow, error) {
	episodes, err := json.Marshal(nonNil(f.Episodes))
	if err != nil {
		return fileScoreRow{}, err
	}
	quality, err := json.Marshal(f.Quality)
	if err != nil {
		return fileScoreRow{}, err
	}
	return fileScoreRow{
		MediaFileID: f.File, TitleID: f.Title, EpisodeIDs: string(episodes), Quality: string(quality),
		ReleaseName: f.ReleaseName, Path: f.Path, SizeBytes: f.SizeBytes, CFScore: f.CFScore, ProfileID: f.Profile,
		CutoffMet: f.CutoffMet, Transcoded: f.Transcoded, Origin: f.Origin, EvaluatedAt: at,
	}, nil
}

func (row fileScoreRow) score() (FileScore, error) {
	var episodes []domain.EpisodeID
	if err := json.Unmarshal([]byte(row.EpisodeIDs), &episodes); err != nil {
		return FileScore{}, fmt.Errorf("decoding episodes of file score %s: %w", row.MediaFileID, err)
	}
	var quality domain.Quality
	if err := json.Unmarshal([]byte(row.Quality), &quality); err != nil {
		return FileScore{}, fmt.Errorf("decoding quality of file score %s: %w", row.MediaFileID, err)
	}
	return FileScore{
		File: row.MediaFileID, Title: row.TitleID, Episodes: episodes, Quality: quality, ReleaseName: row.ReleaseName,
		Path: row.Path, SizeBytes: row.SizeBytes, CFScore: row.CFScore, Profile: row.ProfileID, CutoffMet: row.CutoffMet,
		Transcoded: row.Transcoded, Origin: row.Origin,
	}, nil
}

func fileScoresWhere(conn *gorm.DB, where string, args ...any) ([]FileScore, error) {
	var rows []fileScoreRow
	if err := conn.Where(where, args...).Order("media_file_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	scores := make([]FileScore, 0, len(rows))
	for _, row := range rows {
		s, err := row.score()
		if err != nil {
			return nil, err
		}
		scores = append(scores, s)
	}
	return scores, nil
}

// loadFileScore reports false when Acquisition has no score for the file.
func loadFileScore(conn *gorm.DB, id domain.MediaFileID) (FileScore, bool, error) {
	scores, err := fileScoresWhere(conn, "media_file_id = ?", id)
	if err != nil || len(scores) == 0 {
		return FileScore{}, false, err
	}
	return scores[0], true, nil
}

func titleFileScores(conn *gorm.DB, title domain.TitleID) ([]FileScore, error) {
	return fileScoresWhere(conn, "title_id = ?", title)
}

func saveFileScore(tx *gorm.DB, f FileScore, at time.Time) error {
	row, err := toFileScoreRow(f, at)
	if err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error
}

func deleteFileScore(tx *gorm.DB, id domain.MediaFileID) error {
	return tx.Where("media_file_id = ?", id).Delete(&fileScoreRow{}).Error
}

func markFileTranscoded(tx *gorm.DB, id domain.MediaFileID) error {
	return tx.Model(&fileScoreRow{}).Where("media_file_id = ?", id).Update("transcoded", true).Error
}

// scoredTitles lists the titles with scored files evaluated under profile, or every scored title when profile is empty.
func scoredTitles(conn *gorm.DB, profile domain.ProfileID) ([]domain.TitleID, error) {
	query := conn.Model(&fileScoreRow{}).Distinct("title_id").Order("title_id")
	if profile != "" {
		query = query.Where("profile_id = ?", profile)
	}
	var titles []domain.TitleID
	return titles, query.Pluck("title_id", &titles).Error
}

// cutoffStatus is what the title page shows for a file. Upgrades stop once it is transcoded or when its
// profile doesn't allow them.
func cutoffStatus(conn *gorm.DB, f FileScore) (api.CutoffStatus, error) {
	var allowed []bool
	if err := conn.Model(&profileRow{}).Where("id = ?", f.Profile).Limit(1).Pluck("upgrade_allowed", &allowed).Error; err != nil {
		return api.CutoffStatus{}, err
	}
	return api.CutoffStatus{
		File: f.File, Quality: f.Quality, CFScore: f.CFScore, CutoffMet: f.CutoffMet,
		UpgradesAllowed: !f.Transcoded && len(allowed) > 0 && allowed[0],
	}, nil
}

// CutoffStatus implements api.Reader.
func (r *Repository) CutoffStatus(ctx context.Context, file domain.MediaFileID) (api.CutoffStatus, error) {
	conn := r.db.Read(ctx)
	f, found, err := loadFileScore(conn, file)
	if err != nil {
		return api.CutoffStatus{}, err
	}
	if !found {
		return api.CutoffStatus{}, api.ErrNotFound
	}
	return cutoffStatus(conn, f)
}

// CutoffStatuses is CutoffStatus for every scored file of a title.
func (r *Repository) CutoffStatuses(ctx context.Context, title domain.TitleID) ([]api.CutoffStatus, error) {
	conn := r.db.Read(ctx)
	scores, err := titleFileScores(conn, title)
	if err != nil {
		return nil, err
	}
	statuses := make([]api.CutoffStatus, 0, len(scores))
	for _, f := range scores {
		s, err := cutoffStatus(conn, f)
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, s)
	}
	return statuses, nil
}
