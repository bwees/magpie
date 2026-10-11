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

const closeReasonMissedPongs = "missed heartbeats"

type client struct {
	hub       *Hub
	conn      *websocket.Conn
	principal domain.Principal
	queue     chan entry
	cancel    context.CancelFunc

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

// offer queues e without blocking. A socket too slow to keep up is dropped; it resumes or resets on reconnect.
func (c *client) offer(e entry) {
	select {
	case c.queue <- e:
	default:
		slog.Warn("live: dropping slow socket", "user", c.principal.User)
		c.cancel()
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
	if e.msg.Topic != "" && !c.subscribed(e.msg.Topic) {
		return false
	}
	return c.hub.filter.Allows(ctx, c.principal, e.scope)
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
