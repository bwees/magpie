package library_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	acquisitionapi "magpie/internal/acquisition/api"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	"magpie/internal/library"
	"magpie/internal/library/api"
	"magpie/internal/platform/config"
	"magpie/internal/platform/httpserver"
)

func (f *importFixture) serve(admin bool) http.Handler {
	mux := httpserver.Build(config.Env{}, httpserver.ModeServe, []httpserver.Controller{library.NewController(f.service)}).Mux
	principal := domain.Principal{User: domain.NewID[domain.UserID](), Admin: admin}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(domain.WithPrincipal(r.Context(), principal)))
	})
}

func do(h http.Handler, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func (f *importFixture) deleted(t *testing.T) []api.FileDeleted {
	t.Helper()
	var out []api.FileDeleted
	for _, ev := range f.events.ofType(api.TypeFileDeleted) {
		out = append(out, payload[api.FileDeleted](t, ev))
	}
	return out
}

func (f *importFixture) importTwoEpisodes(t *testing.T) []api.FileImported {
	t.Helper()
	f.importEpisode(t, oldEpisodeRelease, oldEpisodeRelease+".mkv", f.episodes[0].ID)
	f.importEpisode(t, "Show.2020.S01E02.720p.HDTV.x264-OLD", "Show.2020.S01E02.720p.HDTV.x264-OLD.mkv", f.episodes[1].ID)
	return f.imported(t)
}

func (f *importFixture) removeTitle(t *testing.T, deleteFiles bool) {
	t.Helper()
	removed := catalogapi.TitleRemoved{Title: f.show.ID, DeleteFiles: deleteFiles}
	if err := library.Handle(context.Background(), f.db, library.NewTitleRemovedHandler(f.service), removed); err != nil {
		t.Fatal(err)
	}
	f.runJobs(t)
}

func TestAnAdminDeletesAFileFromDisk(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	f.download(t, "Dune.2021.1080p.mkv", 2*time.Hour)
	f.deliver(t, f.movieDownload(acquisitionapi.SeedUntilGoals, "Dune.2021.1080p.mkv"))
	file := f.imported(t)[0]
	path := "/api/library/files/" + string(file.MediaFile)

	if rec := do(f.serve(false), http.MethodDelete, path); rec.Code != http.StatusForbidden {
		t.Errorf("requester delete = %d, want 403", rec.Code)
	}
	if rec := do(f.serve(true), http.MethodDelete, path); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d %s", rec.Code, rec.Body)
	}
	if f.files.Exists(file.Path) {
		t.Error("the file is still on disk")
	}
	if got := f.deleted(t); len(got) != 1 || got[0].MediaFile != file.MediaFile || got[0].Reason != api.DeleteRemoved {
		t.Errorf("FileDeleted = %+v, want one REMOVED", got)
	}
	if rec := do(f.serve(true), http.MethodDelete, path); rec.Code != http.StatusNotFound {
		t.Errorf("second delete = %d, want 404", rec.Code)
	}
}

// Seed library #17.
func TestRemovingATitleWithoutItsFilesUnlinksThem(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	imported := f.importTwoEpisodes(t)

	f.removeTitle(t, false)

	for _, file := range imported {
		if !f.files.Exists(file.Path) {
			t.Errorf("%s was deleted from disk", file.Path)
		}
	}
	if files, err := f.repo.FilesForTitle(context.Background(), f.show.ID); err != nil || len(files) != 0 {
		t.Errorf("files = %+v, %v; want no records", files, err)
	}
	got := f.deleted(t)
	if len(got) != len(imported) {
		t.Fatalf("FileDeleted = %+v, want one per file", got)
	}
	for _, d := range got {
		if d.Reason != api.DeleteTitleRemoved {
			t.Errorf("reason = %s, want TITLE_REMOVED", d.Reason)
		}
	}
}

func TestRemovingATitleWithItsFilesDeletesThemAndSkipsMissingOnes(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	imported := f.importTwoEpisodes(t)
	if err := f.files.Remove(imported[0].Path); err != nil {
		t.Fatal(err)
	}

	f.removeTitle(t, true)
	f.removeTitle(t, true)

	for _, file := range imported {
		if f.files.Exists(file.Path) {
			t.Errorf("%s is still on disk", file.Path)
		}
	}
	if got := f.deleted(t); len(got) != len(imported) {
		t.Errorf("FileDeleted = %d, want one per file however often the title is removed", len(got))
	}
}
