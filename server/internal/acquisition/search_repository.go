package acquisition

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"magpie/internal/acquisition/api"
	"magpie/internal/domain"

	"gorm.io/gorm"
)

type searchRow struct {
	ID          domain.SearchID `gorm:"primaryKey"`
	TitleID     domain.TitleID
	EpisodeIDs  string
	Trigger     api.SearchTrigger
	StartedAt   time.Time
	CompletedAt *time.Time
	Accepted    int
	Total       int
}

func (searchRow) TableName() string { return "acquisition_searches" }

type blocklistRow struct {
	ID          string `gorm:"primaryKey"`
	TitleID     domain.TitleID
	InfoHash    string
	ReleaseName string
	Indexer     string
	Reason      api.BlocklistReason
	At          time.Time
	ExpiresAt   *time.Time
}

func (blocklistRow) TableName() string { return "acquisition_blocklist" }

type blocklistID string

// insertSearch records a started search in tx; results stay in memory.
func insertSearch(tx *gorm.DB, s api.SearchStarted, at time.Time) error {
	episodes, err := json.Marshal(nonNil(s.Episodes))
	if err != nil {
		return err
	}
	return tx.Create(&searchRow{ID: s.Search, TitleID: s.Title, EpisodeIDs: string(episodes), Trigger: s.Trigger, StartedAt: at}).Error
}

func completeSearch(tx *gorm.DB, c api.SearchCompleted, at time.Time) error {
	return tx.Model(&searchRow{}).Where("id = ?", c.Search).Updates(map[string]any{
		"completed_at": at,
		"accepted":     c.Accepted,
		"total":        c.Total,
	}).Error
}

// SearchCompletedAt returns when a search completed, or nil while it runs. Returns api.ErrNotFound for an unknown search.
func (r *Repository) SearchCompletedAt(ctx context.Context, id domain.SearchID) (*time.Time, error) {
	var row searchRow
	result := r.db.Read(ctx).Where("id = ?", id).Limit(1).Find(&row)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, api.ErrNotFound
	}
	return row.CompletedAt, nil
}

// ErrBlocklistEntryNotFound is an unknown blocklist entry ID.
var ErrBlocklistEntryNotFound = errors.New("acquisition: blocklist entry not found")

// Blocklist lists a title's entries, or every entry for an empty title, newest first, expired
// temporary entries included.
func (r *Repository) Blocklist(ctx context.Context, title domain.TitleID) ([]api.BlocklistEntry, error) {
	query := r.db.Read(ctx).Order("at DESC, id DESC")
	if title != "" {
		query = query.Where("title_id = ?", title)
	}
	var rows []blocklistRow
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	entries := make([]api.BlocklistEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, row.entry())
	}
	return entries, nil
}

func (row blocklistRow) entry() api.BlocklistEntry {
	return api.BlocklistEntry{
		ID: row.ID, Title: row.TitleID, InfoHash: row.InfoHash, ReleaseName: row.ReleaseName,
		Indexer: row.Indexer, Reason: row.Reason, At: row.At, ExpiresAt: row.ExpiresAt,
	}
}

// AddToBlocklist stores an entry and returns it, with a new ID unless it has one.
func (r *Repository) AddToBlocklist(ctx context.Context, e api.BlocklistEntry) (api.BlocklistEntry, error) {
	err := r.db.Write(ctx, func(tx *gorm.DB) error {
		var err error
		e, err = insertBlocklistEntry(tx, e)
		return err
	})
	return e, err
}

func insertBlocklistEntry(tx *gorm.DB, e api.BlocklistEntry) (api.BlocklistEntry, error) {
	if e.ID == "" {
		e.ID = string(domain.NewID[blocklistID]())
	}
	row := blocklistRow{
		ID: e.ID, TitleID: e.Title, InfoHash: e.InfoHash, ReleaseName: e.ReleaseName,
		Indexer: e.Indexer, Reason: e.Reason, At: e.At, ExpiresAt: e.ExpiresAt,
	}
	return e, tx.Create(&row).Error
}

// RemoveFromBlocklist deletes an entry and returns it. ErrBlocklistEntryNotFound for an unknown ID.
func (r *Repository) RemoveFromBlocklist(ctx context.Context, id string) (api.BlocklistEntry, error) {
	var removed api.BlocklistEntry
	err := r.db.Write(ctx, func(tx *gorm.DB) error {
		var rows []blocklistRow
		if err := tx.Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return ErrBlocklistEntryNotFound
		}
		removed = rows[0].entry()
		return tx.Delete(&blocklistRow{}, "id = ?", id).Error
	})
	return removed, err
}

// RecordIndexerAnswer applies one search answer to an indexer's health, as the sync does: a failure
// marks it failing with the error, a success clears that and moves last_ok_at. Gone indexers are left alone.
func (r *Repository) RecordIndexerAnswer(ctx context.Context, prowlarrID int, failure string, at time.Time) error {
	return r.db.Write(ctx, func(tx *gorm.DB) error {
		query := tx.Model(&indexerRow{}).Where("prowlarr_id = ? AND status <> ?", prowlarrID, IndexerGone)
		if failure == "" {
			return query.Updates(map[string]any{"status": IndexerOK, "last_error": "", "failing_since": nil, "last_ok_at": at}).Error
		}
		if err := query.Updates(map[string]any{"status": IndexerFailing, "last_error": failure}).Error; err != nil {
			return err
		}
		return tx.Model(&indexerRow{}).Where("prowlarr_id = ? AND failing_since IS NULL AND status = ?", prowlarrID, IndexerFailing).
			Update("failing_since", at).Error
	})
}
