package transcode

import (
	"context"
	"embed"
	"io/fs"
	"log/slog"
	"sync"
	"time"

	"magpie/internal/platform/db"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/owner"

	"go.uber.org/fx"
)

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

const migrationsDir = "migrations"

// Migrations is Transcode's goose directory.
func Migrations() db.MigrationSet {
	sub, err := fs.Sub(embeddedMigrations, migrationsDir)
	if err != nil {
		panic(err) // the directory is embedded at build time, so this cannot fail
	}
	return db.MigrationSet{Owner: owner.Transcode, FS: sub}
}

var Module = fx.Module(string(owner.Transcode),
	db.ProvideMigrations(owner.Transcode, Migrations().FS),
	fx.Supply(DefaultTimings),
	fx.Provide(
		newAgentSecret,
		NewAgentRepository,
		NewAgentHub,
		NewAgentService,
		NewAgentSocket,
		httpserver.AsController(NewController),
	),
	fx.Invoke(startSweeps),
)

// startSweeps checks for silent agents and due retests every SweepEvery while the server runs.
func startSweeps(lc fx.Lifecycle, service *AgentService, timings Timings) {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			wg.Go(func() { runSweeps(ctx, service, timings.SweepEvery) })
			return nil
		},
		OnStop: func(context.Context) error {
			cancel()
			wg.Wait()
			return nil
		},
	})
}

func runSweeps(ctx context.Context, service *AgentService, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := service.Sweep(ctx); err != nil && ctx.Err() == nil {
				slog.ErrorContext(ctx, "transcode: agent sweep failed", "error", err)
			}
		}
	}
}
