package db

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"magpie/internal/platform/owner"

	"gorm.io/gorm"
)

const (
	latestPlatformMigration = 3
	parallelWriters         = 50
)

type testDB struct {
	path      string
	backupDir string
}

func newTestDB(t *testing.T) testDB {
	t.Helper()
	dir := t.TempDir()
	return testDB{path: filepath.Join(dir, FileName), backupDir: BackupDir(dir)}
}

func (td testDB) open(t *testing.T, extra ...MigrationSet) *DB {
	t.Helper()
	d, err := Open(context.Background(), td.path, td.backupDir, append([]MigrationSet{PlatformMigrations()}, extra...))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func sqlSet(o owner.Name, files map[string]string) MigrationSet {
	fsys := fstest.MapFS{}
	for name, body := range files {
		fsys[name] = &fstest.MapFile{Data: []byte("-- +goose Up\n" + body)}
	}
	return MigrationSet{Owner: o, FS: fsys}
}

func backupsOfKind(t *testing.T, dir string, kind BackupKind) []Backup {
	t.Helper()
	all, err := ListBackups(dir)
	if err != nil {
		t.Fatal(err)
	}
	var found []Backup
	for _, b := range all {
		if b.Kind == kind {
			found = append(found, b)
		}
	}
	return found
}

func TestOpenMigratesEmptyDatabase(t *testing.T) {
	d := newTestDB(t).open(t)

	version, err := d.SchemaVersion(context.Background(), PlatformMigrations())
	if err != nil {
		t.Fatalf("schema version: %v", err)
	}
	if version != latestPlatformMigration {
		t.Errorf("schema version = %d, want %d", version, latestPlatformMigration)
	}
	for _, table := range []string{"platform_settings", "platform_outbox", "platform_subscriptions", "platform_handler_queue", "platform_processed_events", "platform_schedules"} {
		if !d.Read(context.Background()).Migrator().HasTable(table) {
			t.Errorf("%s missing after migration", table)
		}
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	td := newTestDB(t)
	for range 2 {
		if err := td.open(t).Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
	}
}

func TestWriterPoolSettings(t *testing.T) {
	d := newTestDB(t).open(t)

	pragmas := map[string]string{
		"journal_mode": "wal",
		"synchronous":  "1",
		"foreign_keys": "1",
	}
	for pragma, want := range pragmas {
		var got string
		if err := d.writer.Raw("PRAGMA " + pragma).Scan(&got).Error; err != nil {
			t.Fatalf("pragma %s: %v", pragma, err)
		}
		if !strings.EqualFold(got, want) {
			t.Errorf("pragma %s = %q, want %q", pragma, got, want)
		}
	}

	writer, err := d.WriterSQL()
	if err != nil {
		t.Fatal(err)
	}
	if got := writer.Stats().MaxOpenConnections; got != 1 {
		t.Errorf("writer max open connections = %d, want 1", got)
	}
	if !strings.Contains(writerDSN(d.Path()), dsnTxLock+"="+txLockImmediate) {
		t.Errorf("writer DSN lacks %s=%s", dsnTxLock, txLockImmediate)
	}
}

func TestReaderPoolIsReadOnly(t *testing.T) {
	d := newTestDB(t).open(t)

	err := d.Read(context.Background()).Exec("INSERT INTO platform_settings (key, value, updated_at) VALUES ('platform.x', '1', CURRENT_TIMESTAMP)").Error
	if err == nil {
		t.Fatal("reader pool accepted a write")
	}
}

func TestParallelWritersNeverSeeLockedDatabase(t *testing.T) {
	d := newTestDB(t).open(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	errs := make(chan error, parallelWriters*2)
	for i := range parallelWriters {
		wg.Go(func() {
			errs <- d.Write(ctx, func(tx *gorm.DB) error {
				return tx.Exec("INSERT INTO platform_settings (key, value, updated_at) VALUES (?, '1', CURRENT_TIMESTAMP)", fmt.Sprintf("platform.test.%d", i)).Error
			})
		})
		wg.Go(func() {
			var count int64
			errs <- d.Read(ctx).Table("platform_settings").Count(&count).Error
		})
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("concurrent access failed: %v", err)
		}
	}
	var count int64
	if err := d.Read(ctx).Table("platform_settings").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != parallelWriters {
		t.Errorf("rows = %d, want %d", count, parallelWriters)
	}
}

func TestAfterCommitRunsOnlyAfterCommit(t *testing.T) {
	d := newTestDB(t).open(t)
	ctx := context.Background()
	errRollback := errors.New("rollback")

	var committed, rolledBack bool
	if err := d.Write(ctx, func(tx *gorm.DB) error {
		return AfterCommit(tx, func() { committed = true })
	}); err != nil {
		t.Fatal(err)
	}
	err := d.Write(ctx, func(tx *gorm.DB) error {
		if err := AfterCommit(tx, func() { rolledBack = true }); err != nil {
			return err
		}
		return errRollback
	})

	if !errors.Is(err, errRollback) {
		t.Fatalf("err = %v, want rollback", err)
	}
	if !committed || rolledBack {
		t.Errorf("committed hook ran = %v, rolled-back hook ran = %v", committed, rolledBack)
	}
}

func TestAfterCommitRequiresWrite(t *testing.T) {
	d := newTestDB(t).open(t)

	err := d.writer.Transaction(func(tx *gorm.DB) error {
		return AfterCommit(tx, func() {})
	})
	if !errors.Is(err, ErrNotInWrite) {
		t.Fatalf("err = %v, want ErrNotInWrite", err)
	}
}

func TestMigrationsRunInOwnerOrder(t *testing.T) {
	identity := sqlSet(owner.Identity, map[string]string{"00001_users.sql": "CREATE TABLE identity_users (id TEXT PRIMARY KEY);"})
	// Catalog's migration needs identity's table, so it only passes when identity runs first.
	catalog := sqlSet(owner.Catalog, map[string]string{"00001_titles.sql": "INSERT INTO identity_users (id) VALUES ('seed');"})

	newTestDB(t).open(t, catalog, identity)
}

func TestOrderMigrationsRejectsUnknownAndRepeatedOwners(t *testing.T) {
	platform := PlatformMigrations()
	cases := map[string][]MigrationSet{
		"unknown":  {platform, {Owner: "billing", FS: fstest.MapFS{}}},
		"repeated": {platform, platform},
	}
	for name, sets := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := OrderMigrations(sets); err == nil {
				t.Fatal("OrderMigrations accepted the sets")
			}
		})
	}
}

