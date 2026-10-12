// Command agent runs GPU jobs for the server. It ships as a separate image.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"magpie/agent/config"
	"magpie/agent/encode"
	"magpie/agent/jobs"
	"magpie/agent/probe"
	"magpie/agent/session"
	"magpie/agent/transfer"
	"magpie/internal/transcode/api/agentproto"

	"golang.org/x/sys/unix"
)

// version is set at build time with -ldflags "-X main.version=…".
var version = "dev"

// samplesDir holds the decode-test clips inside the scratch dir.
const samplesDir = ".magpie-probe"

func main() {
	cfg, err := config.Load(os.Getenv, os.Hostname)
	if err != nil {
		slog.Error("agent: invalid configuration", "error", err)
		os.Exit(1)
	}
	slog.Info("agent: starting", "agent", cfg.AgentID, "name", cfg.Name, "server", cfg.ServerURL.String(), "version", version, "cpu", cfg.CPU)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	prober := probe.New(filepath.Join(cfg.ScratchDir, samplesDir))
	prober.CPU = cfg.CPU
	scratchFree := func() (int64, error) { return freeBytes(cfg.ScratchDir) }
	client := session.New(session.Options{
		ServerURL:   cfg.ServerURL,
		Secret:      cfg.Secret,
		AgentID:     cfg.AgentID,
		Hostname:    cfg.Name,
		Version:     version,
		Mode:        agentproto.ModeDownloadFirst,
		Prober:      prober,
		ScratchFree: scratchFree,
	})
	transfers := transfer.New(client.HTTP(), cfg.ServerURL)
	authHeaders := map[string]string{
		agentproto.HeaderAuthorization: agentproto.BearerPrefix + cfg.Secret,
		agentproto.HeaderAgentID:       string(cfg.AgentID),
	}
	manager := jobs.NewManager(jobs.Options{
		ScratchDir:  cfg.ScratchDir,
		Transfers:   transfers,
		Encoder:     encode.New(),
		ScratchFree: scratchFree,
		DirectRead: func(source agentproto.SourceRef) encode.Input {
			return encode.HTTPInput(transfers.SourceURL(source.URL), authHeaders)
		},
	}, client)
	client.SetJobs(manager)

	var wg sync.WaitGroup
	wg.Go(func() { manager.Run(ctx) })
	err = client.Run(ctx)
	wg.Wait()
	if err != nil {
		slog.Error("agent: stopped", "error", err)
		os.Exit(1)
	}
}

func freeBytes(dir string) (int64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(dir, &stat); err != nil {
		return 0, err
	}
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}
