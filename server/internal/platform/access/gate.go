// Package access decides who may manage a title's media files: view and delete them, search for
// replacements and handle their downloads and subtitles.
package access

import (
	"context"
	"net/http"
	"slices"

	"magpie/internal/domain"
	"magpie/internal/platform/problem"
	requestsapi "magpie/internal/requests/api"
)

// FileGate lets admins manage every title's files, and users with ManageFiles the titles whose requests
// they may see: their own, or every requested title with ViewOthersRequests.
type FileGate struct {
	requests requestsapi.Reader
}

func NewFileGate(requests requestsapi.Reader) *FileGate {
	return &FileGate{requests: requests}
}

// RequireManager returns a problem unless the request's principal is an admin or has ManageFiles. It refuses
// everyone else before a handler looks up what the title is.
func RequireManager(ctx context.Context) error {
	_, err := manager(ctx)
	return err
}

func manager(ctx context.Context) (domain.Principal, error) {
	principal, ok := domain.PrincipalFrom(ctx)
	if !ok {
		return domain.Principal{}, problem.New(problem.TypeUnauthorized, http.StatusUnauthorized, "sign in first")
	}
	if !principal.Admin && !principal.Permissions.ManageFiles {
		return domain.Principal{}, forbidden
	}
	return principal, nil
}

// Require returns a problem unless the request's principal may manage title's files.
func (g *FileGate) Require(ctx context.Context, title domain.TitleID) error {
	principal, err := manager(ctx)
	if err != nil {
		return err
	}
	allowed, err := g.Allows(ctx, principal, title)
	if err != nil {
		return err
	}
	if !allowed {
		return forbidden
	}
	return nil
}

var forbidden = problem.New(problem.TypeForbidden, http.StatusForbidden, "you can't manage this title's files")

func (g *FileGate) Allows(ctx context.Context, principal domain.Principal, title domain.TitleID) (bool, error) {
	if principal.Admin {
		return true, nil
	}
	if !principal.Permissions.ManageFiles {
		return false, nil
	}
	requesters, err := g.requests.Requesters(ctx, title)
	if err != nil {
		return false, err
	}
	return slices.Contains(requesters, principal.User) || (principal.Permissions.ViewOthersRequests && len(requesters) > 0), nil
}

// Filter keeps the items whose title the request's principal may manage.
func Filter[T any](ctx context.Context, g *FileGate, items []T, titleOf func(T) domain.TitleID) ([]T, error) {
	principal, err := manager(ctx)
	if err != nil {
		return nil, err
	}
	if principal.Admin {
		return items, nil
	}
	allowed := map[domain.TitleID]bool{}
	kept := make([]T, 0, len(items))
	for _, item := range items {
		title := titleOf(item)
		ok, seen := allowed[title]
		if !seen {
			if ok, err = g.Allows(ctx, principal, title); err != nil {
				return nil, err
			}
			allowed[title] = ok
		}
		if ok {
			kept = append(kept, item)
		}
	}
	return kept, nil
}
