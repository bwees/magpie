package acquisition_test

import (
	"net/http"
	"strings"
	"testing"

	"magpie/internal/acquisition"
	"magpie/internal/acquisition/api"
	catalogapi "magpie/internal/catalog/api"
	catalogtest "magpie/internal/catalog/api/contracttest"
	"magpie/internal/domain"
	"magpie/internal/platform/access"
	"magpie/internal/platform/config"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/problem"
)

func serveSearches(f searchFixture, principal *domain.Principal) http.Handler {
	controllers := []httpserver.Controller{acquisition.NewSearchController(f.searches, f.downloads, access.NewFileGate(nil)), acquisition.NewDownloadController(f.downloads, access.NewFileGate(nil))}
	mux := httpserver.Build(config.Env{}, httpserver.ModeServe, controllers).Mux
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if principal != nil {
			r = r.WithContext(domain.WithPrincipal(r.Context(), *principal))
		}
		mux.ServeHTTP(w, r)
	})
}

func startBody(title domain.TitleID) string {
	return `{"title":"` + string(title) + `","interactive":true}`
}

func TestSearchEndpointsAreAdminOnly(t *testing.T) {
	f := newSearchFixture(t)
	requester := &domain.Principal{User: domain.NewID[domain.UserID]()}
	routes := []struct{ method, path, body string }{
		{http.MethodPost, "/api/acquisition/searches", startBody(f.title.ID)},
		{http.MethodGet, "/api/acquisition/searches/s1", ""},
		{http.MethodDelete, "/api/acquisition/searches/s1", ""},
		{http.MethodPost, "/api/acquisition/searches/s1/grab", `{"releaseId":"r1","override":true}`},
		{http.MethodGet, "/api/acquisition/downloads", ""},
		{http.MethodDelete, "/api/acquisition/downloads/d1", ""},
		{http.MethodGet, "/api/acquisition/blocklist", ""},
		{http.MethodDelete, "/api/acquisition/blocklist/b1", ""},
		{http.MethodGet, "/api/acquisition/download-options", ""},
		{http.MethodPut, "/api/acquisition/download-options", `{"blockedFilesEnabled":true,"blockedPatterns":[],"metadataTimeoutMinutes":5,"stalledLimitMinutes":360}`},
	}
	for name, tc := range map[string]struct {
		principal *domain.Principal
		want      int
	}{"anonymous": {nil, http.StatusUnauthorized}, "requester": {requester, http.StatusForbidden}} {
		handler := serveSearches(f, tc.principal)
		for _, r := range routes {
			if rec := do(handler, r.method, r.path, r.body); rec.Code != tc.want {
				t.Errorf("%s %s %s = %d, want %d", name, r.method, r.path, rec.Code, tc.want)
			}
		}
	}
	if len(f.events.ofType(api.TypeSearchStarted)) != 0 {
		t.Error("a non-admin started a search")
	}
}

func TestStartSearchReturnsTheSearchWithBucketsAndPendingIndexers(t *testing.T) {
	f := newSearchFixture(t)
	f.indexer(t, stuckID, "Stuck", idCaps, neverAnswers)
	handler := serveSearches(f, admin())

	rec := do(handler, http.MethodPost, "/api/acquisition/searches", startBody(f.title.ID))
	contentType := rec.Result().Header.Get("Content-Type")
	if rec.Code != http.StatusCreated || !strings.Contains(contentType, "json") {
		t.Fatalf("start = %d %s %s", rec.Code, contentType, rec.Body)
	}
	got := decode[acquisition.SearchResponse](t, rec)
	if got.SearchID == "" || got.Trigger != api.TriggerInteractive || got.Completed {
		t.Errorf("search = %+v", got)
	}
	if len(got.Indexers) != 1 || got.Indexers[0].Status != acquisition.IndexerPending {
		t.Errorf("indexers = %+v, want one PENDING", got.Indexers)
	}
	if got.Profile.Name != "1080p" || len(got.Profile.Buckets) != 1 || !got.Profile.Buckets[0].MeetsCutoff || got.Profile.Cutoff != "1080p" {
		t.Errorf("profile = %+v, want the 1080p preset's one cutoff bucket", got.Profile)
	}

	rec = do(handler, http.MethodGet, "/api/acquisition/searches/"+string(got.SearchID), "")
	if rec.Code != http.StatusOK || decode[acquisition.SearchResponse](t, rec).SearchID != got.SearchID {
		t.Errorf("get = %d", rec.Code)
	}
	if rec := do(handler, http.MethodDelete, "/api/acquisition/searches/"+string(got.SearchID), ""); rec.Code != http.StatusNoContent {
		t.Errorf("cancel = %d %s", rec.Code, rec.Body)
	}
}

