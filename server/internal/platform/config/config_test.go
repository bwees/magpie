package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var validAgentSecret = strings.Repeat("s", MinAgentSecretLength)

// envOf answers from vars, with a valid agent secret unless vars sets one.
func envOf(vars map[string]string) func(string) string {
	return func(key string) string {
		if value, ok := vars[key]; ok {
			return value
		}
		if key == EnvAgentSecret {
			return validAgentSecret
		}
		return ""
	}
}

func TestParseAppliesDefaults(t *testing.T) {
	dataDir := t.TempDir()

	env, err := Parse(envOf(map[string]string{EnvDataDir: dataDir}))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if env.Port != DefaultPort {
		t.Errorf("port = %d, want %d", env.Port, DefaultPort)
	}
	if env.BaseURL != nil {
		t.Errorf("base URL = %v, want nil", env.BaseURL)
	}
	if env.DataDir != dataDir {
		t.Errorf("data dir = %q, want %q", env.DataDir, dataDir)
	}
}

func TestParseReadsValidValues(t *testing.T) {
	env, err := Parse(envOf(map[string]string{
		EnvPort:    "8080",
		EnvDataDir: t.TempDir(),
		EnvBaseURL: "https://magpie.example.com/base",
	}))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if env.Port != 8080 {
		t.Errorf("port = %d, want 8080", env.Port)
	}
	if got := env.BaseURL.String(); got != "https://magpie.example.com/base" {
		t.Errorf("base URL = %q", got)
	}
}

func TestParseCreatesMissingDataDir(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "nested", "data")

	env, err := Parse(envOf(map[string]string{EnvDataDir: dataDir}))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if env.DataDir != dataDir {
		t.Errorf("data dir = %q, want %q", env.DataDir, dataDir)
	}
}

func TestParseAgentSecret(t *testing.T) {
	env, err := Parse(envOf(map[string]string{EnvAgentSecret: validAgentSecret, EnvDataDir: t.TempDir()}))
	if err != nil || env.AgentSecret != validAgentSecret {
		t.Fatalf("env = %+v, err = %v; want the secret", env, err)
	}

	_, err = Parse(envOf(map[string]string{EnvAgentSecret: validAgentSecret[1:], EnvDataDir: t.TempDir()}))
	if !errors.Is(err, ErrAgentSecretShort) {
		t.Fatalf("err = %v, want ErrAgentSecretShort", err)
	}

	_, err = Parse(envOf(map[string]string{EnvAgentSecret: "", EnvDataDir: t.TempDir()}))
	if !errors.Is(err, ErrAgentSecretMissing) {
		t.Fatalf("err = %v, want ErrAgentSecretMissing", err)
	}
}

func TestParseRejectsInvalidPort(t *testing.T) {
	for _, port := range []string{"0", "65536", "-1", "http"} {
		t.Run(port, func(t *testing.T) {
			_, err := Parse(envOf(map[string]string{EnvPort: port, EnvDataDir: t.TempDir()}))
			if !errors.Is(err, ErrInvalidPort) {
				t.Fatalf("err = %v, want ErrInvalidPort", err)
			}
		})
	}
}

func TestParseRejectsInvalidBaseURL(t *testing.T) {
	for _, baseURL := range []string{"magpie.local", "/relative", "ftp://magpie.local", "https://"} {
		t.Run(baseURL, func(t *testing.T) {
			_, err := Parse(envOf(map[string]string{EnvBaseURL: baseURL, EnvDataDir: t.TempDir()}))
			if !errors.Is(err, ErrInvalidBaseURL) {
				t.Fatalf("err = %v, want ErrInvalidBaseURL", err)
			}
		})
	}
}

func TestParseRejectsDataDirThatIsAFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Parse(envOf(map[string]string{EnvDataDir: file}))
	if !errors.Is(err, ErrDataDirNotWritable) {
		t.Fatalf("err = %v, want ErrDataDirNotWritable", err)
	}
}

func TestParseRefusesNetworkFilesystem(t *testing.T) {
	detectFilesystem = func(string) (string, bool, error) { return "nfs", true, nil }
	t.Cleanup(func() { detectFilesystem = filesystemType })

	_, err := Parse(envOf(map[string]string{EnvDataDir: t.TempDir()}))
	if !errors.Is(err, ErrNetworkFilesystem) {
		t.Fatalf("err = %v, want ErrNetworkFilesystem", err)
	}
}

func TestTempDirIsLocalFilesystem(t *testing.T) {
	fsType, network, err := filesystemType(t.TempDir())
	if err != nil {
		t.Fatalf("filesystem type: %v", err)
	}
	if network {
		t.Fatalf("temp dir reported as network filesystem %q", fsType)
	}
}

func TestParseReportsEveryInvalidVariable(t *testing.T) {
	_, err := Parse(envOf(map[string]string{
		EnvPort:    "nope",
		EnvDataDir: t.TempDir(),
		EnvBaseURL: "nope",
	}))
	if !errors.Is(err, ErrInvalidPort) || !errors.Is(err, ErrInvalidBaseURL) {
		t.Fatalf("err = %v, want both port and base URL errors", err)
	}
}
