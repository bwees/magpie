package library_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"magpie/internal/domain"
	"magpie/internal/library"
	"magpie/internal/library/api"
	"magpie/internal/library/api/contracttest"
	"magpie/internal/platform/config"
	"magpie/internal/platform/db/dbtest"
	"magpie/internal/platform/httpserver"
)

const (
	batchMoviesRoot = "/media/movies"
	batchFileSize   = 1000
	// tooManyTitles is one over the batch limit.
	tooManyTitles = 501
)

func batchFile(title domain.TitleID, transcoded bool, episodes ...domain.EpisodeRef) contracttest.StoredFile {
	id := domain.NewID[domain.MediaFileID]()
	return contracttest.StoredFile{File: api.MediaFile{
		ID: id, Title: title, Episodes: append([]domain.EpisodeRef{}, episodes...), RootFolder: batchMoviesRoot,
		Path: batchMoviesRoot + "/" + string(id) + ".mkv", Size: batchFileSize, Origin: domain.OriginDownload,
		Transcoded: transcoded, ImportedAt: time.Now().UTC(),
	}}
}

func serveLibrary(t *testing.T, admin bool, files ...contracttest.StoredFile) http.Handler {
	t.Helper()
	d := dbtest.Open(t, library.Migrations())
	if err := library.SeedFiles(context.Background(), d, files); err != nil {
		t.Fatal(err)
	}
	service := library.NewService(library.Deps{DB: d, Repo: library.NewRepository(d)})
	mux := httpserver.Build(config.Env{}, httpserver.ModeServe, []httpserver.Controller{library.NewController(service)}).Mux
	principal := domain.Principal{User: domain.NewID[domain.UserID](), Admin: admin}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(domain.WithPrincipal(r.Context(), principal)))
	})
}

func getJSON[T any](t *testing.T, h http.Handler, path string, wantStatus int) T {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != wantStatus {
		t.Fatalf("GET %s = %d %s, want %d", path, rec.Code, rec.Body, wantStatus)
	}
	var body T
	if wantStatus == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
	}
	return body
}

func TestBatchFilesListsTitlesInOrderAndFiltersOnTranscoded(t *testing.T) {
	first, second := domain.NewID[domain.TitleID](), domain.NewID[domain.TitleID]()
	plain, done := batchFile(first, false), batchFile(second, true)
	h := serveLibrary(t, true, plain, done)

	all := getJSON[[]library.MediaFileResponse](t, h, fmt.Sprintf("/api/library/files?titles=%s&titles=%s", second, first), http.StatusOK)
	if len(all) != 2 || all[0].ID != done.File.ID || all[1].ID != plain.File.ID {
		t.Errorf("files = %+v, want the second title's file then the first's", all)
	}
	pending := getJSON[[]library.MediaFileResponse](t, h, fmt.Sprintf("/api/library/files?titles=%s&titles=%s&transcoded=false", first, second), http.StatusOK)
	if len(pending) != 1 || pending[0].ID != plain.File.ID {
		t.Errorf("not transcoded = %+v, want only the plain file", pending)
	}
}

func TestBatchFilesNeedsExactlyOneTitleSelector(t *testing.T) {
	h := serveLibrary(t, true)
	title := domain.NewID[domain.TitleID]()
	getJSON[any](t, h, "/api/library/files", http.StatusBadRequest)
	getJSON[any](t, h, fmt.Sprintf("/api/library/files?title=%s&titles=%s", title, title), http.StatusBadRequest)
	getJSON[any](t, h, fmt.Sprintf("/api/library/files?titles=%s&transcoded=maybe", title), http.StatusBadRequest)

	tooMany := strings.Repeat("titles=x&", tooManyTitles)
	getJSON[any](t, h, "/api/library/files?"+tooMany, http.StatusBadRequest)
	getJSON[any](t, h, "/api/library/coverage?"+tooMany, http.StatusBadRequest)
}

func TestCoverageCountsEpisodesPerTitleForAdminsOnly(t *testing.T) {
	series := domain.NewID[domain.TitleID]()
	episode := domain.EpisodeRef{Episode: domain.NewID[domain.EpisodeID](), Season: 1, Number: 1}
	empty := domain.NewID[domain.TitleID]()
	file := batchFile(series, false, episode)

	coverage := getJSON[[]library.TitleCoverageResponse](t, serveLibrary(t, true, file),
		fmt.Sprintf("/api/library/coverage?titles=%s&titles=%s", series, empty), http.StatusOK)
	want := []library.TitleCoverageResponse{{Title: series, EpisodesWithFile: 1, SizeBytes: batchFileSize}, {Title: empty}}
	if len(coverage) != len(want) || coverage[0] != want[0] || coverage[1] != want[1] {
		t.Errorf("coverage = %+v, want %+v", coverage, want)
	}
	getJSON[any](t, serveLibrary(t, false), "/api/library/coverage?titles="+string(series), http.StatusForbidden)
}
