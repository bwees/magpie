// Package flows_test boots every module against a temp database and drives the key flows end to end.
package flows_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"magpie/internal/app"
	"magpie/internal/domain"
	"magpie/internal/identity"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/config"
	"magpie/internal/platform/db"
	"magpie/internal/platform/httpserver"

	"github.com/go-fuego/fuego"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	"gorm.io/gorm"
)

const (
	// listenOnAnyPort lets parallel flow tests run without port clashes.
	listenOnAnyPort = 0

	// flowWithin bounds a wait for a chain of handlers and jobs; a passing flow never waits that long.
	flowWithin = 30 * time.Second
	pollEvery  = 20 * time.Millisecond

	setupAdminPath    = "/api/identity/setup/admin"
	testAdminUsername = "admin"
	testAdminPassword = "flow test admin password"
	contentTypeHeader = "Content-Type"
	jsonContentType   = "application/json"

	outboxTable        = "platform_outbox"
	subscriptionsTable = "platform_subscriptions"
	handlerQueueTable  = "platform_handler_queue"
)

type harness struct {
	t          *testing.T
	DB         *db.DB
	Bus        *bus.Bus
	dispatcher *bus.Dispatcher
	server     *fuego.Server
	session    *http.Cookie
}

// boot starts the whole app on a temp data dir. Flow tests pass fake adapters and extra handlers as options.
func boot(t *testing.T, options ...fx.Option) *harness {
	t.Helper()
	h := &harness{t: t}
	all := append([]fx.Option{
		app.Module,
		fx.Supply(httpserver.ModeServe),
		fx.Replace(config.Env{Port: listenOnAnyPort, DataDir: t.TempDir()}),
		fx.Populate(&h.DB, &h.Bus, &h.dispatcher, &h.server),
		fx.NopLogger,
	}, options...)

	fxApp := fxtest.New(t, all...)
	fxApp.RequireStart()
	t.Cleanup(fxApp.RequireStop)
	return h
}

func (h *harness) get(path string) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodGet, path, nil)
}

// do sends body as JSON with the session cookie from signInAsAdmin, if any.
func (h *harness) do(method, path string, body any) *httptest.ResponseRecorder {
	h.t.Helper()
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			h.t.Fatal(err)
		}
	}
	req := httptest.NewRequestWithContext(context.Background(), method, path, &payload)
	req.Header.Set(contentTypeHeader, jsonContentType)
	if h.session != nil {
		req.AddCookie(h.session)
	}
	rec := httptest.NewRecorder()
	h.server.Mux.ServeHTTP(rec, req)
	return rec
}

// signInAsAdmin runs first-run setup, so every later request carries the local admin's session.
// Until setup runs, every API route except health answers 409.
func (h *harness) signInAsAdmin() {
	h.t.Helper()
	rec := h.do(http.MethodPost, setupAdminPath, identity.CredentialsRequest{Username: testAdminUsername, Password: testAdminPassword})
	if rec.Code != http.StatusOK {
		h.t.Fatalf("setup admin = %d: %s", rec.Code, rec.Body)
	}
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == identity.SessionCookie {
			h.session = cookie
			return
		}
	}
	h.t.Fatal("setup set no session cookie")
}

// publish stores m in the outbox, as the context that owns it would.
func (h *harness) publish(m domain.Message) {
	h.t.Helper()
	if err := h.DB.Write(context.Background(), func(tx *gorm.DB) error { return h.Bus.Publish(tx, bus.EventOf(m)) }); err != nil {
		h.t.Fatal(err)
	}
}

// outbox returns the payloads of every durable event of one type, oldest first.
func (h *harness) outbox(eventType string) []string {
	h.t.Helper()
	var payloads []string
	err := h.DB.Read(context.Background()).Table(outboxTable).Where("type = ?", eventType).Order("seq").Pluck("payload", &payloads).Error
	if err != nil {
		h.t.Fatal(err)
	}
	return payloads
}

// published decodes the durable events of one type that match, oldest first.
func published[T any](h *harness, eventType string, match func(T) bool) []T {
	h.t.Helper()
	var matched []T
	for _, raw := range h.outbox(eventType) {
		var ev T
		if err := json.Unmarshal([]byte(raw), &ev); err != nil {
			h.t.Fatal(err)
		}
		if match(ev) {
			matched = append(matched, ev)
		}
	}
	return matched
}

// awaitEvent waits for the first durable event of one type that matches.
func awaitEvent[T any](h *harness, eventType string, match func(T) bool) T {
	h.t.Helper()
	var found []T
	eventually(h.t, "a matching "+eventType, func() bool {
		found = published(h, eventType, match)
		return len(found) > 0
	})
	return found[0]
}

// redeliverAll rewinds every handler to the start of the outbox, as a crash before the cursors moved would,
// so each durable event reaches its handlers a second time, and waits until they caught up again. A handler
// still retrying an event keeps its place.
func (h *harness) redeliverAll() {
	h.t.Helper()
	ctx := context.Background()
	var latest int64
	if err := h.DB.Read(ctx).Table(outboxTable).Select("COALESCE(MAX(seq), 0)").Scan(&latest).Error; err != nil {
		h.t.Fatal(err)
	}
	err := h.DB.Write(ctx, func(tx *gorm.DB) error {
		retrying := tx.Table(handlerQueueTable).Select("handler")
		return tx.Table(subscriptionsTable).Where("handler NOT IN (?)", retrying).Update("cursor", 0).Error
	})
	if err != nil {
		h.t.Fatal(err)
	}
	h.dispatcher.Notify()
	eventually(h.t, "every handler caught up with the outbox again", func() bool {
		var behind int64
		err := h.DB.Read(ctx).Table(subscriptionsTable).Where("cursor < ?", latest).Count(&behind).Error
		if err != nil {
			h.t.Fatal(err)
		}
		return behind == 0
	})
}

func decodeBody[T any](t *testing.T, code int, raw []byte, want int) T {
	t.Helper()
	if code != want {
		t.Fatalf("status %d, want %d: %s", code, want, raw)
	}
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// getJSON reads a 200 answer of path as T.
func getJSON[T any](h *harness, path string) T {
	h.t.Helper()
	rec := h.get(path)
	return decodeBody[T](h.t, rec.Code, rec.Body.Bytes(), http.StatusOK)
}

// eventually polls check until it holds or the flow had its time.
func eventually(t *testing.T, what string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(flowWithin)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(pollEvery)
	}
	t.Fatalf("never: %s", what)
}

// settles polls an end state's check until it finds no problems, and fails with the last ones it found.
func settles(t *testing.T, what string, problems func() []string) {
	t.Helper()
	var last []string
	deadline := time.Now().Add(flowWithin)
	for time.Now().Before(deadline) {
		if last = problems(); len(last) == 0 {
			return
		}
		time.Sleep(pollEvery)
	}
	t.Fatalf("%s never settled:\n%s", what, strings.Join(last, "\n"))
}
