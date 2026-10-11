package main

import (
	"flag"
	"log/slog"

	"magpie/internal/platform"
	"magpie/internal/platform/httpserver"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
)

func main() {
	exportSpec := flag.Bool("open-api", false, "write the OpenAPI spec to "+httpserver.SpecFile+" and exit")
	flag.Parse()

	mode := httpserver.ModeServe
	if *exportSpec {
		mode = httpserver.ModeExportSpec
	}

	fx.New(
		fx.WithLogger(newFxLogger),
		fx.Supply(mode),
		platform.Module,
	).Run()
}

// Keeps fx's wiring chatter at debug while startup errors still surface.
func newFxLogger() fxevent.Logger {
	logger := &fxevent.SlogLogger{Logger: slog.Default()}
	logger.UseLogLevel(slog.LevelDebug)
	return logger
}
