// Package app lists every module the server runs, so main and the flow tests boot the same graph.
package app

import (
	"magpie/internal/acquisition"
	"magpie/internal/activity"
	"magpie/internal/catalog"
	"magpie/internal/identity"
	"magpie/internal/library"
	"magpie/internal/platform"
	"magpie/internal/platform/httpserver"
	"magpie/internal/requests"
	"magpie/internal/subtitles"
	"magpie/internal/transcode"

	"go.uber.org/fx"
)

var Contexts = fx.Options(
	identity.Module,
	catalog.Module,
	requests.Module,
	library.Module,
	acquisition.Module,
	subtitles.Module,
	transcode.Module,
	activity.Module,
)

var Module = fx.Options(platform.Module, Contexts, PortFakes, ReaderFakes, HealthProbes, httpserver.AsSchemas(messageSchemas()))
