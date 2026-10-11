package activity

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/activity/api"
	"magpie/internal/domain"
	"magpie/internal/platform/config"
	"magpie/internal/platform/httpserver"
	requestsapi "magpie/internal/requests/api"
)

func (h *harness) serve(principal *domain.Principal) http.Handler {
	mux := httpserver.Build(config.Env{}, httpserver.ModeServe, []httpserver.Controller{NewController(h.service)}).Mux
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if principal != nil {
			r = r.WithContext(domain.WithPrincipal(r.Context(), *principal))
		}
		mux.ServeHTTP(w, r)
	})
}

func get[T any](t *testing.T, handler http.Handler, path string, wantStatus int) T {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	var body T
	if rec.Code != wantStatus {
		t.Fatalf("GET %s = %d %s, want %d", path, rec.Code, rec.Body, wantStatus)
	}
	if wantStatus == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
	}
	return body
}

func requesterOf(h *harness, title domain.TitleID) *domain.Principal {
	user := domain.NewID[domain.UserID]()
	h.requests.PutRequest(requestsapi.Request{ID: domain.NewID[domain.RequestID](), User: user, Title: &title, Status: requestsapi.StatusApproved})
	return &domain.Principal{User: user}
}

func TestRequestersSeeOnlyTitlesTheyRequested(t *testing.T) {
	h := newHarness(t)
	mine, theirs := h.movie(), h.movie()
	for _, title := range []domain.TitleID{mine, theirs} {
		h.deliver(titleAdded(title, domain.MediaKindMovie))
		h.deliver(acquisitionapi.ReleaseGrabbed{Download: domain.NewID[domain.DownloadID](), Title: title, Release: releaseName})
	}
	me := h.serve(requesterOf(h, mine))
	requesterOf(h, theirs)

	if rows := get[[]PipelineRowResponse](t, me, "/api/activity/pipeline?title="+string(mine), http.StatusOK); len(rows) != 1 {
		t.Errorf("own pipeline has %d rows, want 1", len(rows))
	}
	get[any](t, me, "/api/activity/pipeline?title="+string(theirs), http.StatusNotFound)
	get[any](t, me, "/api/activity/timeline?title="+string(theirs), http.StatusNotFound)

	stages := get[[]PipelineRowResponse](t, me, fmt.Sprintf("/api/activity/pipeline?titles=%s&titles=%s", mine, theirs), http.StatusOK)
	if len(stages) != 1 || stages[0].TitleID != string(mine) {
		t.Errorf("stages = %+v, want only the requested title", stages)
	}
	active := get[[]PipelineRowResponse](t, me, "/api/activity/pipeline?active=true", http.StatusOK)
	if len(active) != 1 || active[0].Stage != string(api.StageDownloading) {
		t.Errorf("active = %+v, want the requested title downloading", active)
	}

	timeline := get[[]TimelineEntryResponse](t, me, "/api/activity/timeline?title="+string(mine), http.StatusOK)
	if len(timeline) != 2 || timeline[0].Detail != "" {
		t.Errorf("timeline = %+v, want 2 entries without release names", timeline)
	}
}

func TestAdminsSeeEverythingAndOnlyAdminsSeeHistoryAndAttention(t *testing.T) {
	h := newHarness(t)
	title := h.movie()
	h.deliver(titleAdded(title, domain.MediaKindMovie))
	h.deliver(acquisitionapi.ReleaseGrabbed{Download: domain.NewID[domain.DownloadID](), Title: title, Release: releaseName})
	admin := h.serve(&domain.Principal{User: domain.NewID[domain.UserID](), Admin: true})
	requester := h.serve(requesterOf(h, title))

	timeline := get[[]TimelineEntryResponse](t, admin, "/api/activity/timeline?title="+string(title), http.StatusOK)
	if len(timeline) != 2 || timeline[0].Detail != releaseName {
		t.Errorf("timeline = %+v, want the release name on the grab", timeline)
	}
	history := get[[]HistoryEntryResponse](t, admin, "/api/activity/history", http.StatusOK)
	if len(history) != 2 || history[0].TitleName != movieName {
		t.Errorf("history = %+v, want 2 entries naming the title", history)
	}
	get[[]AttentionItemResponse](t, admin, "/api/activity/attention", http.StatusOK)

	get[any](t, requester, "/api/activity/history", http.StatusForbidden)
	get[any](t, requester, "/api/activity/attention", http.StatusForbidden)
	get[any](t, h.serve(nil), "/api/activity/timeline?title="+string(title), http.StatusUnauthorized)
}

func TestPageLimitIsCapped(t *testing.T) {
	h := newHarness(t)
	title := h.movie()
	h.deliver(titleAdded(title, domain.MediaKindMovie))
	for range api.MaxPageLimit + 1 {
		h.deliver(acquisitionapi.SearchStarted{Search: domain.NewID[domain.SearchID](), Title: title})
	}
	admin := h.serve(&domain.Principal{Admin: true})

	for limit, want := range map[string]int{"": api.DefaultPageLimit, "500": api.MaxPageLimit, "3": 3} {
		path := "/api/activity/timeline?" + url.Values{"title": {string(title)}, "limit": {limit}}.Encode()
		if got := get[[]TimelineEntryResponse](t, admin, path, http.StatusOK); len(got) != want {
			t.Errorf("limit %q returned %d entries, want %d", limit, len(got), want)
		}
	}
}

func TestBadQueriesAreRejected(t *testing.T) {
	h := newHarness(t)
	admin := h.serve(&domain.Principal{Admin: true})
	for _, path := range []string{
		"/api/activity/pipeline",
		"/api/activity/pipeline?title=a&active=true",
		"/api/activity/timeline",
		"/api/activity/timeline?title=a&before=yesterday",
		"/api/activity/history?severity=LOUD",
	} {
		get[any](t, admin, path, http.StatusBadRequest)
	}
}
