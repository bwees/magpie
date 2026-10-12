package db

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"
)

type BackupKind string

const (
	BackupKindDaily        BackupKind = "DAILY"
	BackupKindPreMigration BackupKind = "PRE_MIGRATION"
	BackupKindPreRestore   BackupKind = "PRE_RESTORE"
	// BackupKindManual is an admin's "Back up now".
	BackupKindManual BackupKind = "MANUAL"
)

const (
	BackupDirName = "backups"

	// Pre-migration, pre-restore and manual snapshots are kept as many as the default daily retention.
	SnapshotsKept = 7

	backupPrefix    = "magpie-"
	backupExt       = ".db"
	dailyLayout     = "2006-01-02"
	timestampLayout = "20060102T150405Z"
	tempSuffix      = ".tmp"
	restoreSuffix   = ".restore"
	backupDirPerm   = 0o750
	walSuffix       = "-wal"
	shmSuffix       = "-shm"
)

var kindInfix = map[BackupKind]string{
	BackupKindDaily:        "",
	BackupKindPreMigration: "pre-migration-",
	BackupKindPreRestore:   "pre-restore-",
	BackupKindManual:       "manual-",
}

var (
	ErrBackupNotFound    = errors.New("backup not found")
	ErrInvalidBackupName = errors.New("invalid backup name")
)

type Backup struct {
	Name      string
	Kind      BackupKind
	CreatedAt time.Time
	Size      int64
}

func BackupDir(dataDir string) string {
	return filepath.Join(dataDir, BackupDirName)
}

// BackupName is the file name for a snapshot of kind taken at at. Daily snapshots are one per date.
func BackupName(kind BackupKind, at time.Time) string {
	at = at.UTC()
	if kind == BackupKindDaily {
		return backupPrefix + at.Format(dailyLayout) + backupExt
	}
	return backupPrefix + kindInfix[kind] + at.Format(timestampLayout) + backupExt
}

// ParseBackupName returns the kind of a backup file name, or false when the name is not a backup.
func ParseBackupName(name string) (BackupKind, bool) {
	if filepath.Base(name) != name || !strings.HasPrefix(name, backupPrefix) || !strings.HasSuffix(name, backupExt) {
		return "", false
	}
	stem := strings.TrimSuffix(strings.TrimPrefix(name, backupPrefix), backupExt)
	for _, kind := range []BackupKind{BackupKindPreMigration, BackupKindPreRestore, BackupKindManual} {
		if stamp, ok := strings.CutPrefix(stem, kindInfix[kind]); ok {
			_, err := time.Parse(timestampLayout, stamp)
			return kind, err == nil
		}
	}
	_, err := time.Parse(dailyLayout, stem)
	return BackupKindDaily, err == nil
}

// Snapshot writes a consistent copy of the database into dir with VACUUM INTO, replacing any file of the same name.
func (d *DB) Snapshot(ctx context.Context, dir string, kind BackupKind, at time.Time) (Backup, error) {
	if err := os.MkdirAll(dir, backupDirPerm); err != nil {
		return Backup{}, err
	}
	name := BackupName(kind, at)
	target := filepath.Join(dir, name)
	if err := vacuumInto(ctx, d.vacuumSource(), target); err != nil {
		return Backup{}, err
	}
	if kind != BackupKindDaily {
		if err := PruneBackups(dir, kind, SnapshotsKept); err != nil {
			return Backup{}, err
		}
	}
	return statBackup(dir, name)
}

// The reader pool snapshots without blocking writers; during migration only the writer exists.
func (d *DB) vacuumSource() *gorm.DB {
	if d.reader != nil {
		return d.reader
	}
	return d.writer
}

// VACUUM INTO refuses an existing file, so it writes a temp file and renames it over the target.
func vacuumInto(ctx context.Context, source *gorm.DB, target string) error {
	temp := target + tempSuffix
	if err := os.Remove(temp); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := source.WithContext(ctx).Exec("VACUUM INTO ?", temp).Error; err != nil {
		return fmt.Errorf("vacuum into %s: %w", temp, err)
	}
	return os.Rename(temp, target)
}

