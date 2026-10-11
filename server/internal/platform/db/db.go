// Package db opens the SQLite writer and reader pools, runs each owner's goose migrations and takes backups.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"magpie/internal/platform/config"
	"magpie/internal/platform/owner"

	"github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/gormlite"
	"go.uber.org/fx"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	FileName = "magpie.db"

	fileScheme      = "file"
	dsnTxLock       = "_txlock"
	dsnTimeFormat   = "_timefmt"
	dsnPragma       = "_pragma"
	dsnMode         = "mode"
	modeReadOnly    = "ro"
	txLockImmediate = "immediate"

	readersPerProc     = 2
	slowQueryThreshold = 200 * time.Millisecond
)

// SQLite's own UTC format with fixed-width milliseconds sorts the same as text and as time, so range
// queries on time columns work, and it still reads CURRENT_TIMESTAMP values.
const timeFormat = sqlite3.TimeFormat4

var writerPragmas = []string{
	"journal_mode(WAL)",
	"synchronous(NORMAL)",
	"foreign_keys(ON)",
	"busy_timeout(5000)",
}

var readerPragmas = []string{
	"busy_timeout(5000)",
}

var ErrNotInWrite = errors.New("db: transaction did not come from DB.Write")

// DB is the single writer connection plus a read-only pool. Every write goes through Write.
type DB struct {
	writer  *gorm.DB
	reader  *gorm.DB
	path    string
	writeMu sync.Mutex
}

var Module = fx.Module("db",
	fx.Provide(New),
	ProvideMigrations(owner.Platform, platformMigrations()),
	// Migrations must run at startup even before any context asks for the database.
	fx.Invoke(func(*DB) {}),
)

type Params struct {
	fx.In

	Lifecycle  fx.Lifecycle
	Env        config.Env
	Migrations []MigrationSet `group:"migrations"`
}

// New applies a staged restore, migrates every owner's schema and closes the pools when the app stops.
func New(p Params) (*DB, error) {
	d, err := Open(context.Background(), filepath.Join(p.Env.DataDir, FileName), BackupDir(p.Env.DataDir), p.Migrations)
	if err != nil {
		return nil, err
	}
	p.Lifecycle.Append(fx.StopHook(d.Close))
	return d, nil
}

// Open prepares the database at path: it swaps in a staged restore, backs up into backupDir when
// migrations are pending, runs them in owner order and opens the reader pool.
func Open(ctx context.Context, path, backupDir string, sets []MigrationSet) (*DB, error) {
	if err := applyStagedRestore(ctx, path, backupDir); err != nil {
		return nil, fmt.Errorf("restoring %s: %w", path, err)
	}

	_, statErr := os.Stat(path)
	existed := statErr == nil

	writer, err := openGorm(writerDSN(path))
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	writerSQL, err := writer.DB()
	if err != nil {
		return nil, err
	}
	writerSQL.SetMaxOpenConns(1)

	d := &DB{writer: writer, path: path}
	if err := d.migrate(ctx, sets, existed, backupDir); err != nil {
		return nil, errors.Join(fmt.Errorf("migrating %s: %w", path, err), closeGorm(writer))
	}

	d.reader, err = openGorm(readerDSN(path))
	if err != nil {
		return nil, errors.Join(fmt.Errorf("opening reader pool: %w", err), closeGorm(writer))
	}
	readerSQL, err := d.reader.DB()
	if err != nil {
		return nil, errors.Join(err, closeGorm(writer))
	}
	readerSQL.SetMaxOpenConns(runtime.GOMAXPROCS(0) * readersPerProc)
	return d, nil
}

func (d *DB) Close() error {
	return errors.Join(closeGorm(d.reader), closeGorm(d.writer))
}

// Read returns the read-only pool. Keep reads short and never hold one across network I/O.
func (d *DB) Read(ctx context.Context) *gorm.DB {
	return d.reader.WithContext(ctx)
}

// Write runs fn in an immediate transaction on the writer. Hooks registered with AfterCommit run after
// a successful commit and before the next write starts, so they see commits in order.
func (d *DB) Write(ctx context.Context, fn func(tx *gorm.DB) error) error {
	d.writeMu.Lock()
	defer d.writeMu.Unlock()

	hooks := &commitHooks{}
	if err := d.writer.WithContext(withCommitHooks(ctx, hooks)).Transaction(fn); err != nil {
		return err
	}
	hooks.run()
	return nil
}

// WriterSQL is the writer connection for libraries that need database/sql, such as backlite.
func (d *DB) WriterSQL() (*sql.DB, error) {
	return d.writer.DB()
}

// Path is the database file.
func (d *DB) Path() string {
	return d.path
}

var gormLogger = logger.New(log.Default(), logger.Config{
	SlowThreshold:             slowQueryThreshold,
	LogLevel:                  logger.Warn,
	IgnoreRecordNotFoundError: true,
})

func openGorm(dsn string) (*gorm.DB, error) {
	return gorm.Open(gormlite.Open(dsn), &gorm.Config{Logger: gormLogger})
}

func closeGorm(gdb *gorm.DB) error {
	if gdb == nil {
		return nil
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func writerDSN(path string) string {
	query := url.Values{}
	query.Set(dsnTxLock, txLockImmediate)
	query.Set(dsnTimeFormat, string(timeFormat))
	for _, pragma := range writerPragmas {
		query.Add(dsnPragma, pragma)
	}
	return fileURL(path, query)
}

func readerDSN(path string) string {
	query := url.Values{}
	query.Set(dsnMode, modeReadOnly)
	query.Set(dsnTimeFormat, string(timeFormat))
	for _, pragma := range readerPragmas {
		query.Add(dsnPragma, pragma)
	}
	return fileURL(path, query)
}

func fileURL(path string, query url.Values) string {
	return (&url.URL{Scheme: fileScheme, Path: path, RawQuery: query.Encode()}).String()
}
