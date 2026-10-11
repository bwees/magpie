package identity

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"magpie/internal/domain"
	"magpie/internal/identity/api"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/config"
	"magpie/internal/platform/db"
	"magpie/internal/platform/db/dbtest"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/problem"

	"github.com/go-fuego/fuego"
	"gorm.io/gorm"
)

const (
	adminUsername   = "admin"
	adminPassword   = "correct horse battery"
	newPassword     = "staple battery horse"
	shortPassword   = "elevenchars"
	wrongPassword   = "not the password"
	clientAddr      = "192.0.2.10:51000"
	otherClientAddr = "198.51.100.7:51000"
	adminOnlyPath   = "/api/test/admin-only"
	pathSetup       = "/api/identity/setup/admin"
	pathLogin       = "/api/identity/login/local"
	pathLogout      = "/api/identity/logout"
	pathMe          = "/api/identity/me"
	pathPassword    = "/api/identity/local-admin/password"
	pathHealth      = "/api/health"
	tokenBytes      = 32
	argon2idPrefix  = "$argon2id$"
	failedSignInCap = 10
)

var startOfTest = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

type adminOnlyController struct{}

type adminOnlyResponse struct {
	OK bool `json:"ok"`
}

func (adminOnlyController) Register(api *fuego.Server) {
	fuego.Get(api, "/test/admin-only", func(c fuego.ContextNoBody) (adminOnlyResponse, error) {
		if err := httpserver.RequireAdmin(c.Context()); err != nil {
			return adminOnlyResponse{}, err
		}
		return adminOnlyResponse{OK: true}, nil
	}, fuego.OptionOperationID("testAdminOnly"))
}

type testEnv struct {
	t       *testing.T
	db      *db.DB
	repo    *Repository
	service *Service
	clock   time.Time
	handler http.Handler
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	d := dbtest.Open(t, Migrations())
	repo := NewRepository(d)
	env := &testEnv{t: t, db: d, repo: repo, clock: startOfTest}
	env.service = NewService(d, bus.New(), repo, nil)
	env.service.now = func() time.Time { return env.clock }
	env.service.limiter.now = env.service.now
	controllers := []httpserver.Controller{httpserver.NewHealthController(), NewController(env.service), adminOnlyController{}}
	env.handler = httpserver.Build(config.Env{}, httpserver.ModeServe, controllers, env.service.Middleware).Mux
	return env
}

func (e *testEnv) advance(d time.Duration) {
	e.clock = e.clock.Add(d)
}

func (e *testEnv) request(method, path string, body any, from string, cookie *http.Cookie) *httptest.ResponseRecorder {
	e.t.Helper()
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			e.t.Fatal(err)
		}
	}
	req := httptest.NewRequestWithContext(context.Background(), method, path, &payload)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = from
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

func (e *testEnv) do(method, path string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.request(method, path, body, clientAddr, cookie)
}

func (e *testEnv) setup() *http.Cookie {
	e.t.Helper()
	rec := e.do(http.MethodPost, pathSetup, CredentialsRequest{Username: adminUsername, Password: adminPassword}, nil)
	if rec.Code != http.StatusOK {
		e.t.Fatalf("setup = %d: %s", rec.Code, rec.Body)
	}
	return sessionCookie(e.t, rec)
}

func (e *testEnv) storedSession(token string) sessionRow {
	e.t.Helper()
	var row sessionRow
	if err := e.db.Read(context.Background()).Where("token_hash = ?", hashToken(token)).Take(&row).Error; err != nil {
		e.t.Fatalf("session row: %v", err)
	}
	return row
}

func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookie {
			return c
		}
	}
	t.Fatalf("no %s cookie in %v", SessionCookie, rec.Result().Header)
	return nil
}

func problemType(t *testing.T, rec *httptest.ResponseRecorder) problem.Type {
	t.Helper()
	var body fuego.HTTPError
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	return problem.Type(body.Type)
}

