package acquisition_test

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"magpie/internal/acquisition"
	"magpie/internal/acquisition/fakes"
	"magpie/internal/domain"
	"magpie/internal/platform/config"
	"magpie/internal/platform/db/dbtest"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/settings"

	"github.com/mikestefanello/backlite"
	"gorm.io/gorm"
)

// recordingQueue keeps every task instead of running it. Searches enqueue from their own goroutines.
type recordingQueue struct {
	mu    sync.Mutex
	tasks []backlite.Task
	later []laterTask
}

type laterTask struct {
	task backlite.Task
	at   time.Time
}

func (q *recordingQueue) Enqueue(_ context.Context, task backlite.Task) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.tasks = append(q.tasks, task)
	return nil
}

func (q *recordingQueue) EnqueueTx(_ *gorm.DB, task backlite.Task) error {
	return q.Enqueue(context.Background(), task)
}

func (q *recordingQueue) EnqueueTxAt(_ *gorm.DB, task backlite.Task, at time.Time) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.tasks = append(q.tasks, task)
	q.later = append(q.later, laterTask{task: task, at: at})
	return nil
}

func (q *recordingQueue) queued() []backlite.Task {
	q.mu.Lock()
	defer q.mu.Unlock()
	return slices.Clone(q.tasks)
}

func (q *recordingQueue) laterTasks() []laterTask {
	q.mu.Lock()
	defer q.mu.Unlock()
	return slices.Clone(q.later)
}

// drop forgets the tasks that ran.
func (q *recordingQueue) drop(ran func(backlite.Task) bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.tasks = slices.DeleteFunc(q.tasks, ran)
}

func serveQuality(t *testing.T, principal *domain.Principal) (http.Handler, *recordingQueue) {
	t.Helper()
	d := dbtest.Open(t, acquisition.Migrations())
	repo := acquisition.NewQualityRepository(d)
	store := settings.NewStore(d)
	rescores := &recordingQueue{}
	quality := acquisition.NewQualityService(d, repo, store, rescores)
	if err := quality.SeedPresets(context.Background()); err != nil {
		t.Fatal(err)
	}
	queue := &recordingQueue{}
	trash := acquisition.NewTrashSyncService(d, repo, store, fakes.NewTrashGuides(), rescores)
	controller := acquisition.NewQualityController(quality, trash, queue)
	mux := httpserver.Build(config.Env{}, httpserver.ModeServe, []httpserver.Controller{controller}).Mux
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if principal != nil {
			r = r.WithContext(domain.WithPrincipal(r.Context(), *principal))
		}
		mux.ServeHTTP(w, r)
	}), queue
}

func TestQualityEndpointsAreAdminOnlyExceptTheProfileList(t *testing.T) {
	requester := &domain.Principal{User: domain.NewID[domain.UserID]()}
	handler, queue := serveQuality(t, requester)

	if rec := do(handler, http.MethodGet, "/api/acquisition/profiles", ""); rec.Code != http.StatusOK {
		t.Errorf("requester listing profiles = %d, want 200", rec.Code)
	}
	for _, r := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/acquisition/profiles/from-preset", `{"preset":"720P"}`},
		{http.MethodPost, "/api/acquisition/custom-formats/block-term", `{"term":"x"}`},
		{http.MethodGet, "/api/acquisition/custom-formats", ""},
		{http.MethodPost, "/api/acquisition/trash/sync", ""},
		{http.MethodGet, "/api/acquisition/settings", ""},
		{http.MethodPut, "/api/acquisition/settings", `{}`},
	} {
		if rec := do(handler, r.method, r.path, r.body); rec.Code != http.StatusForbidden {
			t.Errorf("requester %s %s = %d, want 403", r.method, r.path, rec.Code)
		}
	}
	if len(queue.tasks) != 0 {
		t.Error("a requester queued a TRaSH sync")
	}
}

