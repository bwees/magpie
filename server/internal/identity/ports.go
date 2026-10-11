package identity

import (
	"context"
	"errors"
	"time"
)

// JellyfinAuth is implemented by internal/adapters/jellyfin. The Jellyfin access token returned at
// login is discarded; Magpie talks to Jellyfin with the API key in platform connection config.
type JellyfinAuth interface {
	// AuthenticateByName calls POST /Users/AuthenticateByName.
	// ErrInvalidCredentials on 401, ErrUnavailable on network/5xx.
	AuthenticateByName(ctx context.Context, username, password string) (JellyfinUser, error)
	// QuickConnectEnabled calls GET /QuickConnect/Enabled.
	QuickConnectEnabled(ctx context.Context) (bool, error)
	// QuickConnectInitiate calls POST /QuickConnect/Initiate. Code is 6 digits, valid 10 minutes.
	QuickConnectInitiate(ctx context.Context) (QuickConnect, error)
	// QuickConnectPoll calls GET /QuickConnect/Connect?secret=. ErrExpired once Jellyfin forgets the secret.
	QuickConnectPoll(ctx context.Context, secret string) (authenticated bool, err error)
	// AuthenticateWithQuickConnect calls POST /Users/AuthenticateWithQuickConnect.
	AuthenticateWithQuickConnect(ctx context.Context, secret string) (JellyfinUser, error)
}

type QuickConnect struct {
	Code, Secret string
	ExpiresAt    time.Time
}

type JellyfinUser struct {
	ID, Name                    string
	IsAdministrator, IsDisabled bool
}

var (
	ErrInvalidCredentials = errors.New("jellyfin: invalid credentials")
	ErrUnavailable        = errors.New("jellyfin: unavailable")
	ErrExpired            = errors.New("jellyfin: quick connect secret expired")
)
