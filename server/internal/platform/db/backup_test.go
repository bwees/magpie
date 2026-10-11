package db

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"
)

var backupDay = time.Date(2026, 10, 11, 3, 0, 0, 0, time.UTC)

func TestBackupNamesRoundTrip(t *testing.T) {
	cases := map[BackupKind]string{
		BackupKindDaily:        "magpie-2026-10-11.db",
		BackupKindPreMigration: "magpie-pre-migration-20261011T030000Z.db",
		BackupKindPreRestore:   "magpie-pre-restore-20261011T030000Z.db",
		BackupKindManual:       "magpie-manual-20261011T030000Z.db",
	}
	for kind, want := range cases {
		name := BackupName(kind, backupDay)
		if name != want {
			t.Errorf("BackupName(%s) = %q, want %q", kind, name, want)
		}
		if got, ok := ParseBackupName(name); !ok || got != kind {
			t.Errorf("ParseBackupName(%q) = %s, %v", name, got, ok)
		}
	}
}

func TestParseBackupNameRejectsOtherFiles(t *testing.T) {
	for _, name := range []string{"magpie.db", "magpie-yesterday.db", "../magpie-2026-10-11.db", "magpie-2026-10-11.db.tmp", "notes.txt"} {
		if _, ok := ParseBackupName(name); ok {
			t.Errorf("ParseBackupName(%q) accepted", name)
		}
	}
}

func TestDailySnapshotReplacesSameDate(t *testing.T) {
	td := newTestDB(t)
	d := td.open(t)
	ctx := context.Background()

	for range 2 {
		if _, err := d.Snapshot(ctx, td.backupDir, BackupKindDaily, backupDay); err != nil {
			t.Fatalf("snapshot: %v", err)
		}
	}

	if got := backupsOfKind(t, td.backupDir, BackupKindDaily); len(got) != 1 {
		t.Fatalf("daily backups = %d, want 1", len(got))
	}
}

func TestPruneBackupsKeepsNewestOfKind(t *testing.T) {
	td := newTestDB(t)
	d := td.open(t)
	ctx := context.Background()
	const keep = 2

	for day := range 4 {
		if _, err := d.Snapshot(ctx, td.backupDir, BackupKindDaily, backupDay.AddDate(0, 0, day)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.Snapshot(ctx, td.backupDir, BackupKindPreMigration, backupDay); err != nil {
		t.Fatal(err)
	}

	if err := PruneBackups(td.backupDir, BackupKindDaily, keep); err != nil {
		t.Fatal(err)
	}

	daily := backupsOfKind(t, td.backupDir, BackupKindDaily)
	names := map[string]bool{}
	for _, b := range daily {
		names[b.Name] = true
	}
	if len(daily) != keep || !names[BackupName(BackupKindDaily, backupDay.AddDate(0, 0, 3))] || !names[BackupName(BackupKindDaily, backupDay.AddDate(0, 0, 2))] {
		t.Errorf("kept %v, want the two newest days", names)
	}
	if got := backupsOfKind(t, td.backupDir, BackupKindPreMigration); len(got) != 1 {
		t.Errorf("pruning daily backups touched pre-migration backups")
	}
}

func TestStagedRestoreReplacesDatabaseOnOpen(t *testing.T) {
	td := newTestDB(t)
	ctx := context.Background()
	d := td.open(t)

	setKey := func(key string) {
		t.Helper()
		if err := d.Write(ctx, func(tx *gorm.DB) error {
			return tx.Exec("INSERT INTO platform_settings (key, value, updated_at) VALUES (?, '1', CURRENT_TIMESTAMP)", key).Error
		}); err != nil {
			t.Fatal(err)
		}
	}
	setKey("platform.before")
	backup, err := d.Snapshot(ctx, td.backupDir, BackupKindDaily, backupDay)
	if err != nil {
		t.Fatal(err)
	}
	setKey("platform.after")

	if err := StageRestore(td.path, td.backupDir, backup.Name); err != nil {
		t.Fatalf("stage: %v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}

	restored := td.open(t)
	var keys []string
	if err := restored.Read(ctx).Table("platform_settings").Order("key").Pluck("key", &keys).Error; err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0] != "platform.before" {
		t.Errorf("keys after restore = %v, want [platform.before]", keys)
	}
	if got := backupsOfKind(t, td.backupDir, BackupKindPreRestore); len(got) != 1 {
		t.Errorf("pre-restore backups = %d, want 1", len(got))
	}
	if _, err := os.Stat(td.path + restoreSuffix); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("staged restore file left behind: %v", err)
	}
}

func TestStageRestoreRejectsUnknownBackup(t *testing.T) {
	td := newTestDB(t)
	cases := map[string]error{
		"magpie-2020-01-01.db":  ErrBackupNotFound,
		"../magpie.db":          ErrInvalidBackupName,
		filepath.Join("x", "y"): ErrInvalidBackupName,
	}
	for name, want := range cases {
		if err := StageRestore(td.path, td.backupDir, name); !errors.Is(err, want) {
			t.Errorf("StageRestore(%q) = %v, want %v", name, err, want)
		}
	}
}
