package flows_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"magpie/internal/platform/migrate"
)

const migrationPath = "/api/platform/migration"

// sonarrAt answers system/status as the given Sonarr version and lists one root folder.
func sonarrAt(t *testing.T, version string) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/system/status":
			_ = json.NewEncoder(w).Encode(map[string]string{"appName": "Sonarr", "version": version})
		case "/api/v3/rootfolder":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 1, "path": "/tv"}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func TestTheImportWizardConnectsSonarr4AndRefusesSonarr3(t *testing.T) {
	h := boot(t)
	h.signInAsAdmin()

	rec := h.get(migrationPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET migration = %d: %s", rec.Code, rec.Body)
	}
	var state migrate.MigrationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &state); err != nil || state.Phase != migrate.PhaseSetup || state.CanCommit {
		t.Fatalf("state = %+v, %v; want SETUP", state, err)
	}

	old := migrate.MigrationConnectRequest{App: migrate.AppSonarr, URL: sonarrAt(t, "3.0.10.1567"), APIKey: "key"}
	if rec := h.do(http.MethodPut, migrationPath+"/connections", old); rec.Code != http.StatusBadRequest {
		t.Fatalf("Sonarr 3 = %d: %s, want 400", rec.Code, rec.Body)
	}

	current := migrate.MigrationConnectRequest{App: migrate.AppSonarr, URL: sonarrAt(t, "4.0.20.3014"), APIKey: "key"}
	rec = h.do(http.MethodPut, migrationPath+"/connections", current)
	if rec.Code != http.StatusOK {
		t.Fatalf("Sonarr 4 = %d: %s", rec.Code, rec.Body)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Sources) != 1 || state.Sources[0].Version != "4.0.20.3014" || len(state.Paths) != 1 || state.Paths[0].From != "/tv" {
		t.Fatalf("state = %+v, want Sonarr 4 connected with its /tv root to map", state)
	}
	if rec := h.do(http.MethodPost, migrationPath+"/commit", nil); rec.Code != http.StatusConflict {
		t.Fatalf("commit before a dry run = %d, want 409", rec.Code)
	}
}
