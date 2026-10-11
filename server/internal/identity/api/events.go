package api

import "magpie/internal/domain"

const TypeUserCreated = "identity.UserCreated"

// UserCreated.Source values.
const (
	UserSourceLocal    = "LOCAL"
	UserSourceJellyfin = "JELLYFIN"
)

// UserCreated is durable. Emitted once when a user row is first created (setup, first Jellyfin sign-in).
type UserCreated struct {
	User   domain.UserID `json:"user"`
	Role   Role          `json:"role"`
	Source string        `json:"source"` // UserSourceLocal | UserSourceJellyfin
}

func (UserCreated) Type() string          { return TypeUserCreated }
func (e UserCreated) AggregateID() string { return string(e.User) }
func (UserCreated) Durable() bool         { return true }

// Messages lists every event and command this package declares, for the bus registry and the live schemas.
func Messages() []domain.Message {
	return []domain.Message{UserCreated{}}
}
