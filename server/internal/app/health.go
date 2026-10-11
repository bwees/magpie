package app

import (
	"magpie/internal/adapters/jellyfin"
	"magpie/internal/adapters/opensubtitles"
	"magpie/internal/adapters/prowlarr"
	"magpie/internal/adapters/qbittorrent"
	"magpie/internal/adapters/tmdb"
	"magpie/internal/platform/health"

	"go.uber.org/fx"
)

// HealthProbes gives the platform health checker each adapter's minimal health method.
var HealthProbes = fx.Provide(
	health.AsProbe(tmdb.NewHealthProbe),
	health.AsProbe(qbittorrent.NewHealthProbe),
	health.AsProbe(prowlarr.NewHealthProbe),
	health.AsProbe(jellyfin.NewHealthProbe),
	health.AsProbe(opensubtitles.NewHealthProbe),
)
