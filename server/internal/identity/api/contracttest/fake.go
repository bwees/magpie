package contracttest

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"sync"

	"magpie/internal/domain"
	"magpie/internal/identity/api"
	"magpie/internal/platform/scripted"
)

const (
	ReaderUser   scripted.Method = "identity.Reader.User"
	ReaderUsers  scripted.Method = "identity.Reader.Users"
	ReaderAdmins scripted.Method = "identity.Reader.Admins"
	ReaderAll    scripted.Method = "identity.Reader.All"
)

// Fake is the in-memory identity.Reader other contexts test against. It passes Run.
type Fake struct {
	scripted.Script

	mu    sync.RWMutex
	users map[domain.UserID]api.User
}

var _ api.Reader = (*Fake)(nil)

func NewFake(f Fixture) *Fake {
	fake := &Fake{users: map[domain.UserID]api.User{}}
	for _, u := range f.Users {
		fake.Put(u)
	}
	return fake
}

// Put adds or replaces a user with its stored toggles.
func (f *Fake) Put(u api.User) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.users[u.ID] = u
}

func (f *Fake) User(ctx context.Context, id domain.UserID) (api.User, error) {
	if err := f.Enter(ctx, ReaderUser); err != nil {
		return api.User{}, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	u, ok := f.users[id]
	if !ok {
		return api.User{}, api.ErrNotFound
	}
	return effective(u), nil
}

func (f *Fake) Users(ctx context.Context, ids []domain.UserID) ([]api.User, error) {
	if err := f.Enter(ctx, ReaderUsers); err != nil {
		return nil, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	found := []api.User{}
	for _, id := range ids {
		if u, ok := f.users[id]; ok {
			found = append(found, effective(u))
		}
	}
	return found, nil
}

func (f *Fake) Admins(ctx context.Context) ([]api.User, error) {
	if err := f.Enter(ctx, ReaderAdmins); err != nil {
		return nil, err
	}
	return f.sorted(func(u api.User) bool { return u.IsAdmin() && !u.Disabled }), nil
}

func (f *Fake) All(ctx context.Context) ([]api.User, error) {
	if err := f.Enter(ctx, ReaderAll); err != nil {
		return nil, err
	}
	return f.sorted(func(api.User) bool { return true }), nil
}

func (f *Fake) sorted(keep func(api.User) bool) []api.User {
	f.mu.RLock()
	defer f.mu.RUnlock()
	users := []api.User{}
	for _, u := range f.users {
		if keep(u) {
			users = append(users, effective(u))
		}
	}
	slices.SortFunc(users, func(a, b api.User) int {
		return cmp.Compare(strings.ToLower(a.Username), strings.ToLower(b.Username))
	})
	return users
}

func effective(u api.User) api.User {
	if u.IsAdmin() {
		u.Permissions = api.Permissions{AutoApproveMovies: true, AutoApproveTV: true, ViewOthersRequests: true, ReportIssues: true}
	}
	return u
}
