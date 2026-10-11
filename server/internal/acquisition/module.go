package acquisition

import (
	"embed"
	"io/fs"

	"magpie/internal/platform/db"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/jobs"
	"magpie/internal/platform/owner"

	"go.uber.org/fx"
)

const migrationsDir = "migrations"

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

var Module = fx.Module(string(owner.Acquisition),
	db.ProvideMigrations(owner.Acquisition, migrations()),
	fx.Provide(
		NewRepository,
		NewService,
		httpserver.AsController(NewController),
		jobs.AsQueue(newIndexerSyncQueue),
		jobs.AsPeriodic(newIndexerSyncPeriodic),
	),
	fx.Invoke(syncIndexersOnStart),
	qualityOptions,
)

// Migrations is Acquisition's goose directory, for tests that open a database without the app.
func Migrations() db.MigrationSet {
	return db.MigrationSet{Owner: owner.Acquisition, FS: migrations()}
}

func migrations() fs.FS {
	sub, err := fs.Sub(embeddedMigrations, migrationsDir)
	if err != nil {
		panic(err) // the directory is embedded at build time, so this cannot fail
	}
	return sub
}
