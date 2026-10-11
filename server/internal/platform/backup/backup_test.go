package backup

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"magpie/internal/domain"
	"magpie/internal/platform/config"
	"magpie/internal/platform/db"
	"magpie/internal/platform/db/dbtest"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/jobs"
	"magpie/internal/platform/settings"
)

type fakeRestarter struct{ restarts int }

func (r *fakeRestarter) Restart() error {
	r.restarts++
	return nil
}

type fixture struct {
	service   *Service
	restarter *fakeRestarter
	store     *settings.Store
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	d := dbtest.Open(t)
	store := settings.NewStore(d)
	r := &fakeRestarter{}
	return fixture{service: NewService(d, store, r, filepath.Join(t.TempDir(), db.BackupDirName)), restarter: r, store: store}
}

func TestDailyKeepsConfiguredCount(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := SettingsKey.Set(ctx, f.store, Settings{Enabled: true, Time: DefaultTime, Keep: 2}); err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 10, 11, 3, 0, 0, 0, time.UTC)

	for offset := range 3 {
		f.service.now = func() time.Time { return day.AddDate(0, 0, offset) }
		if _, err := f.service.Daily(ctx); err != nil {
			t.Fatal(err)
		}
	}

	backups, err := f.service.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 2 {
		t.Fatalf("backups = %d, want 2", len(backups))
	}
	for _, b := range backups {
		if b.Name == db.BackupName(db.BackupKindDaily, day) {
			t.Errorf("oldest backup %s was kept", b.Name)
		}
	}
}

func TestScheduleFollowsSettings(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	periodic := newPeriodic(f.service)

	schedule, err := periodic.Schedule(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if schedule != (jobs.DailyAt{Hour: 3}) {
		t.Errorf("default schedule = %+v, want daily at 03:00", schedule)
	}

	if err := SettingsKey.Set(ctx, f.store, Settings{Enabled: false, Time: DefaultTime, Keep: DefaultKeep}); err != nil {
		t.Fatal(err)
	}
	if schedule, err := periodic.Schedule(ctx); err != nil || schedule != nil {
		t.Errorf("disabled schedule = %v, %v; want paused", schedule, err)
	}
}

func TestSettingsValidation(t *testing.T) {
	cases := []Settings{
		{Time: DefaultTime, Keep: 0},
		{Time: DefaultTime, Keep: MaxKeep + 1},
		{Time: "25:00", Keep: DefaultKeep},
		{Time: "3am", Keep: DefaultKeep},
	}
	for _, s := range cases {
		if _, err := s.Validate(); !errors.Is(err, ErrInvalidSettings) {
			t.Errorf("Validate(%+v) = %v, want ErrInvalidSettings", s, err)
		}
	}
}

func serve(t *testing.T, s *Service, principal *domain.Principal) http.Handler {
	t.Helper()
	mux := httpserver.Build(config.Env{}, httpserver.ModeServe, []httpserver.Controller{NewController(s)}).Mux
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if principal != nil {
			r = r.WithContext(domain.WithPrincipal(r.Context(), *principal))
		}
		mux.ServeHTTP(w, r)
	})
}

func do(handler http.Handler, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestEndpointsAreAdminOnly(t *testing.T) {
	f := newFixture(t)
	requester := &domain.Principal{User: domain.NewID[domain.UserID]()}
	cases := map[string]struct {
		principal *domain.Principal
		want      int
	}{
		"anonymous": {nil, http.StatusUnauthorized},
		"requester": {requester, http.StatusForbidden},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			handler := serve(t, f.service, tc.principal)
			if rec := do(handler, http.MethodGet, "/api/platform/backups"); rec.Code != tc.want {
				t.Errorf("list status = %d, want %d", rec.Code, tc.want)
			}
			if rec := do(handler, http.MethodPost, "/api/platform/backups/magpie-2026-10-11.db/restore"); rec.Code != tc.want {
				t.Errorf("restore status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
	if f.restarter.restarts != 0 {
		t.Error("a non-admin triggered a restart")
	}
}

func TestAdminListsAndRestores(t *testing.T) {
	f := newFixture(t)
	backup, err := f.service.Daily(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	handler := serve(t, f.service, &domain.Principal{User: domain.NewID[domain.UserID](), Admin: true})

	rec := do(handler, http.MethodGet, "/api/platform/backups")
	var listed []BackupResponse
	if err := json.NewDecoder(rec.Body).Decode(&listed); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("list: %d %v", rec.Code, err)
	}
	if len(listed) != 1 || listed[0].Name != backup.Name || listed[0].Kind != db.BackupKindDaily {
		t.Fatalf("listed %+v", listed)
	}

	if rec := do(handler, http.MethodPost, "/api/platform/backups/magpie-1999-01-01.db/restore"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown backup status = %d, want 404", rec.Code)
	}

	rec = do(handler, http.MethodPost, "/api/platform/backups/"+backup.Name+"/restore")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("restore status = %d, want 202: %s", rec.Code, rec.Body)
	}
	if f.restarter.restarts != 1 {
		t.Errorf("restarts = %d, want 1", f.restarter.restarts)
	}
	if _, err := os.Stat(f.service.db.Path() + ".restore"); err != nil {
		t.Errorf("restore not staged: %v", err)
	}
}
