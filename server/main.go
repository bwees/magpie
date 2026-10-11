package main

import (
	"flag"
	"log/slog"
	"os"

	"magpie/internal/app"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/restart"

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

	var restarter *restart.Restarter
	fx.New(
		fx.WithLogger(newFxLogger),
		fx.Supply(mode),
		app.Module,
		fx.Populate(&restarter),
	).Run()

	if restarter.Requested() {
		slog.Info("restarting")
		if err := restart.Exec(); err != nil {
			slog.Error("restart failed", "error", err)
			os.Exit(1)
		}
	}
}

// Keeps fx's wiring chatter at debug while startup errors still surface.
func newFxLogger() fxevent.Logger {
	logger := &fxevent.SlogLogger{Logger: slog.Default()}
	logger.UseLogLevel(slog.LevelDebug)
	return logger
}
