package live

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"magpie/internal/domain"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const (
	closeReasonMissedPongs = "missed heartbeats"
	// accessTTL is how long a socket trusts the viewer's role and toggles before reading them again.
	accessTTL = 5 * time.Second
)

type client struct {
	hub       *Hub
	conn      *websocket.Conn
	principal domain.Principal
	queue     chan entry
	cancel    context.CancelFunc

	// access is the viewer's role as read at accessRead. Only the write loop touches either field.
	access     viewerAccess
	accessRead time.Time

	mu       sync.Mutex
	topics   map[string]bool
	attached bool
	// unanswered holds the send times of heartbeats sent since the last pong.
	unanswered []time.Time
}

func newClient(h *Hub, conn *websocket.Conn, principal domain.Principal, cancel context.CancelFunc) *client {
	return &client{
		hub:       h,
		conn:      conn,
		principal: principal,
		queue:     make(chan entry, h.cfg.ClientQueue),
		cancel:    cancel,
		topics:    map[string]bool{},
	}
}

// offer queues e without blocking and reports whether it fit.
func (c *client) offer(e entry) bool {
	select {
	case c.queue <- e:
		return true
	default:
		return false
	}
}

// resync swaps everything queued for reset, which tells the browser to refetch what it shows. Only the hub
// adds to the queue, under its lock, so the emptied queue has room.
func (c *client) resync(reset entry) {
	for {
		select {
		case <-c.queue:
		default:
			c.queue <- reset
			return
		}
	}
}

func (c *client) readLoop(ctx context.Context) {
	defer c.cancel()
	for {
		var msg LiveClientMessage
		if err := wsjson.Read(ctx, c.conn, &msg); err != nil {
			return
		}
		c.handle(msg)
	}
}

func (c *client) handle(msg LiveClientMessage) {
	switch msg.Op {
	case OpResume:
		if c.markAttached() {
			c.hub.attach(c, msg.Epoch, msg.Seq)
		}
	case OpSubscribe:
		c.setTopics(msg.Topics, true)
	case OpUnsubscribe:
		c.setTopics(msg.Topics, false)
	case OpPong:
		c.mu.Lock()
		c.unanswered = c.unanswered[:0]
		c.mu.Unlock()
	}
}

// markAttached reports whether this is the socket's first resume; later ones are ignored.
func (c *client) markAttached() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	first := !c.attached
	c.attached = true
	return first
}

func (c *client) setTopics(topics []string, subscribed bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, topic := range topics {
		if !strings.HasPrefix(topic, SearchTopicPrefix) {
			continue
		}
		if subscribed {
			c.topics[topic] = true
		} else {
			delete(c.topics, topic)
		}
	}
}

func (c *client) subscribed(topic string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.topics[topic]
}

// ping records a heartbeat about to be sent and reports whether the socket already missed too many pongs.
func (c *client) ping() (alive bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.unanswered) >= missedPongLimit {
		return false
	}
	c.unanswered = append(c.unanswered, c.hub.now())
	return true
}

func (c *client) writeLoop(ctx context.Context) {
	heartbeat := time.NewTicker(c.hub.cfg.HeartbeatInterval)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			if !c.ping() {
				_ = c.conn.Close(websocket.StatusPolicyViolation, closeReasonMissedPongs)
				return
			}
			if !c.write(ctx, c.hub.heartbeat()) {
				return
			}
		case e := <-c.queue:
			if c.allows(ctx, e) && !c.write(ctx, e) {
				return
			}
		}
	}
}

func (c *client) allows(ctx context.Context, e entry) bool {
	if e.control {
		return true
	}
	access, err := c.viewerAccess(ctx)
	if err != nil {
		slog.WarnContext(ctx, "live role check failed", "user", c.principal.User, "error", err)
		return false
	}
	if e.msg.Topic != "" {
		return c.subscribed(e.msg.Topic) && access.seesTopics()
	}
	return c.hub.filter.allows(ctx, c.principal, access, e.scope)
}

func (c *client) viewerAccess(ctx context.Context) (viewerAccess, error) {
	now := c.hub.now()
	if !c.accessRead.IsZero() && now.Sub(c.accessRead) < accessTTL {
		return c.access, nil
	}
	access, err := c.hub.filter.access(ctx, c.principal)
	if err != nil {
		return viewerAccess{}, err
	}
	c.access, c.accessRead = access, now
	return access, nil
}

func (c *client) write(ctx context.Context, e entry) bool {
	writeCtx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	if err := wsjson.Write(writeCtx, c.conn, e.msg); err != nil {
		slog.DebugContext(ctx, "live: write failed", "user", c.principal.User, "error", err)
		return false
	}
	return true
}
