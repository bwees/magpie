// Package fakes holds in-memory fakes of Identity's ports, for tests and for booting without real adapters.
package fakes

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"magpie/internal/domain"
	"magpie/internal/identity"
	"magpie/internal/platform/scripted"
)

const (
	JellyfinAuthAuthenticateByName           scripted.Method = "JellyfinAuth.AuthenticateByName"
	JellyfinAuthQuickConnectEnabled          scripted.Method = "JellyfinAuth.QuickConnectEnabled"
	JellyfinAuthQuickConnectInitiate         scripted.Method = "JellyfinAuth.QuickConnectInitiate"
	JellyfinAuthQuickConnectPoll             scripted.Method = "JellyfinAuth.QuickConnectPoll"
	JellyfinAuthAuthenticateWithQuickConnect scripted.Method = "JellyfinAuth.AuthenticateWithQuickConnect"

	quickConnectLifetime  = 10 * time.Minute
	quickConnectCodeSpace = 1_000_000
	quickConnectCodeShape = "%06d"
)

type jellyfinAccount struct {
	user     identity.JellyfinUser
	password string
}

type quickConnectSession struct {
	code      string
	expiresAt time.Time
	approved  *identity.JellyfinUser
}

type quickConnectSecret string

// JellyfinAuth holds Jellyfin accounts and Quick Connect sessions in memory.
type JellyfinAuth struct {
	scripted.Script

	mu           sync.Mutex
	now          func() time.Time
	enabled      bool
	accounts     map[string]jellyfinAccount
	quickConnect map[string]*quickConnectSession
}

var _ identity.JellyfinAuth = (*JellyfinAuth)(nil)

// NewJellyfinAuth starts with no accounts and Quick Connect enabled.
func NewJellyfinAuth() *JellyfinAuth {
	return &JellyfinAuth{
		now:          time.Now,
		enabled:      true,
		accounts:     map[string]jellyfinAccount{},
		quickConnect: map[string]*quickConnectSession{},
	}
}

// AddUser registers an account that AuthenticateByName accepts with password.
func (f *JellyfinAuth) AddUser(u identity.JellyfinUser, password string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.accounts[strings.ToLower(u.Name)] = jellyfinAccount{user: u, password: password}
}

func (f *JellyfinAuth) SetQuickConnectEnabled(enabled bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.enabled = enabled
}

// ApproveQuickConnect signs the account with username in on the session behind secret.
func (f *JellyfinAuth) ApproveQuickConnect(secret, username string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	session, err := f.session(secret)
	if err != nil {
		return err
	}
	account, ok := f.accounts[strings.ToLower(username)]
	if !ok {
		return identity.ErrInvalidCredentials
	}
	session.approved = &account.user
	return nil
}

// SecretForCode finds the secret behind the code a user would type into Jellyfin, or "" if none matches.
func (f *JellyfinAuth) SecretForCode(code string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for secret, session := range f.quickConnect {
		if session.code == code {
			return secret
		}
	}
	return ""
}

// ExpireQuickConnect makes Jellyfin forget secret.
func (f *JellyfinAuth) ExpireQuickConnect(secret string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.quickConnect, secret)
}

func (f *JellyfinAuth) AuthenticateByName(ctx context.Context, username, password string) (identity.JellyfinUser, error) {
	if err := f.Enter(ctx, JellyfinAuthAuthenticateByName); err != nil {
		return identity.JellyfinUser{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	account, ok := f.accounts[strings.ToLower(username)]
	if !ok || account.password != password {
		return identity.JellyfinUser{}, identity.ErrInvalidCredentials
	}
	return account.user, nil
}

func (f *JellyfinAuth) QuickConnectEnabled(ctx context.Context) (bool, error) {
	if err := f.Enter(ctx, JellyfinAuthQuickConnectEnabled); err != nil {
		return false, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.enabled, nil
}

func (f *JellyfinAuth) QuickConnectInitiate(ctx context.Context) (identity.QuickConnect, error) {
	if err := f.Enter(ctx, JellyfinAuthQuickConnectInitiate); err != nil {
		return identity.QuickConnect{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	qc := identity.QuickConnect{
		Code:      fmt.Sprintf(quickConnectCodeShape, rand.IntN(quickConnectCodeSpace)),
		Secret:    string(domain.NewID[quickConnectSecret]()),
		ExpiresAt: f.now().Add(quickConnectLifetime),
	}
	f.quickConnect[qc.Secret] = &quickConnectSession{code: qc.Code, expiresAt: qc.ExpiresAt}
	return qc, nil
}

func (f *JellyfinAuth) QuickConnectPoll(ctx context.Context, secret string) (bool, error) {
	if err := f.Enter(ctx, JellyfinAuthQuickConnectPoll); err != nil {
		return false, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	session, err := f.session(secret)
	if err != nil {
		return false, err
	}
	return session.approved != nil, nil
}

func (f *JellyfinAuth) AuthenticateWithQuickConnect(ctx context.Context, secret string) (identity.JellyfinUser, error) {
	if err := f.Enter(ctx, JellyfinAuthAuthenticateWithQuickConnect); err != nil {
		return identity.JellyfinUser{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	session, err := f.session(secret)
	if err != nil {
		return identity.JellyfinUser{}, err
	}
	if session.approved == nil {
		return identity.JellyfinUser{}, identity.ErrInvalidCredentials
	}
	return *session.approved, nil
}

func (f *JellyfinAuth) session(secret string) (*quickConnectSession, error) {
	session, ok := f.quickConnect[secret]
	if !ok || !f.now().Before(session.expiresAt) {
		return nil, identity.ErrExpired
	}
	return session, nil
}
