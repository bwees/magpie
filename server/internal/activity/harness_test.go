package activity

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"magpie/internal/activity/api"
	catalogapi "magpie/internal/catalog/api"
	catalogtest "magpie/internal/catalog/api/contracttest"
	"magpie/internal/domain"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/db"
	"magpie/internal/platform/db/dbtest"
	"magpie/internal/platform/settings"
	requeststest "magpie/internal/requests/api/contracttest"

	"gorm.io/gorm"
)

const (
	movieName  = "Dune"
	seriesName = "Severance"
)

// harness runs Activity's handlers against a migrated temp database, the way the dispatcher would.
type harness struct {
	t        *testing.T
	db       *db.DB
	bus      *bus.Bus
	catalog  *catalogtest.Fake
	requests *requeststest.Fake
	repo     *Repository
	service  *Service
	handlers []bus.Handler

	mu        sync.Mutex
	published []bus.Event
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	d := dbtest.Open(t, Migrations())
	b := bus.New()
	h := &harness{
		t:        t,
		db:       d,
		bus:      b,
		catalog:  catalogtest.NewFake(catalogtest.Fixture{}),
		requests: requeststest.NewFake(requeststest.Fixture{}),
		repo:     NewRepository(d),
	}
	h.service = NewService(ServiceParams{
		Repository: h.repo,
		DB:         d,
		Bus:        b,
		Catalog:    h.catalog,
		Requests:   h.requests,
		Settings:   settings.NewStore(d),
	})
	h.handlers = []bus.Handler{
		NewTimelineHandler(h.service),
		NewHistoryHandler(h.service),
		NewPipelineHandler(h.service),
		NewAttentionHandler(h.service),
	}
	unsubscribe := b.Subscribe(func(ev bus.Event) {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.published = append(h.published, ev)
	})
	t.Cleanup(unsubscribe)
	return h
}

// movie adds a movie to the catalog fake and returns its ID.
func (h *harness) movie() domain.TitleID {
	id := domain.NewID[domain.TitleID]()
	h.catalog.PutTitle(catalogapi.Title{ID: id, Kind: domain.MediaKindMovie, Name: movieName})
	return id
}

// series adds a series with the given episodes (season, number pairs) to the catalog fake.
func (h *harness) series(numbering ...[2]int) (domain.TitleID, []domain.EpisodeID) {
	id := domain.NewID[domain.TitleID]()
	h.catalog.PutTitle(catalogapi.Title{ID: id, Kind: domain.MediaKindSeries, Name: seriesName})
	episodes := make([]domain.EpisodeID, 0, len(numbering))
	for _, n := range numbering {
		episode := domain.NewID[domain.EpisodeID]()
		h.catalog.PutEpisode(catalogapi.Episode{ID: episode, Title: id, Season: n[0], Number: n[1]})
		episodes = append(episodes, episode)
	}
	return id, episodes
}

// deliver wraps m in a fresh event and hands it to every handler that subscribes to its type.
func (h *harness) deliver(m domain.Message) bus.Event {
	h.t.Helper()
	ev := bus.EventOf(m)
	h.deliverEvent(ev)
	return ev
}

func (h *harness) deliverEvent(ev bus.Event) {
	h.t.Helper()
	for _, handler := range h.handlers {
		if !slices.Contains(handler.EventTypes, ev.Type) {
			continue
		}
		err := h.db.Write(context.Background(), func(tx *gorm.DB) error {
			return handler.Handle(context.Background(), tx, ev)
		})
		if err != nil {
			h.t.Fatalf("%s handling %s: %v", handler.Name, ev.Type, err)
		}
	}
}

func (h *harness) pipeline(title domain.TitleID) []api.PipelineRow {
	h.t.Helper()
	rows, err := h.repo.Pipeline(context.Background(), title)
	if err != nil {
		h.t.Fatal(err)
	}
	return rows
}

// row returns the only pipeline row of a movie.
func (h *harness) row(title domain.TitleID) api.PipelineRow {
	h.t.Helper()
	rows := h.pipeline(title)
	if len(rows) != 1 {
		h.t.Fatalf("title has %d pipeline rows, want 1", len(rows))
	}
	return rows[0]
}

func (h *harness) timeline(title domain.TitleID) []api.TimelineEntry {
	h.t.Helper()
	entries, err := h.repo.Timeline(context.Background(), title, api.Page{})
	if err != nil {
		h.t.Fatal(err)
	}
	return entries
}

func (h *harness) history() []api.TimelineEntry {
	h.t.Helper()
	entries, err := h.repo.History(context.Background(), "", api.Page{Limit: api.MaxPageLimit})
	if err != nil {
		h.t.Fatal(err)
	}
	return entries
}

func (h *harness) attention() []api.AttentionItem {
	h.t.Helper()
	items, err := h.repo.Attention(context.Background())
	if err != nil {
		h.t.Fatal(err)
	}
	return items
}

// attentionChanges returns the AttentionChanged events published so far and forgets them.
func (h *harness) attentionChanges() []api.AttentionChanged {
	h.t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	var changes []api.AttentionChanged
	for _, ev := range h.published {
		if ev.Type != api.TypeAttentionChanged {
			continue
		}
		change, err := bus.Decode[api.AttentionChanged](ev)
		if err != nil {
			h.t.Fatal(err)
		}
		changes = append(changes, change)
	}
	h.published = nil
	return changes
}

// ago returns a time d before now, truncated so it survives the database round trip.
func ago(d time.Duration) time.Time {
	return time.Now().UTC().Add(-d).Truncate(time.Millisecond)
}
