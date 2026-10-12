package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const (
	testURL      = "http://magpie.lan:3000"
	testSecret   = "0123456789abcdef0123456789abcdef"
	testHostname = "gpu-box"
)

func envOf(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

func fixedHostname() (string, error) { return testHostname, nil }

func TestLoadKeepsTheAgentIDAcrossRestarts(t *testing.T) {
	scratch := t.TempDir()
	env := envOf(map[string]string{EnvURL: testURL, EnvSecret: testSecret, EnvScratchDir: scratch})

	first, err := Load(env, fixedHostname)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Load(env, fixedHostname)
	if err != nil {
		t.Fatal(err)
	}

	if first.AgentID == "" || first.AgentID != second.AgentID {
		t.Fatalf("agent IDs %q and %q, want one stable ID", first.AgentID, second.AgentID)
	}
	if first.Name != testHostname || first.ServerURL.String() != testURL || first.Secret != testSecret {
		t.Fatalf("config = %+v", first)
	}
}

func TestLoadReportsEveryMissingVariable(t *testing.T) {
	_, err := Load(envOf(map[string]string{EnvName: "Not Valid"}), fixedHostname)

	for _, want := range []error{ErrMissingURL, ErrMissingSecret, ErrInvalidName} {
		if !errors.Is(err, want) {
			t.Errorf("err = %v, want %v", err, want)
		}
	}
}

func TestLoadReadsTheCPUSwitch(t *testing.T) {
	vars := map[string]string{EnvURL: testURL, EnvSecret: testSecret, EnvScratchDir: t.TempDir(), EnvCPU: "true"}
	if cfg, err := Load(envOf(vars), fixedHostname); err != nil || !cfg.CPU {
		t.Fatalf("config = %+v, %v; want the CPU device on", cfg, err)
	}
	vars[EnvCPU] = "sometimes"
	if _, err := Load(envOf(vars), fixedHostname); !errors.Is(err, ErrInvalidCPU) {
		t.Fatalf("err = %v, want ErrInvalidCPU", err)
	}
}

func TestLoadRejectsNonHTTPURL(t *testing.T) {
	_, err := Load(envOf(map[string]string{EnvURL: "ftp://magpie.lan", EnvSecret: testSecret, EnvScratchDir: t.TempDir()}), fixedHostname)

	if !errors.Is(err, ErrInvalidURL) {
		t.Fatalf("err = %v, want ErrInvalidURL", err)
	}
}

func TestLoadAgentIDRejectsACorruptFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, AgentIDFile), []byte("garbage"), agentIDPerm); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadAgentID(dir); !errors.Is(err, ErrCorruptID) {
		t.Fatalf("err = %v, want ErrCorruptID", err)
	}
}
