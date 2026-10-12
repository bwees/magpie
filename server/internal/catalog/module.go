package catalog

import (
	"magpie/internal/catalog/api"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/db"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/jobs"
	"magpie/internal/platform/owner"

	"go.uber.org/fx"
)

var Module = fx.Module(string(owner.Catalog),
	db.ProvideMigrations(owner.Catalog, Migrations().FS),
	fx.Provide(
		NewRepository,
		func(r *Repository) api.Reader { return r },
		NewImageCache,
		NewService,
		func(s *Service) api.Importer { return s },
		httpserver.AsController(NewController),
		bus.AsHandler(newRequestApprovedHandler),
		bus.AsHandler(newTmdbHealthHandler),
		bus.AsHandler(newFolderFoundHandler),
		httpserver.AsController(NewScanReviewController),
		jobs.AsQueue(newMatchFolderQueue),
		jobs.AsQueue(newAddTitleQueue),
		jobs.AsQueue(newRefreshTitleQueue),
		jobs.AsQueue(newMovieRefreshQueue),
		jobs.AsQueue(newSeriesRefreshQueue),
		jobs.AsQueue(newReleaseClockQueue),
		jobs.AsPeriodic(newMovieRefreshPeriodic),
		jobs.AsPeriodic(newContinuingSeriesRefreshPeriodic),
		jobs.AsPeriodic(newEndedSeriesRefreshPeriodic),
		jobs.AsPeriodic(newReleaseClockPeriodic),
	),
)
