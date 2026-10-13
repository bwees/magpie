package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"magpie/internal/platform/config"
	"magpie/internal/platform/problem"

	"github.com/coder/websocket"
	"github.com/go-fuego/fuego"
)

const (
	// shortServerTimeout stands in for the server's read and write timeouts, which socketIdle outlasts.
	shortServerTimeout = 100 * time.Millisecond
	socketIdle         = 3 * shortServerTimeout
	echoMessage        = "still here"
)

type failingController struct{ err error }

func (c failingController) Register(api *fuego.Server) {
	fuego.Get(api, "/fail", func(fuego.ContextNoBody) (HealthResponse, error) {
		return HealthResponse{}, c.err
	}, fuego.OptionOperationID("fail"))
}

type statusController struct {
	status int
	body   any
	err    error
}

func (c statusController) Register(api *fuego.Server) {
	fuego.Get(api, "/status", func(fuego.ContextNoBody) (any, error) {
		return c.body, c.err
	}, fuego.OptionOperationID("status"), fuego.OptionDefaultStatusCode(c.status))
}

type socketController struct{}

func (socketController) Register(api *fuego.Server) {
	fuego.Handle(api, "/socket", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}), fuego.OptionHide())
}

type echoSocketController struct{}

func (echoSocketController) Register(api *fuego.Server) {
	fuego.Handle(api, "/echo", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		kind, message, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		_ = conn.Write(r.Context(), kind, message)
	}), fuego.OptionHide())
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

func TestNonOKDefaultStatusSendsJSON(t *testing.T) {
	for _, status := range []int{http.StatusCreated, http.StatusAccepted} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			controller := statusController{status: status, body: HealthResponse{Status: HealthStatusOK}}

			rec := get(t, newTestServer(t, controller), "/api/status")

			if rec.Code != status {
				t.Fatalf("status = %d, want %d", rec.Code, status)
			}
			if got := rec.Result().Header.Get("Content-Type"); got != jsonContent {
				t.Fatalf("content type = %q, want %q", got, jsonContent)
			}
			var body HealthResponse
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.Status != HealthStatusOK {
				t.Errorf("status = %q, want %q", body.Status, HealthStatusOK)
			}
		})
	}
}

func TestNoContentDefaultStatusIsSent(t *testing.T) {
	controller := statusController{status: http.StatusNoContent}

	rec := get(t, newTestServer(t, controller), "/api/status")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("body = %q, want empty", rec.Body)
	}
}

func TestWebSocketUpgradeGoesThrough(t *testing.T) {
	server := httptest.NewServer(newTestServer(t, socketController{}))
	t.Cleanup(server.Close)

	conn, _, err := websocket.Dial(t.Context(), "ws"+strings.TrimPrefix(server.URL, "http")+"/api/socket", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = conn.CloseNow()
}

func TestServerBoundsHeadersBodiesAndResponses(t *testing.T) {
	built := Build(config.Env{}, ModeServe, nil)

	if built.ReadHeaderTimeout != readHeaderTimeout || built.ReadTimeout != readTimeout || built.WriteTimeout != writeTimeout {
		t.Errorf("timeouts = %v/%v/%v, want %v/%v/%v", built.ReadHeaderTimeout, built.ReadTimeout, built.WriteTimeout,
			readHeaderTimeout, readTimeout, writeTimeout)
	}
}

func TestWebSocketOutlastsTheServerTimeouts(t *testing.T) {
	server := httptest.NewUnstartedServer(newTestServer(t, echoSocketController{}))
	server.Config.ReadTimeout = shortServerTimeout
	server.Config.WriteTimeout = shortServerTimeout
	server.Start()
	t.Cleanup(server.Close)

	conn, _, err := websocket.Dial(t.Context(), "ws"+strings.TrimPrefix(server.URL, "http")+"/api/echo", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.CloseNow() }()
	time.Sleep(socketIdle)

	if err := conn.Write(t.Context(), websocket.MessageText, []byte(echoMessage)); err != nil {
		t.Fatalf("write after %v: %v", socketIdle, err)
	}
	_, got, err := conn.Read(t.Context())
	if err != nil || string(got) != echoMessage {
		t.Fatalf("echo after %v = %q, %v", socketIdle, got, err)
	}
}

func TestErrorOnCreatedRouteIsProblemJSON(t *testing.T) {
	controller := statusController{status: http.StatusCreated, err: fuego.NotFoundError{Detail: "missing"}}

	rec := get(t, newTestServer(t, controller), "/api/status")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if body := decodeProblem(t, rec); body.Type != string(problem.TypeNotFound) {
		t.Errorf("type = %q, want %q", body.Type, problem.TypeNotFound)
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
