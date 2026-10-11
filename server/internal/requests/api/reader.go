// Package api is Requests' public contract: the request and issue Reader and the events Requests publishes.
// Spec: docs/spec/contexts/requests.md.
package api

import (
	"context"
	"errors"
	"time"

	"magpie/internal/domain"
)

var ErrNotFound = errors.New("requests: not found")

type Status string

const (
	StatusPending            Status = "PENDING"
	StatusApproved           Status = "APPROVED"
	StatusProcessing         Status = "PROCESSING"
	StatusPartiallyAvailable Status = "PARTIALLY_AVAILABLE"
	StatusAvailable          Status = "AVAILABLE"
	StatusDeclined           Status = "DECLINED"
	StatusFailed             Status = "FAILED"
)

// Request.Scope values.
const (
	ScopeMovie      = "MOVIE"
	ScopeAllSeasons = "ALL_SEASONS"
	ScopeSeasons    = "SEASONS"
)

// Issue.Kind values.
const (
	IssueKindVideo    = "VIDEO"
	IssueKindAudio    = "AUDIO"
	IssueKindSubtitle = "SUBTITLE"
	IssueKindOther    = "OTHER"
)

// Issue.Status values.
const (
	IssueStatusOpen          = "OPEN"
	IssueStatusActionPending = "ACTION_PENDING"
	IssueStatusResolved      = "RESOLVED"
)

type Request struct {
	ID    domain.RequestID
	User  domain.UserID
	Kind  domain.MediaKind
	Tmdb  domain.TmdbID
	Title *domain.TitleID // set once Catalog has the Title
	Scope string          // ScopeMovie | ScopeAllSeasons | ScopeSeasons
	// Seasons is set for ScopeSeasons.
	Seasons                    []int
	Profile                    domain.ProfileID
	Status                     Status
	CreatedAt                  time.Time
	AvailableItems, TotalItems int
}

type Issue struct {
	ID        domain.IssueID
	User      domain.UserID
	Title     domain.TitleID
	Episode   *domain.EpisodeID
	Kind      string // IssueKind*
	Text      string
	Status    string // IssueStatus*
	Comments  int
	CreatedAt time.Time
}

// Reader is read-only and backed by Requests' repository.
type Reader interface {
	// Request returns one request. ErrNotFound if unknown.
	Request(ctx context.Context, id domain.RequestID) (Request, error)
	// RequestsForTitle returns all requests linked to a Title, newest first. Empty slice if none.
	RequestsForTitle(ctx context.Context, title domain.TitleID) ([]Request, error)
	// Requesters returns the distinct users with a non-declined request for the Title, in no particular
	// order. Empty slice if none. Activity uses it to route "available" notifications.
	Requesters(ctx context.Context, title domain.TitleID) ([]domain.UserID, error)
	// OpenIssues returns OPEN and ACTION_PENDING issues, newest first. A nil title means all Titles.
	OpenIssues(ctx context.Context, title *domain.TitleID) ([]Issue, error)
	// Issue returns one issue. ErrNotFound if unknown.
	Issue(ctx context.Context, id domain.IssueID) (Issue, error)
}
