package identity_test

import (
	"context"
	"net/http"
	"testing"

	"magpie/internal/domain"
	"magpie/internal/identity"
	"magpie/internal/identity/api"
)

const (
	bobJellyfinID = "b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0"
	bobName       = "bob"
	bobPassword   = "bob jellyfin password"
)

var bob = identity.JellyfinUser{ID: bobJellyfinID, Name: bobName}

func (e *env) localAdmin() api.User {
	e.t.Helper()
	admins, err := e.repo.Admins(context.Background())
	if err != nil || len(admins) != 1 {
		e.t.Fatalf("admins = %+v, %v", admins, err)
	}
	return admins[0]
}

// requester signs a Jellyfin user in and returns their Magpie user and session.
func (e *env) requester(u identity.JellyfinUser, password string) (api.User, *http.Cookie) {
	e.t.Helper()
	e.jellyfin.AddUser(u, password)
	cookie := e.signInJellyfin(u.Name, password)
	return e.userByJellyfinID(u.ID), cookie
}

func (e *env) listUsers() []identity.UserResponse {
	e.t.Helper()
	rec := e.do(http.MethodGet, pathUsers, nil, e.admin)
	if rec.Code != http.StatusOK {
		e.t.Fatalf("list users = %d: %s", rec.Code, rec.Body)
	}
	return decode[[]identity.UserResponse](e.t, rec)
}

func TestAdminListsUsers(t *testing.T) {
	e := newEnv(t)
	e.requester(alice, alicePassword)

	users := e.listUsers()
	if len(users) != 2 || users[0].Username != adminUsername || users[1].Username != aliceName {
		t.Fatalf("users = %+v, want admin then alice", users)
	}
	if users[0].Source != api.UserSourceLocal || !users[0].LocalAdmin || users[1].Source != api.UserSourceJellyfin {
		t.Errorf("sources = %s/%v, %s", users[0].Source, users[0].LocalAdmin, users[1].Source)
	}
	if users[1].LastLoginAt == nil {
		t.Errorf("alice has no last sign-in time")
	}
}

func TestAdminChangesRoleAndToggles(t *testing.T) {
	e := newEnv(t)
	user, _ := e.requester(alice, alicePassword)

	rec := e.patchUser(user.ID, identity.UpdateUserRequest{AutoApproveTV: ptr(true), ReportIssues: ptr(false)}, e.admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch = %d: %s", rec.Code, rec.Body)
	}
	got := decode[identity.UserResponse](t, rec)
	want := api.Permissions{AutoApproveTV: true}
	if got.Permissions != want || got.Role != api.RoleRequester {
		t.Errorf("after patch = %+v, want %+v", got, want)
	}

	rec = e.patchUser(user.ID, identity.UpdateUserRequest{Role: ptr(api.RoleAdmin)}, e.admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("promote = %d: %s", rec.Code, rec.Body)
	}
	if got := decode[identity.UserResponse](t, rec); got.Role != api.RoleAdmin || !got.Permissions.ViewOthersRequests {
		t.Errorf("promoted = %+v, want an admin with every permission", got)
	}
}

func TestUnknownRoleIsRejected(t *testing.T) {
	e := newEnv(t)
	user, _ := e.requester(alice, alicePassword)

	if rec := e.patchUser(user.ID, identity.UpdateUserRequest{Role: ptr(api.Role("OWNER"))}, e.admin); rec.Code != http.StatusBadRequest {
		t.Errorf("patch with role OWNER = %d, want 400", rec.Code)
	}
}

func TestPatchingAnUnknownUserIsNotFound(t *testing.T) {
	e := newEnv(t)
	if rec := e.patchUser(domain.UserID("missing"), identity.UpdateUserRequest{Disabled: ptr(true)}, e.admin); rec.Code != http.StatusNotFound {
		t.Errorf("patch unknown user = %d, want 404", rec.Code)
	}
}

// Seed 9.
func TestTheLastAdminCannotBeDemotedOrDisabled(t *testing.T) {
	e := newEnv(t)
	admin := e.localAdmin()

	for name, change := range map[string]identity.UpdateUserRequest{
		"demote":  {Role: ptr(api.RoleRequester)},
		"disable": {Disabled: ptr(true)},
	} {
		rec := e.patchUser(admin.ID, change, e.admin)
		if rec.Code != http.StatusConflict {
			t.Errorf("%s the last admin = %d, want 409", name, rec.Code)
			continue
		}
		if got := problemType(t, rec); got != string(identity.TypeLastAdmin) {
			t.Errorf("%s type = %q, want %q", name, got, identity.TypeLastAdmin)
		}
	}
	if got := e.localAdmin(); got != admin {
		t.Errorf("admin changed: %+v, was %+v", got, admin)
	}
}

