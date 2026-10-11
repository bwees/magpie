package jellyfin

import (
	"context"
	"errors"
	"fmt"
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
	testAPIKey     = "recorded-jellyfin-key"
	fixtureDir     = "testdata/health"
	fixture12      = "system_info_12.json"
	fixture1011    = "system_info_10_11.json"
	embyTokenParam = "X-Emby-Token"
	apiKeyParam    = "api_key"
)

// recordedJellyfin answers only the MediaBrowser Token header, like Jellyfin 12.
func recordedJellyfin(t *testing.T, fixture string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(embyTokenParam) != "" || r.URL.Query().Has(apiKeyParam) {
			t.Errorf("request used a legacy token: %v %v", r.Header, r.URL)
		}
		if r.Header.Get(authorizationHeader) != fmt.Sprintf(tokenAuthorization, testAPIKey) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != pathSystemInfo {
			w.WriteHeader(http.StatusNotFound)
			return
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

func newProbe(t *testing.T, conn settings.JellyfinConnection) *HealthProbe {
	t.Helper()
	store := settings.NewStore(dbtest.Open(t))
	if err := settings.Jellyfin.Set(context.Background(), store, conn); err != nil {
		t.Fatal(err)
	}
	return NewHealthProbe(store)
}

func TestProbeReportsJellyfin12(t *testing.T) {
	server := recordedJellyfin(t, fixture12)
	result, err := newProbe(t, settings.JellyfinConnection{URL: server.URL, APIKey: testAPIKey}).Probe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Version != "12.0.1" || result.Detail != "Living room" {
		t.Errorf("result = %+v", result)
	}
}

func TestProbeRejectsJellyfinOlderThan12(t *testing.T) {
	server := recordedJellyfin(t, fixture1011)
	_, err := newProbe(t, settings.JellyfinConnection{URL: server.URL, APIKey: testAPIKey}).Probe(context.Background())
	if err == nil || !strings.Contains(err.Error(), "10.11.2") {
		t.Errorf("err = %v, want a too-old error", err)
	}
}

func TestProbeReportsARejectedKey(t *testing.T) {
	server := recordedJellyfin(t, fixture12)
	_, err := newProbe(t, settings.JellyfinConnection{URL: server.URL, APIKey: "wrong"}).Probe(context.Background())
	if err == nil || !strings.Contains(err.Error(), "API key") {
		t.Errorf("err = %v, want a rejected-key error", err)
	}
}

func TestProbeWithoutCredentialsIsNotConfigured(t *testing.T) {
	if _, err := newProbe(t, settings.JellyfinConnection{}).Probe(context.Background()); !errors.Is(err, health.ErrNotConfigured) {
		t.Errorf("err = %v, want ErrNotConfigured", err)
	}
}
