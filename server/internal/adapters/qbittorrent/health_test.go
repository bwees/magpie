package qbittorrent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"magpie/internal/platform/db/dbtest"
	"magpie/internal/platform/health"
	"magpie/internal/platform/settings"
)

const (
	testAPIKey      = "qbt_recorded"
	fixtureDir      = "testdata/health"
	fixtureCategory = "categories.json"
	fixtureVersion  = "app_version.txt"
	fixtureWebAPI52 = "webapi_version_5_2.txt"
	fixtureWebAPI50 = "webapi_version_5_0.txt"
)

type recordedQBittorrent struct {
	t             *testing.T
	webAPIFixture string
	mu            sync.Mutex
	created       []string
}

func (q *recordedQBittorrent) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if got := r.Header.Get(authorizationHeader); got != bearerPrefix+testAPIKey {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	switch r.URL.Path {
	case pathWebAPIVersion:
		q.serve(w, q.webAPIFixture)
	case pathAppVersion:
		q.serve(w, fixtureVersion)
	case pathCategories:
		q.serve(w, fixtureCategory)
	case pathCreateCategory:
		if err := r.ParseForm(); err != nil || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		q.mu.Lock()
		q.created = append(q.created, r.PostForm.Get(formCategory))
		q.mu.Unlock()
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (q *recordedQBittorrent) serve(w http.ResponseWriter, fixture string) {
	body, err := os.ReadFile(filepath.Join(fixtureDir, fixture))
	if err != nil {
		q.t.Error(err)
		return
	}
	_, _ = w.Write(body)
}

func newProbe(t *testing.T, conn settings.QBittorrentConnection) *HealthProbe {
	t.Helper()
	store := settings.NewStore(dbtest.Open(t))
	if err := settings.QBittorrent.Set(context.Background(), store, conn); err != nil {
		t.Fatal(err)
	}
	return NewHealthProbe(store)
}

func TestProbeReportsTheVersionAndCreatesTheCategory(t *testing.T) {
	recorded := &recordedQBittorrent{t: t, webAPIFixture: fixtureWebAPI52}
	server := httptest.NewServer(recorded)
	defer server.Close()
	probe := newProbe(t, settings.QBittorrentConnection{URL: server.URL, APIKey: testAPIKey, Category: settings.DefaultQBittorrentCategory})

	result, err := probe.Probe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Version != "5.2.1" || !strings.Contains(result.Detail, settings.DefaultQBittorrentCategory) {
		t.Errorf("result = %+v", result)
	}
	if len(recorded.created) != 1 || recorded.created[0] != settings.DefaultQBittorrentCategory {
		t.Errorf("created categories %v, want [%s]", recorded.created, settings.DefaultQBittorrentCategory)
	}
}

func TestProbeKeepsAnExistingCategory(t *testing.T) {
	recorded := &recordedQBittorrent{t: t, webAPIFixture: fixtureWebAPI52}
	server := httptest.NewServer(recorded)
	defer server.Close()
	probe := newProbe(t, settings.QBittorrentConnection{URL: server.URL, APIKey: testAPIKey, Category: "radarr"})

	if _, err := probe.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(recorded.created) != 0 {
		t.Errorf("created %v for a category that exists", recorded.created)
	}
}

func TestProbeRejectsWebAPIOlderThan215(t *testing.T) {
	server := httptest.NewServer(&recordedQBittorrent{t: t, webAPIFixture: fixtureWebAPI50})
	defer server.Close()
	probe := newProbe(t, settings.QBittorrentConnection{URL: server.URL, APIKey: testAPIKey, Category: settings.DefaultQBittorrentCategory})

	if _, err := probe.Probe(context.Background()); err == nil || !strings.Contains(err.Error(), "5.2") {
		t.Errorf("err = %v, want a too-old error naming 5.2", err)
	}
}

func TestProbeReportsARejectedKey(t *testing.T) {
	server := httptest.NewServer(&recordedQBittorrent{t: t, webAPIFixture: fixtureWebAPI52})
	defer server.Close()
	probe := newProbe(t, settings.QBittorrentConnection{URL: server.URL, APIKey: "wrong", Category: settings.DefaultQBittorrentCategory})

	if _, err := probe.Probe(context.Background()); err == nil || !strings.Contains(err.Error(), "API key") {
		t.Errorf("err = %v, want a rejected-key error", err)
	}
}

func TestProbeWithoutCredentialsIsNotConfigured(t *testing.T) {
	probe := newProbe(t, settings.QBittorrentConnection{Category: settings.DefaultQBittorrentCategory})
	if _, err := probe.Probe(context.Background()); !errors.Is(err, health.ErrNotConfigured) {
		t.Errorf("err = %v, want ErrNotConfigured", err)
	}
}
