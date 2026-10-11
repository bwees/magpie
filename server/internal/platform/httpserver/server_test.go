package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"magpie/internal/platform/config"
	"magpie/internal/platform/problem"

	"github.com/go-fuego/fuego"
)

type failingController struct{ err error }

func (c failingController) Register(api *fuego.Server) {
	fuego.Get(api, "/fail", func(fuego.ContextNoBody) (HealthResponse, error) {
		return HealthResponse{}, c.err
	}, fuego.OptionOperationID("fail"))
}

func newTestServer(t *testing.T, extra ...Controller) http.Handler {
	t.Helper()
	all := append([]Controller{NewHealthController()}, extra...)
	return Build(config.Env{}, ModeServe, all).Mux
}

func get(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func decodeProblem(t *testing.T, rec *httptest.ResponseRecorder) fuego.HTTPError {
	t.Helper()
	if got := rec.Result().Header.Get("Content-Type"); got != problem.ContentType {
		t.Fatalf("content type = %q, want %q", got, problem.ContentType)
	}
	var body fuego.HTTPError
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	return body
}

func TestHealth(t *testing.T) {
	rec := get(t, newTestServer(t), "/api/health")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body HealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != HealthStatusOK {
		t.Errorf("status = %q, want %q", body.Status, HealthStatusOK)
	}
}

func TestHealthHasStableOperationID(t *testing.T) {
	server := Build(config.Env{}, ModeServe, []Controller{NewHealthController()})

	path := server.OpenAPI.Description().Paths.Find("/api/health")
	if path == nil || path.Get == nil {
		t.Fatal("GET /api/health missing from the OpenAPI spec")
	}
	if path.Get.OperationID != "getHealth" {
		t.Errorf("operation ID = %q, want getHealth", path.Get.OperationID)
	}
}

func TestUnknownAPIRouteIsProblemJSON(t *testing.T) {
	rec := get(t, newTestServer(t), "/api/does-not-exist")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	body := decodeProblem(t, rec)
	if body.Type != string(problem.TypeNotFound) {
		t.Errorf("type = %q, want %q", body.Type, problem.TypeNotFound)
	}
	if body.Status != http.StatusNotFound {
		t.Errorf("problem status = %d, want 404", body.Status)
	}
}

func TestControllerProblemKeepsItsType(t *testing.T) {
	const conflictType problem.Type = "urn:magpie:problem:example-conflict"
	controller := failingController{err: problem.New(conflictType, http.StatusConflict, "already exists")}

	rec := get(t, newTestServer(t, controller), "/api/fail")

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	body := decodeProblem(t, rec)
	if body.Type != string(conflictType) || body.Detail != "already exists" {
		t.Errorf("problem = %+v", body)
	}
}

func TestPlainErrorBecomesInternalProblemWithoutLeaking(t *testing.T) {
	const secret = "connection string with password"
	controller := failingController{err: errors.New(secret)}

	rec := get(t, newTestServer(t, controller), "/api/fail")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	body := decodeProblem(t, rec)
	if body.Type != string(problem.TypeInternal) {
		t.Errorf("type = %q, want %q", body.Type, problem.TypeInternal)
	}
	if body.Detail != "" {
		t.Errorf("detail leaked %q", body.Detail)
	}
}

func TestFuegoErrorGetsTypeFromStatus(t *testing.T) {
	controller := failingController{err: fuego.NotFoundError{Detail: "missing"}}

	rec := get(t, newTestServer(t, controller), "/api/fail")

	body := decodeProblem(t, rec)
	if body.Type != string(problem.TypeNotFound) {
		t.Errorf("type = %q, want %q", body.Type, problem.TypeNotFound)
	}
}
