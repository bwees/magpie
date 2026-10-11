// Package flows_test boots every module against a temp database and drives the key flows end to end.
package flows_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"magpie/internal/app"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/config"
	"magpie/internal/platform/db"
	"magpie/internal/platform/httpserver"

	"github.com/go-fuego/fuego"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

// listenOnAnyPort lets parallel flow tests run without port clashes.
const listenOnAnyPort = 0

type harness struct {
	t      *testing.T
	DB     *db.DB
	Bus    *bus.Bus
	server *fuego.Server
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
	rec := httptest.NewRecorder()
	h.server.Mux.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil))
	return rec
}
