package acquisition

import (
	"context"
	"fmt"
	"time"

	"magpie/internal/acquisition/api"
	"magpie/internal/domain"
	"magpie/internal/platform/db"
	"magpie/internal/platform/parse"

	"gorm.io/gorm"
)

type profileRow struct {
	ID                    domain.ProfileID `gorm:"primaryKey"`
	Name                  string
	UpgradeAllowed        bool
	CutoffItemID          int
	MinFormatScore        int
	CutoffFormatScore     int
	MinUpgradeFormatScore int
	PropersMode           api.PropersMode
	Preset                *api.QualityPreset
	TrashProfileID        *string
	TrashScoreSet         *string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func (profileRow) TableName() string { return "acquisition_quality_profiles" }

type qualityGroupRow struct {
	ProfileID  domain.ProfileID `gorm:"primaryKey"`
	ID         int              `gorm:"primaryKey;autoIncrement:false"`
	Name       string
	QualityIDs []int `gorm:"serializer:json"`
}

func (qualityGroupRow) TableName() string { return "acquisition_quality_groups" }

type profileItemRow struct {
	ProfileID         domain.ProfileID `gorm:"primaryKey"`
	Position          int              `gorm:"primaryKey;autoIncrement:false"`
	QualityID         *int
	GroupID           *int
	Allowed           bool
	MinMBPerMin       float64 `gorm:"column:min_mb_per_min"`
	PreferredMBPerMin float64 `gorm:"column:preferred_mb_per_min"`
	MaxMBPerMin       float64 `gorm:"column:max_mb_per_min"`
}

func (profileItemRow) TableName() string { return "acquisition_quality_profile_items" }

type customFormatRow struct {
	ID                  api.CustomFormatID `gorm:"primaryKey"`
	Name                string
	TrashID             *string
	TrashScores         map[string]int `gorm:"serializer:json"`
	IncludeWhenRenaming bool
	Specs               []api.FormatSpec `gorm:"serializer:json"`
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (customFormatRow) TableName() string { return "acquisition_custom_formats" }

type formatScoreRow struct {
	ProfileID    domain.ProfileID   `gorm:"primaryKey"`
	FormatID     api.CustomFormatID `gorm:"primaryKey"`
	Score        int
	UserOverride bool
}

func (formatScoreRow) TableName() string { return "acquisition_profile_format_scores" }

type trashProfileRow struct {
	TrashID  string `gorm:"primaryKey"`
	App      TrashApp
	Name     string
	Profile  TrashQualityProfile `gorm:"serializer:json"`
	SyncedAt time.Time
}

func (trashProfileRow) TableName() string { return "acquisition_trash_profiles" }

// scoredFormat is a format score joined with its format's name.
type scoredFormat struct {
	FormatID     api.CustomFormatID
	Name         string
	Score        int
	UserOverride bool
}

// QualityRepository stores qualities, profiles, custom formats and the TRaSH guide profiles.
type QualityRepository struct {
	db *db.DB
}

func NewQualityRepository(d *db.DB) *QualityRepository {
	return &QualityRepository{db: d}
}

// Profile implements api.Reader.
func (r *QualityRepository) Profile(ctx context.Context, id domain.ProfileID) (api.Profile, error) {
	return loadProfile(r.db.Read(ctx), id)
}

// Profiles implements api.Reader.
func (r *QualityRepository) Profiles(ctx context.Context) ([]api.ProfileSummary, error) {
	var rows []profileRow
	if err := r.db.Read(ctx).Order("name COLLATE NOCASE, id").Find(&rows).Error; err != nil {
		return nil, err
	}
	summaries := make([]api.ProfileSummary, 0, len(rows))
	for _, row := range rows {
		summaries = append(summaries, api.ProfileSummary{
			ID:             row.ID,
			Name:           row.Name,
			UpgradeAllowed: row.UpgradeAllowed,
			Preset:         deref(row.Preset),
			TrashProfileID: deref(row.TrashProfileID),
		})
	}
	return summaries, nil
}

// DefaultProfile implements api.Reader.
func (r *QualityRepository) DefaultProfile(ctx context.Context) (domain.ProfileID, error) {
	return DefaultProfileKey.GetTx(r.db.Read(ctx))
}

// ScoringProfile loads a profile with every custom format compiled, ready for Evaluate.
func (r *QualityRepository) ScoringProfile(ctx context.Context, id domain.ProfileID) (ScoringProfile, error) {
	return loadScoringProfile(r.db.Read(ctx), id)
}

func loadScoringProfile(conn *gorm.DB, id domain.ProfileID) (ScoringProfile, error) {
	p, err := loadProfile(conn, id)
	if err != nil {
		return ScoringProfile{}, err
	}
	formats, err := loadFormats(conn)
	if err != nil {
		return ScoringProfile{}, err
	}
	compiled := make([]CompiledFormat, 0, len(formats))
	for _, f := range formats {
		c, err := CompileFormat(f)
		if err != nil {
			return ScoringProfile{}, err
		}
		compiled = append(compiled, c)
	}
	return NewScoringProfile(p, compiled), nil
}

// Formats lists every custom format ordered by name.
func (r *QualityRepository) Formats(ctx context.Context) ([]CustomFormat, error) {
	return loadFormats(r.db.Read(ctx))
}

func (r *QualityRepository) Format(ctx context.Context, id api.CustomFormatID) (CustomFormat, error) {
	return loadFormat(r.db.Read(ctx), id)
}

// TrashProfiles lists the guide profiles of the last TRaSH sync, by app then name.
func (r *QualityRepository) TrashProfiles(ctx context.Context) ([]TrashGuideProfile, error) {
	return loadTrashProfiles(r.db.Read(ctx))
}

func profileIDs(conn *gorm.DB) ([]domain.ProfileID, error) {
	var ids []domain.ProfileID
	return ids, conn.Model(&profileRow{}).Order("id").Pluck("id", &ids).Error
}

func profileExists(conn *gorm.DB, id domain.ProfileID) (bool, error) {
	var found int64
	err := conn.Model(&profileRow{}).Where("id = ?", id).Count(&found).Error
	return found > 0, err
}

func profileNameTaken(conn *gorm.DB, name string, except domain.ProfileID) (bool, error) {
	var found int64
	err := conn.Model(&profileRow{}).Where("name = ? AND id <> ?", name, except).Count(&found).Error
	return found > 0, err
}

func formatNameTaken(conn *gorm.DB, name string, except api.CustomFormatID) (bool, error) {
	var found int64
	err := conn.Model(&customFormatRow{}).Where("name = ? AND id <> ?", name, except).Count(&found).Error
	return found > 0, err
}

func loadProfile(conn *gorm.DB, id domain.ProfileID) (api.Profile, error) {
	var rows []profileRow
	if err := conn.Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
		return api.Profile{}, err
	}
	if len(rows) == 0 {
		return api.Profile{}, api.ErrNotFound
	}
	row := rows[0]

	var items []profileItemRow
	if err := conn.Where("profile_id = ?", id).Order("position").Find(&items).Error; err != nil {
		return api.Profile{}, err
	}
	var groups []qualityGroupRow
	if err := conn.Where("profile_id = ?", id).Find(&groups).Error; err != nil {
		return api.Profile{}, err
	}
	var scores []scoredFormat
	err := conn.Table("acquisition_profile_format_scores AS s").
		Select("s.format_id, f.name, s.score, s.user_override").
		Joins("JOIN acquisition_custom_formats AS f ON f.id = s.format_id").
		Where("s.profile_id = ?", id).
		Order("f.name COLLATE NOCASE, f.id").
		Find(&scores).Error
	if err != nil {
		return api.Profile{}, err
	}
	return profileFromRows(row, items, groups, scores)
}

func profileFromRows(row profileRow, items []profileItemRow, groups []qualityGroupRow, scores []scoredFormat) (api.Profile, error) {
	groupsByID := make(map[int]qualityGroupRow, len(groups))
	for _, g := range groups {
		groupsByID[g.ID] = g
	}

	p := api.Profile{
		ID:                    row.ID,
		Name:                  row.Name,
		UpgradeAllowed:        row.UpgradeAllowed,
		Preset:                deref(row.Preset),
		Items:                 make([]api.ProfileItem, 0, len(items)),
		MinFormatScore:        row.MinFormatScore,
		CutoffFormatScore:     row.CutoffFormatScore,
		MinUpgradeFormatScore: row.MinUpgradeFormatScore,
		PropersMode:           row.PropersMode,
		FormatScores:          make([]api.FormatScore, 0, len(scores)),
		TrashProfileID:        deref(row.TrashProfileID),
		TrashScoreSet:         deref(row.TrashScoreSet),
	}
	for index, item := range items {
		profileItem := api.ProfileItem{
			Allowed: item.Allowed,
			Sizes:   api.SizeLimits{MinMBPerMin: item.MinMBPerMin, PreferredMBPerMin: item.PreferredMBPerMin, MaxMBPerMin: item.MaxMBPerMin},
		}
		var ids []int
		if item.GroupID != nil {
			g, ok := groupsByID[*item.GroupID]
			if !ok {
				return api.Profile{}, fmt.Errorf("profile %s: item %d names missing group %d", row.ID, item.Position, *item.GroupID)
			}
			profileItem.Group = &api.QualityGroup{ID: g.ID, Name: g.Name}
			ids = g.QualityIDs
			if g.ID == row.CutoffItemID {
				p.Cutoff = index
			}
		} else if item.QualityID != nil {
			ids = []int{*item.QualityID}
			if *item.QualityID == row.CutoffItemID {
				p.Cutoff = index
			}
		}
		for _, id := range ids {
			q, err := catalogueQuality(id)
			if err != nil {
				return api.Profile{}, err
			}
			profileItem.Qualities = append(profileItem.Qualities, q)
		}
		p.Items = append(p.Items, profileItem)
	}
	for _, s := range scores {
		p.FormatScores = append(p.FormatScores, api.FormatScore{Format: s.FormatID, Name: s.Name, Score: s.Score, UserOverride: s.UserOverride})
	}
	return p, nil
}

func catalogueQuality(id int) (domain.Quality, error) {
	d, ok := parse.QualityByID(parse.QualityID(id))
	if !ok {
		return domain.Quality{}, fmt.Errorf("unknown quality %d", id)
	}
	return d.Quality(0), nil
}

// saveProfile inserts or replaces a profile with its items, groups and format scores.
// Group IDs are numbered from 1000 in item order.
func saveProfile(tx *gorm.DB, p api.Profile, now time.Time) error {
	createdAt := now
	var existing []profileRow
	if err := tx.Where("id = ?", p.ID).Limit(1).Find(&existing).Error; err != nil {
		return err
	}
	if len(existing) > 0 {
		createdAt = existing[0].CreatedAt
	}
	if err := tx.Where("id = ?", p.ID).Delete(&profileRow{}).Error; err != nil {
		return err
	}
	row := profileRow{
		ID:                    p.ID,
		Name:                  p.Name,
		UpgradeAllowed:        p.UpgradeAllowed,
		MinFormatScore:        p.MinFormatScore,
		CutoffFormatScore:     p.CutoffFormatScore,
		MinUpgradeFormatScore: p.MinUpgradeFormatScore,
		PropersMode:           p.PropersMode,
		Preset:                nonZero(p.Preset),
		TrashProfileID:        nonZero(p.TrashProfileID),
		TrashScoreSet:         nonZero(p.TrashScoreSet),
		CreatedAt:             createdAt,
		UpdatedAt:             now,
	}

	var groups []qualityGroupRow
	items := make([]profileItemRow, 0, len(p.Items))
	nextGroupID := firstGroupID
	for position, item := range p.Items {
		itemRow := profileItemRow{
			ProfileID:         p.ID,
			Position:          position,
			Allowed:           item.Allowed,
			MinMBPerMin:       item.Sizes.MinMBPerMin,
			PreferredMBPerMin: item.Sizes.PreferredMBPerMin,
			MaxMBPerMin:       item.Sizes.MaxMBPerMin,
		}
		itemID := 0
		if item.Group != nil {
			ids := make([]int, 0, len(item.Qualities))
			for _, q := range item.Qualities {
				ids = append(ids, q.ID)
			}
			groupID := nextGroupID
			nextGroupID++
			groups = append(groups, qualityGroupRow{ProfileID: p.ID, ID: groupID, Name: item.Group.Name, QualityIDs: ids})
			itemRow.GroupID = &groupID
			itemID = groupID
		} else {
			qualityID := item.Qualities[0].ID
			itemRow.QualityID = &qualityID
			itemID = qualityID
		}
		if position == p.Cutoff {
			row.CutoffItemID = itemID
		}
		items = append(items, itemRow)
	}

	if err := tx.Create(&row).Error; err != nil {
		return err
	}
	if len(groups) > 0 {
		if err := tx.Create(&groups).Error; err != nil {
			return err
		}
	}
	if len(items) > 0 {
		if err := tx.Create(&items).Error; err != nil {
			return err
		}
	}
	scores := make([]formatScoreRow, 0, len(p.FormatScores))
	for _, s := range p.FormatScores {
		scores = append(scores, formatScoreRow{ProfileID: p.ID, FormatID: s.Format, Score: s.Score, UserOverride: s.UserOverride})
	}
	if len(scores) > 0 {
		return tx.Create(&scores).Error
	}
	return nil
}

func deleteProfile(tx *gorm.DB, id domain.ProfileID) error {
	result := tx.Where("id = ?", id).Delete(&profileRow{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return api.ErrNotFound
	}
	return nil
}

// upsertFormatScore sets one profile's score for one format.
func upsertFormatScore(tx *gorm.DB, s formatScoreRow) error {
	return tx.Save(&s).Error
}

func loadFormats(conn *gorm.DB) ([]CustomFormat, error) {
	var rows []customFormatRow
	if err := conn.Order("name COLLATE NOCASE, id").Find(&rows).Error; err != nil {
		return nil, err
	}
	formats := make([]CustomFormat, 0, len(rows))
	for _, row := range rows {
		formats = append(formats, formatFromRow(row))
	}
	return formats, nil
}

func loadFormat(conn *gorm.DB, id api.CustomFormatID) (CustomFormat, error) {
	var rows []customFormatRow
	if err := conn.Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
		return CustomFormat{}, err
	}
	if len(rows) == 0 {
		return CustomFormat{}, api.ErrNotFound
	}
	return formatFromRow(rows[0]), nil
}

func formatFromRow(row customFormatRow) CustomFormat {
	return CustomFormat{
		ID:                  row.ID,
		Name:                row.Name,
		TrashID:             deref(row.TrashID),
		TrashScores:         row.TrashScores,
		IncludeWhenRenaming: row.IncludeWhenRenaming,
		Specs:               row.Specs,
	}
}

// saveFormat inserts or updates a format, keeping its creation time.
func saveFormat(tx *gorm.DB, f CustomFormat, now time.Time) error {
	trashScores := f.TrashScores
	if trashScores == nil {
		trashScores = map[string]int{}
	}
	specs := f.Specs
	if specs == nil {
		specs = []api.FormatSpec{}
	}
	row := customFormatRow{
		ID:                  f.ID,
		Name:                f.Name,
		TrashID:             nonZero(f.TrashID),
		TrashScores:         trashScores,
		IncludeWhenRenaming: f.IncludeWhenRenaming,
		Specs:               specs,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	result := tx.Model(&customFormatRow{}).Where("id = ?", f.ID).
		Select("name", "trash_id", "trash_scores", "include_when_renaming", "specs", "updated_at").
		Updates(&row)
	if result.Error != nil || result.RowsAffected > 0 {
		return result.Error
	}
	return tx.Create(&row).Error
}

func deleteFormat(tx *gorm.DB, id api.CustomFormatID) error {
	result := tx.Where("id = ?", id).Delete(&customFormatRow{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return api.ErrNotFound
	}
	return nil
}

// TrashGuideProfile is a TRaSH guide profile as of the last sync.
type TrashGuideProfile struct {
	App      TrashApp
	Profile  TrashQualityProfile
	SyncedAt time.Time
}

func loadTrashProfiles(conn *gorm.DB) ([]TrashGuideProfile, error) {
	var rows []trashProfileRow
	if err := conn.Order("app, name COLLATE NOCASE").Find(&rows).Error; err != nil {
		return nil, err
	}
	profiles := make([]TrashGuideProfile, 0, len(rows))
	for _, row := range rows {
		profiles = append(profiles, TrashGuideProfile{App: row.App, Profile: row.Profile, SyncedAt: row.SyncedAt})
	}
	return profiles, nil
}

func replaceTrashProfiles(tx *gorm.DB, profiles []TrashGuideProfile) error {
	if err := tx.Where("1 = 1").Delete(&trashProfileRow{}).Error; err != nil {
		return err
	}
	rows := make([]trashProfileRow, 0, len(profiles))
	seen := map[string]bool{}
	for _, p := range profiles {
		if seen[p.Profile.TrashID] {
			continue
		}
		seen[p.Profile.TrashID] = true
		rows = append(rows, trashProfileRow{TrashID: p.Profile.TrashID, App: p.App, Name: p.Profile.Name, Profile: p.Profile, SyncedAt: p.SyncedAt})
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.Create(&rows).Error
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

func nonZero[T comparable](v T) *T {
	var zero T
	if v == zero {
		return nil
	}
	return &v
}