func outboxUserCreated(t *testing.T, d *db.DB) []api.UserCreated {
	t.Helper()
	var payloads []string
	if err := d.Read(context.Background()).Table("platform_outbox").Where("type = ?", api.TypeUserCreated).Pluck("payload", &payloads).Error; err != nil {
		t.Fatal(err)
	}
	events := make([]api.UserCreated, 0, len(payloads))
	for _, p := range payloads {
		var ev api.UserCreated
		if err := json.Unmarshal([]byte(p), &ev); err != nil {
			t.Fatal(err)
		}
		events = append(events, ev)
	}
	return events
}

// Seed 1.
func TestSetupCreatesTheLocalAdminOnce(t *testing.T) {
	env := newTestEnv(t)
	cookie := env.setup()

	users, err := env.repo.All(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || !users[0].LocalAdmin || users[0].Role != api.RoleAdmin {
		t.Fatalf("users = %+v, want one local admin", users)
	}
	events := outboxUserCreated(t, env.db)
	want := api.UserCreated{User: users[0].ID, Role: api.RoleAdmin, Source: api.UserSourceLocal}
	if len(events) != 1 || events[0] != want {
		t.Errorf("outbox UserCreated = %+v, want [%+v]", events, want)
	}
	if rec := env.do(http.MethodGet, pathMe, nil, cookie); rec.Code != http.StatusOK {
		t.Errorf("me with the setup cookie = %d", rec.Code)
	}

	rec := env.do(http.MethodPost, pathSetup, CredentialsRequest{Username: "second", Password: adminPassword}, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("second setup = %d, want 409", rec.Code)
	}
	if got := problemType(t, rec); got != TypeSetupComplete {
		t.Errorf("type = %q, want %q", got, TypeSetupComplete)
	}
}

// Seed 2.
func TestEveryOtherRouteAnswersSetupRequired(t *testing.T) {
	env := newTestEnv(t)

	for _, route := range []struct{ method, path string }{
		{http.MethodGet, pathMe},
		{http.MethodPost, pathLogin},
		{http.MethodPost, pathLogout},
		{http.MethodGet, adminOnlyPath},
		{http.MethodGet, "/api/unknown"},
	} {
		rec := env.do(route.method, route.path, CredentialsRequest{Username: adminUsername, Password: adminPassword}, nil)
		if rec.Code != http.StatusConflict {
			t.Errorf("%s %s = %d, want 409", route.method, route.path, rec.Code)
			continue
		}
		if got := problemType(t, rec); got != TypeSetupRequired {
			t.Errorf("%s %s type = %q, want %q", route.method, route.path, got, TypeSetupRequired)
		}
	}
	if rec := env.do(http.MethodGet, pathHealth, nil, nil); rec.Code != http.StatusOK {
		t.Errorf("health during setup = %d, want 200", rec.Code)
	}

	env.setup()
	if rec := env.do(http.MethodGet, pathMe, nil, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("me without a session after setup = %d, want 401", rec.Code)
	}
}

func TestLocalAdminPasswordNeedsTwelveCharacters(t *testing.T) {
	env := newTestEnv(t)
	rec := env.do(http.MethodPost, pathSetup, CredentialsRequest{Username: adminUsername, Password: shortPassword}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("setup with %d chars = %d, want 400", len(shortPassword), rec.Code)
	}
	if got := problemType(t, rec); got != TypeWeakPassword {
		t.Errorf("type = %q, want %q", got, TypeWeakPassword)
	}
}

func TestLocalAdminPasswordIsStoredWithArgon2id(t *testing.T) {
	env := newTestEnv(t)
	env.setup()

	var hash string
	if err := env.db.Read(context.Background()).Table("identity_users").Where("is_local_admin").Pluck("password_hash", &hash).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, argon2idPrefix) || strings.Contains(hash, adminPassword) {
		t.Errorf("stored hash %q is not argon2id", hash)
	}
}

