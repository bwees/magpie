package subtitles_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"magpie/internal/domain"
	"magpie/internal/platform/config"
	"magpie/internal/platform/httpserver"
	"magpie/internal/subtitles"
	"magpie/internal/subtitles/api"
)

func (h *harness) serve(admin bool) http.Handler {
	mux := httpserver.Build(config.Env{}, httpserver.ModeServe, []httpserver.Controller{subtitles.NewController(h.service)}).Mux
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(domain.WithPrincipal(r.Context(), domain.Principal{User: domain.NewID[domain.UserID](), Admin: admin}))
		mux.ServeHTTP(w, r)
	})
}

func call[T any](t *testing.T, handler http.Handler, method, path string, body any, wantStatus int) T {
	t.Helper()
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &payload)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("%s %s = %d %s, want %d", method, path, rec.Code, rec.Body, wantStatus)
	}
	var out T
	if rec.Body.Len() > 0 && wantStatus < http.StatusBadRequest {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func TestFileSubtitlesCandidatesAndDownloadOverHTTP(t *testing.T) {
	h := newHarness(t, english)
	file := h.episodeFile(spanishImage())
	h.imported(file)
	h.runDue()
	admin := h.serve(true)
	base := "/api/subtitles/files/" + string(file.ID)

	got := call[subtitles.FileSubtitlesResponse](t, admin, http.MethodGet, base, nil, http.StatusOK)
	if len(got.Subtitles) != 1 || got.Subtitles[0].Source != api.SourceEmbedded || len(got.Coverage) != 1 ||
		got.Coverage[0].Status != api.CoverageMissing || got.Coverage[0].Reason != api.MissingNoMatch {
		t.Fatalf("file = %+v, want the Spanish image track and English missing", got)
	}

	h.os.AddCandidates(h.episodeQuery(english), perfectEpisode(40, english))
	h.os.SetContent(40, srt)
	cands := call[[]subtitles.CandidateResponse](t, admin, http.MethodGet, base+"/candidates?language=en", nil, http.StatusOK)
	if len(cands) != 1 || cands[0].Score != 100 || !cands[0].Acceptable || !cands[0].HashMatch {
		t.Fatalf("candidates = %+v, want one perfect match", cands)
	}
	got = call[subtitles.FileSubtitlesResponse](t, admin, http.MethodPost, base+"/download",
		subtitles.DownloadSubtitleRequest{FileID: 40, Language: english}, http.StatusOK)
	if got.Coverage[0].Status != api.CoverageSatisfied || len(got.Subtitles) != 2 {
		t.Fatalf("after download = %+v, want English satisfied", got)
	}

	call[any](t, admin, http.MethodGet, base+"/candidates?language=klingon", nil, http.StatusBadRequest)
	call[any](t, admin, http.MethodGet, "/api/subtitles/files/"+string(domain.NewID[domain.MediaFileID]())+"/candidates?language=en", nil, http.StatusNotFound)
	call[any](t, h.serve(false), http.MethodGet, base, nil, http.StatusForbidden)

	var embedded subtitles.SubtitleID
	for _, sub := range got.Subtitles {
		if sub.Source == api.SourceEmbedded {
			embedded = sub.ID
		}
	}
	call[any](t, admin, http.MethodDelete, "/api/subtitles/"+string(embedded), nil, http.StatusConflict)
}

func TestSettingsAndQuotaOverHTTP(t *testing.T) {
	h := newHarness(t, english)
	admin := h.serve(true)

	cfg := call[subtitles.SubtitleSettingsBody](t, admin, http.MethodGet, "/api/subtitles/settings", nil, http.StatusOK)
	want := subtitles.SubtitleSettingsBody{
		WhisperEnabled: true, WhisperModel: "large-v3-turbo", AutoSync: true, MinScoreEpisode: 90, MinScoreMovie: 70,
		UpgradeWindowDays: 7, UpgradeCheckHours: 6,
	}
	if cfg != want {
		t.Fatalf("defaults = %+v, want %+v", cfg, want)
	}
	cfg.WhisperModel, cfg.MinScoreMovie, cfg.AutoSync = "medium", 80, false
	if saved := call[subtitles.SubtitleSettingsBody](t, admin, http.MethodPut, "/api/subtitles/settings", cfg, http.StatusOK); saved != cfg {
		t.Errorf("saved = %+v, want %+v", saved, cfg)
	}
	bad := cfg
	bad.MinScoreEpisode = 101
	call[any](t, admin, http.MethodPut, "/api/subtitles/settings", bad, http.StatusBadRequest)
	bad = cfg
	bad.WhisperModel = "tiny"
	call[any](t, admin, http.MethodPut, "/api/subtitles/settings", bad, http.StatusBadRequest)

	quota := call[subtitles.QuotaResponse](t, admin, http.MethodGet, "/api/subtitles/quota", nil, http.StatusOK)
	if quota.Remaining != nil || quota.Problem != "" {
		t.Errorf("quota before any OpenSubtitles call = %+v, want unknown", quota)
	}
}
