package requests

import (
	"magpie/internal/platform/bus"
	"magpie/internal/platform/db"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/owner"
	"magpie/internal/requests/api"

	"go.uber.org/fx"
)

var Module = fx.Module(string(owner.Requests),
	db.ProvideMigrations(owner.Requests, Migrations().FS),
	fx.Provide(
		fx.Annotate(NewRepository, fx.As(fx.Self()), fx.As(new(api.Reader))),
		NewService,
		httpserver.AsController(NewController),
		bus.AsHandler(newTitleAddedHandler),
		bus.AsHandler(newMonitoringHandler),
		bus.AsHandler(newEpisodesHandler),
		bus.AsHandler(newTitleAddFailedHandler),
		bus.AsHandler(newItemAvailableHandler),
		bus.AsHandler(newRegrabDoneHandler),
		bus.AsHandler(newSubtitlesDoneHandler),
		bus.AsHandler(newTitleRemovedHandler),
	),
)
