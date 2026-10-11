package requests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	acquisitionapi "magpie/internal/acquisition/api"
	acquisitiontest "magpie/internal/acquisition/api/contracttest"
	catalogapi "magpie/internal/catalog/api"
	catalogtest "magpie/internal/catalog/api/contracttest"
	"magpie/internal/domain"
	identityapi "magpie/internal/identity/api"
	identitytest "magpie/internal/identity/api/contracttest"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/config"
	"magpie/internal/platform/db"
	"magpie/internal/platform/db/dbtest"
	"magpie/internal/platform/httpserver"
	"magpie/internal/requests/api"

	"github.com/go-fuego/fuego"
	"gorm.io/gorm"
)

const (
	requestsPath   = "/api/requests"
	duneTmdb       = domain.TmdbID(974576)
	severanceTmdb  = domain.TmdbID(95396)
	contentType    = "Content-Type"
	jsonType       = "application/json"
	adminName      = "Admin"
	requesterName  = "Alice"
	otherName      = "Bob"
	defaultProfile = domain.ProfileID("1080p")
	otherProfile   = domain.ProfileID("2160p")
)

// harness runs Requests' service, handlers and HTTP routes against a migrated temp database, with
// in-memory Identity, Catalog and Acquisition Readers.
type harness struct {
	t           *testing.T
	db          *db.DB
	repo        *Repository
	service     *Service
	identity    *identitytest.Fake
	catalog     *catalogtest.Fake
	acquisition *acquisitiontest.Fake
	server      *fuego.Server
	handlers    []bus.Handler

	admin, requester, other domain.Principal
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	d := dbtest.Open(t, Migrations())
	h := &harness{
		t:        t,
		db:       d,
		repo:     NewRepository(d),
		identity: identitytest.NewFake(identitytest.Fixture{}),
		catalog:  catalogtest.NewFake(catalogtest.Fixture{}),
		acquisition: acquisitiontest.NewFake(acquisitiontest.Fixture{
			Profiles:       []acquisitionapi.Profile{{ID: defaultProfile, Name: string(defaultProfile)}, {ID: otherProfile, Name: string(otherProfile)}},
			DefaultProfile: defaultProfile,
		}),
		admin:     domain.Principal{User: domain.NewID[domain.UserID](), Admin: true},
		requester: domain.Principal{User: domain.NewID[domain.UserID]()},
		other:     domain.Principal{User: domain.NewID[domain.UserID]()},
	}
	h.putUser(h.admin, adminName, identityapi.RoleAdmin, identityapi.Permissions{})
	h.putUser(h.requester, requesterName, identityapi.RoleRequester, identityapi.Permissions{})
	h.putUser(h.other, otherName, identityapi.RoleRequester, identityapi.Permissions{})
	h.service = NewService(ServiceParams{
		Repository:  h.repo,
		DB:          d,
		Bus:         bus.New(),
		Identity:    h.identity,
		Catalog:     h.catalog,
		Acquisition: h.acquisition,
	})
	h.handlers = []bus.Handler{
		newTitleAddedHandler(h.service),
		newMonitoringHandler(h.service),
		newEpisodesHandler(h.service),
		newTitleAddFailedHandler(h.service),
	}
	h.server = httpserver.Build(config.Env{}, httpserver.ModeServe, []httpserver.Controller{NewController(h.service)})
	return h
}

func (h *harness) putUser(p domain.Principal, name string, role identityapi.Role, perms identityapi.Permissions) {
	h.identity.Put(identityapi.User{ID: p.User, Username: name, DisplayName: name, Role: role, Permissions: perms})
}

// grant gives a requester toggles in Identity and on their principal, as the session middleware would.
func (h *harness) grant(p *domain.Principal, name string, perms identityapi.Permissions) {
	p.Permissions = perms
	h.putUser(*p, name, identityapi.RoleRequester, perms)
}

