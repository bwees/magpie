package access

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"magpie/internal/domain"
	requestsapi "magpie/internal/requests/api"
	requeststest "magpie/internal/requests/api/contracttest"

	"github.com/go-fuego/fuego"
)

type gateFixture struct {
	gate                            *FileGate
	me, someone                     domain.UserID
	myTitle, otherTitle, quietTitle domain.TitleID
}

func newGateFixture() gateFixture {
	f := gateFixture{
		me:         domain.NewID[domain.UserID](),
		someone:    domain.NewID[domain.UserID](),
		myTitle:    domain.NewID[domain.TitleID](),
		otherTitle: domain.NewID[domain.TitleID](),
		quietTitle: domain.NewID[domain.TitleID](),
	}
	f.gate = NewFileGate(requeststest.NewFake(requeststest.Fixture{Requests: []requestsapi.Request{
		{ID: domain.NewID[domain.RequestID](), User: f.me, Title: &f.myTitle, Status: requestsapi.StatusAvailable},
		{ID: domain.NewID[domain.RequestID](), User: f.someone, Title: &f.otherTitle, Status: requestsapi.StatusAvailable},
	}}))
	return f
}

func TestFileGateAllows(t *testing.T) {
	f := newGateFixture()
	manager := domain.Permissions{ManageFiles: true}
	managerSeeingAll := domain.Permissions{ManageFiles: true, ViewOthersRequests: true}
	cases := []struct {
		name        string
		admin       bool
		permissions domain.Permissions
		title       domain.TitleID
		want        bool
	}{
		{"admin, unrequested title", true, domain.Permissions{}, f.quietTitle, true},
		{"requester without the toggle, own title", false, domain.Permissions{ViewOthersRequests: true}, f.myTitle, false},
		{"file manager, own title", false, manager, f.myTitle, true},
		{"file manager, someone else's title", false, manager, f.otherTitle, false},
		{"file manager seeing others' requests, someone else's title", false, managerSeeingAll, f.otherTitle, true},
		{"file manager seeing others' requests, unrequested title", false, managerSeeingAll, f.quietTitle, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			principal := domain.Principal{User: f.me, Admin: c.admin, Permissions: c.permissions}
			got, err := f.gate.Allows(context.Background(), principal, c.title)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Fatalf("Allows = %v, want %v", got, c.want)
			}
		})
	}
}

func TestFileGateRequireForbidsOthersTitles(t *testing.T) {
	f := newGateFixture()
	ctx := domain.WithPrincipal(context.Background(), domain.Principal{User: f.me, Permissions: domain.Permissions{ManageFiles: true}})

	if err := f.gate.Require(ctx, f.myTitle); err != nil {
		t.Fatalf("own title: %v", err)
	}
	var p fuego.HTTPError
	if err := f.gate.Require(ctx, f.otherTitle); !errors.As(err, &p) || p.Status != http.StatusForbidden {
		t.Fatalf("someone else's title: got %v, want 403", err)
	}
}

func TestFilterKeepsManagedTitles(t *testing.T) {
	f := newGateFixture()
	titles := []domain.TitleID{f.myTitle, f.otherTitle, f.myTitle}
	same := func(t domain.TitleID) domain.TitleID { return t }

	manager := domain.WithPrincipal(context.Background(), domain.Principal{User: f.me, Permissions: domain.Permissions{ManageFiles: true}})
	kept, err := Filter(manager, f.gate, titles, same)
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) != 2 || kept[0] != f.myTitle || kept[1] != f.myTitle {
		t.Fatalf("Filter kept %v, want only %s twice", kept, f.myTitle)
	}

	requester := domain.WithPrincipal(context.Background(), domain.Principal{User: f.me})
	var p fuego.HTTPError
	if _, err := Filter(requester, f.gate, titles, same); !errors.As(err, &p) || p.Status != http.StatusForbidden {
		t.Fatalf("requester without the toggle: got %v, want 403", err)
	}
}
