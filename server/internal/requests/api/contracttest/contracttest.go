// Package contracttest holds the requests.Reader contract suite and the in-memory Fake that passes it.
// The real repository runs the same suite in its own tests.
package contracttest

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"magpie/internal/domain"
	"magpie/internal/requests/api"
)

// Fixture is the stored state a Reader starts with.
type Fixture struct {
	Requests []api.Request
	Issues   []api.Issue
}

// Factory returns a Reader holding exactly the fixture's state.
type Factory func(t *testing.T, f Fixture) api.Reader

const (
	duneTmdb domain.TmdbID = 693134
	step                   = time.Hour
)

var start = time.Date(2026, time.October, 1, 12, 0, 0, 0, time.UTC)

// Run checks a Reader against the whole requests contract.
func Run(t *testing.T, newReader Factory) {
	RunRequests(t, newReader)
	runIssues(t, newReader)
}

// RunRequests checks Request, RequestsForTitle and Requesters. The fixture holds requests only, so a
// repository that doesn't store issues yet can run this part.
func RunRequests(t *testing.T, newReader Factory) {
	ctx := context.Background()
	dune, other := domain.NewID[domain.TitleID](), domain.NewID[domain.TitleID]()
	alice, bob, carol := domain.NewID[domain.UserID](), domain.NewID[domain.UserID](), domain.NewID[domain.UserID]()

	oldest := request(alice, &dune, api.StatusAvailable, 0)
	declined := request(bob, &dune, api.StatusDeclined, 1)
	again := request(alice, &dune, api.StatusPartiallyAvailable, 2)
	again.AvailableItems, again.TotalItems = 1, 3
	newest := request(carol, &dune, api.StatusFailed, 3)
	unlinked := request(bob, nil, api.StatusPending, 4)

	fixture := Fixture{Requests: []api.Request{again, unlinked, oldest, newest, declined}}

	t.Run("Request returns ErrNotFound for an unknown ID", func(t *testing.T) {
		r := newReader(t, fixture)
		if _, err := r.Request(ctx, domain.NewID[domain.RequestID]()); !errors.Is(err, api.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
		got, err := r.Request(ctx, again.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.User != alice || got.Status != api.StatusPartiallyAvailable || got.Title == nil || *got.Title != dune {
			t.Errorf("Request = %+v", got)
		}
		if got.AvailableItems != again.AvailableItems || got.TotalItems != again.TotalItems {
			t.Errorf("items = %d/%d, want %d/%d", got.AvailableItems, got.TotalItems, again.AvailableItems, again.TotalItems)
		}
	})

	t.Run("RequestsForTitle lists linked requests newest first", func(t *testing.T) {
		r := newReader(t, fixture)
		got, err := r.RequestsForTitle(ctx, dune)
		if err != nil {
			t.Fatal(err)
		}
		requestID := func(x api.Request) domain.RequestID { return x.ID }
		assertIDs(t, mapIDs(got, requestID), mapIDs([]api.Request{newest, again, declined, oldest}, requestID))
	})

	t.Run("RequestsForTitle is empty for a Title without requests", func(t *testing.T) {
		r := newReader(t, fixture)
		got, err := r.RequestsForTitle(ctx, other)
		if err != nil {
			t.Fatal(err)
		}
		if got == nil || len(got) != 0 {
			t.Errorf("RequestsForTitle = %#v, want an empty slice", got)
		}
	})

	t.Run("Requesters lists distinct users with a non-declined request", func(t *testing.T) {
		r := newReader(t, fixture)
		got, err := r.Requesters(ctx, dune)
		if err != nil {
			t.Fatal(err)
		}
		slices.Sort(got)
		want := []domain.UserID{alice, carol}
		slices.Sort(want)
		assertIDs(t, got, want)

		none, err := r.Requesters(ctx, other)
		if err != nil {
			t.Fatal(err)
		}
		if len(none) != 0 {
			t.Errorf("Requesters without requests = %v", none)
		}
	})
}

func runIssues(t *testing.T, newReader Factory) {
	ctx := context.Background()
	dune, other := domain.NewID[domain.TitleID](), domain.NewID[domain.TitleID]()
	alice, bob, carol := domain.NewID[domain.UserID](), domain.NewID[domain.UserID](), domain.NewID[domain.UserID]()

	resolved := issue(alice, dune, api.IssueStatusResolved, 0)
	openDune := issue(alice, dune, api.IssueStatusOpen, 1)
	openDune.Comments = 2
	episode := domain.NewID[domain.EpisodeID]()
	openDune.Episode = &episode
	pendingOther := issue(bob, other, api.IssueStatusActionPending, 2)
	openDuneLater := issue(carol, dune, api.IssueStatusOpen, 3)

	fixture := Fixture{Issues: []api.Issue{openDune, resolved, openDuneLater, pendingOther}}

	t.Run("OpenIssues lists open and action-pending issues newest first", func(t *testing.T) {
		r := newReader(t, fixture)
		issueID := func(x api.Issue) domain.IssueID { return x.ID }
		got, err := r.OpenIssues(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		assertIDs(t, mapIDs(got, issueID), mapIDs([]api.Issue{openDuneLater, pendingOther, openDune}, issueID))

		got, err = r.OpenIssues(ctx, &dune)
		if err != nil {
			t.Fatal(err)
		}
		assertIDs(t, mapIDs(got, issueID), mapIDs([]api.Issue{openDuneLater, openDune}, issueID))
	})

	t.Run("Issue returns resolved issues and ErrNotFound for unknown IDs", func(t *testing.T) {
		r := newReader(t, fixture)
		got, err := r.Issue(ctx, resolved.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != api.IssueStatusResolved {
			t.Errorf("Status = %q", got.Status)
		}
		if _, err := r.Issue(ctx, domain.NewID[domain.IssueID]()); !errors.Is(err, api.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("Issue returns the episode and the comment count", func(t *testing.T) {
		r := newReader(t, fixture)
		got, err := r.Issue(ctx, openDune.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Comments != openDune.Comments || got.Episode == nil || *got.Episode != episode || got.User != alice || !got.CreatedAt.Equal(openDune.CreatedAt) {
			t.Errorf("Issue = %+v, want %+v", got, openDune)
		}
	})
}

func request(user domain.UserID, title *domain.TitleID, status api.Status, age int) api.Request {
	return api.Request{
		ID:        domain.NewID[domain.RequestID](),
		User:      user,
		Kind:      domain.MediaKindMovie,
		Tmdb:      duneTmdb,
		Title:     title,
		Scope:     api.ScopeMovie,
		Status:    status,
		CreatedAt: start.Add(time.Duration(age) * step),
	}
}

func issue(user domain.UserID, title domain.TitleID, status string, age int) api.Issue {
	return api.Issue{
		ID:        domain.NewID[domain.IssueID](),
		User:      user,
		Title:     title,
		Kind:      api.IssueKindVideo,
		Status:    status,
		CreatedAt: start.Add(time.Duration(age) * step),
	}
}

func assertIDs[ID comparable](t *testing.T, got, want []ID) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("ids = %v, want %v", got, want)
	}
}

func mapIDs[T any, ID any](items []T, id func(T) ID) []ID {
	ids := make([]ID, 0, len(items))
	for _, item := range items {
		ids = append(ids, id(item))
	}
	return ids
}
