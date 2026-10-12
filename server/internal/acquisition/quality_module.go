package acquisition

import (
	"context"

	"magpie/internal/acquisition/api"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/jobs"

	"go.uber.org/fx"
)

// qualityOptions wires the quality model: profiles, custom formats, presets and TRaSH sync.
var qualityOptions = fx.Options(
	fx.Provide(
		NewQualityRepository,
		fx.Annotate(NewReader, fx.As(new(api.Reader))),
		NewQualityService,
		NewTrashSyncService,
		fx.Annotate(NewImporter, fx.As(new(api.Importer))),
		httpserver.AsController(NewQualityController),
		jobs.AsQueue(newTrashSyncQueue),
		jobs.AsPeriodic(newTrashSyncPeriodic),
	),
	fx.Invoke(seedPresetsOnStart),
)

func seedPresetsOnStart(lc fx.Lifecycle, s *QualityService) {
	lc.Append(fx.StartHook(func(ctx context.Context) error { return s.SeedPresets(ctx) }))
}
