package db

import (
	"cmp"
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"slices"
	"time"

	"magpie/internal/platform/owner"

	"github.com/pressly/goose/v3"
	"go.uber.org/fx"
)

const (
	migrationsGroup    = `group:"migrations"`
	migrationsDir      = "migrations"
	versionTableSuffix = "_goose_db_version"
)

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

// MigrationSet is one owner's goose directory. Each owner keeps its own version table.
type MigrationSet struct {
	Owner owner.Name
	FS    fs.FS
}

// ProvideMigrations registers an owner's goose SQL files to run at startup.
func ProvideMigrations(o owner.Name, fsys fs.FS) fx.Option {
	set := MigrationSet{Owner: o, FS: fsys}
	return fx.Provide(fx.Annotate(func() MigrationSet { return set }, fx.ResultTags(migrationsGroup)))
}

func platformMigrations() fs.FS {
	sub, err := fs.Sub(embeddedMigrations, migrationsDir)
	if err != nil {
		panic(err) // the directory is embedded at build time, so this cannot fail
	}
	return sub
}

// OrderMigrations sorts sets into owner.All order and rejects unknown or repeated owners.
func OrderMigrations(sets []MigrationSet) ([]MigrationSet, error) {
	seen := make(map[owner.Name]bool, len(sets))
	for _, set := range sets {
		if !set.Owner.Known() {
			return nil, fmt.Errorf("migrations for unknown owner %q", set.Owner)
		}
		if seen[set.Owner] {
			return nil, fmt.Errorf("migrations for %q registered twice", set.Owner)
		}
		seen[set.Owner] = true
	}
	ordered := slices.Clone(sets)
	slices.SortFunc(ordered, func(a, b MigrationSet) int {
		return cmp.Compare(slices.Index(owner.All, a.Owner), slices.Index(owner.All, b.Owner))
	})
	return ordered, nil
}

func (d *DB) migrate(ctx context.Context, sets []MigrationSet, existed bool, backupDir string) error {
	ordered, err := OrderMigrations(sets)
	if err != nil {
		return err
	}
	sqlDB, err := d.writer.DB()
	if err != nil {
		return err
	}

	providers := make([]*goose.Provider, 0, len(ordered))
	pending := false
	for _, set := range ordered {
		provider, err := newProvider(sqlDB, set)
		if err != nil {
			return fmt.Errorf("%s migrations: %w", set.Owner, err)
		}
		hasPending, err := provider.HasPending(ctx)
		if err != nil {
			return fmt.Errorf("%s migrations: %w", set.Owner, err)
		}
		pending = pending || hasPending
		providers = append(providers, provider)
	}

	if pending && existed {
		backup, err := d.Snapshot(ctx, backupDir, BackupKindPreMigration, time.Now())
		if err != nil {
			return fmt.Errorf("pre-migration backup: %w", err)
		}
		slog.InfoContext(ctx, "pre-migration backup taken", "backup", backup.Name)
	}

	for i, provider := range providers {
		results, err := provider.Up(ctx)
		for _, result := range results {
			slog.InfoContext(ctx, "applied migration", "owner", ordered[i].Owner, "source", result.Source.Path, "duration", result.Duration)
		}
		if err != nil {
			return fmt.Errorf("%s migrations: %w", ordered[i].Owner, err)
		}
	}
	return nil
}

// SchemaVersion returns the highest applied migration version of one owner.
func (d *DB) SchemaVersion(ctx context.Context, set MigrationSet) (int64, error) {
	sqlDB, err := d.writer.DB()
	if err != nil {
		return 0, err
	}
	provider, err := newProvider(sqlDB, set)
	if err != nil {
		return 0, err
	}
	return provider.GetDBVersion(ctx)
}

func newProvider(sqlDB *sql.DB, set MigrationSet) (*goose.Provider, error) {
	return goose.NewProvider(goose.DialectSQLite3, sqlDB, set.FS,
		goose.WithTableName(string(set.Owner)+versionTableSuffix),
		goose.WithDisableGlobalRegistry(true),
	)
}

// PlatformMigrations is the platform's own goose directory.
func PlatformMigrations() MigrationSet {
	return MigrationSet{Owner: owner.Platform, FS: platformMigrations()}
}
