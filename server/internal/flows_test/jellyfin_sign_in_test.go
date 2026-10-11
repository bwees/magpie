package flows_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"magpie/internal/identity"
	identityapi "magpie/internal/identity/api"
	identityfakes "magpie/internal/identity/fakes"

	"go.uber.org/fx"
)

const (
	jellyfinSignInPath = "/api/identity/login/jellyfin"
	usersPath          = "/api/identity/users"
	requesterName      = "alice"
	requesterPassword  = "alice jellyfin password"
)

func TestJellyfinRequesterSignsInAndIsDisabledByTheAdmin(t *testing.T) {
	jellyfin := identityfakes.NewJellyfinAuth()
	jellyfin.AddUser(identity.JellyfinUser{ID: "jf-alice", Name: requesterName, IsAdministrator: true}, requesterPassword)
	h := boot(t, fx.Decorate(func(identity.JellyfinAuth) identity.JellyfinAuth { return jellyfin }))
	h.signInAsAdmin()
	admin := h.session

	h.session = nil
	rec := h.do(http.MethodPost, jellyfinSignInPath, identity.CredentialsRequest{Username: requesterName, Password: requesterPassword})
	if rec.Code != http.StatusOK {
		t.Fatalf("Jellyfin sign-in = %d: %s", rec.Code, rec.Body)
	}
	var me identity.MeResponse
	if err := json.NewDecoder(rec.Body).Decode(&me); err != nil {
		t.Fatal(err)
	}
	if me.Role != identityapi.RoleRequester {
		t.Errorf("Jellyfin administrator signed in as %s, want %s", me.Role, identityapi.RoleRequester)
	}
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == identity.SessionCookie {
			h.session = cookie
		}
	}
	if rec := h.get(mePath); rec.Code != http.StatusOK {
		t.Errorf("me as the requester = %d", rec.Code)
	}
	if rec := h.get(connectionsPath); rec.Code != http.StatusForbidden {
		t.Errorf("connections as a requester = %d, want 403", rec.Code)
	}
	requester := h.session

	h.session = admin
	disabled := true
	if rec := h.do(http.MethodPatch, usersPath+"/"+string(me.ID), identity.UpdateUserRequest{Disabled: &disabled}); rec.Code != http.StatusOK {
		t.Fatalf("disable = %d: %s", rec.Code, rec.Body)
	}
	h.session = requester
	if rec := h.get(mePath); rec.Code != http.StatusUnauthorized {
		t.Errorf("me after being disabled = %d, want 401", rec.Code)
	}
}
