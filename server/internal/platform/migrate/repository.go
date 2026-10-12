package migrate

import (
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// sessionID is the one row: Magpie runs one migration.
const sessionID = 1

type sessionRow struct {
	ID        int `gorm:"primaryKey"`
	Session   string
	UpdatedAt time.Time
}

func (sessionRow) TableName() string { return "platform_migration" }

func loadSession(conn *gorm.DB) (Session, error) {
	var rows []sessionRow
	if err := conn.Where("id = ?", sessionID).Limit(1).Find(&rows).Error; err != nil {
		return Session{}, err
	}
	if len(rows) == 0 {
		return newSession(), nil
	}
	var s Session
	if err := json.Unmarshal([]byte(rows[0].Session), &s); err != nil {
		return Session{}, fmt.Errorf("decoding the migration: %w", err)
	}
	return s, nil
}

func saveSession(tx *gorm.DB, s Session, now time.Time) error {
	encoded, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("encoding the migration: %w", err)
	}
	row := sessionRow{ID: sessionID, Session: string(encoded), UpdatedAt: now}
	return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error
}

func deleteSession(tx *gorm.DB) error {
	return tx.Where("id = ?", sessionID).Delete(&sessionRow{}).Error
}
