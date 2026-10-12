package library

import (
	"embed"
	"io/fs"

	"magpie/internal/library/api"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/db"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/jobs"
	"magpie/internal/platform/owner"

	"go.uber.org/fx"
)

const migrationsDir = "migrations"

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

var Module = fx.Module(string(owner.Library),
	db.ProvideMigrations(owner.Library, Migrations().FS),
	fx.Provide(
		NewRepository,
		func(r *Repository) api.Reader { return r },
		NewService,
		httpserver.AsController(NewController),
		bus.AsHandler(newDownloadCompletedHandler),
		jobs.AsQueue(newImportQueue),
	),
)

// Migrations is Library's goose directory, for tests that open a database without the app.
func Migrations() db.MigrationSet {
	sub, err := fs.Sub(embeddedMigrations, migrationsDir)
	if err != nil {
		panic(err) // the directory is embedded at build time, so this cannot fail
	}
	return db.MigrationSet{Owner: owner.Library, FS: sub}
}
