package jellyfin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"magpie/internal/identity"
	"magpie/internal/platform/settings"
)

const (
	authFixtureDir          = "testdata/auth"
	fixtureAuthByName       = "authenticate_by_name.json"
	fixtureQuickConnectOn   = "quick_connect_enabled.json"
	fixtureInitiate         = "quick_connect_initiate.json"
	fixtureApproved         = "quick_connect_connect_approved.json"
	fixtureAuthQuickConnect = "authenticate_with_quick_connect.json"

	recordedUsername  = "alice"
	recordedPassword  = "alice jellyfin password"
	recordedUserID    = "a1b2c3d4e5f60718293a4b5c6d7e8f90"
	disabledUsername  = "carol"
	brokenUsername    = "boom"
	recordedSecret    = "8f3b2a1c9d7e6f5a4b3c2d1e0f9a8b7c6d5e4f3a2b1c0d9e8f7a6b5c4d3e2f1a"
	recordedCode      = "482913"
	quickConnectUser  = "b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0"
	clientHeaderStart = `MediaBrowser Client="Magpie", `
	embyAuthorization = "X-Emby-Authorization"
)

// recordedAuthServer replays Jellyfin 12's answers to the sign-in endpoints. It refuses the legacy
// headers and API key parameter, which Jellyfin 12 no longer accepts.
type recordedAuthServer struct {
	t        *testing.T
	approved atomic.Bool
	server   *httptest.Server
}

func newRecordedAuthServer(t *testing.T) *recordedAuthServer {
	t.Helper()
	r := &recordedAuthServer{t: t}
	r.server = httptest.NewServer(http.HandlerFunc(r.serve))
	t.Cleanup(r.server.Close)
	return r
}

