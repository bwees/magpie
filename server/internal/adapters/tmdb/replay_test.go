package tmdb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

const (
	replayAPIKey   = "0123456789abcdef0123456789abcdef"
	replayDir      = "testdata"
	headerJSONType = "application/json;charset=utf-8"
	headerRetry    = "Retry-After"
)

// response is one recorded TMDB answer.
type response struct {
	status  int
	fixture string // file in testdata; "" sends no body
	header  map[string]string
}

func ok(fixture string) response { return response{status: http.StatusOK, fixture: fixture} }

// replay serves recorded fixtures by URL path. Each path answers its responses in order and repeats the last.
type replay struct {
	t      *testing.T
	mu     sync.Mutex
	routes map[string][]response
	seen   []*http.Request
	server *httptest.Server
}

func newReplay(t *testing.T, routes map[string][]response) *replay {
	t.Helper()
	r := &replay{t: t, routes: routes}
	r.server = httptest.NewServer(http.HandlerFunc(r.serve))
	t.Cleanup(r.server.Close)
	return r
}

func (r *replay) serve(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	r.seen = append(r.seen, req)
	queue, found := r.routes[req.URL.Path]
	var next response
	if found {
		next = queue[0]
		if len(queue) > 1 {
			r.routes[req.URL.Path] = queue[1:]
		}
	}
	r.mu.Unlock()

	if !found {
		r.t.Errorf("unexpected request %s", req.URL)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	for name, value := range next.header {
		w.Header().Set(name, value)
	}
	w.Header().Set("Content-Type", headerJSONType)
	w.WriteHeader(next.status)
	if next.fixture == "" {
		return
	}
	body, err := os.ReadFile(filepath.Join(replayDir,next.fixture))
	if err != nil {
		r.t.Errorf("reading fixture: %v", err)
		return
	}
	_, _ = w.Write(body)
}

func (r *replay) requests() []*http.Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*http.Request(nil), r.seen...)
}

// sleeps records every wait the client asks for without waiting.
type sleeps struct {
	mu    sync.Mutex
	waits []time.Duration
}

func (s *sleeps) sleep(_ context.Context, d time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.waits = append(s.waits, d)
	return nil
}

func (s *sleeps) all() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]time.Duration(nil), s.waits...)
}

func newTestClient(r *replay, key string, s *sleeps) *Client {
	return NewClient(Config{
		APIKey:       func(context.Context) (string, error) { return key, nil },
		BaseURL:      r.server.URL,
		ImageBaseURL: r.server.URL + imagePathPrefix,
		HTTP:         r.server.Client(),
		Sleep:        s.sleep,
	})
}

// imagePathPrefix stands in for image.tmdb.org's "/t/p" in tests.
const imagePathPrefix = "/t/p"
