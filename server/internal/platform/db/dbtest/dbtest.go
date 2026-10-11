// Package dbtest opens a migrated database in a test's temp dir.
package dbtest

import (
	"context"
	"path/filepath"
	"testing"

	"magpie/internal/platform/db"
)

// Open migrates a fresh database with the platform's migrations plus extra and closes it after the test.
func Open(t testing.TB, extra ...db.MigrationSet) *db.DB {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(context.Background(), filepath.Join(dir, db.FileName), db.BackupDir(dir), append([]db.MigrationSet{db.PlatformMigrations()}, extra...))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}
