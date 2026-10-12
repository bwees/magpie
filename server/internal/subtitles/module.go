package subtitles

import (
	"embed"
	"io/fs"

	"magpie/internal/platform/bus"
	"magpie/internal/platform/db"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/jobs"
	"magpie/internal/platform/owner"
	"magpie/internal/subtitles/api"

	"go.uber.org/fx"
)

const migrationsDir = "migrations"

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

var Module = fx.Module(string(owner.Subtitles),
	db.ProvideMigrations(owner.Subtitles, Migrations().FS),
	fx.Provide(
		NewRepository,
		func(r *Repository) api.Reader { return r },
		func(q *jobs.Queue) TaskQueue { return q },
		NewService,
		httpserver.AsController(NewController),
		bus.AsHandler(newLibraryHandler),
		bus.AsHandler(newIssueActionHandler),
		bus.AsHandler(newLanguagesChangedHandler),
		jobs.AsQueue(newSearchQueue),
		jobs.AsQueue(newDownloadsQueue),
		jobs.AsQueue(newSweepQueue),
		jobs.AsQueue(newRecomputeQueue),
		jobs.AsQueue(newRemoveSidecarsQueue),
		jobs.AsPeriodic(newSweepPeriodic),
	),
)

// Migrations is Subtitles' goose directory, for tests that open a database without the app.
func Migrations() db.MigrationSet {
	sub, err := fs.Sub(embeddedMigrations, migrationsDir)
	if err != nil {
		panic(err) // the directory is embedded at build time, so this cannot fail
	}
	return db.MigrationSet{Owner: owner.Subtitles, FS: sub}
}
