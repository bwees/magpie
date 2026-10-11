package acquisition_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"magpie/internal/acquisition"
	"magpie/internal/acquisition/fakes"
	"magpie/internal/domain"
	"magpie/internal/platform/config"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/problem"
)

const prowlarrURL = "http://prowlarr:9696"

func serve(t *testing.T, f fixture, principal *domain.Principal) http.Handler {
	t.Helper()
	mux := httpserver.Build(config.Env{}, httpserver.ModeServe, []httpserver.Controller{acquisition.NewController(f.service)}).Mux
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if principal != nil {
			r = r.WithContext(domain.WithPrincipal(r.Context(), *principal))
		}
		mux.ServeHTTP(w, r)
	})
}

func do(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)
	return rec
}

func admin() *domain.Principal {
	return &domain.Principal{User: domain.NewID[domain.UserID](), Admin: true}
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(rec.Body).Decode(&v); err != nil {
		t.Fatalf("decoding %d response: %v", rec.Code, err)
	}
	return v
}

func TestIndexerEndpointsAreAdminOnly(t *testing.T) {
	f := newFixture(t)
	requester := &domain.Principal{User: domain.NewID[domain.UserID]()}
	routes := []struct{ method, path, body string }{
		{http.MethodGet, "/api/acquisition/indexers", ""},
		{http.MethodPut, "/api/acquisition/indexers/1", `{"priority":10,"minSeeders":1,"seedingOverride":"USE_GLOBAL"}`},
		{http.MethodPost, "/api/acquisition/indexers/sync", ""},
		{http.MethodGet, "/api/acquisition/indexer-options", ""},
		{http.MethodPut, "/api/acquisition/indexer-options", `{"syncIntervalMinutes":60,"searchTimeoutSeconds":60,"resultCacheMinutes":30}`},
	}
	for name, tc := range map[string]struct {
		principal *domain.Principal
		want      int
	}{"anonymous": {nil, http.StatusUnauthorized}, "requester": {requester, http.StatusForbidden}} {
		handler := serve(t, f, tc.principal)
		for _, r := range routes {
			if rec := do(handler, r.method, r.path, r.body); rec.Code != tc.want {
				t.Errorf("%s %s %s = %d, want %d", name, r.method, r.path, rec.Code, tc.want)
			}
		}
	}
	if f.prowlarr.Called(fakes.ProwlarrIndexers) != 0 {
		t.Error("a non-admin triggered a sync")
	}
}

func TestSyncNowListsAndUpdates(t *testing.T) {
	f := newFixture(t)
	f.prowlarr.SetWebURL(prowlarrURL)
	f.prowlarr.AddIndexer(healthy(nyaa, "Nyaa", tvCaps))
	handler := serve(t, f, admin())

	rec := do(handler, http.MethodPost, "/api/acquisition/indexers/sync", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("sync = %d: %s", rec.Code, rec.Body)
	}
	list := decode[acquisition.IndexerListResponse](t, rec)
	if list.ProwlarrURL != prowlarrURL || list.LastSyncedAt == nil || list.SyncError != "" || len(list.Indexers) != 1 {
		t.Fatalf("list = %+v", list)
	}
	ix := list.Indexers[0]
	if ix.ID != nyaa || ix.Priority != 25 || ix.SeedingOverride != acquisition.OverrideUseGlobal || ix.Status != acquisition.IndexerOK || ix.AverageResponseMs != 420 {
		t.Errorf("indexer = %+v", ix)
	}

	rec = do(handler, http.MethodPut, "/api/acquisition/indexers/1", `{"priority":10,"minSeeders":5,"seedingOverride":"SEED_UNTIL_GOALS","seedRatio":1.5,"seedDays":7}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update = %d: %s", rec.Code, rec.Body)
	}
	updated := decode[acquisition.IndexerResponse](t, rec)
	if updated.Priority != 10 || updated.MinSeeders != 5 || *updated.SeedRatio != 1.5 || *updated.SeedDays != 7 {
		t.Errorf("updated = %+v", updated)
	}

	rec = do(handler, http.MethodPut, "/api/acquisition/indexers/1", `{"priority":51,"minSeeders":1,"seedingOverride":"USE_GLOBAL"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("priority 51 = %d, want 400", rec.Code)
	}
	rec = do(handler, http.MethodPut, "/api/acquisition/indexers/99", `{"priority":10,"minSeeders":1,"seedingOverride":"USE_GLOBAL"}`)
	if p := decode[problemBody](t, rec); rec.Code != http.StatusNotFound || p.Type != string(acquisition.TypeIndexerNotFound) {
		t.Errorf("unknown indexer = %d %+v, want 404", rec.Code, p)
	}
}

type problemBody struct {
	Type   string `json:"type"`
	Detail string `json:"detail"`
}

func TestSyncNowReportsProwlarrErrors(t *testing.T) {
	cases := map[string]struct {
		err  error
		want problem.Type
	}{
		"unreachable": {acquisition.ErrUnavailable, acquisition.TypeProwlarrUnavailable},
		"bad API key": {acquisition.ErrUnauthorized, acquisition.TypeProwlarrUnauthorized},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.prowlarr.FailNext(fakes.ProwlarrIndexers, tc.err)
			handler := serve(t, f, admin())

			rec := do(handler, http.MethodPost, "/api/acquisition/indexers/sync", "")
			if p := decode[problemBody](t, rec); rec.Code != http.StatusBadGateway || p.Type != string(tc.want) {
				t.Errorf("sync = %d %+v, want 502 %s", rec.Code, p, tc.want)
			}

			list := decode[acquisition.IndexerListResponse](t, do(handler, http.MethodGet, "/api/acquisition/indexers", ""))
			if list.SyncError == "" || len(list.Indexers) != 0 {
				t.Errorf("list after failed sync = %+v, want the error and no indexers", list)
			}
		})
	}
}

func TestIndexerOptionsEndpoints(t *testing.T) {
	f := newFixture(t)
	handler := serve(t, f, admin())

	got := decode[acquisition.IndexerOptionsResponse](t, do(handler, http.MethodGet, "/api/acquisition/indexer-options", ""))
	if got != (acquisition.IndexerOptionsResponse{SyncIntervalMinutes: 60, SearchTimeoutSeconds: 60, ResultCacheMinutes: 30}) {
		t.Errorf("defaults = %+v", got)
	}

	rec := do(handler, http.MethodPut, "/api/acquisition/indexer-options", `{"syncIntervalMinutes":120,"searchTimeoutSeconds":90,"resultCacheMinutes":0}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("set = %d: %s", rec.Code, rec.Body)
	}
	got = decode[acquisition.IndexerOptionsResponse](t, do(handler, http.MethodGet, "/api/acquisition/indexer-options", ""))
	if got != (acquisition.IndexerOptionsResponse{SyncIntervalMinutes: 120, SearchTimeoutSeconds: 90, ResultCacheMinutes: 0}) {
		t.Errorf("stored = %+v", got)
	}

	rec = do(handler, http.MethodPut, "/api/acquisition/indexer-options", `{"syncIntervalMinutes":60,"searchTimeoutSeconds":5,"resultCacheMinutes":30}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("timeout 5 s = %d, want 400", rec.Code)
	}
}
