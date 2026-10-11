package flows_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	identityapi "magpie/internal/identity/api"
	"magpie/internal/platform/connections"
	"magpie/internal/platform/health"

	"go.uber.org/fx"
)

const (
	mePath          = "/api/identity/me"
	backupsPath     = "/api/platform/backups"
	connectionsPath = "/api/platform/connections"
)

func TestFirstRunGatesTheAPIUntilTheLocalAdminExists(t *testing.T) {
	var reader identityapi.Reader
	h := boot(t, fx.Populate(&reader))

	for _, path := range []string{mePath, backupsPath} {
		if rec := h.get(path); rec.Code != http.StatusConflict {
			t.Errorf("%s before setup = %d, want 409", path, rec.Code)
		}
	}

	h.signInAsAdmin()
	for _, path := range []string{mePath, backupsPath, connectionsPath} {
		if rec := h.get(path); rec.Code != http.StatusOK {
			t.Errorf("%s as the local admin = %d, want 200: %s", path, rec.Code, rec.Body)
		}
	}

	rec := h.do(http.MethodPost, connectionsPath+"/tmdb/test", nil)
	var tested connections.ServiceHealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&tested); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("test TMDB = %d, %v", rec.Code, err)
	}
	if tested.Status != health.StatusNotConfigured {
		t.Errorf("unconfigured TMDB tests as %s, want %s", tested.Status, health.StatusNotConfigured)
	}

	admins, err := reader.Admins(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(admins) != 1 || !admins[0].LocalAdmin {
		t.Errorf("identity.Reader admins = %+v, want the local admin", admins)
	}
}
