// Package prowlarr implements Acquisition's Prowlarr port: the indexer list from Prowlarr's v1 API
// and each indexer's own Torznab endpoint (/{id}/api). It never uses the aggregate search.
package prowlarr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"magpie/internal/acquisition"
	"magpie/internal/platform/settings"
)

const (
	apiKeyHeader = "X-Api-Key"
	acceptHeader = "Accept"
	jsonContent  = "application/json"

	// maxBodyBytes bounds any answer read into memory: API lists, Torznab feeds and .torrent files.
	maxBodyBytes = 16 << 20
)

// Connection reads Prowlarr's URL and API key. It is called on every request so settings changes apply at once.
type Connection func(ctx context.Context) (settings.ProwlarrConnection, error)

// FromSettings reads the platform's Prowlarr connection config.
func FromSettings(store *settings.Store) Connection {
	return func(ctx context.Context) (settings.ProwlarrConnection, error) {
		return settings.Prowlarr.Get(ctx, store)
	}
}

type Client struct {
	connection Connection
	http       *http.Client
	now        func() time.Time
}

var _ acquisition.Prowlarr = (*Client)(nil)

func New(connection Connection) *Client {
	return &Client{connection: connection, http: &http.Client{CheckRedirect: stopAtMagnet}, now: time.Now}
}

// NewFromSettings is the constructor the app wires.
func NewFromSettings(store *settings.Store) *Client {
	return New(FromSettings(store))
}

var errNotConfigured = fmt.Errorf("%w: %w", acquisition.ErrUnavailable, errors.New("no Prowlarr connection is set up"))

// httpStatus is the cause of an ErrUnavailable from an unexpected HTTP answer.
type httpStatus int

func (s httpStatus) Error() string {
	return fmt.Sprintf("HTTP %d %s", int(s), http.StatusText(int(s)))
}

func (c *Client) WebURL(ctx context.Context) (string, error) {
	conn, err := c.connection(ctx)
	if err != nil {
		return "", err
	}
	return conn.URL, nil
}

// endpoint resolves a path under the configured base URL, which may itself carry a path prefix.
func (c *Client) endpoint(ctx context.Context, path string, query url.Values) (*url.URL, string, error) {
	conn, err := c.connection(ctx)
	if err != nil {
		return nil, "", err
	}
	if conn.URL == "" {
		return nil, "", errNotConfigured
	}
	base, err := url.Parse(conn.URL)
	if err != nil {
		return nil, "", fmt.Errorf("%w: invalid Prowlarr URL: %w", acquisition.ErrUnavailable, err)
	}
	target := base.JoinPath(path)
	target.RawQuery = query.Encode()
	return target, conn.APIKey, nil
}

// answer is a response read into memory. statusErr is nil for 2xx.
type answer struct {
	body      []byte
	statusErr error
}

// get sends an authenticated GET and reads the answer whatever its status: Torznab endpoints
// explain failures in an XML body. An auth failure is always an error.
func (c *Client) get(ctx context.Context, path string, query url.Values) (answer, error) {
	target, apiKey, err := c.endpoint(ctx, path, query)
	if err != nil {
		return answer{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return answer{}, err
	}
	req.Header.Set(apiKeyHeader, apiKey)
	req.Header.Set(acceptHeader, jsonContent)

	resp, err := c.http.Do(req)
	if err != nil {
		return answer{}, transportError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()

	statusErr := statusError(resp.StatusCode)
	if errors.Is(statusErr, acquisition.ErrUnauthorized) {
		return answer{}, fmt.Errorf("GET %s: %w", target.Path, statusErr)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return answer{}, transportError(ctx, err)
	}
	if statusErr != nil {
		statusErr = fmt.Errorf("GET %s: %w", target.Path, statusErr)
	}
	return answer{body: body, statusErr: statusErr}, nil
}

// getJSON decodes a 2xx JSON answer into out.
func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	a, err := c.get(ctx, path, nil)
	if err != nil {
		return err
	}
	if a.statusErr != nil {
		return a.statusErr
	}
	if err := json.Unmarshal(a.body, out); err != nil {
		return fmt.Errorf("%w: decoding %s: %w", acquisition.ErrUnavailable, path, err)
	}
	return nil
}

func statusError(status int) error {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return acquisition.ErrUnauthorized
	case status >= http.StatusOK && status < http.StatusMultipleChoices:
		return nil
	}
	return fmt.Errorf("%w: %w", acquisition.ErrUnavailable, httpStatus(status))
}

func transportError(ctx context.Context, err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("%w: %w", acquisition.ErrTimeout, err)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return fmt.Errorf("%w: %w", acquisition.ErrUnavailable, err)
}
