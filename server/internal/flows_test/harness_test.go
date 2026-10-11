// Package flows_test boots every module against a temp database and drives the key flows end to end.
package flows_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"magpie/internal/app"
	"magpie/internal/identity"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/config"
	"magpie/internal/platform/db"
	"magpie/internal/platform/httpserver"

	"github.com/go-fuego/fuego"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

const (
	// listenOnAnyPort lets parallel flow tests run without port clashes.
	listenOnAnyPort = 0

	setupAdminPath    = "/api/identity/setup/admin"
	testAdminUsername = "admin"
	testAdminPassword = "flow test admin password"
	contentTypeHeader = "Content-Type"
	jsonContentType   = "application/json"
)

type harness struct {
	t       *testing.T
	DB      *db.DB
	Bus     *bus.Bus
	server  *fuego.Server
	session *http.Cookie
}

// boot starts the whole app on a temp data dir. Flow tests pass fake adapters and extra handlers as options.
func boot(t *testing.T, options ...fx.Option) *harness {
	t.Helper()
	h := &harness{t: t}
	all := append([]fx.Option{
		app.Module,
		fx.Supply(httpserver.ModeServe),
		fx.Replace(config.Env{Port: listenOnAnyPort, DataDir: t.TempDir()}),
		fx.Populate(&h.DB, &h.Bus, &h.server),
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