func TestSessionCookieIsARandomTokenStoredOnlyAsItsHash(t *testing.T) {
	env := newTestEnv(t)
	cookie := env.setup()

	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie HttpOnly=%v SameSite=%v, want HttpOnly Lax", cookie.HttpOnly, cookie.SameSite)
	}
	raw, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil || len(raw) != tokenBytes {
		t.Fatalf("token decodes to %d bytes (%v), want %d", len(raw), err, tokenBytes)
	}
	sum := sha256.Sum256([]byte(cookie.Value))
	var stored []string
	if err := env.db.Read(context.Background()).Table("identity_sessions").Pluck("token_hash", &stored).Error; err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0] != hex.EncodeToString(sum[:]) {
		t.Errorf("stored %v, want only the SHA-256 of the cookie", stored)
	}
}

func TestLocalSignInAndSignOut(t *testing.T) {
	env := newTestEnv(t)
	env.setup()

	if rec := env.do(http.MethodPost, pathLogin, CredentialsRequest{Username: adminUsername, Password: wrongPassword}, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password = %d, want 401", rec.Code)
	}
	rec := env.do(http.MethodPost, pathLogin, CredentialsRequest{Username: strings.ToUpper(adminUsername), Password: adminPassword}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("sign-in = %d: %s", rec.Code, rec.Body)
	}
	cookie := sessionCookie(t, rec)

	rec = env.do(http.MethodGet, pathMe, nil, cookie)
	var me MeResponse
	if err := json.NewDecoder(rec.Body).Decode(&me); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("me = %d, %v", rec.Code, err)
	}
	if me.Role != api.RoleAdmin || !me.LocalAdmin || !me.Permissions.ReportIssues {
		t.Errorf("me = %+v", me)
	}

	if rec := env.do(http.MethodPost, pathLogout, nil, cookie); rec.Code != http.StatusOK {
		t.Fatalf("logout = %d", rec.Code)
	}
	if rec := env.do(http.MethodGet, pathMe, nil, cookie); rec.Code != http.StatusUnauthorized {
		t.Errorf("me after logout = %d, want 401", rec.Code)
	}
}

// Seed 14.
func TestTenFailedSignInsFromOneIPBlockTheEleventh(t *testing.T) {
	env := newTestEnv(t)
	env.setup()
	wrong := CredentialsRequest{Username: adminUsername, Password: wrongPassword}
	right := CredentialsRequest{Username: adminUsername, Password: adminPassword}

	for range failedSignInCap {
		if rec := env.do(http.MethodPost, pathLogin, wrong, nil); rec.Code != http.StatusUnauthorized {
			t.Fatalf("failed sign-in = %d, want 401", rec.Code)
		}
	}
	rec := env.do(http.MethodPost, pathLogin, right, nil)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("11th sign-in = %d, want 429", rec.Code)
	}
	if got := problemType(t, rec); got != TypeTooManySignIns {
		t.Errorf("type = %q, want %q", got, TypeTooManySignIns)
	}
	if rec := env.request(http.MethodPost, pathLogin, right, otherClientAddr, nil); rec.Code != http.StatusOK {
		t.Errorf("sign-in from another IP = %d, want 200", rec.Code)
	}

	env.advance(SignInWindow)
	if rec := env.do(http.MethodPost, pathLogin, right, nil); rec.Code != http.StatusOK {
		t.Errorf("sign-in after the window = %d, want 200", rec.Code)
	}
}

// Seed 15.
func TestSessionUnusedForThirtyDaysIsRejected(t *testing.T) {
	env := newTestEnv(t)
	cookie := env.setup()

	env.advance(SessionLifetime)
	if rec := env.do(http.MethodGet, pathMe, nil, cookie); rec.Code != http.StatusUnauthorized {
		t.Fatalf("me after 30 idle days = %d, want 401", rec.Code)
	}
	var left int64
	if err := env.db.Read(context.Background()).Table("identity_sessions").Count(&left).Error; err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Errorf("%d expired sessions left in the table", left)
	}
}

