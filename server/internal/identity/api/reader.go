// Package api is Identity's public contract: the user Reader and the events Identity publishes.
// Spec: docs/spec/contexts/identity.md.
package api

import (
	"context"
	"errors"

	"magpie/internal/domain"
)

type Role string

const (
	RoleAdmin     Role = "ADMIN"
	RoleRequester Role = "REQUESTER"
)

// Permissions are the per-user toggles. They are the same fields the request principal carries.
type Permissions = domain.Permissions

type User struct {
	ID          domain.UserID
	Username    string
	DisplayName string
	Role        Role
	// Permissions reads all true for admins, whatever the stored toggles are.
	Permissions    Permissions
	JellyfinUserID *string
	LocalAdmin     bool
	Disabled       bool
}

func (u User) IsAdmin() bool { return u.Role == RoleAdmin }

var ErrNotFound = errors.New("identity: user not found")

// Reader is read-only and backed by Identity's repository.
type Reader interface {
	// User returns one user, including disabled users. ErrNotFound if the ID is unknown.
	User(ctx context.Context, id domain.UserID) (User, error)
	// Users returns users by ID; unknown IDs are omitted (no error). Order follows the input.
	Users(ctx context.Context, ids []domain.UserID) ([]User, error)
	// Admins returns all enabled admins ordered by username. Never empty after setup.
	Admins(ctx context.Context) ([]User, error)
	// All returns every user ordered by username, for admin screens.
	All(ctx context.Context) ([]User, error)
}
