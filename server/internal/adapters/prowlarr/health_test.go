package prowlarr

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"magpie/internal/platform/db/dbtest"
	"magpie/internal/platform/health"
	"magpie/internal/platform/settings"
)

const (
	healthFixtureDir = "testdata/health"
	fixtureStatus    = "system_status.json"
	fixtureIndexer   = "indexers.json"
)

func recordedProwlarr(t *testing.T) *httptest.Server {
	t.Helper()
	fixtures := map[string]string{pathSystemStatus: fixtureStatus, healthPathIndexers: fixtureIndexer}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(apiKeyHeader) != testAPIKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fixture, ok := fixtures[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, err := os.ReadFile(filepath.Join(healthFixtureDir, fixture))
		if err != nil {
			t.Error(err)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	return server
}

func newProbe(t *testing.T, conn settings.ProwlarrConnection) *HealthProbe {
	t.Helper()
	store := settings.NewStore(dbtest.Open(t))
	if err := settings.Prowlarr.Set(context.Background(), store, conn); err != nil {
		t.Fatal(err)
	}
	return NewHealthProbe(store)
}

func TestProbeListsIndexers(t *testing.T) {
	server := recordedProwlarr(t)
	result, err := newProbe(t, settings.ProwlarrConnection{URL: server.URL, APIKey: testAPIKey}).Probe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Version != "1.30.2.4939" || result.Detail != "2 indexers found" {
		t.Errorf("result = %+v", result)
	}
}

func TestProbeReportsARejectedKey(t *testing.T) {
	server := recordedProwlarr(t)
	_, err := newProbe(t, settings.ProwlarrConnection{URL: server.URL, APIKey: "wrong"}).Probe(context.Background())
	if err == nil || !strings.Contains(err.Error(), "API key") {
		t.Errorf("err = %v, want a rejected-key error", err)
	}
}

func TestProbeWithoutCredentialsIsNotConfigured(t *testing.T) {
	if _, err := newProbe(t, settings.ProwlarrConnection{}).Probe(context.Background()); !errors.Is(err, health.ErrNotConfigured) {
		t.Errorf("err = %v, want ErrNotConfigured", err)
	}
}
