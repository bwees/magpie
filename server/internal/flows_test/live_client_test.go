package flows_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/live"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// liveClient is a browser tab's live socket: it keeps every event it receives and resumes from the last one
// after a disconnect, as the web client does.
type liveClient struct {
	t      *testing.T
	url    string
	cookie *http.Cookie

	mu       sync.Mutex
	conn     *websocket.Conn
	closed   chan struct{}
	received []live.LiveMessage
	resets   []live.LiveMessage
	epoch    string
	lastSeq  uint64
}

// openLive connects with session and waits for the first connect's reset.
func (s *stack) openLive(session *http.Cookie) *liveClient {
	s.t.Helper()
	srv := httptest.NewServer(s.server.Mux)
	s.t.Cleanup(srv.Close)
	c := &liveClient{t: s.t, url: "ws" + strings.TrimPrefix(srv.URL, "http") + httpserver.APIPrefix + live.Path, cookie: session}
	s.t.Cleanup(c.drop)
	c.connect(live.LiveClientMessage{Op: live.OpResume})
	eventually(s.t, "the live socket attaches", func() bool { return len(c.resetsSeen()) > 0 })
	return c
}

func (c *liveClient) connect(hello live.LiveClientMessage) {
	c.t.Helper()
	conn, _, err := websocket.Dial(context.Background(), c.url, &websocket.DialOptions{HTTPHeader: http.Header{"Cookie": {c.cookie.String()}}})
	if err != nil {
		c.t.Fatalf("dial: %v", err)
	}
	if err := wsjson.Write(context.Background(), conn, hello); err != nil {
		c.t.Fatal(err)
	}
	closed := make(chan struct{})
	c.mu.Lock()
	c.conn, c.closed = conn, closed
	c.mu.Unlock()
	go func() {
		defer close(closed)
		for {
			var msg live.LiveMessage
			if err := wsjson.Read(context.Background(), conn, &msg); err != nil {
				return
			}
			c.keep(msg)
		}
	}()
}

func (c *liveClient) keep(msg live.LiveMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch msg.Type {
	case live.TypeHeartbeat:
		return
	case live.TypeReset:
		c.resets = append(c.resets, msg)
		c.epoch = msg.Epoch
	default:
		c.received = append(c.received, msg)
	}
	c.lastSeq = msg.Seq
}

// drop closes the socket, as a lost connection would, and waits until its reader stopped.
func (c *liveClient) drop() {
	c.mu.Lock()
	conn, closed := c.conn, c.closed
	c.mu.Unlock()
	if conn == nil {
		return
	}
	_ = conn.CloseNow()
	<-closed
}

// resume reconnects from the last message received.
func (c *liveClient) resume() {
	c.t.Helper()
	c.mu.Lock()
	hello := live.LiveClientMessage{Op: live.OpResume, Epoch: c.epoch, Seq: c.lastSeq}
	c.mu.Unlock()
	c.connect(hello)
}

func (c *liveClient) messages() []live.LiveMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.received)
}

func (c *liveClient) resetsSeen() []live.LiveMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.resets)
}

// ofType returns the received events of one type, in arrival order.
func (c *liveClient) ofType(eventType string) []live.LiveMessage {
	return slices.DeleteFunc(c.messages(), func(m live.LiveMessage) bool { return m.Type != eventType })
}

func (c *liveClient) awaitType(eventType string) {
	c.t.Helper()
	eventually(c.t, "the live socket receives "+eventType, func() bool { return len(c.ofType(eventType)) > 0 })
}
