package transcode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"magpie/internal/domain"
	"magpie/internal/platform/config"
	"magpie/internal/platform/httpserver"
	"magpie/internal/transcode/api"

	"github.com/go-fuego/fuego"
	"gorm.io/gorm"
)

type apiClient struct {
	t      *testing.T
	server *fuego.Server
}

func (f *fixture) api() *apiClient {
	controller := NewController(f.service, f.jobs, f.socket, f.transfers, testSecret)
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
	for _, path := range []string{"/transcode/agents", "/transcode/secret", "/transcode/settings", "/transcode/backlog", "/transcode/exclusions"} {
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
	if defaults.TransferCap != defaultTransferCap || defaults.DefaultSlotsPerGPU != defaultSlotsPerGPU ||
		!defaults.Enabled || defaults.TargetCodec != api.TargetHEVC10 || defaults.Preset != api.PresetHigh ||
		defaults.BitrateCapsMbps[api.Resolution1080] != defaultCap1080Mbps || defaults.LowBitratePct != defaultLowBitratePct ||
		defaults.MinSavingPct != defaultMinSavingPct || !defaults.SkipAlreadyTarget || !defaults.SkipLowBitrate ||
		len(defaults.Encoders) != len(calibration.Encoders) {
		t.Fatalf("defaults = %+v", defaults)
	}

	valid := func() UpdateTranscodeSettingsRequest {
		return UpdateTranscodeSettingsRequest{
			Enabled: true, TargetCodec: api.TargetAV110, Preset: api.PresetSmall,
			BitrateCapsMbps: map[api.ResolutionClass]int{
				api.Resolution2160: MaxCapMbps, api.Resolution1080: 8, api.Resolution720: 4, api.ResolutionSD: MinCapMbps,
			},
			SkipLowBitrate: true, LowBitratePct: MaxLowBitratePct, MinSavingPct: MinSavingPct,
			EncoderOverrides: map[string]int{EncoderAV1NVENC: 30},
			TransferCap:      MaxTransferCap, DefaultSlotsPerGPU: MaxSlotsPerGPU,
		}
	}
	outOfRange := map[string]func(*UpdateTranscodeSettingsRequest){
		"transfer cap": func(r *UpdateTranscodeSettingsRequest) { r.TransferCap = MaxTransferCap + 1 },
		"bitrate cap":  func(r *UpdateTranscodeSettingsRequest) { r.BitrateCapsMbps[api.Resolution720] = MaxCapMbps + 1 },
		"missing cap":  func(r *UpdateTranscodeSettingsRequest) { delete(r.BitrateCapsMbps, api.ResolutionSD) },
		"low bitrate":  func(r *UpdateTranscodeSettingsRequest) { r.LowBitratePct = MinLowBitratePct - 1 },
		"saving":       func(r *UpdateTranscodeSettingsRequest) { r.MinSavingPct = MaxSavingPct + 1 },
		"target":       func(r *UpdateTranscodeSettingsRequest) { r.TargetCodec = "H264" },
		"preset":       func(r *UpdateTranscodeSettingsRequest) { r.Preset = "ULTRA" },
		"override range": func(r *UpdateTranscodeSettingsRequest) {
			r.EncoderOverrides[EncoderSVTAV1] = calibration.Encoders[EncoderSVTAV1].Quality.Max + 1
		},
		"override encoder": func(r *UpdateTranscodeSettingsRequest) { r.EncoderOverrides["h264_nvenc"] = 20 },
	}
	for name, change := range outOfRange {
		body := valid()
		change(&body)
		if rec := c.do(admin, http.MethodPut, "/transcode/settings", body); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d, want 400", name, rec.Code)
		}
	}
	if got := decodeJSON[TranscodeSettingsResponse](t, c.do(admin, http.MethodGet, "/transcode/settings", nil)); got.TransferCap != defaultTransferCap {
		t.Fatalf("a refused update stored the transfer cap %d", got.TransferCap)
	}

	updated := valid()
	if rec := c.do(admin, http.MethodPut, "/transcode/settings", updated); rec.Code != http.StatusOK {
		t.Fatalf("update = %d: %s", rec.Code, rec.Body.String())
	}
	got := decodeJSON[TranscodeSettingsResponse](t, c.do(admin, http.MethodGet, "/transcode/settings", nil))
	if got.TransferCap != MaxTransferCap || got.DefaultSlotsPerGPU != MaxSlotsPerGPU || got.TargetCodec != api.TargetAV110 ||
		got.Preset != api.PresetSmall || got.BitrateCapsMbps[api.Resolution2160] != MaxCapMbps || got.SkipAlreadyTarget ||
		got.LowBitratePct != MaxLowBitratePct || got.EncoderOverrides[EncoderAV1NVENC] != 30 {
		t.Fatalf("settings = %+v, want %+v", got, updated)
	}
}