func TestProfileEndpoints(t *testing.T) {
	handler, _ := serveQuality(t, admin())

	profiles := decode[[]acquisition.ProfileSummaryResponse](t, do(handler, http.MethodGet, "/api/acquisition/profiles", ""))
	if len(profiles) != 3 || profiles[0].Name != "1080p" {
		t.Fatalf("profiles = %+v, want the three presets", profiles)
	}
	cfg := decode[acquisition.AcquisitionSettingsResponse](t, do(handler, http.MethodGet, "/api/acquisition/settings", ""))
	if cfg.DefaultProfileID != profiles[0].ID || cfg.SettleWindowSeconds != 30 || cfg.DefaultDelayMinutes != 0 {
		t.Errorf("settings = %+v", cfg)
	}

	if rec := do(handler, http.MethodPost, "/api/acquisition/profiles/from-preset", `{"preset":"1080P"}`); rec.Code != http.StatusConflict {
		t.Errorf("duplicate preset = %d, want 409", rec.Code)
	} else if p := decode[problemBody](t, rec); p.Type != string(acquisition.TypeNameTaken) {
		t.Errorf("problem type = %q", p.Type)
	}

	body := `{"name":"Remux","upgradeAllowed":true,"cutoff":0,"propersMode":"PREFER_AND_UPGRADE","formatScores":[],
		"items":[{"qualityIds":[30],"allowed":true,"minMbPerMin":10,"preferredMbPerMin":100,"maxMbPerMin":200},
		         {"groupName":"WEB 1080p","qualityIds":[3,15],"allowed":true,"minMbPerMin":0,"preferredMbPerMin":0,"maxMbPerMin":0}]}`
	rec := do(handler, http.MethodPost, "/api/acquisition/profiles", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body)
	}
	created := decode[acquisition.ProfileResponse](t, rec)
	if len(created.Items) != 2 || created.Items[1].Group == nil || created.Items[1].Qualities[1].Name != "WEBRip-1080p" {
		t.Errorf("created = %+v", created)
	}

	if rec := do(handler, http.MethodPut, "/api/acquisition/profiles/"+string(created.ID), `{"name":"Remux","cutoff":5,"propersMode":"PREFER_AND_UPGRADE","items":[],"formatScores":[]}`); rec.Code != http.StatusBadRequest {
		t.Errorf("invalid update = %d, want 400", rec.Code)
	}
	if rec := do(handler, http.MethodGet, "/api/acquisition/profiles/nope", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown profile = %d, want 404", rec.Code)
	}
	if rec := do(handler, http.MethodDelete, "/api/acquisition/profiles/"+string(cfg.DefaultProfileID), ""); rec.Code != http.StatusConflict {
		t.Errorf("deleting the default = %d, want 409", rec.Code)
	}
	if rec := do(handler, http.MethodDelete, "/api/acquisition/profiles/"+string(created.ID), ""); rec.Code != http.StatusNoContent {
		t.Errorf("delete = %d, want 204", rec.Code)
	}
}

func TestSettingsBlockTermAndTrashEndpoints(t *testing.T) {
	handler, queue := serveQuality(t, admin())
	profiles := decode[[]acquisition.ProfileSummaryResponse](t, do(handler, http.MethodGet, "/api/acquisition/profiles", ""))

	put := func(delay, settle int) int {
		body := fmt.Sprintf(`{"defaultProfileId":%q,"defaultDelayMinutes":%d,"settleWindowSeconds":%d,"trashSyncEnabled":false}`, profiles[1].ID, delay, settle)
		return do(handler, http.MethodPut, "/api/acquisition/settings", body).Code
	}
	if code := put(1441, 30); code != http.StatusBadRequest {
		t.Errorf("delay 1441 = %d, want 400", code)
	}
	if code := put(120, 4); code != http.StatusBadRequest {
		t.Errorf("settle 4 s = %d, want 400", code)
	}
	if code := put(120, 60); code != http.StatusOK {
		t.Errorf("valid settings = %d", code)
	}

	rec := do(handler, http.MethodPost, "/api/acquisition/custom-formats/block-term", `{"term":"CAM"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("block term = %d %s", rec.Code, rec.Body)
	}
	blocked := decode[acquisition.CustomFormatResponse](t, rec)
	p := decode[acquisition.ProfileResponse](t, do(handler, http.MethodGet, "/api/acquisition/profiles/"+string(profiles[2].ID), ""))
	if len(p.FormatScores) != 1 || p.FormatScores[0].FormatID != blocked.ID || p.FormatScores[0].Score != acquisition.BlockTermScore {
		t.Errorf("scores = %+v", p.FormatScores)
	}

	status := decode[acquisition.TrashStatusResponse](t, do(handler, http.MethodPost, "/api/acquisition/trash/sync", ""))
	if status.Enabled || len(queue.tasks) != 1 {
		t.Errorf("sync now = %+v with %d tasks queued, want the setting off and one task", status, len(queue.tasks))
	}
	if guide := decode[[]acquisition.TrashProfileResponse](t, do(handler, http.MethodGet, "/api/acquisition/trash/profiles", "")); len(guide) != 0 {
		t.Errorf("guide profiles before a sync = %+v", guide)
	}
}
