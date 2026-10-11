package acquisition

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"magpie/internal/acquisition/api"
	"magpie/internal/platform/db"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type indexerRow struct {
	ProwlarrID          int `gorm:"primaryKey;autoIncrement:false"`
	Name                string
	Privacy             IndexerPrivacy
	Caps                string
	Status              IndexerStatus
	LastError           string
	FailingSince        *time.Time
	AverageResponseMs   int64
	LastOKAt            *time.Time `gorm:"column:last_ok_at"`
	SyncedAt            time.Time
	Priority            int
	MinSeeders          int
	SeedingModeOverride SeedingOverride
	SeedRatio           *float64
	SeedDays            *int
}

func (indexerRow) TableName() string { return "acquisition_indexers" }

// Repository is Acquisition's data access.
type Repository struct {
	db *db.DB
}

func NewRepository(d *db.DB) *Repository {
	return &Repository{db: d}
}

// Indexers lists stored indexers by name, gone ones included only when asked.
func (r *Repository) Indexers(ctx context.Context, includeGone bool) ([]Indexer, error) {
	query := r.db.Read(ctx).Order("name COLLATE NOCASE, prowlarr_id")
	if !includeGone {
		query = query.Where("status <> ?", IndexerGone)
	}
	var rows []indexerRow
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	return fromRows(rows)
}

func (r *Repository) Indexer(ctx context.Context, prowlarrID int) (Indexer, error) {
	var row indexerRow
	err := r.db.Read(ctx).Where("prowlarr_id = ?", prowlarrID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Indexer{}, api.ErrNotFound
	}
	if err != nil {
		return Indexer{}, err
	}
	return row.indexer()
}

// ApplyIndexerSync reads every stored indexer and saves what merge returns, in one transaction.
func (r *Repository) ApplyIndexerSync(ctx context.Context, merge func([]Indexer) []Indexer) error {
	return r.db.Write(ctx, func(tx *gorm.DB) error {
		var rows []indexerRow
		if err := tx.Find(&rows).Error; err != nil {
			return err
		}
		stored, err := fromRows(rows)
		if err != nil {
			return err
		}
		changed := merge(stored)
		if len(changed) == 0 {
			return nil
		}
		save := make([]indexerRow, 0, len(changed))
		for _, ix := range changed {
			row, err := toRow(ix)
			if err != nil {
				return err
			}
			save = append(save, row)
		}
		return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&save).Error
	})
}

// UpdateIndexerSettings replaces Magpie's settings for one indexer and returns it.
func (r *Repository) UpdateIndexerSettings(ctx context.Context, prowlarrID int, s IndexerSettings) (Indexer, error) {
	err := r.db.Write(ctx, func(tx *gorm.DB) error {
		result := tx.Model(&indexerRow{}).Where("prowlarr_id = ?", prowlarrID).Updates(map[string]any{
			"priority":              s.Priority,
			"min_seeders":           s.MinSeeders,
			"seeding_mode_override": s.SeedingOverride,
			"seed_ratio":            s.SeedRatio,
			"seed_days":             s.SeedDays,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return api.ErrNotFound
		}
		return nil
	})
	if err != nil {
		return Indexer{}, err
	}
	return r.Indexer(ctx, prowlarrID)
}

func fromRows(rows []indexerRow) ([]Indexer, error) {
	list := make([]Indexer, 0, len(rows))
	for _, row := range rows {
		ix, err := row.indexer()
		if err != nil {
			return nil, err
		}
		list = append(list, ix)
	}
	return list, nil
}

func (row indexerRow) indexer() (Indexer, error) {
	var caps TorznabCaps
	if err := json.Unmarshal([]byte(row.Caps), &caps); err != nil {
		return Indexer{}, fmt.Errorf("decoding caps of indexer %d: %w", row.ProwlarrID, err)
	}
	return Indexer{
		ProwlarrID:      row.ProwlarrID,
		Name:            row.Name,
		Privacy:         row.Privacy,
		Caps:            caps,
		Status:          row.Status,
		LastError:       row.LastError,
		FailingSince:    row.FailingSince,
		AverageResponse: time.Duration(row.AverageResponseMs) * time.Millisecond,
		LastOKAt:        row.LastOKAt,
		SyncedAt:        row.SyncedAt,
		Settings: IndexerSettings{
			Priority:        row.Priority,
			MinSeeders:      row.MinSeeders,
			SeedingOverride: row.SeedingModeOverride,
			SeedRatio:       row.SeedRatio,
			SeedDays:        row.SeedDays,
		},
	}, nil
}

func toRow(ix Indexer) (indexerRow, error) {
	caps, err := json.Marshal(ix.Caps)
	if err != nil {
		return indexerRow{}, fmt.Errorf("encoding caps of indexer %d: %w", ix.ProwlarrID, err)
	}
	return indexerRow{
		ProwlarrID:          ix.ProwlarrID,
		Name:                ix.Name,
		Privacy:             ix.Privacy,
		Caps:                string(caps),
		Status:              ix.Status,
		LastError:           ix.LastError,
		FailingSince:        ix.FailingSince,
		AverageResponseMs:   ix.AverageResponse.Milliseconds(),
		LastOKAt:            ix.LastOKAt,
		SyncedAt:            ix.SyncedAt,
		Priority:            ix.Settings.Priority,
		MinSeeders:          ix.Settings.MinSeeders,
		SeedingModeOverride: ix.Settings.SeedingOverride,
		SeedRatio:           ix.Settings.SeedRatio,
		SeedDays:            ix.Settings.SeedDays,
	}, nil
}
