// Command agent runs GPU jobs for the server. It ships as a separate image.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"magpie/agent/config"
	"magpie/agent/probe"
	"magpie/agent/session"
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
	slog.Info("agent: starting", "agent", cfg.AgentID, "name", cfg.Name, "server", cfg.ServerURL.String(), "version", version)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := session.New(session.Options{
		ServerURL:   cfg.ServerURL,
		Secret:      cfg.Secret,
		AgentID:     cfg.AgentID,
		Hostname:    cfg.Name,
		Version:     version,
		Mode:        agentproto.ModeDownloadFirst,
		Prober:      probe.New(filepath.Join(cfg.ScratchDir, samplesDir)),
		ScratchFree: func() (int64, error) { return freeBytes(cfg.ScratchDir) },
	})
	if err := client.Run(ctx); err != nil {
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
