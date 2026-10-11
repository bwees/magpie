package live

import (
	"context"
	"log/slog"
	"slices"

	"magpie/internal/domain"
	identityapi "magpie/internal/identity/api"
	requestsapi "magpie/internal/requests/api"
)

// RoleFilter passes nothing to disabled users, a personal message only to its recipient, and everything
// else to admins. A requester gets events naming
// them or one of their requests; an event that names no user or request reaches them only when it is
// about a title they requested. With ViewOthersRequests, every request's events and every requested
// title's events reach them too, but other users' notifications still don't. Lookup errors deny.
type RoleFilter struct {
	// Identity resolves the viewer's role and toggles. Without it the principal's own flags are used.
	Identity identityapi.Reader
	Requests requestsapi.Reader
}

type viewerAccess struct {
	admin              bool
	disabled           bool
	viewOthersRequests bool
}

func (f RoleFilter) Allows(ctx context.Context, viewer domain.Principal, s scope) bool {
	access, err := f.access(ctx, viewer)
	if err != nil {
		slog.WarnContext(ctx, "live role check failed", "user", viewer.User, "error", err)
		return false
	}
	switch {
	case access.disabled:
		return false
	case s.recipient != "":
		return s.recipient == viewer.User
	case access.admin:
		return true
	case len(s.requests) > 0 && access.viewOthersRequests:
		return true
	case len(s.users) > 0 || len(s.requests) > 0:
		return slices.Contains(s.users, viewer.User) || f.ownsAny(ctx, viewer.User, s.requests)
	default:
		return f.requestedAny(ctx, viewer.User, s.titles, access.viewOthersRequests)
	}
}

func (f RoleFilter) access(ctx context.Context, viewer domain.Principal) (viewerAccess, error) {
	if f.Identity == nil {
		return viewerAccess{admin: viewer.Admin, viewOthersRequests: viewer.Permissions.ViewOthersRequests}, nil
	}
	user, err := f.Identity.User(ctx, viewer.User)
	if err != nil {
		return viewerAccess{}, err
	}
	return viewerAccess{
		admin:              user.IsAdmin(),
		disabled:           user.Disabled,
		viewOthersRequests: user.Permissions.ViewOthersRequests,
	}, nil
}

func (f RoleFilter) ownsAny(ctx context.Context, user domain.UserID, requests []domain.RequestID) bool {
	if f.Requests == nil {
		return false
	}
	for _, id := range requests {
		request, err := f.Requests.Request(ctx, id)
		if err != nil {
			slog.WarnContext(ctx, "live request lookup failed", "request", id, "error", err)
			continue
		}
		if request.User == user {
			return true
		}
	}
	return false
}

// requestedAny reports whether user requested any of titles, or, with anyRequester, whether anyone did.
func (f RoleFilter) requestedAny(ctx context.Context, user domain.UserID, titles []domain.TitleID, anyRequester bool) bool {
	if f.Requests == nil {
		return false
	}
	for _, title := range titles {
		requesters, err := f.Requests.Requesters(ctx, title)
		if err != nil {
			slog.WarnContext(ctx, "live requesters lookup failed", "title", title, "error", err)
			continue
		}
		if slices.Contains(requesters, user) || (anyRequester && len(requesters) > 0) {
			return true
		}
	}
	return false
}
