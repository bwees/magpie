package flows_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"magpie/internal/domain"
	"magpie/internal/identity"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/live"
	"magpie/internal/requests"
	requestsapi "magpie/internal/requests/api"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"go.uber.org/fx"
)

const (
	otherRequesterName = "bob"
	otherPassword      = "bob jellyfin password"
	duneTmdb           = domain.TmdbID(693134)
	conclaveTmdb       = domain.TmdbID(974576)
	movieYear          = 2024
	requestsEventType  = "requests."
	socketBuffer       = 64
)

// requestFlow is the stack with two Jellyfin requesters and a TMDB that knows two movies.
type requestFlow struct {
	*stack
	admin, alice, bob *http.Cookie
}

func newRequestFlow(t *testing.T, options ...fx.Option) *requestFlow {
	t.Helper()
	s := newStack(t, options...)
	s.jellyfinAuth.AddUser(identity.JellyfinUser{ID: "jf-alice", Name: requesterName}, requesterPassword)
	s.jellyfinAuth.AddUser(identity.JellyfinUser{ID: "jf-bob", Name: otherRequesterName}, otherPassword)
	s.movieOnTMDB(duneTmdb, "Dune: Part Two", movieYear)
	s.movieOnTMDB(conclaveTmdb, "Conclave", movieYear)
	f := &requestFlow{stack: s, admin: s.session}
	f.alice = f.signIn(requesterName, requesterPassword)
	f.bob = f.signIn(otherRequesterName, otherPassword)
	return f
}

func (f *requestFlow) signIn(name, password string) *http.Cookie {
	f.t.Helper()
	f.session = nil
	rec := f.do(http.MethodPost, jellyfinSignInPath, identity.CredentialsRequest{Username: name, Password: password})
	if rec.Code != http.StatusOK {
		f.t.Fatalf("sign in %s = %d: %s", name, rec.Code, rec.Body)
	}
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == identity.SessionCookie {
			return cookie
		}
	}
	f.t.Fatalf("no session cookie for %s", name)
	return nil
}

// socket opens the live socket with a session and returns its request events as they arrive.
func (f *requestFlow) socket(session *http.Cookie) <-chan live.LiveMessage {
	f.t.Helper()
	srv := httptest.NewServer(f.server.Mux)
	f.t.Cleanup(srv.Close)
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + httpserver.APIPrefix + live.Path
	conn, _, err := websocket.Dial(context.Background(), url, &websocket.DialOptions{HTTPHeader: http.Header{"Cookie": {session.String()}}})
	if err != nil {
		f.t.Fatalf("dial: %v", err)
	}
	f.t.Cleanup(func() { _ = conn.CloseNow() })
	if err := wsjson.Write(context.Background(), conn, live.LiveClientMessage{Op: live.OpResume}); err != nil {
		f.t.Fatal(err)
	}
	attached := make(chan struct{})
	events := make(chan live.LiveMessage, socketBuffer)
	go func() {
		for {
			var msg live.LiveMessage
			if err := wsjson.Read(context.Background(), conn, &msg); err != nil {
				return
			}
			switch {
			case msg.Type == live.TypeReset:
				close(attached)
			case strings.HasPrefix(msg.Type, requestsEventType):
				events <- msg
			}
		}
	}()
	select {
	case <-attached:
	case <-time.After(waitForHandler):
		f.t.Fatal("the live socket never attached")
	}
	return events
}

func nextEvent(t *testing.T, events <-chan live.LiveMessage) live.LiveMessage {
	t.Helper()
	select {
	case msg := <-events:
		return msg
	case <-time.After(waitForHandler):
		t.Fatal("no request event arrived")
		return live.LiveMessage{}
	}
}

func TestRequesterSocketGetsOnlyTheirOwnRequestEvents(t *testing.T) {
	f := newRequestFlow(t)
	aliceEvents := f.socket(f.alice)
	adminEvents := f.socket(f.admin)

	bobs := f.request(f.bob, duneTmdb)
	alices := f.request(f.alice, conclaveTmdb)

	for _, want := range []domain.RequestID{bobs.ID, alices.ID} {
		var created requestsapi.RequestCreated
		if err := json.Unmarshal(nextEvent(t, adminEvents).Data, &created); err != nil || created.Request != want {
			t.Fatalf("admin got %+v (%v), want RequestCreated for %s", created, err, want)
		}
	}
	msg := nextEvent(t, aliceEvents)
	var created requestsapi.RequestCreated
	if err := json.Unmarshal(msg.Data, &created); err != nil || msg.Type != requestsapi.TypeRequestCreated || created.Request != alices.ID {
		t.Fatalf("alice got %s %+v, want only her own RequestCreated", msg.Type, created)
	}
}

func TestApprovedRequestFollowsCatalogToProcessing(t *testing.T) {
	f := newRequestFlow(t)
	pending := f.request(f.alice, duneTmdb)
	if pending.Status != requestsapi.StatusPending {
		t.Fatalf("status = %s, want PENDING", pending.Status)
	}

	if rec := f.as(f.admin, http.MethodPost, requestsPath+"/"+string(pending.ID)+"/approve", requests.ApproveRequestRequest{}); rec.Code != http.StatusOK {
		t.Fatalf("approve = %d: %s", rec.Code, rec.Body)
	}

	deadline := time.Now().Add(waitForHandler)
	for time.Now().Before(deadline) {
		var got requests.RequestResponse
		rec := f.as(f.alice, http.MethodGet, requestsPath+"/"+string(pending.ID), nil)
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if got.Status == requestsapi.StatusProcessing && got.TitleID != nil && got.TotalItems == 1 {
			return
		}
		time.Sleep(pollEvery)
	}
	t.Fatal("the request never reached PROCESSING with its Title linked")
}
