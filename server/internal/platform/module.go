// Package platform holds the runtime every context plugs into.
package platform

import (
	"magpie/internal/platform/backup"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/config"
	"magpie/internal/platform/db"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/jobs"
	"magpie/internal/platform/live"
	"magpie/internal/platform/restart"
	"magpie/internal/platform/settings"

	"go.uber.org/fx"
)

var Module = fx.Module("platform",
	config.Module,
	db.Module,
	settings.Module,
	jobs.Module,
	bus.Module,
	live.Module,
	restart.Module,
	backup.Module,
	httpserver.Module,
)