func TestSessionSlidesAndIsTouchedAtMostEveryFiveMinutes(t *testing.T) {
	env := newTestEnv(t)
	cookie := env.setup()
	created := env.storedSession(cookie.Value)

	env.advance(SessionTouchInterval - time.Minute)
	if rec := env.do(http.MethodGet, pathMe, nil, cookie); rec.Code != http.StatusOK {
		t.Fatalf("me = %d", rec.Code)
	}
	if got := env.storedSession(cookie.Value); !got.LastSeenAt.Equal(created.LastSeenAt) {
		t.Errorf("last_seen_at written after %v: %v", SessionTouchInterval-time.Minute, got.LastSeenAt)
	}

	env.advance(time.Minute)
	if rec := env.do(http.MethodGet, pathMe, nil, cookie); rec.Code != http.StatusOK {
		t.Fatalf("me = %d", rec.Code)
	}
	touched := env.storedSession(cookie.Value)
	if !touched.LastSeenAt.Equal(env.clock) || !touched.ExpiresAt.Equal(env.clock.Add(SessionLifetime)) {
		t.Errorf("after %v: last_seen=%v expires=%v, want %v and +30d", SessionTouchInterval, touched.LastSeenAt, touched.ExpiresAt, env.clock)
	}

	env.advance(SessionLifetime - time.Minute)
	if rec := env.do(http.MethodGet, pathMe, nil, cookie); rec.Code != http.StatusOK {
		t.Errorf("me inside the slid window = %d, want 200", rec.Code)
	}
}

func TestAdminOnlyRoutesForbidRequesters(t *testing.T) {
	env := newTestEnv(t)
	adminCookie := env.setup()

	requester := userRow{ID: domain.NewID[domain.UserID](), Username: "bob", DisplayName: "Bob", Role: api.RoleRequester, CreatedAt: env.clock}
	var issued issuedSession
	err := env.db.Write(context.Background(), func(tx *gorm.DB) error {
		if err := env.repo.insertUser(tx, requester); err != nil {
			return err
		}
		var err error
		issued, err = env.service.startSession(tx, requester.ID, "")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	requesterCookie := &http.Cookie{Name: SessionCookie, Value: issued.Token}

	cases := map[string]struct {
		cookie *http.Cookie
		want   int
	}{
		"admin":     {adminCookie, http.StatusOK},
		"requester": {requesterCookie, http.StatusForbidden},
		"anonymous": {nil, http.StatusUnauthorized},
	}
	for name, tc := range cases {
		if rec := env.do(http.MethodGet, adminOnlyPath, nil, tc.cookie); rec.Code != tc.want {
			t.Errorf("%s = %d, want %d", name, rec.Code, tc.want)
		}
	}
}

func TestChangeLocalAdminPassword(t *testing.T) {
	env := newTestEnv(t)
	cookie := env.setup()

	if rec := env.do(http.MethodPut, pathPassword, ChangePasswordRequest{CurrentPassword: wrongPassword, NewPassword: newPassword}, cookie); rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong current password = %d, want 401", rec.Code)
	}
	if rec := env.do(http.MethodPut, pathPassword, ChangePasswordRequest{CurrentPassword: adminPassword, NewPassword: shortPassword}, cookie); rec.Code != http.StatusBadRequest {
		t.Errorf("short new password = %d, want 400", rec.Code)
	}
	if rec := env.do(http.MethodPut, pathPassword, ChangePasswordRequest{CurrentPassword: adminPassword, NewPassword: newPassword}, cookie); rec.Code != http.StatusOK {
		t.Fatalf("change = %d: %s", rec.Code, rec.Body)
	}

	if rec := env.do(http.MethodGet, pathMe, nil, cookie); rec.Code != http.StatusOK {
		t.Errorf("the session that changed the password = %d, want 200", rec.Code)
	}
	if rec := env.do(http.MethodPost, pathLogin, CredentialsRequest{Username: adminUsername, Password: adminPassword}, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("old password = %d, want 401", rec.Code)
	}
	if rec := env.do(http.MethodPost, pathLogin, CredentialsRequest{Username: adminUsername, Password: newPassword}, nil); rec.Code != http.StatusOK {
		t.Errorf("new password = %d, want 200", rec.Code)
	}
}
