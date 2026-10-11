package connections

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"magpie/internal/domain"
	platformapi "magpie/internal/platform/api"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/config"
	"magpie/internal/platform/db/dbtest"
	"magpie/internal/platform/health"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/settings"

	"github.com/mikestefanello/backlite"
	"gorm.io/gorm"
)

const (
	pathConnections = "/api/platform/connections"
	qbURL           = "http://qbittorrent:8080"
	qbKey           = "qbt_secret"
	tmdbKey         = "tmdb-secret"
	probedVersion   = "5.2.1"
)

type recordingQueue struct {
	tasks []backlite.Task
}

func (q *recordingQueue) Enqueue(_ context.Context, task backlite.Task) error {
	q.tasks = append(q.tasks, task)
	return nil
}

func (q *recordingQueue) EnqueueTx(_ *gorm.DB, task backlite.Task) error {
	q.tasks = append(q.tasks, task)
	return nil
}

type stubProbe struct{ service platformapi.Service }

func (p stubProbe) Service() platformapi.Service { return p.service }

func (p stubProbe) Probe(context.Context) (health.Result, error) {
	return health.Result{Version: probedVersion}, nil
}

type fixture struct {
	handler http.Handler
	store   *settings.Store
	queue   *recordingQueue
}

func newFixture(t *testing.T, principal *domain.Principal) fixture {
	t.Helper()
	d := dbtest.Open(t)
	store := settings.NewStore(d)
	queue := &recordingQueue{}
	checker := health.NewChecker(health.CheckerParams{DB: d, Bus: bus.New(), Store: store, Probes: []health.Probe{stubProbe{platformapi.ServiceQBittorrent}}})
	mux := httpserver.Build(config.Env{}, httpserver.ModeServe, []httpserver.Controller{NewController(NewService(d, store, queue, checker))}).Mux
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if principal != nil {
			r = r.WithContext(domain.WithPrincipal(r.Context(), *principal))
		}
		mux.ServeHTTP(w, r)
	})
	return fixture{handler: handler, store: store, queue: queue}
}

func admin() *domain.Principal {
	return &domain.Principal{User: domain.NewID[domain.UserID](), Admin: true}
}

func (f fixture) do(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequestWithContext(context.Background(), method, path, &payload)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if err := json.NewDecoder(rec.Body).Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestSecretsAreMaskedAndTheMaskKeepsTheStoredSecret(t *testing.T) {
	f := newFixture(t, admin())
	ctx := context.Background()

	saved := decode[ConnectionsResponse](t, f.do(t, http.MethodPut, pathConnections, UpdateConnectionsRequest{
		TMDB:        &settings.TMDBConnection{APIKey: tmdbKey},
		QBittorrent: &settings.QBittorrentConnection{URL: qbURL + "/", APIKey: qbKey},
	}))
	if saved.TMDB.APIKey != SecretMask || saved.QBittorrent.APIKey != SecretMask {
		t.Errorf("response leaks secrets: %+v %+v", saved.TMDB, saved.QBittorrent)
	}
	if saved.QBittorrent.URL != qbURL || saved.QBittorrent.Category != settings.DefaultQBittorrentCategory {
		t.Errorf("qBittorrent = %+v, want trimmed URL and the default category", saved.QBittorrent)
	}
	if saved.Prowlarr.APIKey != "" {
		t.Errorf("unset secret reads %q, want empty", saved.Prowlarr.APIKey)
	}

	got := decode[ConnectionsResponse](t, f.do(t, http.MethodGet, pathConnections, nil))
	if got.TMDB.APIKey != SecretMask {
		t.Errorf("GET tmdb key = %q, want mask", got.TMDB.APIKey)
	}

	decode[ConnectionsResponse](t, f.do(t, http.MethodPut, pathConnections, UpdateConnectionsRequest{
		QBittorrent: &settings.QBittorrentConnection{URL: qbURL, APIKey: SecretMask, Category: settings.DefaultQBittorrentCategory},
	}))
	stored, err := settings.QBittorrent.Get(ctx, f.store)
	if err != nil {
		t.Fatal(err)
	}
	if stored.APIKey != qbKey {
		t.Errorf("stored key = %q after sending the mask, want %q", stored.APIKey, qbKey)
	}
	tmdb, err := settings.TMDB.Get(ctx, f.store)
	if err != nil || tmdb.APIKey != tmdbKey {
		t.Errorf("TMDB key = %q, %v; a qBittorrent-only update must not touch it", tmdb.APIKey, err)
	}
}

func TestSavingQueuesAHealthCheckPerChangedService(t *testing.T) {
	f := newFixture(t, admin())
	decode[ConnectionsResponse](t, f.do(t, http.MethodPut, pathConnections, UpdateConnectionsRequest{
		TMDB:     &settings.TMDBConnection{APIKey: tmdbKey},
		Jellyfin: &settings.JellyfinConnection{URL: "http://jellyfin:8096", APIKey: "jf"},
	}))

	want := []backlite.Task{health.CheckTask{Service: platformapi.ServiceTMDB}, health.CheckTask{Service: platformapi.ServiceJellyfin}}
	if !slices.Equal(f.queue.tasks, want) {
		t.Errorf("queued %+v, want %+v", f.queue.tasks, want)
	}
}

func TestInvalidConnectionsAreRejected(t *testing.T) {
	f := newFixture(t, admin())
	for name, update := range map[string]UpdateConnectionsRequest{
		"relative URL":     {Prowlarr: &settings.ProwlarrConnection{URL: "prowlarr:9696"}},
		"ftp URL":          {Jellyfin: &settings.JellyfinConnection{URL: "ftp://jellyfin"}},
		"category casing":  {QBittorrent: &settings.QBittorrentConnection{URL: qbURL, Category: "Magpie"}},
		"category symbols": {QBittorrent: &settings.QBittorrentConnection{URL: qbURL, Category: "tv_shows"}},
	} {
		if rec := f.do(t, http.MethodPut, pathConnections, update); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", name, rec.Code)
		}
	}
	if len(f.queue.tasks) != 0 {
		t.Errorf("rejected updates queued %d checks", len(f.queue.tasks))
	}
}

func TestTestProbesTheServiceAndReportsItsVersion(t *testing.T) {
	f := newFixture(t, admin())

	got := decode[ServiceHealthResponse](t, f.do(t, http.MethodPost, pathConnections+"/qbittorrent/test", nil))
	if got.Status != health.StatusHealthy || got.Version != probedVersion || got.CheckedAt == nil {
		t.Errorf("test = %+v", got)
	}
	if rec := f.do(t, http.MethodPost, pathConnections+"/sonarr/test", nil); rec.Code != http.StatusNotFound {
		t.Errorf("unknown service = %d, want 404", rec.Code)
	}

	listed := decode[ConnectionsResponse](t, f.do(t, http.MethodGet, pathConnections, nil))
	for _, h := range listed.Health {
		if h.Service == platformapi.ServiceQBittorrent && h.Status != health.StatusHealthy {
			t.Errorf("listed qBittorrent health = %s after a passing test", h.Status)
		}
	}
}

func TestConnectionsAreAdminOnly(t *testing.T) {
	requester := &domain.Principal{User: domain.NewID[domain.UserID]()}
	f := newFixture(t, requester)
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, pathConnections},
		{http.MethodPut, pathConnections},
		{http.MethodPost, pathConnections + "/tmdb/test"},
	} {
		if rec := f.do(t, route.method, route.path, UpdateConnectionsRequest{}); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s = %d, want 403", route.method, route.path, rec.Code)
		}
	}
}
