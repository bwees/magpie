package live

import (
	"context"
	"log/slog"
	"slices"

	"magpie/internal/domain"
)

// IdentityReader is the part of identity.Reader the role filter needs. Ticket 03 backs it with the real api.
type IdentityReader interface {
	IsAdmin(ctx context.Context, user domain.UserID) (bool, error)
}

// RequestsReader is the part of requests.Reader the role filter needs. Ticket 03 backs it with the real api.
type RequestsReader interface {
	RequestOwner(ctx context.Context, request domain.RequestID) (domain.UserID, error)
	// Requesters returns the users with a non-declined request for the title.
	Requesters(ctx context.Context, title domain.TitleID) ([]domain.UserID, error)
}

// RoleFilter passes everything to admins. A requester gets events naming them or one of their requests;
// an event that names no user or request reaches them only when it is about a title they requested.
// Lookup errors deny.
type RoleFilter struct {
	Identity IdentityReader
	Requests RequestsReader
}

func (f RoleFilter) Allows(ctx context.Context, viewer domain.Principal, s scope) bool {
	admin, err := f.isAdmin(ctx, viewer)
	if err != nil {
		slog.WarnContext(ctx, "live role check failed", "user", viewer.User, "error", err)
		return false
	}
	if admin {
		return true
	}
	if len(s.users) > 0 || len(s.requests) > 0 {
		return slices.Contains(s.users, viewer.User) || f.ownsAny(ctx, viewer.User, s.requests)
	}
	return f.requestedAny(ctx, viewer.User, s.titles)
}

func (f RoleFilter) isAdmin(ctx context.Context, viewer domain.Principal) (bool, error) {
	if f.Identity == nil {
		return viewer.Admin, nil
	}
	return f.Identity.IsAdmin(ctx, viewer.User)
}

func (f RoleFilter) ownsAny(ctx context.Context, user domain.UserID, requests []domain.RequestID) bool {
	if f.Requests == nil {
		return false
	}
	for _, request := range requests {
		owner, err := f.Requests.RequestOwner(ctx, request)
		if err != nil {
			slog.WarnContext(ctx, "live request lookup failed", "request", request, "error", err)
			continue
		}
		if owner == user {
			return true
		}
	}
	return false
}

func (f RoleFilter) requestedAny(ctx context.Context, user domain.UserID, titles []domain.TitleID) bool {
	if f.Requests == nil {
		return false
	}
	for _, title := range titles {
		requesters, err := f.Requests.Requesters(ctx, title)
		if err != nil {
			slog.WarnContext(ctx, "live requesters lookup failed", "title", title, "error", err)
			continue
		}
		if slices.Contains(requesters, user) {
			return true
		}
	}
	return false
}
