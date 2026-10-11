package tmdb

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
	testAPIKey     = "recorded-tmdb-key"
	fixtureDir     = "testdata/health"
	fixtureValid   = "authentication_ok.json"
	fixtureInvalid = "authentication_invalid_key.json"
)

func recordedTMDB(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathAuthentication {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fixture := fixtureValid
		if r.URL.Query().Get(apiKeyParam) != testAPIKey {
			fixture = fixtureInvalid
			w.WriteHeader(http.StatusUnauthorized)
		}
		body, err := os.ReadFile(filepath.Join(fixtureDir, fixture))
		if err != nil {
			t.Error(err)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	return server
}

func newProbe(t *testing.T, key, baseURL string) *HealthProbe {
	t.Helper()
	store := settings.NewStore(dbtest.Open(t))
	if err := settings.TMDB.Set(context.Background(), store, settings.TMDBConnection{APIKey: key}); err != nil {
		t.Fatal(err)
	}
	probe := NewHealthProbe(store)
	probe.baseURL = baseURL
	return probe
}

func TestProbeAcceptsAValidKey(t *testing.T) {
	server := recordedTMDB(t)
	result, err := newProbe(t, testAPIKey, server.URL).Probe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Detail != keyValidDetail {
		t.Errorf("result = %+v", result)
	}
}

func TestProbeReportsAnInvalidKey(t *testing.T) {
	server := recordedTMDB(t)
	_, err := newProbe(t, "wrong", server.URL).Probe(context.Background())
	if err == nil || !strings.Contains(err.Error(), "Invalid API key") {
		t.Errorf("err = %v, want TMDB's invalid-key message", err)
	}
}

func TestProbeWithoutAKeyIsNotConfigured(t *testing.T) {
	if _, err := newProbe(t, "", defaultBaseURL).Probe(context.Background()); !errors.Is(err, health.ErrNotConfigured) {
		t.Errorf("err = %v, want ErrNotConfigured", err)
	}
}
