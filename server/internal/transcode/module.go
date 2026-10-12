package transcode

import (
	"context"
	"embed"
	"io/fs"
	"log/slog"
	"time"

	"magpie/internal/platform/bus"
	"magpie/internal/platform/db"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/jobs"
	"magpie/internal/platform/owner"
	"magpie/internal/transcode/api"
	"magpie/internal/transcode/api/agentproto"

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
	// Agents present the shared secret instead of a session; these routes check it themselves.
	httpserver.AsSelfAuthenticated(agentproto.ControlPath),
	httpserver.AsSelfAuthenticated(agentproto.JobsPath),
	fx.Provide(
		newAgentSecret,
		NewAgentRepository,
		NewJobRepository,
		func(r *JobRepository) api.Reader { return r },
		NewAgentHub,
		NewLifetime,
		NewWake,
		NewTransferGate,
		NewAgentService,
		NewJobService,
		NewAgentSocket,
		NewTransfers,
		httpserver.AsController(NewController),
		bus.AsHandler(newCutoffMetHandler),
		bus.AsHandler(newFileTranscodedHandler),
		bus.AsHandler(newSwapRejectedHandler),
		bus.AsHandler(newFileDeletedHandler),
		bus.AsHandler(newTitleRemovedHandler),
		bus.AsHandler(newRunAgentJobHandler),
		func(q *jobs.Queue) TaskQueue { return q },
		jobs.AsQueue(newVerifyQueue),
		jobs.AsQueue(newSweepQueue),
	),
	fx.Invoke(startLoops),
)

// startLoops runs the agent sweep and the scheduler while the server runs. Its stop hook ends the
// Lifetime too; it is appended after the HTTP server's, so it runs first and open transfers end before
// the server waits for them.
func startLoops(lc fx.Lifecycle, agents *AgentService, jobs *JobService, lifetime *Lifetime, wake *Wake, timings Timings) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			lifetime.Go(func(ctx context.Context) { runLoops(ctx, agents, jobs, wake, timings.SweepEvery) })
			return nil
		},
		OnStop: func(context.Context) error {
			lifetime.Stop()
			return nil
		},
	})
}

func runLoops(ctx context.Context, agents *AgentService, jobs *JobService, wake *Wake, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := agents.Sweep(ctx); err != nil && ctx.Err() == nil {
				slog.ErrorContext(ctx, "transcode: agent sweep failed", "error", err)
			}
		case <-wake.C():
		}
		if err := jobs.Schedule(ctx); err != nil && ctx.Err() == nil {
			slog.ErrorContext(ctx, "transcode: scheduling failed", "error", err)
		}
	}
}
