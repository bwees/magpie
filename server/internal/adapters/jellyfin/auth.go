package jellyfin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"magpie/internal/identity"
	"magpie/internal/platform/settings"
)

const (
	pathAuthenticateByName       = "/Users/AuthenticateByName"
	pathQuickConnectEnabled      = "/QuickConnect/Enabled"
	pathQuickConnectInitiate     = "/QuickConnect/Initiate"
	pathQuickConnectConnect      = "/QuickConnect/Connect"
	pathAuthenticateQuickConnect = "/Users/AuthenticateWithQuickConnect"
	querySecret                  = "secret"

	contentTypeHeader = "Content-Type"
	jsonContent       = "application/json"

	// Jellyfin 12 reads the client only from this header; X-Emby-Authorization is disabled.
	clientAuthorization = `MediaBrowser Client="Magpie", Device="Magpie server", DeviceId="magpie-server", Version="%s"`
	// ClientVersion is reported to Jellyfin as the client version.
	ClientVersion = "1.0.0"

	// quickConnectLifetime is how long Jellyfin keeps an unapproved code.
	quickConnectLifetime = 10 * time.Minute
	// maxBodyBytes bounds any Jellyfin answer read into memory.
	maxBodyBytes = 1 << 20
)

var (
	errNotConfigured = fmt.Errorf("%w: %w", identity.ErrUnavailable, errors.New("no Jellyfin connection is set up"))
	// errForbidden is Jellyfin refusing a known account, which it does for disabled users.
	errForbidden = errors.New("jellyfin: forbidden")
	// Jellyfin answers 404 on these once it no longer knows a Quick Connect secret.
	quickConnectSecretPaths = map[string]bool{pathQuickConnectConnect: true, pathAuthenticateQuickConnect: true}
)

type authenticationResult struct {
	User struct {
		ID     string `json:"Id"`
		Name   string `json:"Name"`
		Policy struct {
			IsAdministrator bool `json:"IsAdministrator"`
			IsDisabled      bool `json:"IsDisabled"`
		} `json:"Policy"`
	} `json:"User"`
}

type quickConnectResult struct {
	Authenticated bool   `json:"Authenticated"`
	Secret        string `json:"Secret"`
	Code          string `json:"Code"`
}

// Auth signs users in against Jellyfin for Identity. The access token Jellyfin returns is discarded.
type Auth struct {
	store  *settings.Store
	client *http.Client
	now    func() time.Time
}

var _ identity.JellyfinAuth = (*Auth)(nil)

func NewAuth(store *settings.Store) *Auth {
	return &Auth{store: store, client: &http.Client{}, now: time.Now}
}

func (a *Auth) AuthenticateByName(ctx context.Context, username, password string) (identity.JellyfinUser, error) {
	body := map[string]string{"Username": username, "Pw": password}
	var result authenticationResult
	if err := a.call(ctx, http.MethodPost, pathAuthenticateByName, nil, body, &result); err != nil {
		if errors.Is(err, errForbidden) {
			return identity.JellyfinUser{Name: username, IsDisabled: true}, nil
		}
		return identity.JellyfinUser{}, err
	}
	return result.user(), nil
}

func (a *Auth) QuickConnectEnabled(ctx context.Context) (bool, error) {
	var enabled bool
	err := a.call(ctx, http.MethodGet, pathQuickConnectEnabled, nil, nil, &enabled)
	return enabled, err
}

func (a *Auth) QuickConnectInitiate(ctx context.Context) (identity.QuickConnect, error) {
	var result quickConnectResult
	if err := a.call(ctx, http.MethodPost, pathQuickConnectInitiate, nil, nil, &result); err != nil {
		return identity.QuickConnect{}, err
	}
	// Timed from Magpie's clock, so a skewed Jellyfin clock can't expire the code early.
	return identity.QuickConnect{Code: result.Code, Secret: result.Secret, ExpiresAt: a.now().Add(quickConnectLifetime)}, nil
}

func (a *Auth) QuickConnectPoll(ctx context.Context, secret string) (bool, error) {
	var result quickConnectResult
	err := a.call(ctx, http.MethodGet, pathQuickConnectConnect, url.Values{querySecret: {secret}}, nil, &result)
	return result.Authenticated, err
}

func (a *Auth) AuthenticateWithQuickConnect(ctx context.Context, secret string) (identity.JellyfinUser, error) {
	var result authenticationResult
	body := map[string]string{"Secret": secret}
	if err := a.call(ctx, http.MethodPost, pathAuthenticateQuickConnect, nil, body, &result); err != nil {
		return identity.JellyfinUser{}, err
	}
	return result.user(), nil
}

func (r authenticationResult) user() identity.JellyfinUser {
	return identity.JellyfinUser{
		ID:              r.User.ID,
		Name:            r.User.Name,
		IsAdministrator: r.User.Policy.IsAdministrator,
		IsDisabled:      r.User.Policy.IsDisabled,
	}
}

// call sends one request and decodes the JSON answer into out. It maps 401 to ErrInvalidCredentials,
// a forgotten Quick Connect secret to ErrExpired, and network errors and other statuses to ErrUnavailable.
func (a *Auth) call(ctx context.Context, method, path string, query url.Values, body, out any) error {
	conn, err := settings.Jellyfin.Get(ctx, a.store)
	if err != nil {
		return err
	}
	if conn.URL == "" {
		return errNotConfigured
	}
	target, err := url.Parse(conn.URL)
	if err != nil {
		return fmt.Errorf("%w: invalid Jellyfin URL: %w", identity.ErrUnavailable, err)
	}
	target = target.JoinPath(path)
	target.RawQuery = query.Encode()

	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, target.String(), payload)
	if err != nil {
		return err
	}
	req.Header.Set(authorizationHeader, fmt.Sprintf(clientAuthorization, ClientVersion))
	if body != nil {
		req.Header.Set(contentTypeHeader, jsonContent)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", identity.ErrUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return fmt.Errorf("%s %s: %w", method, path, identity.ErrInvalidCredentials)
	case resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("%s %s: %w", method, path, errForbidden)
	case resp.StatusCode == http.StatusNotFound && quickConnectSecretPaths[path]:
		return fmt.Errorf("%s %s: %w", method, path, identity.ErrExpired)
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("%w: %s %s answered %s", identity.ErrUnavailable, method, path, resp.Status)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodyBytes)).Decode(out); err != nil {
		return fmt.Errorf("%w: unreadable answer from %s: %w", identity.ErrUnavailable, path, err)
	}
	return nil
}