func TestTheLocalAdminCannotBeChanged(t *testing.T) {
	e := newEnv(t)
	local := e.localAdmin()
	promoted, cookie := e.requester(alice, alicePassword)
	if rec := e.patchUser(promoted.ID, identity.UpdateUserRequest{Role: ptr(api.RoleAdmin)}, e.admin); rec.Code != http.StatusOK {
		t.Fatalf("promote = %d", rec.Code)
	}

	for name, change := range map[string]identity.UpdateUserRequest{
		"demote":  {Role: ptr(api.RoleRequester)},
		"disable": {Disabled: ptr(true)},
	} {
		rec := e.patchUser(local.ID, change, cookie)
		if rec.Code != http.StatusConflict {
			t.Errorf("%s the local admin = %d, want 409", name, rec.Code)
			continue
		}
		if got := problemType(t, rec); got != string(identity.TypeLocalAdminLocked) {
			t.Errorf("%s type = %q, want %q", name, got, identity.TypeLocalAdminLocked)
		}
	}
}

func TestAdminsCannotChangeThemselves(t *testing.T) {
	e := newEnv(t)
	promoted, cookie := e.requester(alice, alicePassword)
	if rec := e.patchUser(promoted.ID, identity.UpdateUserRequest{Role: ptr(api.RoleAdmin)}, e.admin); rec.Code != http.StatusOK {
		t.Fatalf("promote = %d", rec.Code)
	}

	rec := e.patchUser(promoted.ID, identity.UpdateUserRequest{Role: ptr(api.RoleRequester)}, cookie)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("demote yourself = %d, want 403", rec.Code)
	}
	if got := problemType(t, rec); got != string(identity.TypeOwnAccount) {
		t.Errorf("type = %q, want %q", got, identity.TypeOwnAccount)
	}
}

// Seed 10.
func TestRequestersCannotPatchTheirOwnToggles(t *testing.T) {
	e := newEnv(t)
	user, cookie := e.requester(alice, alicePassword)

	rec := e.patchUser(user.ID, identity.UpdateUserRequest{AutoApproveMovies: ptr(true)}, cookie)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("requester patching themselves = %d, want 403", rec.Code)
	}
	if got := e.userByJellyfinID(aliceJellyfinID).Permissions.AutoApproveMovies; got {
		t.Errorf("auto-approve movies changed to %v", got)
	}
}

// Seed 13.
func TestDisablingAUserDeletesTheirSessions(t *testing.T) {
	e := newEnv(t)
	user, first := e.requester(alice, alicePassword)
	second := e.signInJellyfin(aliceName, alicePassword)
	_, other := e.requester(bob, bobPassword)

	if rec := e.patchUser(user.ID, identity.UpdateUserRequest{Disabled: ptr(true)}, e.admin); rec.Code != http.StatusOK {
		t.Fatalf("disable = %d: %s", rec.Code, rec.Body)
	}

	for name, cookie := range map[string]*http.Cookie{"first": first, "second": second} {
		if rec := e.do(http.MethodGet, pathMe, nil, cookie); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s session after disable = %d, want 401", name, rec.Code)
		}
	}
	if left := e.sessionsOf(user.ID); left != 0 {
		t.Errorf("%d sessions left for the disabled user", left)
	}
	if rec := e.do(http.MethodGet, pathMe, nil, other); rec.Code != http.StatusOK {
		t.Errorf("another user's session = %d, want 200", rec.Code)
	}
}

func TestDefaultsStartWithReportIssuesOnly(t *testing.T) {
	e := newEnv(t)

	rec := e.do(http.MethodGet, pathDefaults, nil, e.admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("get defaults = %d", rec.Code)
	}
	if got, want := decode[api.Permissions](t, rec), (api.Permissions{ReportIssues: true}); got != want {
		t.Errorf("defaults = %+v, want %+v", got, want)
	}
	settings := decode[identity.SignInSettings](t, e.do(http.MethodGet, pathSignInSettings, nil, e.admin))
	if !settings.AllowJellyfin {
		t.Errorf("Jellyfin sign-in is off by default")
	}
}

func TestUserAdminRoutesAreForAdminsOnly(t *testing.T) {
	e := newEnv(t)
	_, cookie := e.requester(alice, alicePassword)

	for _, route := range []struct{ method, path string }{
		{http.MethodGet, pathUsers},
		{http.MethodGet, pathDefaults},
		{http.MethodPut, pathDefaults},
		{http.MethodGet, pathSignInSettings},
		{http.MethodPut, pathSignInSettings},
	} {
		if rec := e.do(route.method, route.path, api.Permissions{}, cookie); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s as a requester = %d, want 403", route.method, route.path, rec.Code)
		}
	}
}

func TestJellyfinUserNamedLikeTheLocalAdminGetsADistinctUsername(t *testing.T) {
	e := newEnv(t)
	namesake := identity.JellyfinUser{ID: aliceJellyfinID, Name: adminUsername}
	e.requester(namesake, alicePassword)

	user := e.userByJellyfinID(aliceJellyfinID)
	if user.Username == adminUsername || user.LocalAdmin || user.Role != api.RoleRequester {
		t.Errorf("namesake = %+v, want a separate requester", user)
	}
	if user.DisplayName != adminUsername {
		t.Errorf("display name = %q, want %q", user.DisplayName, adminUsername)
	}
}

func (e *env) sessionsOf(id domain.UserID) int64 {
	e.t.Helper()
	var n int64
	if err := e.db.Read(context.Background()).Table(identitySessionTable).Where("user_id = ?", id).Count(&n).Error; err != nil {
		e.t.Fatal(err)
	}
	return n
}