// ListBackups returns the backups in dir, newest first.
func ListBackups(dir string) ([]Backup, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Backup{}, nil
	}
	if err != nil {
		return nil, err
	}
	backups := make([]Backup, 0, len(entries))
	for _, entry := range entries {
		if _, ok := ParseBackupName(entry.Name()); !ok || entry.IsDir() {
			continue
		}
		backup, err := statBackup(dir, entry.Name())
		if err != nil {
			return nil, err
		}
		backups = append(backups, backup)
	}
	slices.SortFunc(backups, func(a, b Backup) int {
		return cmp.Or(b.CreatedAt.Compare(a.CreatedAt), cmp.Compare(b.Name, a.Name))
	})
	return backups, nil
}

// PruneBackups deletes the oldest backups of kind beyond keep.
func PruneBackups(dir string, kind BackupKind, keep int) error {
	backups, err := ListBackups(dir)
	if err != nil {
		return err
	}
	ofKind := slices.DeleteFunc(backups, func(b Backup) bool { return b.Kind != kind })
	// Names of one kind sort chronologically, which survives mtime changes from copies.
	slices.SortFunc(ofKind, func(a, b Backup) int { return cmp.Compare(b.Name, a.Name) })
	if len(ofKind) <= keep {
		return nil
	}
	for _, old := range ofKind[keep:] {
		if err := os.Remove(filepath.Join(dir, old.Name)); err != nil {
			return err
		}
	}
	return nil
}

func statBackup(dir, name string) (Backup, error) {
	kind, ok := ParseBackupName(name)
	if !ok {
		return Backup{}, fmt.Errorf("%w: %q", ErrInvalidBackupName, name)
	}
	info, err := os.Stat(filepath.Join(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return Backup{}, fmt.Errorf("%w: %q", ErrBackupNotFound, name)
	}
	if err != nil {
		return Backup{}, err
	}
	return Backup{Name: name, Kind: kind, CreatedAt: info.ModTime().UTC(), Size: info.Size()}, nil
}

// StageRestore copies a backup next to the database. The next Open swaps it in.
func StageRestore(dbPath, backupDir, name string) error {
	if _, err := statBackup(backupDir, name); err != nil {
		return err
	}
	source, err := os.Open(filepath.Join(backupDir, name))
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()

	staged := dbPath + restoreSuffix
	temp := staged + tempSuffix
	target, err := os.Create(temp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(target, source); err != nil {
		return errors.Join(err, target.Close())
	}
	if err := errors.Join(target.Sync(), target.Close()); err != nil {
		return err
	}
	return os.Rename(temp, staged)
}

// applyStagedRestore snapshots the current database as PRE_RESTORE, then replaces it with the staged file.
// The old WAL and shared-memory files go too, so none of their pages land on the restored database.
func applyStagedRestore(ctx context.Context, dbPath, backupDir string) error {
	staged := dbPath + restoreSuffix
	if _, err := os.Stat(staged); errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if _, err := os.Stat(dbPath); err == nil {
		if err := snapshotFile(ctx, dbPath, backupDir); err != nil {
			return err
		}
	}
	for _, leftover := range []string{dbPath, dbPath + walSuffix, dbPath + shmSuffix} {
		if err := os.Remove(leftover); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return os.Rename(staged, dbPath)
}

func snapshotFile(ctx context.Context, dbPath, backupDir string) error {
	current, err := openGorm(writerDSN(dbPath))
	if err != nil {
		return err
	}
	d := newDB(current, dbPath)
	_, snapErr := d.Snapshot(ctx, backupDir, BackupKindPreRestore, time.Now())
	return errors.Join(snapErr, closeGorm(current))
}
