package migrate

import (
	"context"

	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/jobs"

	"go.uber.org/fx"
)

// Module runs the migration. internal/app provides its ports and the contexts' Importers.
var Module = fx.Module("migrate",
	fx.Provide(
		NewService,
		httpserver.AsController(NewController),
		jobs.AsQueue(newQueue),
	),
	fx.Invoke(func(s *Service) error { return s.MarkInterrupted(context.Background()) }),
)
