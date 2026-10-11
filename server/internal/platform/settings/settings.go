// Package settings stores typed values in platform_settings under owner-prefixed keys.
package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"magpie/internal/platform/db"
	"magpie/internal/platform/owner"

	"go.uber.org/fx"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const keySeparator = "."

var ErrUnknownOwner = errors.New("settings: key owner is not the platform or a context")

var Module = fx.Module("settings", fx.Provide(NewStore))

type row struct {
	Key       string `gorm:"primaryKey"`
	Value     string
	UpdatedAt time.Time
}

func (row) TableName() string { return "platform_settings" }

type Store struct {
	db *db.DB
}

func NewStore(d *db.DB) *Store {
	return &Store{db: d}
}

// Key is one setting. Unset keys read as Default.
type Key[T any] struct {
	Owner   owner.Name
	Name    string
	Default T
}

// NewKey names a setting "<owner>.<name>".
func NewKey[T any](o owner.Name, name string, def T) Key[T] {
	return Key[T]{Owner: o, Name: name, Default: def}
}

func (k Key[T]) String() string {
	return string(k.Owner) + keySeparator + k.Name
}

func (k Key[T]) Get(ctx context.Context, s *Store) (T, error) {
	return k.get(s.db.Read(ctx))
}

// GetTx reads inside a write transaction, seeing its uncommitted changes.
func (k Key[T]) GetTx(tx *gorm.DB) (T, error) {
	return k.get(tx)
}

func (k Key[T]) Set(ctx context.Context, s *Store, value T) error {
	return s.db.Write(ctx, func(tx *gorm.DB) error { return k.SetTx(tx, value) })
}

// SetTx writes in the caller's transaction so a setting can change together with other state.
func (k Key[T]) SetTx(tx *gorm.DB, value T) error {
	if !k.Owner.Known() {
		return fmt.Errorf("%w: %q", ErrUnknownOwner, k)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encoding setting %s: %w", k, err)
	}
	return tx.Clauses(clause.OnConflict{UpdateAll: true}).
		Create(&row{Key: k.String(), Value: string(encoded), UpdatedAt: time.Now()}).Error
}

func (k Key[T]) get(conn *gorm.DB) (T, error) {
	if !k.Owner.Known() {
		return k.Default, fmt.Errorf("%w: %q", ErrUnknownOwner, k)
	}
	var stored row
	err := conn.Where("key = ?", k.String()).Take(&stored).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return k.Default, nil
	}
	if err != nil {
		return k.Default, err
	}
	var value T
	if err := json.Unmarshal([]byte(stored.Value), &value); err != nil {
		return k.Default, fmt.Errorf("decoding setting %s: %w", k, err)
	}
	return value, nil
}
