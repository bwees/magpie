package live

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"time"

	"magpie/internal/domain"
	identityapi "magpie/internal/identity/api"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/config"
	"magpie/internal/platform/problem"
	requestsapi "magpie/internal/requests/api"

	"github.com/coder/websocket"
	"go.uber.org/fx"
)

type Config struct {
	// BufferWindow is how long messages stay available for resume.
	BufferWindow time.Duration
	// HeartbeatInterval is how often the server pings; two unanswered pings close the socket.
	HeartbeatInterval time.Duration
	// ClientQueue is how many messages may wait for a slow socket before it is dropped.
	ClientQueue int
	// OriginPatterns are extra browser origins allowed to connect besides the server's own host.
	OriginPatterns []string
}

const (
	missedPongLimit = 2
	writeTimeout    = 10 * time.Second
)

var DefaultConfig = Config{
	BufferWindow:      2 * time.Minute,
	HeartbeatInterval: 20 * time.Second,
	ClientQueue:       256,
}

type entry struct {
	msg     LiveMessage
	at      time.Time
	scope   scope
	control bool
}

// Hub keeps the replay buffer and fans every bus event out to connected sockets.
type Hub struct {
	cfg    Config
	filter RoleFilter
	now    func() time.Time

	mu      sync.Mutex
	epoch   string
	seq     uint64
	buffer  []entry
	clients map[*client]struct{}
}

type epochID string

func newHub(cfg Config, filter RoleFilter) *Hub {
	return &Hub{
		cfg:     cfg,
		filter:  filter,
		now:     time.Now,
		epoch:   string(domain.NewID[epochID]()),
		clients: map[*client]struct{}{},
	}
}

type Params struct {
	fx.In

	Lifecycle fx.Lifecycle
	Env       config.Env
	Bus       *bus.Bus
	Identity  identityapi.Reader `optional:"true"`
	Requests  requestsapi.Reader `optional:"true"`
}

func NewHub(p Params) *Hub {
	cfg := DefaultConfig
	if p.Env.BaseURL != nil {
		cfg.OriginPatterns = []string{p.Env.BaseURL.Host}
	}
	h := newHub(cfg, RoleFilter{Identity: p.Identity, Requests: p.Requests})
	unsubscribe := p.Bus.Subscribe(h.Publish)
	p.Lifecycle.Append(fx.StopHook(func() {
		unsubscribe()
		h.Close()
	}))
	return h
}

// Publish buffers ev and hands it to every attached socket. It never blocks.
func (h *Hub) Publish(ev bus.Event) {
	data, err := json.Marshal(ev.Payload)
	if err != nil {
		slog.Error("live: encoding payload", "type", ev.Type, "error", err)
		return
	}
	s := scopeOf(ev.Payload)
	e := entry{
		msg:   LiveMessage{Type: ev.Type, Topic: topicFor(ev.Type, ev.AggregateID, s), Data: data},
		scope: s,
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.seq++
	e.msg.Seq = h.seq
	e.msg.Epoch = h.epoch
	e.at = h.now()
	h.buffer = append(h.buffer, e)
	h.trim()
	for c := range h.clients {
		c.offer(e)
	}
}

// Close drops every socket.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		c.cancel()
	}
}

func (h *Hub) trim() {
	cutoff := h.now().Add(-h.cfg.BufferWindow)
	fresh := slices.IndexFunc(h.buffer, func(e entry) bool { return !e.at.Before(cutoff) })
	if fresh < 0 {
		fresh = len(h.buffer)
	}
	h.buffer = slices.Delete(h.buffer, 0, fresh)
}

// attach replays what c missed since (epoch, seq), or sends a reset, then streams new messages to it.
func (h *Hub) attach(c *client, epoch string, seq uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.trim()
	if epoch == h.epoch && h.canReplayAfter(seq) {
		for _, e := range h.buffer {
			if e.msg.Seq > seq {
				c.offer(e)
			}
		}
	} else {
		c.offer(h.control(TypeReset))
	}
	h.clients[c] = struct{}{}
}

func (h *Hub) canReplayAfter(seq uint64) bool {
	if seq > h.seq {
		return false
	}
	if len(h.buffer) == 0 {
		return seq == h.seq
	}
	return h.buffer[0].msg.Seq <= seq+1
}

func (h *Hub) detach(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, c)
}

func (h *Hub) heartbeat() entry {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.control(TypeHeartbeat)
}

func (h *Hub) control(messageType string) entry {
	return entry{msg: LiveMessage{Seq: h.seq, Epoch: h.epoch, Type: messageType}, control: true}
}

// ServeHTTP upgrades a signed-in request to the live socket.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	principal, ok := domain.PrincipalFrom(r.Context())
	if !ok {
		problem.Write(w, r, problem.New(problem.TypeUnauthorized, http.StatusUnauthorized, "sign in to open the live socket"))
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: h.cfg.OriginPatterns})
	if err != nil {
		slog.WarnContext(r.Context(), "live: upgrade failed", "error", err)
		return
	}
	defer func() { _ = conn.CloseNow() }()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	c := newClient(h, conn, principal, cancel)
	defer h.detach(c)

	go c.readLoop(ctx)
	c.writeLoop(ctx)
}
