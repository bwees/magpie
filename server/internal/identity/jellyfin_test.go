package identity_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"magpie/internal/identity"
	"magpie/internal/identity/api"
	"magpie/internal/identity/fakes"
)

const (
	aliceJellyfinID = "a1b2c3d4e5f60718293a4b5c6d7e8f90"
	aliceName       = "alice"
	alicePassword   = "alice jellyfin password"
	wrongPassword   = "not the password"
	failedSignInCap = 10
)

var alice = identity.JellyfinUser{ID: aliceJellyfinID, Name: aliceName}

// Seed 3.
func TestFirstJellyfinSignInCreatesARequesterWithTheDefaults(t *testing.T) {
	e := newEnv(t)
	defaults := api.Permissions{AutoApproveMovies: true, ViewOthersRequests: true}
	if rec := e.do(http.MethodPut, pathDefaults, defaults, e.admin); rec.Code != http.StatusOK {
		t.Fatalf("put defaults = %d: %s", rec.Code, rec.Body)
	}
	e.jellyfin.AddUser(alice, alicePassword)

	cookie := e.signInJellyfin(aliceName, alicePassword)

	user := e.userByJellyfinID(aliceJellyfinID)
	if user.Role != api.RoleRequester || user.Permissions != defaults || user.Username != aliceName {
		t.Errorf("user = %+v, want a requester named %s with %+v", user, aliceName, defaults)
	}
	var jellyfinEvents []api.UserCreated
	for _, ev := range e.userCreated() {
		if ev.Source == api.UserSourceJellyfin {
			jellyfinEvents = append(jellyfinEvents, ev)
		}
	}
	want := api.UserCreated{User: user.ID, Role: api.RoleRequester, Source: api.UserSourceJellyfin}
	if len(jellyfinEvents) != 1 || jellyfinEvents[0] != want {
		t.Errorf("UserCreated{JELLYFIN} = %+v, want [%+v]", jellyfinEvents, want)
	}

	me := decode[identity.MeResponse](t, e.do(http.MethodGet, pathMe, nil, cookie))
	if me.ID != user.ID || me.Role != api.RoleRequester {
		t.Errorf("me = %+v", me)
	}
}

// Seed 4.
func TestRepeatJellyfinSignInReusesTheUser(t *testing.T) {
	e := newEnv(t)
	e.jellyfin.AddUser(alice, alicePassword)
	e.signInJellyfin(aliceName, alicePassword)
	first := e.lastLogin(aliceJellyfinID)
	users, events := e.rowCount(identityUsersTable), len(e.userCreated())

	renamed := alice
	renamed.Name = "Alice"
	e.jellyfin.AddUser(renamed, alicePassword)
	e.signInJellyfin(aliceName, alicePassword)

	if got := e.rowCount(identityUsersTable); got != users {
		t.Errorf("users = %d after the second sign-in, want %d", got, users)
	}
	if got := len(e.userCreated()); got != events {
		t.Errorf("UserCreated events = %d after the second sign-in, want %d", got, events)
	}
	if second := e.lastLogin(aliceJellyfinID); !second.After(first) {
		t.Errorf("last_login_at %v did not move past %v", second, first)
	}
	if got := e.userByJellyfinID(aliceJellyfinID).DisplayName; got != renamed.Name {
		t.Errorf("display name = %q, want Jellyfin's %q", got, renamed.Name)
	}
}

// Seed 5.
func TestJellyfinAdministratorsSignInAsRequesters(t *testing.T) {
	e := newEnv(t)
	jellyfinAdmin := alice
	jellyfinAdmin.IsAdministrator = true
	e.jellyfin.AddUser(jellyfinAdmin, alicePassword)

	e.signInJellyfin(aliceName, alicePassword)

	if got := e.userByJellyfinID(aliceJellyfinID).Role; got != api.RoleRequester {
		t.Errorf("role = %s, want %s", got, api.RoleRequester)
	}
}

// Seed 6.
func TestJellyfinDownAnswers503AndLocalAdminStillSignsIn(t *testing.T) {
	e := newEnv(t)
	e.jellyfin.FailAlways(fakes.JellyfinAuthAuthenticateByName, identity.ErrUnavailable)

	rec := e.do(http.MethodPost, pathLoginJellyfin, identity.CredentialsRequest{Username: aliceName, Password: alicePassword}, nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("Jellyfin sign-in = %d, want 503", rec.Code)
	}
	if got := problemType(t, rec); got != string(identity.TypeJellyfinUnavailable) {
		t.Errorf("type = %q, want %q", got, identity.TypeJellyfinUnavailable)
	}
	rec = e.do(http.MethodPost, pathLoginLocal, identity.CredentialsRequest{Username: adminUsername, Password: adminPassword}, nil)
	if rec.Code != http.StatusOK {
		t.Errorf("local admin sign-in while Jellyfin is down = %d, want 200", rec.Code)
	}
}

