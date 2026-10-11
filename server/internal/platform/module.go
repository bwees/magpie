// Package platform holds the runtime every context plugs into.
package platform

import (
	"magpie/internal/platform/config"
	"magpie/internal/platform/db"
	"magpie/internal/platform/httpserver"

	"go.uber.org/fx"
)

var Module = fx.Module("platform",
	config.Module,
	db.Module,
	httpserver.Module,
)
