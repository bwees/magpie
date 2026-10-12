// Package config reads the bootstrap environment. Everything else is a setting stored in SQLite.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"go.uber.org/fx"
)

const (
	EnvPort    = "MAGPIE_PORT"
	EnvDataDir = "MAGPIE_DATA_DIR"
	EnvBaseURL = "MAGPIE_BASE_URL"
	EnvWebDir  = "MAGPIE_WEB_DIR"

	EnvAgentSecret       = "MAGPIE_AGENT_SECRET"
	MinAgentSecretLength = 32

	DefaultPort    = 3000
	DefaultDataDir = "/data"

	minPort        = 1
	maxPort        = 65535
	dataDirPerm    = 0o750
	writeProbeName = ".magpie-write-check-*"
)

var (
	ErrInvalidPort        = errors.New("port must be a number from 1 to 65535")
	ErrDataDirNotWritable = errors.New("data dir is not a writable directory")
	ErrInvalidBaseURL     = errors.New("base URL must be an absolute http(s) URL")
	ErrNetworkFilesystem  = errors.New("data dir is on a network filesystem; SQLite needs local disk")
	ErrAgentSecretMissing = errors.New("agent secret is required")
	ErrAgentSecretShort   = fmt.Errorf("agent secret must be at least %d characters", MinAgentSecretLength)
)

var baseURLSchemes = map[string]bool{"http": true, "https": true}

// Env is the validated bootstrap environment.
type Env struct {
	Port    int
	DataDir string
	// BaseURL is nil when MAGPIE_BASE_URL is unset.
	BaseURL *url.URL
	// WebDir is the built SPA to serve. Empty in development, where Vite serves it.
	WebDir string
	// AgentSecret is the shared secret transcode agents present. Load requires it; an Env built in a test
	// may leave it empty, which refuses every agent.
	AgentSecret string
}

var Module = fx.Module("config", fx.Provide(Load))

// Load reads and validates the process environment, creating the data dir if needed.
func Load() (Env, error) {
	return Parse(os.Getenv)
}

// Parse validates the environment returned by getenv and reports every invalid variable at once.
func Parse(getenv func(string) string) (Env, error) {
	port, portErr := parsePort(getenv(EnvPort))
	dataDir, dataDirErr := prepareDataDir(getenv(EnvDataDir))
	baseURL, baseURLErr := parseBaseURL(getenv(EnvBaseURL))
	agentSecret, agentSecretErr := parseAgentSecret(getenv(EnvAgentSecret))

	if err := errors.Join(portErr, dataDirErr, baseURLErr, agentSecretErr); err != nil {
		return Env{}, err
	}

	return Env{
		Port:        port,
		DataDir:     dataDir,
		BaseURL:     baseURL,
		WebDir:      getenv(EnvWebDir),
		AgentSecret: agentSecret,
	}, nil
}

func parseAgentSecret(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("%s: %w", EnvAgentSecret, ErrAgentSecretMissing)
	}
	if len(raw) < MinAgentSecretLength {
		return "", fmt.Errorf("%s: %w", EnvAgentSecret, ErrAgentSecretShort)
	}
	return raw, nil
}

func parsePort(raw string) (int, error) {
	if raw == "" {
		return DefaultPort, nil
	}
	port, err := strconv.Atoi(raw)
	if err != nil || port < minPort || port > maxPort {
		return 0, fmt.Errorf("%s=%q: %w", EnvPort, raw, ErrInvalidPort)
	}
	return port, nil
}

func prepareDataDir(raw string) (string, error) {
	if raw == "" {
		raw = DefaultDataDir
	}
	dir, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("%s=%q: %w: %w", EnvDataDir, raw, ErrDataDirNotWritable, err)
	}
	if err := ensureWritableDir(dir); err != nil {
		return "", fmt.Errorf("%s=%q: %w: %w", EnvDataDir, dir, ErrDataDirNotWritable, err)
	}
	if err := ensureLocalFilesystem(dir); err != nil {
		return "", fmt.Errorf("%s=%q: %w", EnvDataDir, dir, err)
	}
	return dir, nil
}

func ensureWritableDir(dir string) error {
	if err := os.MkdirAll(dir, dataDirPerm); err != nil {
		return err
	}
	probe, err := os.CreateTemp(dir, writeProbeName)
	if err != nil {
		return err
	}
	return errors.Join(probe.Close(), os.Remove(probe.Name()))
}

func parseBaseURL(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, nil
	}
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.Host == "" || !baseURLSchemes[u.Scheme] {
		return nil, fmt.Errorf("%s=%q: %w", EnvBaseURL, raw, ErrInvalidBaseURL)
	}
	return u, nil
}