// Seed 7.
func TestJellyfinDisabledAccountIsRejectedWithoutCreatingAUser(t *testing.T) {
	e := newEnv(t)
	disabled := alice
	disabled.IsDisabled = true
	e.jellyfin.AddUser(disabled, alicePassword)
	users := e.rowCount(identityUsersTable)

	rec := e.do(http.MethodPost, pathLoginJellyfin, identity.CredentialsRequest{Username: aliceName, Password: alicePassword}, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("sign-in = %d, want 403", rec.Code)
	}
	if got := problemType(t, rec); got != string(identity.TypeJellyfinUserDisabled) {
		t.Errorf("type = %q, want %q", got, identity.TypeJellyfinUserDisabled)
	}
	if got := e.rowCount(identityUsersTable); got != users {
		t.Errorf("users = %d, want %d", got, users)
	}
}

func TestDisabledMagpieUserCannotSignInThroughJellyfin(t *testing.T) {
	e := newEnv(t)
	e.jellyfin.AddUser(alice, alicePassword)
	e.signInJellyfin(aliceName, alicePassword)
	user := e.userByJellyfinID(aliceJellyfinID)
	if rec := e.patchUser(user.ID, identity.UpdateUserRequest{Disabled: ptr(true)}, e.admin); rec.Code != http.StatusOK {
		t.Fatalf("disable = %d: %s", rec.Code, rec.Body)
	}

	rec := e.do(http.MethodPost, pathLoginJellyfin, identity.CredentialsRequest{Username: aliceName, Password: alicePassword}, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("sign-in = %d, want 403", rec.Code)
	}
	if got := problemType(t, rec); got != string(identity.TypeUserDisabled) {
		t.Errorf("type = %q, want %q", got, identity.TypeUserDisabled)
	}
}

func TestWrongJellyfinPasswordCountsTowardsTheRateLimit(t *testing.T) {
	e := newEnv(t)
	e.jellyfin.AddUser(alice, alicePassword)
	wrong := identity.CredentialsRequest{Username: aliceName, Password: wrongPassword}

	for range failedSignInCap {
		rec := e.do(http.MethodPost, pathLoginJellyfin, wrong, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("wrong password = %d, want 401", rec.Code)
		}
	}
	rec := e.do(http.MethodPost, pathLoginJellyfin, identity.CredentialsRequest{Username: aliceName, Password: alicePassword}, nil)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("11th sign-in = %d, want 429", rec.Code)
	}
}

func TestJellyfinSignInCanBeTurnedOff(t *testing.T) {
	e := newEnv(t)
	e.jellyfin.AddUser(alice, alicePassword)
	if rec := e.do(http.MethodPut, pathSignInSettings, identity.SignInSettings{AllowJellyfin: false}, e.admin); rec.Code != http.StatusOK {
		t.Fatalf("put sign-in settings = %d: %s", rec.Code, rec.Body)
	}

	methods := decode[identity.SignInMethodsResponse](t, e.do(http.MethodGet, pathLoginMethods, nil, nil))
	if methods.Jellyfin || methods.QuickConnect {
		t.Errorf("methods = %+v, want Jellyfin hidden", methods)
	}
	rec := e.do(http.MethodPost, pathLoginJellyfin, identity.CredentialsRequest{Username: aliceName, Password: alicePassword}, nil)
	if rec.Code != http.StatusForbidden {
		t.Errorf("Jellyfin sign-in while off = %d, want 403", rec.Code)
	}
	if rec := e.do(http.MethodPost, pathQuickConnect, nil, nil); rec.Code != http.StatusForbidden {
		t.Errorf("Quick Connect while off = %d, want 403", rec.Code)
	}
	if rec := e.do(http.MethodPost, pathLoginLocal, identity.CredentialsRequest{Username: adminUsername, Password: adminPassword}, nil); rec.Code != http.StatusOK {
		t.Errorf("local admin while Jellyfin sign-in is off = %d, want 200", rec.Code)
	}
}

