package live

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"magpie/internal/domain"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/config"
	"magpie/internal/platform/httpserver"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const (
	headerTestUser  = "X-Test-User"
	headerTestAdmin = "X-Test-Admin"
	waitForMessage  = 2 * time.Second
	quietPeriod     = 100 * time.Millisecond
	fastHeartbeat   = 30 * time.Millisecond

	typeNotificationCreated = "activity.NotificationCreated"
	typeSearchResults       = "acquisition.SearchResultsReceived"
	typeFileImported        = "library.FileImported"
)

type notificationCreated struct {
	User  domain.UserID   `json:"user"`
	Title *domain.TitleID `json:"title,omitempty"`
	Text  string          `json:"text"`
}

type searchResults struct {
	Search  domain.SearchID `json:"search"`
	Indexer string          `json:"indexer"`
}

type fileImported struct {
	Title domain.TitleID `json:"title"`
}

var (
	admin     = domain.Principal{User: domain.NewID[domain.UserID](), Admin: true}
	requester = domain.Principal{User: domain.NewID[domain.UserID]()}
	stranger  = domain.NewID[domain.UserID]()
)

func testConfig() Config {
	cfg := DefaultConfig
	cfg.HeartbeatInterval = time.Hour
	return cfg
}

// serve mounts the hub behind a stand-in for Identity's middleware that reads the principal from headers.
func serve(t *testing.T, h *Hub) string {
	t.Helper()
	mux := httpserver.Build(config.Env{}, httpserver.ModeServe, []httpserver.Controller{NewController(h)}).Mux
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user := r.Header.Get(headerTestUser); user != "" {
			p := domain.Principal{User: domain.UserID(user), Admin: r.Header.Get(headerTestAdmin) != ""}
			r = r.WithContext(domain.WithPrincipal(r.Context(), p))
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(func() {
		h.Close()
		srv.Close()
	})
	return "ws" + strings.TrimPrefix(srv.URL, "http") + httpserver.APIPrefix + Path
}

type socket struct {
	t        *testing.T
	conn     *websocket.Conn
	messages chan LiveMessage
	closed   chan error
}

func dial(t *testing.T, url string, p domain.Principal) *socket {
	t.Helper()
	header := http.Header{headerTestUser: {string(p.User)}}
	if p.Admin {
		header.Set(headerTestAdmin, "1")
	}
	conn, _, err := websocket.Dial(context.Background(), url, &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	s := &socket{t: t, conn: conn, messages: make(chan LiveMessage, 64), closed: make(chan error, 1)}
	go func() {
		for {
			var msg LiveMessage
			if err := wsjson.Read(context.Background(), conn, &msg); err != nil {
				s.closed <- err
				return
			}
			s.messages <- msg
		}
	}()
	t.Cleanup(func() { _ = conn.CloseNow() })
	return s
}

func (s *socket) send(msg LiveClientMessage) {
	s.t.Helper()
	if err := wsjson.Write(context.Background(), s.conn, msg); err != nil {
		s.t.Fatalf("send: %v", err)
	}
}

// next returns the next non-heartbeat message.
func (s *socket) next() LiveMessage {
	s.t.Helper()
	for {
		select {
		case msg := <-s.messages:
			if msg.Type != TypeHeartbeat {
				return msg
			}
		case err := <-s.closed:
			s.t.Fatalf("socket closed while waiting: %v", err)
		case <-time.After(waitForMessage):
			s.t.Fatal("no message")
		}
	}
}

func (s *socket) expectQuiet() {
	s.t.Helper()
	select {
	case msg := <-s.messages:
		s.t.Fatalf("unexpected message %+v", msg)
	case <-time.After(quietPeriod):
	}
}

// attach resumes from a stale epoch and waits for the reset that confirms the socket is attached.
func (s *socket) attach() LiveMessage {
	s.t.Helper()
	s.send(LiveClientMessage{Op: OpResume, Epoch: "stale"})
	msg := s.next()
	if msg.Type != TypeReset {
		s.t.Fatalf("first message %q, want %q", msg.Type, TypeReset)
	}
	return msg
}

func publish(h *Hub, eventType, aggregate string, payload any) {
	h.Publish(bus.NewEvent(eventType, aggregate, payload))
}

func decode[T any](t *testing.T, msg LiveMessage) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(msg.Data, &v); err != nil {
		t.Fatalf("decode %s: %v", msg.Type, err)
	}
	return v
}

func TestRequiresSignedInUser(t *testing.T) {
	url := serve(t, newHub(testConfig(), RoleFilter{}))
	_, resp, err := websocket.Dial(context.Background(), url, nil)
	if err == nil {
		t.Fatal("anonymous socket connected")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("response = %v, want 401", resp)
	}
}

func TestResumeReplaysMissedMessages(t *testing.T) {
	h := newHub(testConfig(), RoleFilter{})
	url := serve(t, h)
	publish(h, typeFileImported, "t1", fileImported{Title: "t1"})
	publish(h, typeFileImported, "t2", fileImported{Title: "t2"})

	s := dial(t, url, admin)
	s.send(LiveClientMessage{Op: OpResume, Epoch: h.epoch, Seq: 1})

	replayed := s.next()
	if replayed.Seq != 2 || decode[fileImported](t, replayed).Title != "t2" {
		t.Fatalf("replayed %+v, want seq 2", replayed)
	}
	publish(h, typeFileImported, "t3", fileImported{Title: "t3"})
	if live := s.next(); live.Seq != 3 || live.Epoch != h.epoch {
		t.Fatalf("live message %+v, want seq 3 in epoch %s", live, h.epoch)
	}
}

func TestResumeFromOtherEpochResets(t *testing.T) {
	h := newHub(testConfig(), RoleFilter{})
	url := serve(t, h)
	publish(h, typeFileImported, "t1", fileImported{Title: "t1"})

	for name, epoch := range map[string]string{"previous process": "01OLDEPOCH", "first connect": ""} {
		t.Run(name, func(t *testing.T) {
			s := dial(t, url, admin)
			s.send(LiveClientMessage{Op: OpResume, Epoch: epoch, Seq: 1})

			reset := s.next()
			if reset.Type != TypeReset || reset.Epoch != h.epoch || reset.Seq != 1 {
				t.Fatalf("got %+v, want reset to epoch %s seq 1", reset, h.epoch)
			}
		})
	}
}

func TestResumeAfterSeqAgedOutResets(t *testing.T) {
	h := newHub(testConfig(), RoleFilter{})
	clock := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	h.now = func() time.Time { mu.Lock(); defer mu.Unlock(); return clock }
	url := serve(t, h)

	publish(h, typeFileImported, "t1", fileImported{Title: "t1"})
	mu.Lock()
	clock = clock.Add(DefaultConfig.BufferWindow + time.Second)
	mu.Unlock()
	publish(h, typeFileImported, "t2", fileImported{Title: "t2"})

	aged := dial(t, url, admin)
	aged.send(LiveClientMessage{Op: OpResume, Epoch: h.epoch, Seq: 0})
	if msg := aged.next(); msg.Type != TypeReset {
		t.Fatalf("got %+v, want reset: seq 1 left the buffer", msg)
	}

	current := dial(t, url, admin)
	current.send(LiveClientMessage{Op: OpResume, Epoch: h.epoch, Seq: 1})
	if msg := current.next(); msg.Seq != 2 {
		t.Fatalf("got %+v, want replay of seq 2", msg)
	}
}

func TestSearchEventsOnlyReachSubscribers(t *testing.T) {
	h := newHub(testConfig(), RoleFilter{})
	url := serve(t, h)
	search := domain.NewID[domain.SearchID]()

	subscribed := dial(t, url, admin)
	subscribed.send(LiveClientMessage{Op: OpSubscribe, Topics: []string{SearchTopicPrefix + string(search)}})
	subscribed.attach()
	other := dial(t, url, admin)
	other.attach()

	publish(h, typeSearchResults, string(search), searchResults{Search: search, Indexer: "nyaa"})
	publish(h, typeFileImported, "t1", fileImported{Title: "t1"})

	got := subscribed.next()
	if got.Type != typeSearchResults || got.Topic != SearchTopicPrefix+string(search) {
		t.Fatalf("subscriber got %+v, want search results on its topic", got)
	}
	if got := subscribed.next(); got.Type != typeFileImported {
		t.Fatalf("subscriber missed the broadcast: %+v", got)
	}
	if got := other.next(); got.Type != typeFileImported {
		t.Fatalf("non-subscriber got %+v, want only the broadcast", got)
	}

	subscribed.send(LiveClientMessage{Op: OpUnsubscribe, Topics: []string{SearchTopicPrefix + string(search)}})
	time.Sleep(quietPeriod)
	publish(h, typeSearchResults, string(search), searchResults{Search: search})
	subscribed.expectQuiet()
}

func TestHeartbeatClosesAfterTwoMissedPongs(t *testing.T) {
	cfg := testConfig()
	cfg.HeartbeatInterval = fastHeartbeat
	h := newHub(cfg, RoleFilter{})
	url := serve(t, h)

	silent := dial(t, url, admin)
	answering := dial(t, url, admin)
	go func() {
		for msg := range answering.messages {
			if msg.Type == TypeHeartbeat {
				_ = wsjson.Write(context.Background(), answering.conn, LiveClientMessage{Op: OpPong})
			}
		}
	}()

	var heartbeats []LiveMessage
	select {
	case err := <-silent.closed:
		if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
			t.Fatalf("closed with %v, want policy violation", err)
		}
	case <-time.After(waitForMessage):
		t.Fatal("silent socket was never closed")
	}
	for len(silent.messages) > 0 {
		heartbeats = append(heartbeats, <-silent.messages)
	}
	if len(heartbeats) != missedPongLimit {
		t.Errorf("silent socket got %d heartbeats before closing, want %d", len(heartbeats), missedPongLimit)
	}

	select {
	case err := <-answering.closed:
		t.Fatalf("answering socket closed: %v", err)
	case <-time.After(10 * fastHeartbeat):
	}
}

func TestRequesterSocketIsFiltered(t *testing.T) {
	h := newHub(testConfig(), RoleFilter{})
	url := serve(t, h)
	s := dial(t, url, requester)
	s.attach()

	publish(h, typeNotificationCreated, string(stranger), notificationCreated{User: stranger, Text: "theirs"})
	publish(h, typeNotificationCreated, string(requester.User), notificationCreated{User: requester.User, Text: "mine"})

	got := s.next()
	if got.Type != typeNotificationCreated || decode[notificationCreated](t, got).Text != "mine" {
		t.Fatalf("requester got %+v, want only their own notification", got)
	}
}

func TestBusEventsReachTheGateway(t *testing.T) {
	h := newHub(testConfig(), RoleFilter{})
	b := bus.New()
	b.Subscribe(h.Publish)
	url := serve(t, h)
	s := dial(t, url, admin)
	s.attach()

	b.Emit(bus.NewEvent(typeFileImported, "t1", fileImported{Title: "t1"}))

	if got := s.next(); got.Type != typeFileImported {
		t.Fatalf("got %+v", got)
	}
}
