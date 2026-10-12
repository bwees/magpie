//go:build smoke

// Package smoke checks a running Magpie stack from outside, the way an operator would. `mise run smoke`
// starts deploy/compose.smoke.yaml and runs it with MAGPIE_SMOKE_URL pointing at the server.
package smoke

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"os"
	"testing"
	"time"

	"magpie/internal/platform/httpserver"
	"magpie/internal/transcode/api/agentproto"
)

const (
	envURL = "MAGPIE_SMOKE_URL"

	setupAdminPath = httpserver.APIPrefix + "/identity/setup/admin"
	agentsPath     = httpserver.APIPrefix + "/transcode/agents"

	adminUsername = "smoke-admin"
	adminPassword = "smoke-admin-password"

	// The agent probes its encoders before it says hello; under emulation that takes a while.
	agentOnlineTimeout = 5 * time.Minute
	pollInterval       = 2 * time.Second
	requestTimeout     = 10 * time.Second
)

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type agentCapability struct {
	Capability agentproto.Capability `json:"capability"`
	OK         bool                  `json:"ok"`
	Error      string                `json:"error"`
}

type agentDevice struct {
	Vendor       agentproto.Vendor `json:"vendor"`
	Capabilities []agentCapability `json:"capabilities"`
}

type agent struct {
	Hostname string        `json:"hostname"`
	Online   bool          `json:"online"`
	Devices  []agentDevice `json:"devices"`
}

func TestStackServesHealthAndAgentComesOnline(t *testing.T) {
	baseURL := os.Getenv(envURL)
	if baseURL == "" {
		t.Fatalf("%s is not set", envURL)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, Timeout: requestTimeout}

	var health httpserver.HealthResponse
	if status := call(t, client, http.MethodGet, baseURL+httpserver.HealthPath, nil, &health); status != http.StatusOK {
		t.Fatalf("health status = %d", status)
	}
	if health.Status != httpserver.HealthStatusOK {
		t.Fatalf("health = %+v", health)
	}

	admin := credentials{Username: adminUsername, Password: adminPassword}
	if status := call(t, client, http.MethodPost, baseURL+setupAdminPath, admin, nil); status != http.StatusOK {
		t.Fatalf("setup admin status = %d", status)
	}

	deadline := time.Now().Add(agentOnlineTimeout)
	var agents []agent
	for time.Now().Before(deadline) {
		agents = nil
		if status := call(t, client, http.MethodGet, baseURL+agentsPath, nil, &agents); status != http.StatusOK {
			t.Fatalf("list agents status = %d", status)
		}
		if cpuEncoderPassed(agents) {
			return
		}
		time.Sleep(pollInterval)
	}
	t.Fatalf("no online agent with a working CPU HEVC encoder after %s; agents = %+v", agentOnlineTimeout, agents)
}

func cpuEncoderPassed(agents []agent) bool {
	for _, a := range agents {
		if !a.Online {
			continue
		}
		for _, device := range a.Devices {
			if device.Vendor != agentproto.VendorCPU {
				continue
			}
			for _, c := range device.Capabilities {
				if c.Capability == agentproto.CapEncHEVC10 && c.OK {
					return true
				}
			}
		}
	}
	return false
}

func call(t *testing.T, client *http.Client, method, url string, body, out any) int {
	t.Helper()
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequestWithContext(t.Context(), method, url, &payload)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if out != nil && resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatal(fmt.Errorf("%s %s: decoding: %w", method, url, err))
		}
	}
	return resp.StatusCode
}