func (r *recordedAuthServer) serve(w http.ResponseWriter, req *http.Request) {
	legacy := req.Header.Get(embyAuthorization) != "" || req.Header.Get(embyTokenParam) != "" || req.URL.Query().Has(apiKeyParam)
	if legacy || !strings.HasPrefix(req.Header.Get(authorizationHeader), clientHeaderStart) {
		r.t.Errorf("%s %s sent %v, want only the MediaBrowser Client header", req.Method, req.URL, req.Header)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	var body struct{ Username, Pw, Secret string }
	if req.Method == http.MethodPost && req.ContentLength > 0 {
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	route := req.Method + " " + req.URL.Path
	switch {
	case route == "POST "+pathAuthenticateByName && body.Username == brokenUsername:
		w.WriteHeader(http.StatusInternalServerError)
	case route == "POST "+pathAuthenticateByName && body.Username == disabledUsername:
		w.WriteHeader(http.StatusForbidden)
	case route == "POST "+pathAuthenticateByName && body.Username == recordedUsername && body.Pw == recordedPassword:
		r.reply(w, fixtureAuthByName)
	case route == "POST "+pathAuthenticateByName:
		w.WriteHeader(http.StatusUnauthorized)
	case route == "GET "+pathQuickConnectEnabled:
		r.reply(w, fixtureQuickConnectOn)
	case route == "POST "+pathQuickConnectInitiate:
		r.reply(w, fixtureInitiate)
	case req.URL.Query().Get(querySecret) != recordedSecret && body.Secret != recordedSecret:
		w.WriteHeader(http.StatusNotFound)
	case route == "GET "+pathQuickConnectConnect && r.approved.Load():
		r.reply(w, fixtureApproved)
	case route == "GET "+pathQuickConnectConnect:
		r.reply(w, fixtureInitiate)
	case route == "POST "+pathAuthenticateQuickConnect && r.approved.Load():
		r.reply(w, fixtureAuthQuickConnect)
	case route == "POST "+pathAuthenticateQuickConnect:
		w.WriteHeader(http.StatusUnauthorized)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (r *recordedAuthServer) reply(w http.ResponseWriter, fixture string) {
	body, err := os.ReadFile(filepath.Join(authFixtureDir, fixture))
	if err != nil {
		r.t.Error(err)
		return
	}
	w.Header().Set(contentTypeHeader, jsonContent)
	_, _ = w.Write(body)
}

func newAuth(t *testing.T, url string) *Auth {
	t.Helper()
	return NewAuth(newProbe(t, settings.JellyfinConnection{URL: url, APIKey: testAPIKey}).store)
}

func TestAuthenticateByNameReturnsTheJellyfinUser(t *testing.T) {
	recorded := newRecordedAuthServer(t)
	user, err := newAuth(t, recorded.server.URL).AuthenticateByName(context.Background(), recordedUsername, recordedPassword)
	if err != nil {
		t.Fatal(err)
	}
	want := identity.JellyfinUser{ID: recordedUserID, Name: recordedUsername, IsAdministrator: true}
	if user != want {
		t.Errorf("user = %+v, want %+v", user, want)
	}
}

func TestAuthenticateByNameMapsFailures(t *testing.T) {
	recorded := newRecordedAuthServer(t)
	auth := newAuth(t, recorded.server.URL)
	ctx := context.Background()

	if _, err := auth.AuthenticateByName(ctx, recordedUsername, "wrong"); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Errorf("wrong password err = %v, want ErrInvalidCredentials", err)
	}
	if _, err := auth.AuthenticateByName(ctx, brokenUsername, recordedPassword); !errors.Is(err, identity.ErrUnavailable) {
		t.Errorf("500 err = %v, want ErrUnavailable", err)
	}
	user, err := auth.AuthenticateByName(ctx, disabledUsername, recordedPassword)
	if err != nil || !user.IsDisabled {
		t.Errorf("disabled account = %+v, %v, want IsDisabled", user, err)
	}
}

func TestUnreachableOrUnconfiguredJellyfinIsUnavailable(t *testing.T) {
	recorded := newRecordedAuthServer(t)
	closedURL := recorded.server.URL
	recorded.server.Close()
	ctx := context.Background()

	for name, url := range map[string]string{"unreachable": closedURL, "not configured": ""} {
		if _, err := newAuth(t, url).AuthenticateByName(ctx, recordedUsername, recordedPassword); !errors.Is(err, identity.ErrUnavailable) {
			t.Errorf("%s err = %v, want ErrUnavailable", name, err)
		}
		if _, err := newAuth(t, url).QuickConnectEnabled(ctx); !errors.Is(err, identity.ErrUnavailable) {
			t.Errorf("%s Quick Connect err = %v, want ErrUnavailable", name, err)
		}
	}
}

func TestQuickConnectAgainstRecordedJellyfin(t *testing.T) {
	recorded := newRecordedAuthServer(t)
	auth := newAuth(t, recorded.server.URL)
	started := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	auth.now = func() time.Time { return started }
	ctx := context.Background()

	enabled, err := auth.QuickConnectEnabled(ctx)
	if err != nil || !enabled {
		t.Fatalf("enabled = %v, %v", enabled, err)
	}
	qc, err := auth.QuickConnectInitiate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := identity.QuickConnect{Code: recordedCode, Secret: recordedSecret, ExpiresAt: started.Add(quickConnectLifetime)}
	if qc != want {
		t.Errorf("initiate = %+v, want %+v", qc, want)
	}

	if ok, err := auth.QuickConnectPoll(ctx, qc.Secret); ok || err != nil {
		t.Errorf("poll before approval = %v, %v", ok, err)
	}
	if _, err := auth.AuthenticateWithQuickConnect(ctx, qc.Secret); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Errorf("authenticate before approval err = %v, want ErrInvalidCredentials", err)
	}

	recorded.approved.Store(true)
	if ok, err := auth.QuickConnectPoll(ctx, qc.Secret); !ok || err != nil {
		t.Errorf("poll after approval = %v, %v", ok, err)
	}
	user, err := auth.AuthenticateWithQuickConnect(ctx, qc.Secret)
	if err != nil || user.ID != quickConnectUser || user.IsAdministrator {
		t.Errorf("authenticate = %+v, %v", user, err)
	}

	if _, err := auth.QuickConnectPoll(ctx, "forgotten"); !errors.Is(err, identity.ErrExpired) {
		t.Errorf("poll of a forgotten secret err = %v, want ErrExpired", err)
	}
	if _, err := auth.AuthenticateWithQuickConnect(ctx, "forgotten"); !errors.Is(err, identity.ErrExpired) {
		t.Errorf("authenticate with a forgotten secret err = %v, want ErrExpired", err)
	}
}
