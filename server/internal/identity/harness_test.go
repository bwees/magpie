package identity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"magpie/internal/domain"
	"magpie/internal/identity"
	"magpie/internal/identity/api"
	"magpie/internal/identity/fakes"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/config"
	"magpie/internal/platform/db"
	"magpie/internal/platform/db/dbtest"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/settings"

	"github.com/go-fuego/fuego"
)

const (
	adminUsername = "admin"
	adminPassword = "correct horse battery"
	clientAddr    = "192.0.2.10:51000"

	pathSetup            = "/api/identity/setup/admin"
	pathLoginLocal       = "/api/identity/login/local"
	pathLoginJellyfin    = "/api/identity/login/jellyfin"
	pathLoginMethods     = "/api/identity/login/methods"
	pathQuickConnect     = "/api/identity/login/quick-connect"
	pathMe               = "/api/identity/me"
	pathUsers            = "/api/identity/users"
	pathDefaults         = "/api/identity/defaults"
	pathSignInSettings   = "/api/identity/sign-in-settings"
	contentTypeHeader    = "Content-Type"
	jsonContentType      = "application/json"
	urlPathSeparator     = "/"
	identityUsersTable   = "identity_users"
	identitySessionTable = "identity_sessions"
)

// env runs Identity's routes behind its middleware, with a fake Jellyfin, after first-run setup.
type env struct {
	t        *testing.T
	db       *db.DB
	repo     *identity.Repository
	jellyfin *fakes.JellyfinAuth
	handler  http.Handler
	admin    *http.Cookie
}

func newEnv(t *testing.T) *env {
	t.Helper()
	d := dbtest.Open(t, identity.Migrations())
	repo := identity.NewRepository(d)
	jellyfin := fakes.NewJellyfinAuth()
	service := identity.NewService(d, bus.New(), repo, jellyfin, settings.NewStore(d), nil)
	controllers := []httpserver.Controller{identity.NewController(service)}
	e := &env{
		t:        t,
		db:       d,
		repo:     repo,
		jellyfin: jellyfin,
		handler:  httpserver.Build(config.Env{}, httpserver.ModeServe, controllers, service.Middleware).Mux,
	}
	rec := e.do(http.MethodPost, pathSetup, identity.CredentialsRequest{Username: adminUsername, Password: adminPassword}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup = %d: %s", rec.Code, rec.Body)
	}
	e.admin = sessionCookie(t, rec)
	return e
}

func (e *env) do(method, path string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
	e.t.Helper()
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			e.t.Fatal(err)
		}
	}
	req := httptest.NewRequestWithContext(context.Background(), method, path, &payload)
	req.Header.Set(contentTypeHeader, jsonContentType)
	req.RemoteAddr = clientAddr
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

// signInJellyfin signs in through the Jellyfin form and returns the session cookie.
func (e *env) signInJellyfin(username, password string) *http.Cookie {
	e.t.Helper()
	rec := e.do(http.MethodPost, pathLoginJellyfin, identity.CredentialsRequest{Username: username, Password: password}, nil)
	if rec.Code != http.StatusOK {
		e.t.Fatalf("Jellyfin sign-in as %s = %d: %s", username, rec.Code, rec.Body)
	}
	return sessionCookie(e.t, rec)
}

func (e *env) userByJellyfinID(jellyfinID string) api.User {
	e.t.Helper()
	users, err := e.repo.All(context.Background())
	if err != nil {
		e.t.Fatal(err)
	}
	for _, u := range users {
		if u.JellyfinUserID != nil && *u.JellyfinUserID == jellyfinID {
			return u
		}
	}
	e.t.Fatalf("no user for Jellyfin ID %s in %+v", jellyfinID, users)
	return api.User{}
}

func (e *env) rowCount(table string) int64 {
	e.t.Helper()
	var n int64
	if err := e.db.Read(context.Background()).Table(table).Count(&n).Error; err != nil {
		e.t.Fatal(err)
	}
	return n
}

func (e *env) userCreated() []api.UserCreated {
	e.t.Helper()
	var payloads []string
	if err := e.db.Read(context.Background()).Table("platform_outbox").Where("type = ?", api.TypeUserCreated).Pluck("payload", &payloads).Error; err != nil {
		e.t.Fatal(err)
	}
	events := make([]api.UserCreated, 0, len(payloads))
	for _, p := range payloads {
		var ev api.UserCreated
		if err := json.Unmarshal([]byte(p), &ev); err != nil {
			e.t.Fatal(err)
		}
		events = append(events, ev)
	}
	return events
}

func (e *env) patchUser(id domain.UserID, change identity.UpdateUserRequest, cookie *http.Cookie) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.do(http.MethodPatch, pathUsers+urlPathSeparator+string(id), change, cookie)
}

func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == identity.SessionCookie {
			return c
		}
	}
	t.Fatalf("no %s cookie in %v", identity.SessionCookie, rec.Result().Header)
	return nil
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(rec.Body).Decode(&v); err != nil {
		t.Fatalf("decode %T: %v (%d)", v, err, rec.Code)
	}
	return v
}

func problemType(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	return decode[fuego.HTTPError](t, rec).Type
}

func ptr[T any](v T) *T { return &v }
