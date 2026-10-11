package db

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

const latestMigration = 1

func TestOpenMigratesEmptyDatabase(t *testing.T) {
	ctx := context.Background()
	gdb, err := Open(ctx, filepath.Join(t.TempDir(), FileName))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = Close(gdb) })

	sqlDB, err := gdb.DB()
	if err != nil {
		t.Fatal(err)
	}
	version, err := SchemaVersion(ctx, sqlDB)
	if err != nil {
		t.Fatalf("schema version: %v", err)
	}
	if version != latestMigration {
		t.Errorf("schema version = %d, want %d", version, latestMigration)
	}
	if !gdb.Migrator().HasTable("platform_settings") {
		t.Error("platform_settings table missing after migration")
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), FileName)

	for range 2 {
		gdb, err := Open(ctx, path)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		if err := Close(gdb); err != nil {
			t.Fatalf("close: %v", err)
		}
	}
}

func TestOpenAppliesConnectionPragmas(t *testing.T) {
	gdb, err := Open(context.Background(), filepath.Join(t.TempDir(), FileName))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = Close(gdb) })

	pragmas := map[string]string{
		"journal_mode": "wal",
		"synchronous":  "1",
		"foreign_keys": "1",
	}
	for pragma, want := range pragmas {
		var got string
		if err := gdb.Raw("PRAGMA " + pragma).Scan(&got).Error; err != nil {
			t.Fatalf("pragma %s: %v", pragma, err)
		}
		if !strings.EqualFold(got, want) {
			t.Errorf("pragma %s = %q, want %q", pragma, got, want)
		}
	}
}