func (h *harness) do(method, path string, body any, p domain.Principal) *httptest.ResponseRecorder {
	h.t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			h.t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	}
	req := httptest.NewRequestWithContext(domain.WithPrincipal(context.Background(), p), method, path, reader)
	req.Header.Set(contentType, jsonType)
	rec := httptest.NewRecorder()
	h.server.Mux.ServeHTTP(rec, req)
	return rec
}

// create posts a request and decodes the response; it fails the test on an error status.
func (h *harness) create(p domain.Principal, body CreateRequestRequest) RequestResponse {
	h.t.Helper()
	return decode[RequestResponse](h.t, h.do(http.MethodPost, requestsPath, body, p))
}

func (h *harness) list(query string, p domain.Principal) RequestListResponse {
	h.t.Helper()
	return decode[RequestListResponse](h.t, h.do(http.MethodGet, requestsPath+query, nil, p))
}

func movie(tmdb domain.TmdbID) CreateRequestRequest {
	return CreateRequestRequest{Kind: domain.MediaKindMovie, TmdbID: tmdb, Scope: api.ScopeMovie}
}

func seasons(tmdb domain.TmdbID, numbers ...int) CreateRequestRequest {
	return CreateRequestRequest{Kind: domain.MediaKindSeries, TmdbID: tmdb, Scope: api.ScopeSeasons, Seasons: numbers}
}

func wholeSeries(tmdb domain.TmdbID) CreateRequestRequest {
	return CreateRequestRequest{Kind: domain.MediaKindSeries, TmdbID: tmdb, Scope: api.ScopeAllSeasons}
}

// deliver hands an event to every handler subscribed to its type, as the dispatcher would.
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

// series adds a tracked series with the given (season, number) episodes, all monitored and released.
func (h *harness) series(tmdb domain.TmdbID, numbering ...[2]int) (domain.TitleID, []domain.EpisodeID) {
	id := domain.NewID[domain.TitleID]()
	h.catalog.PutTitle(catalogapi.Title{ID: id, Kind: domain.MediaKindSeries, External: domain.ExternalIDs{Tmdb: tmdb}, Monitored: true})
	episodes := make([]domain.EpisodeID, 0, len(numbering))
	for _, n := range numbering {
		episode := domain.NewID[domain.EpisodeID]()
		h.catalog.PutEpisode(catalogapi.Episode{ID: episode, Title: id, Season: n[0], Number: n[1], Monitored: true, Released: true})
		episodes = append(episodes, episode)
	}
	return id, episodes
}

func (h *harness) movieTitle(tmdb domain.TmdbID, monitored bool) domain.TitleID {
	id := domain.NewID[domain.TitleID]()
	h.catalog.PutTitle(catalogapi.Title{ID: id, Kind: domain.MediaKindMovie, External: domain.ExternalIDs{Tmdb: tmdb}, Monitored: monitored, Released: true})
	return id
}

func (h *harness) request(id domain.RequestID) api.Request {
	h.t.Helper()
	r, err := h.repo.Request(context.Background(), id)
	if err != nil {
		h.t.Fatal(err)
	}
	return r
}

func (h *harness) items(id domain.RequestID) []itemRow {
	h.t.Helper()
	var rows []itemRow
	if err := h.db.Read(context.Background()).Where("request_id = ?", id).Find(&rows).Error; err != nil {
		h.t.Fatal(err)
	}
	return rows
}

// events decodes every published event of one type, in publish order.
func events[T domain.Message](h *harness) []T {
	h.t.Helper()
	var zero T
	var rows []struct{ Payload string }
	err := h.db.Read(context.Background()).Table("platform_outbox").Select("payload").
		Where("type = ?", zero.Type()).Order("seq").Find(&rows).Error
	if err != nil {
		h.t.Fatal(err)
	}
	decoded := make([]T, 0, len(rows))
	for _, row := range rows {
		var payload T
		if err := json.Unmarshal([]byte(row.Payload), &payload); err != nil {
			h.t.Fatal(err)
		}
		decoded = append(decoded, payload)
	}
	return decoded
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if rec.Code >= http.StatusBadRequest {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decoding %s: %v", rec.Body, err)
	}
	return v
}

func ptr[T any](v T) *T {
	return &v
}