func TestFreshDatabaseSkipsPreMigrationBackup(t *testing.T) {
	td := newTestDB(t)
	td.open(t)

	if got := backupsOfKind(t, td.backupDir, BackupKindPreMigration); len(got) != 0 {
		t.Errorf("fresh database took %d pre-migration backups", len(got))
	}
}

func TestPendingMigrationTakesBackupFirst(t *testing.T) {
	td := newTestDB(t)
	if err := td.open(t).Close(); err != nil {
		t.Fatal(err)
	}

	identity := sqlSet(owner.Identity, map[string]string{"00001_users.sql": "CREATE TABLE identity_users (id TEXT PRIMARY KEY);"})
	td.open(t, identity)

	if got := backupsOfKind(t, td.backupDir, BackupKindPreMigration); len(got) != 1 {
		t.Fatalf("pre-migration backups = %d, want 1", len(got))
	}
}

func TestFailingMigrationAbortsAndKeepsBackup(t *testing.T) {
	td := newTestDB(t)
	if err := td.open(t).Close(); err != nil {
		t.Fatal(err)
	}

	broken := sqlSet(owner.Identity, map[string]string{"00001_broken.sql": "CREATE TABLE identity_users (;"})
	_, err := Open(context.Background(), td.path, td.backupDir, []MigrationSet{PlatformMigrations(), broken})
	if err == nil {
		t.Fatal("open succeeded with a broken migration")
	}

	backups := backupsOfKind(t, td.backupDir, BackupKindPreMigration)
	if len(backups) != 1 {
		t.Fatalf("pre-migration backups = %d, want 1", len(backups))
	}
	restored, err := Open(context.Background(), filepath.Join(td.backupDir, backups[0].Name), t.TempDir(), []MigrationSet{PlatformMigrations()})
	if err != nil {
		t.Fatalf("backup does not open: %v", err)
	}
	_ = restored.Close()
}
