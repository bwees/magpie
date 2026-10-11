package live

import (
	"context"
	"errors"
	"testing"

	"magpie/internal/domain"
	identityapi "magpie/internal/identity/api"
	identitytest "magpie/internal/identity/api/contracttest"
	requestsapi "magpie/internal/requests/api"
	requeststest "magpie/internal/requests/api/contracttest"
)

type requestEvent struct {
	Request domain.RequestID
	Title   domain.TitleID
}

type roleFixture struct {
	filter                          RoleFilter
	identity                        *identitytest.Fake
	myRequest, otherRequest         domain.RequestID
	myTitle, otherTitle, quietTitle domain.TitleID
}

func newRoleFixture() roleFixture {
	f := roleFixture{
		myRequest:    domain.NewID[domain.RequestID](),
		otherRequest: domain.NewID[domain.RequestID](),
		myTitle:      domain.NewID[domain.TitleID](),
		otherTitle:   domain.NewID[domain.TitleID](),
		quietTitle:   domain.NewID[domain.TitleID](),
	}
	f.identity = identitytest.NewFake(identitytest.Fixture{Users: []identityapi.User{
		{ID: admin.User, Username: "admin", Role: identityapi.RoleAdmin},
		{ID: requester.User, Username: "requester", Role: identityapi.RoleRequester},
		{ID: stranger, Username: "stranger", Role: identityapi.RoleRequester},
	}})
	requests := requeststest.NewFake(requeststest.Fixture{Requests: []requestsapi.Request{
		{ID: f.myRequest, User: requester.User, Title: &f.myTitle, Status: requestsapi.StatusProcessing},
		{ID: f.otherRequest, User: stranger, Title: &f.otherTitle, Status: requestsapi.StatusProcessing},
	}})
	f.filter = RoleFilter{Identity: f.identity, Requests: requests}
	return f
}

func TestRoleFilter(t *testing.T) {
	f := newRoleFixture()
	cases := []struct {
		name    string
		payload any
		want    bool
	}{
		{"own notification", notificationCreated{User: requester.User}, true},
		{"someone else's notification about my title", notificationCreated{User: stranger, Title: &f.myTitle}, false},
		{"own request", requestEvent{Request: f.myRequest, Title: f.otherTitle}, true},
		{"someone else's request", requestEvent{Request: f.otherRequest, Title: f.myTitle}, false},
		{"my request's title", fileImported{Title: f.myTitle}, true},
		{"unrelated title", fileImported{Title: f.otherTitle}, false},
		{"no scope", searchResults{Indexer: "nyaa"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := scopeOf(tc.payload)
			if got := f.filter.Allows(context.Background(), requester, s); got != tc.want {
				t.Errorf("requester allowed = %v, want %v", got, tc.want)
			}
			if !f.filter.Allows(context.Background(), admin, s) {
				t.Error("admin was filtered")
			}
		})
	}
}

func TestRoleFilterViewOthersRequests(t *testing.T) {
	f := newRoleFixture()
	f.identity.Put(identityapi.User{
		ID: requester.User, Username: "requester", Role: identityapi.RoleRequester,
		Permissions: identityapi.Permissions{ViewOthersRequests: true},
	})

	cases := []struct {
		name    string
		payload any
		want    bool
	}{
		{"someone else's request", requestEvent{Request: f.otherRequest, Title: f.otherTitle}, true},
		{"someone else's requested title", fileImported{Title: f.otherTitle}, true},
		{"a title nobody requested", fileImported{Title: f.quietTitle}, false},
		{"someone else's notification", notificationCreated{User: stranger}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := f.filter.Allows(context.Background(), requester, scopeOf(tc.payload)); got != tc.want {
				t.Errorf("allowed = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRoleFilterUsesTheStoredRoleNotThePrincipal(t *testing.T) {
	f := newRoleFixture()
	demoted := domain.Principal{User: requester.User, Admin: true}

	if f.filter.Allows(context.Background(), demoted, scopeOf(fileImported{Title: f.otherTitle})) {
		t.Error("a stale admin flag on the principal let a requester through")
	}
}

func TestRoleFilterDeniesDisabledUsers(t *testing.T) {
	f := newRoleFixture()
	f.identity.Put(identityapi.User{ID: admin.User, Username: "admin", Role: identityapi.RoleAdmin, Disabled: true})

	if f.filter.Allows(context.Background(), admin, scopeOf(fileImported{Title: f.myTitle})) {
		t.Error("a disabled admin was allowed")
	}
}

func TestRoleFilterDeniesOnLookupErrors(t *testing.T) {
	f := newRoleFixture()
	f.identity.FailAlways(identitytest.ReaderUser, errors.New("database gone"))

	if f.filter.Allows(context.Background(), requester, scopeOf(fileImported{Title: f.myTitle})) {
		t.Error("allowed despite a failed lookup")
	}
}

func TestRoleFilterWithoutReadersUsesPrincipal(t *testing.T) {
	var filter RoleFilter
	s := scopeOf(fileImported{Title: domain.NewID[domain.TitleID]()})

	if !filter.Allows(context.Background(), admin, s) || filter.Allows(context.Background(), requester, s) {
		t.Error("without readers, only the principal's admin flag should pass")
	}
}
