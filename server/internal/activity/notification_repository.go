package activity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"magpie/internal/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type notificationURLRow struct {
	ID         string `gorm:"primaryKey"`
	UserID     string `gorm:"column:user_id"`
	Label      string
	URL        string `gorm:"column:url"`
	Kinds      string
	CreatedAt  time.Time
	LastSentAt *time.Time
	LastError  string
}

func (notificationURLRow) TableName() string { return "activity_notification_urls" }

func (r notificationURLRow) kinds() ([]string, error) {
	kinds := []string{}
	if err := json.Unmarshal([]byte(r.Kinds), &kinds); err != nil {
		return nil, fmt.Errorf("decoding notification kinds: %w", err)
	}
	return kinds, nil
}

func (r *notificationURLRow) setKinds(kinds []string) error {
	if len(kinds) == 0 {
		r.Kinds = emptyJSONList
		return nil
	}
	encoded, err := encodeJSON(kinds)
	r.Kinds = encoded
	return err
}

// addNotification stores an in-app notification once per (user, event, kind) and reports whether it was new.
func (r *Repository) addNotification(tx *gorm.DB, row *notificationRow) (bool, error) {
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(row)
	return result.RowsAffected > 0, result.Error
}

// markRead marks one of the user's notifications read. ErrNotificationNotFound if it isn't theirs.
func (r *Repository) markRead(ctx context.Context, user domain.UserID, id string, at time.Time) error {
	return r.db.Write(ctx, func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&notificationRow{}).Where("id = ? AND user_id = ?", id, user).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return ErrNotificationNotFound
		}
		return tx.Model(&notificationRow{}).Where("id = ? AND read_at IS NULL", id).Update("read_at", at.UTC()).Error
	})
}

func (r *Repository) markAllRead(ctx context.Context, user domain.UserID, at time.Time) error {
	return r.db.Write(ctx, func(tx *gorm.DB) error {
		return tx.Model(&notificationRow{}).Where("user_id = ? AND read_at IS NULL", user).Update("read_at", at.UTC()).Error
	})
}

// pruneNotifications deletes in-app notifications created before cutoff.
func (r *Repository) pruneNotifications(ctx context.Context, cutoff time.Time) error {
	return r.db.Write(ctx, func(tx *gorm.DB) error {
		return tx.Where("created_at < ?", cutoff.UTC()).Delete(&notificationRow{}).Error
	})
}

func (r *Repository) notificationURLs(ctx context.Context, user domain.UserID) ([]notificationURLRow, error) {
	var rows []notificationURLRow
	return rows, r.db.Read(ctx).Where("user_id = ?", user).Order("created_at, id").Find(&rows).Error
}

// notificationURL returns a URL by ID. ErrURLNotFound if there is none.
func (r *Repository) notificationURL(ctx context.Context, id string) (notificationURLRow, error) {
	var row notificationURLRow
	err := r.db.Read(ctx).Where("id = ?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row, ErrURLNotFound
	}
	return row, err
}

// addNotificationURL stores row unless the user already has limit URLs.
func (r *Repository) addNotificationURL(ctx context.Context, row *notificationURLRow, limit int) error {
	return r.db.Write(ctx, func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&notificationURLRow{}).Where("user_id = ?", row.UserID).Count(&count).Error; err != nil {
			return err
		}
		if count >= int64(limit) {
			return ErrTooManyURLs
		}
		return tx.Create(row).Error
	})
}

func (r *Repository) saveNotificationURL(ctx context.Context, row *notificationURLRow) error {
	return r.db.Write(ctx, func(tx *gorm.DB) error { return tx.Save(row).Error })
}

func (r *Repository) deleteNotificationURL(ctx context.Context, id string) error {
	return r.db.Write(ctx, func(tx *gorm.DB) error { return tx.Delete(&notificationURLRow{ID: id}).Error })
}

// urlsSubscribedTo returns every URL whose kinds include kind.
func (r *Repository) urlsSubscribedTo(tx *gorm.DB, kind string) ([]notificationURLRow, error) {
	var rows []notificationURLRow
	err := tx.Where("EXISTS (SELECT 1 FROM json_each(kinds) WHERE json_each.value = ?)", kind).Order("created_at, id").Find(&rows).Error
	return rows, err
}

// recordSent clears the URL's last error after a delivery.
func (r *Repository) recordSent(ctx context.Context, id string, at time.Time) error {
	return r.db.Write(ctx, func(tx *gorm.DB) error {
		return tx.Model(&notificationURLRow{}).Where("id = ?", id).
			Updates(map[string]any{"last_sent_at": at.UTC(), "last_error": ""}).Error
	})
}

func (r *Repository) recordFailure(ctx context.Context, id string, reason string) error {
	return r.db.Write(ctx, func(tx *gorm.DB) error {
		return tx.Model(&notificationURLRow{}).Where("id = ?", id).Update("last_error", reason).Error
	})
}