func TestSearchEndpointProblems(t *testing.T) {
	f := newSearchFixture(t)
	f.indexer(t, fastID, "Fast", idCaps, 0, release(acceptedName), release(rejectedName))
	handler := serveSearches(f, admin())
	started := f.start(t)
	f.completed(t)
	snapshot, err := f.searches.Search(started.ID)
	if err != nil {
		t.Fatal(err)
	}
	var accepted, rejected domain.ReleaseID
	for _, r := range snapshot.Releases {
		if len(r.Rejections) == 0 {
			accepted = r.Release
		} else {
			rejected = r.Release
		}
	}
	grab := "/api/acquisition/searches/" + string(started.ID) + "/grab"

	cases := []struct {
		name, method, path, body string
		status                   int
		problemType              problem.Type
	}{
		{"automatic search of an unknown title", http.MethodPost, "/api/acquisition/searches", `{"title":"missing","interactive":false}`, http.StatusNotFound, acquisition.TypeTitleNotFound},
		{"unknown title", http.MethodPost, "/api/acquisition/searches", startBody("missing"), http.StatusNotFound, acquisition.TypeTitleNotFound},
		{"unknown episode", http.MethodPost, "/api/acquisition/searches", `{"title":"` + string(f.title.ID) + `","episodes":["missing"],"interactive":true}`, http.StatusBadRequest, problem.TypeBadRequest},
		{"unknown search", http.MethodGet, "/api/acquisition/searches/missing", "", http.StatusNotFound, acquisition.TypeSearchNotFound},
		{"rejected without override", http.MethodPost, grab, `{"releaseId":"` + string(rejected) + `","override":false}`, http.StatusConflict, acquisition.TypeReleaseRejected},
		{"unknown release", http.MethodPost, grab, `{"releaseId":"missing","override":true}`, http.StatusNotFound, acquisition.TypeReleaseNotFound},
		{"qBittorrent too old", http.MethodPost, grab, `{"releaseId":"` + string(accepted) + `","override":false}`, http.StatusBadGateway, acquisition.TypeQBittorrentUnsupported},
		{"unknown download", http.MethodDelete, "/api/acquisition/downloads/missing", "", http.StatusNotFound, acquisition.TypeDownloadNotFound},
		{"unknown blocklist entry", http.MethodDelete, "/api/acquisition/blocklist/missing", "", http.StatusNotFound, acquisition.TypeBlocklistEntryNotFound},
		{"bad pattern", http.MethodPut, "/api/acquisition/download-options", `{"blockedFilesEnabled":true,"blockedPatterns":["regex:("],"metadataTimeoutMinutes":5,"stalledLimitMinutes":360}`, http.StatusBadRequest, problem.TypeBadRequest},
		{"metadata timeout out of range", http.MethodPut, "/api/acquisition/download-options", `{"blockedFilesEnabled":true,"blockedPatterns":[],"metadataTimeoutMinutes":0,"stalledLimitMinutes":360}`, http.StatusBadRequest, problem.TypeBadRequest},
	}
	f.qbit.SetVersion(acquisition.WebAPIVersion{Major: 2, Minor: 11})
	for _, tc := range cases {
		rec := do(handler, tc.method, tc.path, tc.body)
		if rec.Code != tc.status {
			t.Errorf("%s = %d %s, want %d", tc.name, rec.Code, rec.Body, tc.status)
			continue
		}
		if got := decode[problemBody](t, rec); got.Type != string(tc.problemType) {
			t.Errorf("%s type = %q, want %q", tc.name, got.Type, tc.problemType)
		}
	}
}

func TestATitleRemovedWhileASearchStartsIsATitleNotFound(t *testing.T) {
	f := newSearchFixture(t)
	f.catalog.FailNext(catalogtest.ReaderWantedLanguages, catalogapi.ErrNotFound)

	rec := do(serveSearches(f, admin()), http.MethodPost, "/api/acquisition/searches", startBody(f.title.ID))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("start = %d %s, want 404", rec.Code, rec.Body)
	}
	if got := decode[problemBody](t, rec); got.Type != string(acquisition.TypeTitleNotFound) {
		t.Errorf("type = %q, want %q", got.Type, acquisition.TypeTitleNotFound)
	}
}
