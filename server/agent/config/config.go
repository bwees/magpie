// Package config reads the agent's environment and its persisted agent ID.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"magpie/internal/domain"
)

const (
	EnvURL        = "MAGPIE_URL"
	EnvSecret     = "MAGPIE_AGENT_SECRET"
	EnvName       = "MAGPIE_AGENT_NAME"
	EnvScratchDir = "MAGPIE_SCRATCH_DIR"
	// EnvCPU, set to true, offers the software encoders as a device, for testing without a GPU.
	EnvCPU = "MAGPIE_AGENT_CPU"
	// EnvWhisperModels is where whisper.cpp's models are kept; a missing one is downloaded on first use.
	EnvWhisperModels = "MAGPIE_WHISPER_MODELS"

	DefaultScratchDir = "/scratch"
	// defaultWhisperModels sits in the scratch dir, the agent's persistent volume.
	defaultWhisperModels = "whisper-models"

	// AgentIDFile sits in the scratch dir, which is the agent's persistent volume.
	AgentIDFile = ".magpie-agent-id"

	scratchDirPerm = 0o750
	agentIDPerm    = 0o600
)

var (
	ErrMissingURL    = errors.New(EnvURL + " is required")
	ErrInvalidURL    = errors.New(EnvURL + " must be an absolute http(s) URL")
	ErrMissingSecret = errors.New(EnvSecret + " is required")
	ErrInvalidName   = errors.New(EnvName + " may only contain a-z, 0-9 and -")
	ErrCorruptID     = errors.New("agent ID file does not hold a ULID")
	ErrInvalidCPU    = errors.New(EnvCPU + " must be true or false")
)

var (
	urlSchemes  = map[string]bool{"http": true, "https": true}
	namePattern = regexp.MustCompile(`^[a-z0-9-]+$`)
)

type Config struct {
	ServerURL  *url.URL
	Secret     string
	Name       string // shown in the UI; defaults to the hostname
	ScratchDir string
	AgentID    domain.AgentID
	// CPU offers the software encoders as a device.
	CPU              bool
	WhisperModelsDir string
}

// Load reads the environment, creates the scratch dir and loads or creates the agent ID in it.
func Load(getenv func(string) string, hostname func() (string, error)) (Config, error) {
	serverURL, urlErr := parseURL(getenv(EnvURL))
	secret := getenv(EnvSecret)
	var secretErr error
	if secret == "" {
		secretErr = ErrMissingSecret
	}
	name, nameErr := parseName(getenv(EnvName), hostname)
	var cpu bool
	var cpuErr error
	if raw := getenv(EnvCPU); raw != "" {
		if cpu, cpuErr = strconv.ParseBool(raw); cpuErr != nil {
			cpuErr = fmt.Errorf("%q: %w", raw, ErrInvalidCPU)
		}
	}
	if err := errors.Join(urlErr, secretErr, nameErr, cpuErr); err != nil {
		return Config{}, err
	}

	scratch := getenv(EnvScratchDir)
	if scratch == "" {
		scratch = DefaultScratchDir
	}
	if err := os.MkdirAll(scratch, scratchDirPerm); err != nil {
		return Config{}, fmt.Errorf("%s=%q: %w", EnvScratchDir, scratch, err)
	}
	id, err := LoadAgentID(scratch)
	if err != nil {
		return Config{}, err
	}
	models := getenv(EnvWhisperModels)
	if models == "" {
		models = filepath.Join(scratch, defaultWhisperModels)
	}
	return Config{ServerURL: serverURL, Secret: secret, Name: name, ScratchDir: scratch, AgentID: id, CPU: cpu, WhisperModelsDir: models}, nil
}

// LoadAgentID returns the ID stored in dir, creating one on first start so the agent keeps its identity
// across restarts.
func LoadAgentID(dir string) (domain.AgentID, error) {
	path := filepath.Join(dir, AgentIDFile)
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		id := domain.NewID[domain.AgentID]()
		if err := os.WriteFile(path, []byte(id), agentIDPerm); err != nil {
			return "", fmt.Errorf("saving agent ID: %w", err)
		}
		return id, nil
	}
	if err != nil {
		return "", fmt.Errorf("reading agent ID: %w", err)
	}
	id := domain.AgentID(strings.TrimSpace(string(raw)))
	if !domain.ValidID(id) {
		return "", fmt.Errorf("%s: %w", path, ErrCorruptID)
	}
	return id, nil
}

func parseURL(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, ErrMissingURL
	}
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.Host == "" || !urlSchemes[u.Scheme] {
		return nil, fmt.Errorf("%q: %w", raw, ErrInvalidURL)
	}
	return u, nil
}

func parseName(raw string, hostname func() (string, error)) (string, error) {
	if raw == "" {
		return hostname()
	}
	if !namePattern.MatchString(raw) {
		return "", fmt.Errorf("%q: %w", raw, ErrInvalidName)
	}
	return raw, nil
}
