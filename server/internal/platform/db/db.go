// Package db opens the SQLite database and applies the goose migrations.
package db

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"

	"magpie/internal/platform/config"

	"github.com/ncruces/go-sqlite3/gormlite"
	"go.uber.org/fx"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	FileName = "magpie.db"

	fileScheme      = "file"
	dsnTxLock       = "_txlock"
	dsnPragma       = "_pragma"
	txLockImmediate = "immediate"
)

var connectionPragmas = []string{
	"journal_mode(WAL)",
	"synchronous(NORMAL)",
	"foreign_keys(ON)",
	"busy_timeout(5000)",
}

var Module = fx.Module("db",
	fx.Provide(New),
	// Migrations must run at startup even before any context asks for the database.
	fx.Invoke(func(*gorm.DB) {}),
)

// New opens the database in the data dir, migrates it, and closes it when the app stops.
func New(lc fx.Lifecycle, env config.Env) (*gorm.DB, error) {
	gdb, err := Open(context.Background(), filepath.Join(env.DataDir, FileName))
	if err != nil {
		return nil, err
	}
	lc.Append(fx.StopHook(func() error { return Close(gdb) }))
	return gdb, nil
}

// Open opens the SQLite file at path and runs every pending migration.
func Open(ctx context.Context, path string) (*gorm.DB, error) {
	gdb, err := gorm.Open(gormlite.Open(dsn(path)), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}

	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, err
	}
	if err := Migrate(ctx, sqlDB); err != nil {
		return nil, fmt.Errorf("migrating %s: %w", path, err)
	}
	return gdb, nil
}

func Close(gdb *gorm.DB) error {
	sqlDB, err := gdb.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func dsn(path string) string {
	query := url.Values{}
	query.Set(dsnTxLock, txLockImmediate)
	for _, pragma := range connectionPragmas {
		query.Add(dsnPragma, pragma)
	}
	return (&url.URL{Scheme: fileScheme, Path: path, RawQuery: query.Encode()}).String()
}