func TestJobEndpointsListCancelAndRetry(t *testing.T) {
	f := newFixture(t)
	queued := f.addFile(h264Movie(0))
	f.cutoff(queued, domain.OriginLibraryScan)
	c := f.api()
	job := f.onlyJob(queued)

	if rec := c.do(requester, http.MethodGet, "/transcode/jobs", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("requester list = %d, want 403", rec.Code)
	}
	list := decodeJSON[JobListResponse](t, c.do(admin, http.MethodGet, "/transcode/jobs?state=QUEUED", nil))
	if len(list.Jobs) != 1 || list.Backlog != 1 || list.Counts[api.StateQueued] != 1 {
		t.Fatalf("list = %+v", list)
	}
	if got := list.Jobs[0]; got.ID != string(job.ID) || got.TitleName != "Movie" || got.TargetCodec != api.TargetHEVC10 || got.MaxAttempts != MaxAttempts {
		t.Fatalf("job = %+v", got)
	}
	if rec := c.do(admin, http.MethodGet, "/transcode/jobs?state=NOPE", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown state = %d, want 400", rec.Code)
	}

	if rec := c.do(admin, http.MethodPost, "/transcode/jobs/"+string(job.ID)+"/retry", nil); rec.Code != http.StatusConflict {
		t.Fatalf("retry of a queued job = %d, want 409", rec.Code)
	}
	rec := c.do(admin, http.MethodPost, "/transcode/jobs/"+string(job.ID)+"/cancel", nil)
	if got := decodeJSON[JobResponse](t, rec); rec.Code != http.StatusOK || got.State != api.StateCancelled {
		t.Fatalf("cancel = %d %+v", rec.Code, got)
	}
	if rec := c.do(admin, http.MethodPost, "/transcode/jobs/"+string(job.ID)+"/cancel", nil); rec.Code != http.StatusConflict {
		t.Fatalf("second cancel = %d, want 409", rec.Code)
	}
	if rec := c.do(admin, http.MethodPost, "/transcode/jobs/"+string(domain.NewID[domain.JobID]())+"/cancel", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown job = %d, want 404", rec.Code)
	}
}

func TestJobEndpointDismissesOnlyFailedJobs(t *testing.T) {
	f := newFixture(t)
	queued := f.addFile(h264Movie(0))
	f.cutoff(queued, domain.OriginLibraryScan)
	c := f.api()
	job := f.onlyJob(queued)
	path := "/transcode/jobs/" + string(job.ID)

	if rec := c.do(admin, http.MethodDelete, path, nil); rec.Code != http.StatusConflict {
		t.Fatalf("dismiss of a queued job = %d, want 409", rec.Code)
	}
	err := f.db.Write(context.Background(), func(tx *gorm.DB) error {
		return tx.Model(&jobRow{}).Where("id = ?", job.ID).Update("state", api.StateFailed).Error
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec := c.do(requester, http.MethodDelete, path, nil); rec.Code != http.StatusForbidden {
		t.Fatalf("requester dismiss = %d, want 403", rec.Code)
	}
	if rec := c.do(admin, http.MethodDelete, path, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("dismiss = %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := jobIn(f.db.Read(context.Background()), job.ID); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("after dismiss err = %v, want ErrJobNotFound", err)
	}
	if rec := c.do(admin, http.MethodDelete, path, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("second dismiss = %d, want 404", rec.Code)
	}
}

func TestExclusionEndpointsExcludeAndInclude(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.cutoff(file, domain.OriginLibraryScan)
	c := f.api()
	path := "/transcode/exclusions/TITLE/" + string(file.Title)

	if rec := c.do(requester, http.MethodPut, path, nil); rec.Code != http.StatusForbidden {
		t.Fatalf("requester exclude = %d, want 403", rec.Code)
	}
	if rec := c.do(admin, http.MethodPut, "/transcode/exclusions/SHELF/x", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown scope = %d, want 400", rec.Code)
	}
	if rec := c.do(admin, http.MethodPut, path, nil); rec.Code != http.StatusOK {
		t.Fatalf("exclude = %d: %s", rec.Code, rec.Body.String())
	}
	listed := decodeJSON[[]ExclusionResponse](t, c.do(admin, http.MethodGet, "/transcode/exclusions", nil))
	if len(listed) != 1 || listed[0].Scope != ExcludeTitle || listed[0].Ref != string(file.Title) {
		t.Fatalf("exclusions = %+v", listed)
	}
	backlog := decodeJSON[BacklogResponse](t, c.do(admin, http.MethodGet, "/transcode/backlog", nil))
	if backlog.Backlog != 0 || backlog.Eligible != 1 || backlog.Skipped[api.SkipExcluded] != 1 {
		t.Fatalf("backlog = %+v, want one excluded file", backlog)
	}

	if rec := c.do(admin, http.MethodDelete, path, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("include = %d: %s", rec.Code, rec.Body.String())
	}
	backlog = decodeJSON[BacklogResponse](t, c.do(admin, http.MethodGet, "/transcode/backlog", nil))
	if backlog.Backlog != 1 || len(backlog.Skipped) != 0 {
		t.Fatalf("backlog = %+v, want the file queued again", backlog)
	}
}

func TestSweepEndpointRunsInTheBackground(t *testing.T) {
	f := newFixture(t)
	c := f.api()

	if rec := c.do(admin, http.MethodPost, "/transcode/backlog/sweep", nil); rec.Code != http.StatusAccepted {
		t.Fatalf("sweep = %d: %s", rec.Code, rec.Body.String())
	}
	tasks := f.queue.drain()
	if len(tasks) != 1 || tasks[0] != (sweepTask{}) {
		t.Fatalf("tasks = %+v, want one sweep", tasks)
	}
}
