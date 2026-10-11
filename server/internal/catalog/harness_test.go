package catalog_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"magpie/internal/catalog"
	"magpie/internal/catalog/fakes"
	"magpie/internal/domain"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/config"
	"magpie/internal/platform/db"
	"magpie/internal/platform/db/dbtest"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/settings"

	"github.com/go-fuego/fuego"
	"github.com/mikestefanello/backlite"
	"gorm.io/gorm"
)

const (
	posterPath   = "/poster.jpg"
	backdropPath = "/backdrop.jpg"
	posterWidth  = 400
	posterHeight = 600
)

var startOfTest = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

// testClock is read by the service and moved by the test between steps.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type recordedTask struct {
	task backlite.Task
	wait time.Duration
}

// taskRecorder stands in for backlite: tasks are kept until runJobs processes them.
type taskRecorder struct {
	mu    sync.Mutex
	tasks []recordedTask
}

func (r *taskRecorder) add(task backlite.Task, wait time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks = append(r.tasks, recordedTask{task: task, wait: wait})
}

func (r *taskRecorder) Enqueue(_ context.Context, task backlite.Task) error {
	r.add(task, 0)
	return nil
}

func (r *taskRecorder) EnqueueTx(tx *gorm.DB, task backlite.Task) error {
	return db.AfterCommit(tx, func() { r.add(task, 0) })
}

func (r *taskRecorder) later(_ context.Context, task backlite.Task, wait time.Duration) error {
	r.add(task, wait)
	return nil
}

func (r *taskRecorder) next() (recordedTask, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.tasks) == 0 {
		return recordedTask{}, false
	}
	next := r.tasks[0]
	r.tasks = r.tasks[1:]
	return next, true
}

type harness struct {
	t       *testing.T
	db      *db.DB
	tmdb    *fakes.TMDB
	service *catalog.Service
	repo    *catalog.Repository
	store   *settings.Store
	clock   *testClock
	tasks   *taskRecorder
	queues  map[string]backlite.Queue
	waits   []time.Duration
	server  *fuego.Server
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	d := dbtest.Open(t, catalog.Migrations())
	h := &harness{
		t:     t,
		db:    d,
		tmdb:  fakes.NewTMDB(),
		repo:  catalog.NewRepository(d),
		store: settings.NewStore(d),
		clock: &testClock{now: startOfTest},
		tasks: &taskRecorder{},
	}
	images := catalog.NewImageCache(config.Env{DataDir: t.TempDir()}, h.tmdb)
	h.service = catalog.NewService(h.repo, d, bus.New(), h.tmdb, h.store, images)
	h.service.SetClock(h.clock.Now)
	h.queues = map[string]backlite.Queue{}
	for _, q := range []backlite.Queue{
		catalog.AddTitleQueue(h.service, h.tasks.later),
		catalog.NewRefreshTitleQueue(h.service),
		catalog.NewMovieRefreshQueue(h.service),
		catalog.NewReleaseClockQueue(h.service),
	} {
		h.queues[q.Config().Name] = q
	}
	h.server = httpserver.Build(config.Env{}, httpserver.ModeServe, []httpserver.Controller{catalog.NewController(h.service, h.repo, h.tasks)})
	h.tmdb.PutImage(posterPath, testJPEG(t, posterWidth, posterHeight))
	h.tmdb.PutImage(backdropPath, testJPEG(t, posterHeight, posterWidth))
	return h
}

func (h *harness) handle(handler bus.Handler, m domain.Message) error {
	ev := bus.EventOf(m)
	return h.db.Write(context.Background(), func(tx *gorm.DB) error { return handler.Handle(context.Background(), tx, ev) })
}

// deliver hands an event to the RequestApproved handler, as the dispatcher would.
func (h *harness) deliver(m domain.Message) {
	h.t.Helper()
	if err := h.handle(catalog.NewRequestApprovedHandler(h.service, h.tasks), m); err != nil {
		h.t.Fatalf("handling %s: %v", m.Type(), err)
	}
}

func (h *harness) deliverHealth(m domain.Message) {
	h.t.Helper()
	if err := h.handle(catalog.NewTmdbHealthHandler(h.service), m); err != nil {
		h.t.Fatal(err)
	}
}

// runJobs processes queued tasks, including ones they queue, until none are left.
func (h *harness) runJobs() {
	h.t.Helper()
	for {
		next, ok := h.tasks.next()
		if !ok {
			return
		}
		if next.wait > 0 {
			h.waits = append(h.waits, next.wait)
		}
		payload, err := json.Marshal(next.task)
		if err != nil {
			h.t.Fatal(err)
		}
		if err := h.queues[next.task.Config().Name].Process(context.Background(), payload); err != nil {
			h.t.Fatalf("job %s: %v", next.task.Config().Name, err)
		}
	}
}

func (h *harness) runTask(task backlite.Task) {
	h.t.Helper()
	h.tasks.add(task, 0)
	h.runJobs()
}

// events decodes every published event of one type, in publish order.
func events[T domain.Message](h *harness) []T {
	h.t.Helper()
	var zero T
	var rows []struct{ Payload string }
	err := h.db.Read(context.Background()).Table("platform_outbox").Select("payload").
		Where("type = ?", zero.Type()).Order("seq").Find(&rows).Error
	if err != nil {
		h.t.Fatal(err)
	}
	decoded := make([]T, 0, len(rows))
	for _, row := range rows {
		var payload T
		if err := json.Unmarshal([]byte(row.Payload), &payload); err != nil {
			h.t.Fatal(err)
		}
		decoded = append(decoded, payload)
	}
	return decoded
}

type cachedImage struct {
	LocalPath string
	FetchedAt time.Time
}

func (h *harness) image(title domain.TitleID, kind string) cachedImage {
	h.t.Helper()
	var img cachedImage
	err := h.db.Read(context.Background()).Table("catalog_images").Select("local_path, fetched_at").
		Where("title_id = ? AND kind = ?", title, kind).Take(&img).Error
	if err != nil {
		h.t.Fatal(err)
	}
	return img
}

func (h *harness) ageImage(title domain.TitleID, kind string, fetchedAt time.Time) {
	h.t.Helper()
	err := h.db.Write(context.Background(), func(tx *gorm.DB) error {
		return tx.Table("catalog_images").Where("title_id = ? AND kind = ?", title, kind).Update("fetched_at", fetchedAt).Error
	})
	if err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) request(method, path string, body any, principal *domain.Principal) *httptest.ResponseRecorder {
	h.t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			h.t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	}
	ctx := context.Background()
	if principal != nil {
		ctx = domain.WithPrincipal(ctx, *principal)
	}
	req := httptest.NewRequestWithContext(ctx, method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.server.Mux.ServeHTTP(rec, req)
	return rec
}

var (
	admin     = &domain.Principal{User: domain.NewID[domain.UserID](), Admin: true}
	requester = &domain.Principal{User: domain.NewID[domain.UserID]()}
)

func testJPEG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func decodeJSON[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if rec.Code >= http.StatusBadRequest {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decoding %s: %v", rec.Body, err)
	}
	return v
}

func ptr[T any](v T) *T {
	return &v
}
