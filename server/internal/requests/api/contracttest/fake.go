package contracttest

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"time"

	"magpie/internal/domain"
	"magpie/internal/platform/scripted"
	"magpie/internal/requests/api"
)

const (
	ReaderRequest          scripted.Method = "requests.Reader.Request"
	ReaderRequestsForTitle scripted.Method = "requests.Reader.RequestsForTitle"
	ReaderRequesters       scripted.Method = "requests.Reader.Requesters"
	ReaderOpenIssues       scripted.Method = "requests.Reader.OpenIssues"
	ReaderIssue            scripted.Method = "requests.Reader.Issue"
)

// Fake is the in-memory requests.Reader other contexts test against. It passes Run.
type Fake struct {
	scripted.Script

	mu       sync.RWMutex
	requests map[domain.RequestID]api.Request
	issues   map[domain.IssueID]api.Issue
}

var _ api.Reader = (*Fake)(nil)

func NewFake(f Fixture) *Fake {
	fake := &Fake{requests: map[domain.RequestID]api.Request{}, issues: map[domain.IssueID]api.Issue{}}
	for _, r := range f.Requests {
		fake.PutRequest(r)
	}
	for _, i := range f.Issues {
		fake.PutIssue(i)
	}
	return fake
}

// PutRequest adds or replaces a request.
func (f *Fake) PutRequest(r api.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests[r.ID] = r
}

// PutIssue adds or replaces an issue.
func (f *Fake) PutIssue(i api.Issue) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.issues[i.ID] = i
}

func (f *Fake) Request(ctx context.Context, id domain.RequestID) (api.Request, error) {
	if err := f.Enter(ctx, ReaderRequest); err != nil {
		return api.Request{}, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	r, ok := f.requests[id]
	if !ok {
		return api.Request{}, api.ErrNotFound
	}
	return r, nil
}

func (f *Fake) RequestsForTitle(ctx context.Context, title domain.TitleID) ([]api.Request, error) {
	if err := f.Enter(ctx, ReaderRequestsForTitle); err != nil {
		return nil, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	found := []api.Request{}
	for _, r := range f.requests {
		if r.Title != nil && *r.Title == title {
			found = append(found, r)
		}
	}
	slices.SortFunc(found, func(a, b api.Request) int { return newestFirst(a.CreatedAt, b.CreatedAt, a.ID, b.ID) })
	return found, nil
}

func (f *Fake) Requesters(ctx context.Context, title domain.TitleID) ([]domain.UserID, error) {
	if err := f.Enter(ctx, ReaderRequesters); err != nil {
		return nil, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	users := []domain.UserID{}
	for _, r := range f.requests {
		if r.Title != nil && *r.Title == title && r.Status != api.StatusDeclined && !slices.Contains(users, r.User) {
			users = append(users, r.User)
		}
	}
	return users, nil
}

func (f *Fake) OpenIssues(ctx context.Context, title *domain.TitleID) ([]api.Issue, error) {
	if err := f.Enter(ctx, ReaderOpenIssues); err != nil {
		return nil, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	open := []api.Issue{}
	for _, i := range f.issues {
		if i.Status != api.IssueStatusResolved && (title == nil || i.Title == *title) {
			open = append(open, i)
		}
	}
	slices.SortFunc(open, func(a, b api.Issue) int { return newestFirst(a.CreatedAt, b.CreatedAt, a.ID, b.ID) })
	return open, nil
}

func (f *Fake) Issue(ctx context.Context, id domain.IssueID) (api.Issue, error) {
	if err := f.Enter(ctx, ReaderIssue); err != nil {
		return api.Issue{}, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	i, ok := f.issues[id]
	if !ok {
		return api.Issue{}, api.ErrNotFound
	}
	return i, nil
}

// newestFirst orders by creation time, then by ULID, both descending.
func newestFirst[ID ~string](aAt, bAt time.Time, aID, bID ID) int {
	return cmp.Or(bAt.Compare(aAt), cmp.Compare(bID, aID))
}
