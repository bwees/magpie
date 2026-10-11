// Package contracttest holds the identity.Reader contract suite and the in-memory Fake that passes it.
// The real repository runs the same suite in its own tests.
package contracttest

import (
	"context"
	"errors"
	"slices"
	"testing"

	"magpie/internal/domain"
	"magpie/internal/identity/api"
)

// Fixture is the stored state a Reader starts with. User.Permissions are the stored toggles.
type Fixture struct {
	Users []api.User
}

// Factory returns a Reader holding exactly the fixture's state.
type Factory func(t *testing.T, f Fixture) api.Reader

var allPermissions = api.Permissions{AutoApproveMovies: true, AutoApproveTV: true, ViewOthersRequests: true, ReportIssues: true}

// Run checks a Reader against the identity contract.
func Run(t *testing.T, newReader Factory) {
	ctx := context.Background()
	carol := user("carol", api.RoleAdmin)
	alice := user("alice", api.RoleAdmin)
	bob := user("bob", api.RoleRequester)
	bob.Permissions.ReportIssues = true
	dave := user("dave", api.RoleAdmin)
	dave.Disabled = true
	fixture := Fixture{Users: []api.User{carol, alice, bob, dave}}

	t.Run("User returns ErrNotFound for an unknown ID", func(t *testing.T) {
		r := newReader(t, fixture)
		if _, err := r.User(ctx, domain.NewID[domain.UserID]()); !errors.Is(err, api.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("User returns admins with every permission and requesters with their toggles", func(t *testing.T) {
		r := newReader(t, fixture)
		got, err := r.User(ctx, alice.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Permissions != allPermissions {
			t.Errorf("admin permissions = %+v, want all true", got.Permissions)
		}
		got, err = r.User(ctx, bob.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Permissions != bob.Permissions {
			t.Errorf("requester permissions = %+v, want stored %+v", got.Permissions, bob.Permissions)
		}
	})

	t.Run("User returns disabled users", func(t *testing.T) {
		r := newReader(t, fixture)
		got, err := r.User(ctx, dave.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !got.Disabled {
			t.Error("Disabled = false")
		}
	})

	t.Run("Users omits unknown IDs and keeps input order", func(t *testing.T) {
		r := newReader(t, fixture)
		got, err := r.Users(ctx, []domain.UserID{bob.ID, domain.NewID[domain.UserID](), carol.ID})
		if err != nil {
			t.Fatal(err)
		}
		assertIDs(t, got, bob, carol)
	})

	t.Run("Admins lists enabled admins ordered by username", func(t *testing.T) {
		r := newReader(t, fixture)
		got, err := r.Admins(ctx)
		if err != nil {
			t.Fatal(err)
		}
		assertIDs(t, got, alice, carol)
	})

	t.Run("All lists every user ordered by username", func(t *testing.T) {
		r := newReader(t, fixture)
		got, err := r.All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		assertIDs(t, got, alice, bob, carol, dave)
	})
}

func user(username string, role api.Role) api.User {
	return api.User{ID: domain.NewID[domain.UserID](), Username: username, DisplayName: username, Role: role}
}

func assertIDs(t *testing.T, got []api.User, want ...api.User) {
	t.Helper()
	gotIDs := make([]domain.UserID, 0, len(got))
	for _, u := range got {
		gotIDs = append(gotIDs, u.ID)
	}
	wantIDs := make([]domain.UserID, 0, len(want))
	for _, u := range want {
		wantIDs = append(wantIDs, u.ID)
	}
	if !slices.Equal(gotIDs, wantIDs) {
		t.Errorf("users = %v, want %v", gotIDs, wantIDs)
	}
}