func TestSignInMethodsDefaultToJellyfinAndQuickConnect(t *testing.T) {
	e := newEnv(t)

	methods := decode[identity.SignInMethodsResponse](t, e.do(http.MethodGet, pathLoginMethods, nil, nil))
	if !methods.Jellyfin || !methods.QuickConnect || !methods.JellyfinReachable {
		t.Errorf("methods = %+v, want everything on", methods)
	}

	e.jellyfin.SetQuickConnectEnabled(false)
	methods = decode[identity.SignInMethodsResponse](t, e.do(http.MethodGet, pathLoginMethods, nil, nil))
	if !methods.Jellyfin || methods.QuickConnect {
		t.Errorf("methods with Quick Connect off in Jellyfin = %+v", methods)
	}

	e.jellyfin.FailAlways(fakes.JellyfinAuthQuickConnectEnabled, identity.ErrUnavailable)
	methods = decode[identity.SignInMethodsResponse](t, e.do(http.MethodGet, pathLoginMethods, nil, nil))
	if !methods.Jellyfin || methods.JellyfinReachable {
		t.Errorf("methods with Jellyfin down = %+v, want the form shown as unreachable", methods)
	}
}

// Seed 8.
func TestQuickConnectPendingApprovedAndExpired(t *testing.T) {
	e := newEnv(t)
	e.jellyfin.AddUser(alice, alicePassword)

	start := e.startQuickConnect()
	if len(start.Code) != quickConnectDigits || start.Handle == "" {
		t.Fatalf("start = %+v", start)
	}
	poll := e.do(http.MethodGet, pathQuickConnect+urlPathSeparator+start.Handle, nil, nil)
	if poll.Code != http.StatusAccepted {
		t.Fatalf("poll before approval = %d, want 202: %s", poll.Code, poll.Body)
	}
	if got := decode[identity.QuickConnectPollResponse](t, poll).Status; got != identity.QuickConnectPending {
		t.Errorf("status = %s, want %s", got, identity.QuickConnectPending)
	}

	if err := e.jellyfin.ApproveQuickConnect(e.jellyfin.SecretForCode(start.Code), aliceName); err != nil {
		t.Fatal(err)
	}
	poll = e.do(http.MethodGet, pathQuickConnect+urlPathSeparator+start.Handle, nil, nil)
	if poll.Code != http.StatusOK {
		t.Fatalf("poll after approval = %d, want 200: %s", poll.Code, poll.Body)
	}
	cookie := sessionCookie(t, poll)
	approved := decode[identity.QuickConnectPollResponse](t, poll)
	if approved.Status != identity.QuickConnectApproved || approved.User == nil || approved.User.Role != api.RoleRequester {
		t.Errorf("approved = %+v", approved)
	}
	if rec := e.do(http.MethodGet, pathMe, nil, cookie); rec.Code != http.StatusOK {
		t.Errorf("me with the Quick Connect session = %d", rec.Code)
	}

	expiring := e.startQuickConnect()
	e.jellyfin.ExpireQuickConnect(e.jellyfin.SecretForCode(expiring.Code))
	if rec := e.do(http.MethodGet, pathQuickConnect+urlPathSeparator+expiring.Handle, nil, nil); rec.Code != http.StatusGone {
		t.Errorf("poll after expiry = %d, want 410", rec.Code)
	}
	if rec := e.do(http.MethodGet, pathQuickConnect+urlPathSeparator+"unknown-handle", nil, nil); rec.Code != http.StatusGone {
		t.Errorf("poll of an unknown handle = %d, want 410", rec.Code)
	}
}

func TestQuickConnectTurnedOffInJellyfin(t *testing.T) {
	e := newEnv(t)
	e.jellyfin.SetQuickConnectEnabled(false)

	rec := e.do(http.MethodPost, pathQuickConnect, nil, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("start = %d, want 403", rec.Code)
	}
	if got := problemType(t, rec); got != string(identity.TypeQuickConnectOff) {
		t.Errorf("type = %q, want %q", got, identity.TypeQuickConnectOff)
	}
}

const quickConnectDigits = 6

func (e *env) startQuickConnect() identity.QuickConnectStartResponse {
	e.t.Helper()
	rec := e.do(http.MethodPost, pathQuickConnect, nil, nil)
	if rec.Code != http.StatusOK {
		e.t.Fatalf("start Quick Connect = %d: %s", rec.Code, rec.Body)
	}
	return decode[identity.QuickConnectStartResponse](e.t, rec)
}

func (e *env) lastLogin(jellyfinID string) time.Time {
	e.t.Helper()
	var values []time.Time
	err := e.db.Read(context.Background()).Table(identityUsersTable).
		Where("jellyfin_user_id = ?", jellyfinID).Pluck("last_login_at", &values).Error
	if err != nil || len(values) != 1 {
		e.t.Fatalf("last_login_at = %v, %v", values, err)
	}
	return values[0]
}
