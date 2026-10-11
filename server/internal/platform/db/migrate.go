package db

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"
	"log/slog"

	"github.com/pressly/goose/v3"
)

const migrationsDir = "migrations"

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

// Migrate applies every pending goose migration. The schema is owned here, never by GORM AutoMigrate.
func Migrate(ctx context.Context, sqlDB *sql.DB) error {
	provider, err := newMigrationProvider(sqlDB)
	if err != nil {
		return err
	}
	results, err := provider.Up(ctx)
	for _, result := range results {
		slog.InfoContext(ctx, "applied migration", "source", result.Source.Path, "duration", result.Duration)
	}
	return err
}

// SchemaVersion returns the highest applied migration version.
func SchemaVersion(ctx context.Context, sqlDB *sql.DB) (int64, error) {
	provider, err := newMigrationProvider(sqlDB)
	if err != nil {
		return 0, err
	}
	return provider.GetDBVersion(ctx)
}

func newMigrationProvider(sqlDB *sql.DB) (*goose.Provider, error) {
	migrations, err := fs.Sub(embeddedMigrations, migrationsDir)
	if err != nil {
		return nil, err
	}
	return goose.NewProvider(goose.DialectSQLite3, sqlDB, migrations)
}
