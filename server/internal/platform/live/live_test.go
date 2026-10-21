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
	identityapi "magpie/internal/identity/api"
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
	typeSearchCompleted     = "acquisition.SearchCompleted"
	typeFileImported        = "library.FileImported"
)

type notificationCreated struct {
	User  domain.UserID   `json:"user"`
	Title *domain.TitleID `json:"title,omitempty"`
	Text  string          `json:"text"`
}

func (n notificationCreated) Recipient() domain.UserID { return n.User }

type searchResults struct {
	Search  domain.SearchID `json:"search"`
	Indexer string          `json:"indexer"`
}

// searchCompleted names the title searched, which a requester of that title would otherwise see.
type searchCompleted struct {
	Search domain.SearchID `json:"search"`
	Title  domain.TitleID  `json:"title"`
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

func TestResumeResetsWhenTheGapOutgrowsTheQueue(t *testing.T) {
	cases := []struct {
		name      string
		missed    int
		wantReset bool
	}{
		{"gap fits the queue", smallQueue, false},
		{"gap larger than the queue", smallQueue + 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testConfig()
			cfg.ClientQueue = smallQueue
			h := newHub(cfg, RoleFilter{})
			url := serve(t, h)
			for range tc.missed {
				publish(h, typeFileImported, "t1", fileImported{Title: "t1"})
			}

			s := dial(t, url, admin)
			s.send(LiveClientMessage{Op: OpResume, Epoch: h.epoch, Seq: 0})

			first := s.next()
			if gotReset := first.Type == TypeReset; gotReset != tc.wantReset {
				t.Fatalf("first message %+v, want reset %v", first, tc.wantReset)
			}
			if tc.wantReset && first.Seq != uint64(tc.missed) {
				t.Fatalf("reset at seq %d, want %d", first.Seq, tc.missed)
			}
			if !tc.wantReset {
				for range tc.missed - 1 {
					s.next()
				}
			}
			publish(h, typeFileImported, "t2", fileImported{Title: "t2"})
			if live := s.next(); live.Seq != uint64(tc.missed+1) {
				t.Fatalf("got %+v after the resume, want seq %d streamed live", live, tc.missed+1)
			}
		})
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

func TestSearchEventsReachOnlySubscribedAdmins(t *testing.T) {
	roles := newRoleFixture()
	h := newHub(testConfig(), roles.filter)
	url := serve(t, h)
	search := domain.NewID[domain.SearchID]()
	topic := SearchTopicPrefix + string(search)

	nosy := dial(t, url, requester)
	nosy.send(LiveClientMessage{Op: OpSubscribe, Topics: []string{topic}})
	nosy.attach()
	watching := dial(t, url, admin)
	watching.send(LiveClientMessage{Op: OpSubscribe, Topics: []string{topic}})
	watching.attach()

	publish(h, typeSearchCompleted, string(roles.myTitle), searchCompleted{Search: search, Title: roles.myTitle})

	if got := watching.next(); got.Type != typeSearchCompleted || got.Topic != topic {
		t.Fatalf("admin got %+v, want the completed search on its topic", got)
	}
	nosy.expectQuiet()
}

func TestResumeReplaysSearchEventsAfterReconnect(t *testing.T) {
	h := newHub(testConfig(), RoleFilter{})
	url := serve(t, h)
	search := domain.NewID[domain.SearchID]()
	topic := SearchTopicPrefix + string(search)

	first := dial(t, url, admin)
	first.send(LiveClientMessage{Op: OpSubscribe, Topics: []string{topic}})
	first.attach()
	publish(h, typeSearchResults, string(search), searchResults{Search: search, Indexer: "nyaa"})
	seen := first.next()
	_ = first.conn.CloseNow()
	publish(h, typeSearchResults, string(search), searchResults{Search: search, Indexer: "beyond"})

	again := dial(t, url, admin)
	again.send(LiveClientMessage{Op: OpSubscribe, Topics: []string{topic}})
	again.send(LiveClientMessage{Op: OpResume, Epoch: seen.Epoch, Seq: seen.Seq})
	missed := again.next()
	if missed.Topic != topic || decode[searchResults](t, missed).Indexer != "beyond" {
		t.Fatalf("replayed %+v, want the answer missed while disconnected", missed)
	}
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

func TestSearchEventsReachFileManagersOfRequestedTitles(t *testing.T) {
	roles := newRoleFixture()
	roles.identity.Put(identityapi.User{
		ID: requester.User, Username: "requester", Role: identityapi.RoleRequester,
		Permissions: domain.Permissions{ManageFiles: true},
	})
	h := newHub(testConfig(), roles.filter)
	url := serve(t, h)
	mine, theirs := domain.NewID[domain.SearchID](), domain.NewID[domain.SearchID]()

	manager := dial(t, url, requester)
	manager.send(LiveClientMessage{Op: OpSubscribe, Topics: []string{SearchTopicPrefix + string(mine), SearchTopicPrefix + string(theirs)}})
	manager.attach()

	publish(h, typeSearchCompleted, string(roles.otherTitle), searchCompleted{Search: theirs, Title: roles.otherTitle})
	publish(h, typeSearchCompleted, string(roles.myTitle), searchCompleted{Search: mine, Title: roles.myTitle})

	if got := manager.next(); got.Topic != SearchTopicPrefix+string(mine) {
		t.Fatalf("file manager got %+v, want only the search for their requested title", got)
	}
	manager.expectQuiet()
}
