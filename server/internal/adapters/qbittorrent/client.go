package qbittorrent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"magpie/internal/acquisition"
	"magpie/internal/platform/settings"
)

const (
	pathAdd            = "/api/v2/torrents/add"
	pathStart          = "/api/v2/torrents/start"
	pathDelete         = "/api/v2/torrents/delete"
	pathFiles          = "/api/v2/torrents/files"
	pathSetShareLimits = "/api/v2/torrents/setShareLimits"
	pathMainData       = "/api/v2/sync/maindata"

	hashSeparator = "|"

	// maxBodyBytes bounds any answer read into memory; a full maindata of a large client stays well below it.
	maxBodyBytes = 32 << 20
)

// Connection reads qBittorrent's URL, API key, category and path mapping. It is called on every request,
// so settings changes apply at once.
type Connection func(ctx context.Context) (settings.QBittorrentConnection, error)

func FromSettings(store *settings.Store) Connection {
	return func(ctx context.Context) (settings.QBittorrentConnection, error) {
		return settings.QBittorrent.Get(ctx, store)
	}
}

// Client implements Acquisition's QBittorrent port over WebAPI 2.15+ with an API key.
type Client struct {
	connection Connection
	http       *http.Client

	// mu guards the merged sync/maindata state: every torrent's fields as last sent, by hash.
	mu     sync.Mutex
	rid    int64
	merged map[string]map[string]json.RawMessage
}

var _ acquisition.QBittorrent = (*Client)(nil)

func New(connection Connection) *Client {
	return &Client{connection: connection, http: &http.Client{}, merged: map[string]map[string]json.RawMessage{}}
}

// NewFromSettings is the constructor the app wires.
func NewFromSettings(store *settings.Store) *Client {
	return New(FromSettings(store))
}

var errNotConfigured = fmt.Errorf("%w: %w", acquisition.ErrUnavailable, errors.New("no qBittorrent connection is set up"))

// httpStatus is the cause of an ErrUnavailable from an unexpected HTTP answer.
type httpStatus int

func (s httpStatus) Error() string {
	return fmt.Sprintf("HTTP %d %s", int(s), http.StatusText(int(s)))
}

func (c *Client) conn(ctx context.Context) (settings.QBittorrentConnection, error) {
	conn, err := c.connection(ctx)
	if err != nil {
		return settings.QBittorrentConnection{}, err
	}
	if conn.URL == "" || conn.APIKey == "" {
		return settings.QBittorrentConnection{}, errNotConfigured
	}
	return conn, nil
}

// request is one WebAPI call. A nil body sends a GET.
type request struct {
	path        string
	query       url.Values
	body        io.Reader
	contentType string
}

func formRequest(path string, form url.Values) request {
	return request{path: path, body: strings.NewReader(form.Encode()), contentType: formContentType}
}

// answer is a response read into memory with its status.
type answer struct {
	status int
	body   []byte
}

func (c *Client) do(ctx context.Context, conn settings.QBittorrentConnection, r request) (answer, error) {
	target, err := url.Parse(conn.URL)
	if err != nil {
		return answer{}, fmt.Errorf("%w: invalid qBittorrent URL: %w", acquisition.ErrUnavailable, err)
	}
	target = target.JoinPath(r.path)
	target.RawQuery = r.query.Encode()
	method := http.MethodGet
	if r.body != nil {
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(ctx, method, target.String(), r.body)
	if err != nil {
		return answer{}, err
	}
	req.Header.Set(authorizationHeader, bearerPrefix+conn.APIKey)
	if r.contentType != "" {
		req.Header.Set(contentTypeHeader, r.contentType)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return answer{}, transportError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return answer{}, transportError(ctx, err)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return answer{}, fmt.Errorf("%s %s: %w", method, r.path, acquisition.ErrUnauthorized)
	}
	return answer{status: resp.StatusCode, body: body}, nil
}

// call sends r and fails unless qBittorrent answers 2xx. A 404 on a torrent route is a hash that's gone.
func (c *Client) call(ctx context.Context, r request) ([]byte, error) {
	conn, err := c.conn(ctx)
	if err != nil {
		return nil, err
	}
	a, err := c.do(ctx, conn, r)
	if err != nil {
		return nil, err
	}
	switch {
	case a.status == http.StatusNotFound:
		return nil, fmt.Errorf("%s: %w", r.path, acquisition.ErrTorrentNotFound)
	case a.status < http.StatusOK || a.status >= http.StatusMultipleChoices:
		return nil, fmt.Errorf("%s: %w: %w", r.path, acquisition.ErrUnavailable, httpStatus(a.status))
	}
	return a.body, nil
}

func decode(path string, body []byte, out any) error {
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%w: decoding %s: %w", acquisition.ErrUnavailable, path, err)
	}
	return nil
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

func joinHashes(hashes []string) string {
	return strings.Join(hashes, hashSeparator)
}
