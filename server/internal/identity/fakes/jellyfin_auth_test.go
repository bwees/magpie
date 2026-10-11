package fakes

import (
	"context"
	"errors"
	"testing"

	"magpie/internal/identity"
)

const (
	testUsername = "alice"
	testPassword = "hunter2"
	codeDigits   = 6
)

func TestJellyfinAuthSignsInKnownAccounts(t *testing.T) {
	ctx := context.Background()
	f := NewJellyfinAuth()
	f.AddUser(identity.JellyfinUser{ID: "jf-1", Name: testUsername}, testPassword)

	if u, err := f.AuthenticateByName(ctx, testUsername, testPassword); err != nil || u.ID != "jf-1" {
		t.Fatalf("AuthenticateByName = %+v, %v", u, err)
	}
	if _, err := f.AuthenticateByName(ctx, testUsername, "wrong"); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Errorf("wrong password err = %v", err)
	}
}

func TestJellyfinAuthQuickConnectLifecycle(t *testing.T) {
	ctx := context.Background()
	f := NewJellyfinAuth()
	f.AddUser(identity.JellyfinUser{ID: "jf-1", Name: testUsername}, testPassword)

	qc, err := f.QuickConnectInitiate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(qc.Code) != codeDigits {
		t.Errorf("code %q is not 6 digits", qc.Code)
	}
	if ok, err := f.QuickConnectPoll(ctx, qc.Secret); ok || err != nil {
		t.Fatalf("poll before approval = %v, %v", ok, err)
	}
	if err := f.ApproveQuickConnect(qc.Secret, testUsername); err != nil {
		t.Fatal(err)
	}
	if ok, err := f.QuickConnectPoll(ctx, qc.Secret); !ok || err != nil {
		t.Fatalf("poll after approval = %v, %v", ok, err)
	}
	if u, err := f.AuthenticateWithQuickConnect(ctx, qc.Secret); err != nil || u.ID != "jf-1" {
		t.Fatalf("AuthenticateWithQuickConnect = %+v, %v", u, err)
	}

	f.ExpireQuickConnect(qc.Secret)
	if _, err := f.QuickConnectPoll(ctx, qc.Secret); !errors.Is(err, identity.ErrExpired) {
		t.Errorf("poll after expiry err = %v", err)
	}
}

func TestJellyfinAuthScriptedOutage(t *testing.T) {
	f := NewJellyfinAuth()
	f.FailAlways(JellyfinAuthAuthenticateByName, identity.ErrUnavailable)

	if _, err := f.AuthenticateByName(context.Background(), testUsername, testPassword); !errors.Is(err, identity.ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
}
