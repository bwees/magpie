package transcode

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"magpie/internal/domain"
	"magpie/internal/platform/config"
	"magpie/internal/platform/httpserver"
	"magpie/internal/transcode/api"

	"github.com/go-fuego/fuego"
)

type apiClient struct {
	t      *testing.T
	server *fuego.Server
}

func (f *fixture) api() *apiClient {
	controller := NewController(f.service, NewAgentSocket(f.service, NewAgentHub(), testSecret, DefaultTimings), testSecret)
	return &apiClient{t: f.t, server: httpserver.Build(config.Env{}, httpserver.ModeServe, []httpserver.Controller{controller})}
}

func (c *apiClient) do(principal *domain.Principal, method, path string, body any) *httptest.ResponseRecorder {
	c.t.Helper()
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			c.t.Fatal(err)
		}
	}
	ctx := context.Background()
	if principal != nil {
		ctx = domain.WithPrincipal(ctx, *principal)
	}
	req := httptest.NewRequestWithContext(ctx, method, httpserver.APIPrefix+path, &payload)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c.server.Mux.ServeHTTP(rec, req)
	return rec
}

var (
	admin     = &domain.Principal{User: "admin", Admin: true}
	requester = &domain.Principal{User: "requester"}
)

func decodeJSON[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decoding %s: %v", rec.Body.String(), err)
	}
	return v
}

func TestAgentEndpointsAreAdminOnly(t *testing.T) {
	c := newFixture(t).api()
	for _, path := range []string{"/transcode/agents", "/transcode/secret", "/transcode/settings"} {
		if rec := c.do(requester, http.MethodGet, path, nil); rec.Code != http.StatusForbidden {
			t.Errorf("GET %s as requester = %d, want 403", path, rec.Code)
		}
	}
}

func TestListAgentsShowsEffectiveSlots(t *testing.T) {
	f := newFixture(t)
	id := domain.NewID[domain.AgentID]()
	f.connect(testHello(id))

	rec := f.api().do(admin, http.MethodGet, "/transcode/agents", nil)

	agents := decodeJSON[[]AgentResponse](t, rec)
	if rec.Code != http.StatusOK || len(agents) != 1 {
		t.Fatalf("status %d, agents %+v", rec.Code, agents)
	}
	got := agents[0]
	if got.ID != string(id) || !got.Online || got.SlotsOverride != nil || got.Devices[0].Slots != defaultSlotsPerGPU {
		t.Fatalf("agent = %+v", got)
	}
}

func TestUpdateAgentValidatesAndFindsTheAgent(t *testing.T) {
	f := newFixture(t)
	id := domain.NewID[domain.AgentID]()
	f.connect(testHello(id))
	c := f.api()
	tooMany := MaxSlotsOverride + 1
	two := 2

	if rec := c.do(admin, http.MethodPatch, "/transcode/agents/"+string(id), UpdateAgentRequest{SlotsOverride: &tooMany, Mode: api.TransferDownloadFirst}); rec.Code != http.StatusBadRequest {
		t.Errorf("slots %d = %d, want 400", tooMany, rec.Code)
	}
	if rec := c.do(admin, http.MethodPatch, "/transcode/agents/"+string(domain.NewID[domain.AgentID]()), UpdateAgentRequest{Mode: api.TransferDownloadFirst}); rec.Code != http.StatusNotFound {
		t.Errorf("unknown agent = %d, want 404", rec.Code)
	}

	rec := c.do(admin, http.MethodPatch, "/transcode/agents/"+string(id), UpdateAgentRequest{SlotsOverride: &two, Mode: api.TransferDirectRead})
	got := decodeJSON[AgentResponse](t, rec)
	if rec.Code != http.StatusOK || got.Mode != api.TransferDirectRead || got.Devices[0].Slots != two {
		t.Fatalf("status %d, agent %+v", rec.Code, got)
	}
}

func TestRetestOfADisconnectedAgentConflicts(t *testing.T) {
	c := newFixture(t).api()

	if rec := c.do(admin, http.MethodPost, "/transcode/agents/"+string(domain.NewID[domain.AgentID]())+"/retest", nil); rec.Code != http.StatusConflict {
		t.Fatalf("retest = %d, want 409", rec.Code)
	}
}

func TestSecretIsMasked(t *testing.T) {
	rec := newFixture(t).api().do(admin, http.MethodGet, "/transcode/secret", nil)

	got := decodeJSON[AgentSecretResponse](t, rec)
	if !got.Configured || got.Secret != testSecret || strings.Contains(got.Masked, testSecret[:maskLength]) ||
		!strings.HasSuffix(got.Masked, testSecret[len(testSecret)-maskVisibleTail:]) {
		t.Fatalf("secret = %+v", got)
	}
}

func TestSettingsRoundTripWithinBounds(t *testing.T) {
	c := newFixture(t).api()

	defaults := decodeJSON[TranscodeSettingsResponse](t, c.do(admin, http.MethodGet, "/transcode/settings", nil))
	if defaults.TransferCap != defaultTransferCap || defaults.DefaultSlotsPerGPU != defaultSlotsPerGPU {
		t.Fatalf("defaults = %+v", defaults)
	}

	outOfRange := UpdateTranscodeSettingsRequest{TransferCap: MaxTransferCap + 1, DefaultSlotsPerGPU: MinSlotsPerGPU}
	if rec := c.do(admin, http.MethodPut, "/transcode/settings", outOfRange); rec.Code != http.StatusBadRequest {
		t.Fatalf("transfer cap %d = %d, want 400", outOfRange.TransferCap, rec.Code)
	}

	updated := UpdateTranscodeSettingsRequest{TransferCap: MaxTransferCap, DefaultSlotsPerGPU: MaxSlotsPerGPU}
	if rec := c.do(admin, http.MethodPut, "/transcode/settings", updated); rec.Code != http.StatusOK {
		t.Fatalf("update = %d: %s", rec.Code, rec.Body.String())
	}
	if got := decodeJSON[TranscodeSettingsResponse](t, c.do(admin, http.MethodGet, "/transcode/settings", nil)); got != TranscodeSettingsResponse(updated) {
		t.Fatalf("settings = %+v, want %+v", got, updated)
	}
}
