package opensubtitles

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"magpie/internal/platform/db/dbtest"
	"magpie/internal/platform/health"
	"magpie/internal/platform/settings"
)

const (
	testAppKey       = "recorded-app-key"
	testUsername     = "magpie-user"
	testPassword     = "recorded-password"
	recordedToken    = "eyJhbGciOiJIUzI1NiJ9.recorded.token"
	fixtureDir       = "testdata/health"
	fixtureLogin     = "login.json"
	fixtureLoginDeny = "login_unauthorized.json"
	fixtureUserInfo  = "user_info.json"
)

type recordedOpenSubtitles struct {
	t            *testing.T
	mu           sync.Mutex
	paths        []string
	tokenRevoked bool
}

func (o *recordedOpenSubtitles) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	o.mu.Lock()
	o.paths = append(o.paths, r.URL.Path)
	revoked := o.tokenRevoked
	o.mu.Unlock()
	if r.Header.Get(apiKeyHeader) != testAppKey || r.Header.Get(userAgentHeader) == "" {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	switch r.URL.Path {
	case pathLogin:
		var body loginRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Username != testUsername || body.Password != testPassword {
			w.WriteHeader(http.StatusUnauthorized)
			o.serve(w, fixtureLoginDeny)
			return
		}
		o.serve(w, fixtureLogin)
	case pathUserInfo:
		if revoked || r.Header.Get(authorizationHeader) != bearerPrefix+recordedToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		o.serve(w, fixtureUserInfo)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (o *recordedOpenSubtitles) serve(w http.ResponseWriter, fixture string) {
	body, err := os.ReadFile(filepath.Join(fixtureDir, fixture))
	if err != nil {
		o.t.Error(err)
		return
	}
	_, _ = w.Write(body)
}

func newProbe(t *testing.T, conn settings.OpenSubtitlesConnection, baseURL string) *HealthProbe {
	t.Helper()
	store := settings.NewStore(dbtest.Open(t))
	if err := settings.OpenSubtitles.Set(context.Background(), store, conn); err != nil {
		t.Fatal(err)
	}
	probe := NewHealthProbe(store)
	probe.baseURL = baseURL
	probe.appKey = testAppKey
	return probe
}

func TestProbeLogsInOnceThenReusesTheToken(t *testing.T) {
	recorded := &recordedOpenSubtitles{t: t}
	server := httptest.NewServer(recorded)
	defer server.Close()
	probe := newProbe(t, settings.OpenSubtitlesConnection{Username: testUsername, Password: testPassword}, server.URL)
	ctx := context.Background()

	for range 2 {
		result, err := probe.Probe(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if result.Version != "Sub leecher" || result.Detail != "20 downloads/day" {
			t.Errorf("result = %+v", result)
		}
	}
	recorded.tokenRevoked = true
	if _, err := probe.Probe(ctx); err != nil {
		t.Fatal(err)
	}

	want := []string{pathLogin, pathUserInfo, pathUserInfo, pathLogin}
	if strings.Join(recorded.paths, " ") != strings.Join(want, " ") {
		t.Errorf("calls = %v, want %v", recorded.paths, want)
	}
}

func TestProbeReportsWrongCredentials(t *testing.T) {
	server := httptest.NewServer(&recordedOpenSubtitles{t: t})
	defer server.Close()
	_, err := newProbe(t, settings.OpenSubtitlesConnection{Username: testUsername, Password: "wrong"}, server.URL).Probe(context.Background())
	if err == nil || !strings.Contains(err.Error(), "password") {
		t.Errorf("err = %v, want a rejected-login error", err)
	}
}

func TestProbeWithoutALoginIsNotConfigured(t *testing.T) {
	if _, err := newProbe(t, settings.OpenSubtitlesConnection{}, defaultBaseURL).Probe(context.Background()); !errors.Is(err, health.ErrNotConfigured) {
		t.Errorf("err = %v, want ErrNotConfigured", err)
	}
}

func TestProbeNeedsTheBuiltInAppKey(t *testing.T) {
	probe := newProbe(t, settings.OpenSubtitlesConnection{Username: testUsername, Password: testPassword}, defaultBaseURL)
	probe.appKey = ""
	if _, err := probe.Probe(context.Background()); err == nil || err.Error() != missingAppKey {
		t.Errorf("err = %v, want %q", err, missingAppKey)
	}
}
