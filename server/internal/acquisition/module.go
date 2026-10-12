package acquisition

import (
	"context"
	"embed"
	"io/fs"
	"log/slog"

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
	searchOptions,
	downloadOptions,
	automaticSearchOptions,
	cutoffOptions,
)

// cutoffOptions wires Acquisition's scores of library files and the cutoff they're held to.
var cutoffOptions = fx.Options(
	fx.Provide(
		NewCutoffService,
		bus.AsHandler(newFileScoresHandler),
		jobs.AsQueue(newRescoreQueue),
	),
)

// automaticSearchOptions wires the searches nobody starts by hand: on add and release, RSS, the pending
// queue and the backlog sweeps, and the automatic grab they end in.
var automaticSearchOptions = fx.Options(
	fx.Provide(
		func(q *jobs.Queue) searchJobs { return q },
		httpserver.AsController(NewAutomaticSearchController),
		jobs.AsQueue(newSettleQueue),
		jobs.AsQueue(newRemoveTitleQueue),
		jobs.AsQueue(newRSSSyncQueue),
		jobs.AsQueue(newRSSFeedQueue),
		jobs.AsQueue(newPendingCheckQueue),
		jobs.AsQueue(newMissingSweepQueue),
		jobs.AsQueue(newCutoffSweepQueue),
		jobs.AsPeriodic(newRSSSyncPeriodic),
		jobs.AsPeriodic(newMissingSweepPeriodic),
		jobs.AsPeriodic(newCutoffSweepPeriodic),
		jobs.AsTicker(newPendingCheckTicker),
		bus.AsHandler(newTitleAddedHandler),
		bus.AsHandler(newMonitoringChangedHandler),
		bus.AsHandler(newWantedReleasedHandler),
		bus.AsHandler(newTitleRemovedHandler),
		bus.AsHandler(newIssueActionHandler),
	),
)

// searchOptions wires interactive search: the fan-out, decision engine and result cache.
var searchOptions = fx.Options(
	fx.Provide(
		NewSearchRegistry,
		NewSearchService,
		httpserver.AsController(NewSearchController),
		jobs.AsQueue(newSearchQueue),
	),
	fx.Invoke(closeSearchesOnStop),
)

func closeSearchesOnStop(lc fx.Lifecycle, s *SearchService) {
	lc.Append(fx.StopHook(s.Close))
}

// downloadOptions wires grabbing into qBittorrent, the blocked-file check and the download poll.
var downloadOptions = fx.Options(
	fx.Provide(
		NewDownloadService,
		httpserver.AsController(NewDownloadController),
		jobs.AsQueue(newDownloadCheckQueue),
		jobs.AsTicker(newDownloadPollTicker),
		bus.AsHandler(newFileImportedHandler),
		bus.AsHandler(newImportFailedHandler),
		bus.AsHandler(newImportRemovedHandler),
		jobs.AsQueue(newReleaseImportedQueue),
		jobs.AsQueue(newImportFailedQueue),
		jobs.AsQueue(newSeedingGoalQueue),
		jobs.AsPeriodic(newSeedingGoalPeriodic),
	),
	fx.Invoke(checkClientOnStart),
)

// checkClientOnStart logs an unsupported or unreachable qBittorrent at boot. Grabs check it again
// and are refused while it is below 5.2.
func checkClientOnStart(lc fx.Lifecycle, s *DownloadService) {
	lc.Append(fx.StartHook(func(ctx context.Context) {
		if err := s.CheckClient(ctx); err != nil {
			slog.WarnContext(ctx, "qBittorrent check failed; grabs are refused until it passes", "error", err)
		}
	}))
}

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
