package activity

import (
	"embed"
	"io/fs"

	"magpie/internal/activity/api"
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

// Migrations are Activity's goose files.
func Migrations() db.MigrationSet {
	sub, err := fs.Sub(embeddedMigrations, migrationsDir)
	if err != nil {
		panic(err) // the directory is embedded at build time, so this cannot fail
	}
	return db.MigrationSet{Owner: owner.Activity, FS: sub}
}

var Module = fx.Module(string(owner.Activity),
	db.ProvideMigrations(owner.Activity, Migrations().FS),
	fx.Provide(
		fx.Annotate(NewRepository, fx.As(fx.Self()), fx.As(new(api.Reader))),
		NewService,
		bus.AsHandler(NewTimelineHandler),
		bus.AsHandler(NewHistoryHandler),
		bus.AsHandler(NewPipelineHandler),
		bus.AsHandler(NewAttentionHandler),
		bus.AsHandler(NewNotifyHandler),
		httpserver.AsController(NewController),
		jobs.AsQueue(newPruneQueue),
		jobs.AsQueue(newSendQueue),
		jobs.AsPeriodic(newPrunePeriodic),
	),
)
