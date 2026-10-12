package acquisition

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"magpie/internal/acquisition/api"
	"magpie/internal/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PendingRelease is a release held by the delay until it may be grabbed.
type PendingRelease struct {
	ID        string
	Title     domain.TitleID
	Episodes  []domain.EpisodeID // empty for a movie
	IndexerID int
	Release   TorznabItem
	// Trigger is what found it, AUTO or RSS; the grab reports it.
	Trigger   api.SearchTrigger
	Reason    api.RejectionReason
	PublishAt time.Time
	AddedAt   time.Time
}

type pendingID string

type pendingRow struct {
	ID         string `gorm:"primaryKey"`
	TitleID    domain.TitleID
	EpisodeIDs string
	ReleaseKey string
	IndexerID  int
	Release    string
	Trigger    api.SearchTrigger
	Reason     api.RejectionReason
	PublishAt  time.Time
	AddedAt    time.Time
}

func (pendingRow) TableName() string { return "acquisition_pending_releases" }

type searchJobRow struct {
	Key      string `gorm:"primaryKey"`
	QueuedAt time.Time
}

func (searchJobRow) TableName() string { return "acquisition_search_jobs" }

func (row pendingRow) pending() (PendingRelease, error) {
	p := PendingRelease{
		ID: row.ID, Title: row.TitleID, IndexerID: row.IndexerID, Trigger: row.Trigger,
		Reason: row.Reason, PublishAt: row.PublishAt, AddedAt: row.AddedAt,
	}
	if err := json.Unmarshal([]byte(row.EpisodeIDs), &p.Episodes); err != nil {
		return PendingRelease{}, err
	}
	if err := json.Unmarshal([]byte(row.Release), &p.Release); err != nil {
		return PendingRelease{}, err
	}
	return p, nil
}

func pendingFrom(rows []pendingRow) ([]PendingRelease, error) {
	list := make([]PendingRelease, 0, len(rows))
	for _, row := range rows {
		p, err := row.pending()
		if err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, nil
}

// PendingReleases lists a title's held releases, or every one for an empty title, oldest publish first.
func (r *Repository) PendingReleases(ctx context.Context, title domain.TitleID) ([]PendingRelease, error) {
	return pendingReleases(r.db.Read(ctx), title)
}

func pendingReleases(conn *gorm.DB, title domain.TitleID) ([]PendingRelease, error) {
	query := conn.Order("publish_at, id")
	if title != "" {
		query = query.Where("title_id = ?", title)
	}
	var rows []pendingRow
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	return pendingFrom(rows)
}

func (r *Repository) hasPending(ctx context.Context) (bool, error) {
	var found int64
	err := r.db.Read(ctx).Model(&pendingRow{}).Limit(1).Count(&found).Error
	return found > 0, err
}

// holdReleases stores releases for later; one already held for the title is kept as it is.
func holdReleases(tx *gorm.DB, held []PendingRelease) error {
	for _, p := range held {
		episodes, err := json.Marshal(nonNil(p.Episodes))
		if err != nil {
			return err
		}
		release, err := json.Marshal(p.Release)
		if err != nil {
			return err
		}
		row := pendingRow{
			ID: string(domain.NewID[pendingID]()), TitleID: p.Title, EpisodeIDs: string(episodes),
			ReleaseKey: pendingKey(p.IndexerID, p.Release), IndexerID: p.IndexerID, Release: string(release),
			Trigger: p.Trigger, Reason: p.Reason, PublishAt: p.PublishAt, AddedAt: p.AddedAt,
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func pendingKey(indexerID int, item TorznabItem) string {
	return fmt.Sprintf("%d|%s", indexerID, releaseGUID(item))
}

// dropPending deletes a title's held releases that cover any of items; a nil items drops them all.
func dropPending(tx *gorm.DB, title domain.TitleID, items itemSet) error {
	held, err := pendingReleases(tx, title)
	if err != nil {
		return err
	}
	ids := []string{}
	for _, p := range held {
		if items == nil || items.overlaps(p.Episodes) {
			ids = append(ids, p.ID)
		}
	}
	return deletePending(tx, ids)
}

func deletePending(tx *gorm.DB, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	return tx.Delete(&pendingRow{}, "id IN ?", ids).Error
}

// claimSearchKey records a search job about to be queued. It is false when one with the key is still queued.
func claimSearchKey(tx *gorm.DB, key string, at time.Time) (bool, error) {
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&searchJobRow{Key: key, QueuedAt: at})
	return result.RowsAffected > 0, result.Error
}

func (r *Repository) releaseSearchKey(ctx context.Context, key string) error {
	return r.db.Write(ctx, func(tx *gorm.DB) error {
		return tx.Delete(&searchJobRow{}, "key = ?", key).Error
	})
}

// itemSet holds the items grabs or holds cover: episode IDs, or movieItem for a movie.
type itemSet map[domain.EpisodeID]bool

// movieItem stands for a movie, which has no episodes.
const movieItem domain.EpisodeID = ""

func itemsOf(episodes []domain.EpisodeID) []domain.EpisodeID {
	if len(episodes) == 0 {
		return []domain.EpisodeID{movieItem}
	}
	return episodes
}

func (s itemSet) add(episodes []domain.EpisodeID) {
	for _, item := range itemsOf(episodes) {
		s[item] = true
	}
}

func (s itemSet) overlaps(episodes []domain.EpisodeID) bool {
	return slices.ContainsFunc(itemsOf(episodes), func(item domain.EpisodeID) bool { return s[item] })
}
