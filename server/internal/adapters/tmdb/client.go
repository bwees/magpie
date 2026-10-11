// Package tmdb implements Catalog's TMDB port against the TMDB v3 API.
package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"magpie/internal/catalog"
	"magpie/internal/platform/settings"
)

const (
	DefaultBaseURL      = "https://api.themoviedb.org/3"
	DefaultImageBaseURL = "https://image.tmdb.org/t/p"

	requestsPerSecond = 40
	maxRetries        = 5
	initialBackoff    = time.Second
	maxBackoff        = 30 * time.Second
	requestTimeout    = 30 * time.Second

	paramAPIKey       = "api_key"
	headerAuth        = "Authorization"
	headerAccept      = "Accept"
	headerRetryAfter  = "Retry-After"
	bearerPrefix      = "Bearer "
	acceptJSON        = "application/json"
	pathSeparator     = "/"
	jwtPartSeparators = 2
)

// Config wires the client. Zero fields take production defaults.
type Config struct {
	// APIKey returns the current key: a v3 API key or a v4 read access token.
	APIKey       func(ctx context.Context) (string, error)
	BaseURL      string
	ImageBaseURL string
	HTTP         *http.Client
	Sleep        func(ctx context.Context, d time.Duration) error
	Now          func() time.Time
}

// Client is safe for concurrent use. Every request shares one 40 req/s token bucket.
type Client struct {
	apiKey    func(ctx context.Context) (string, error)
	baseURL   string
	imageBase string
	http      *http.Client
	sleep     func(ctx context.Context, d time.Duration) error
	now       func() time.Time
	bucket    *tokenBucket
}

var _ catalog.TMDB = (*Client)(nil)

// New reads the key from the platform's TMDB connection setting on every call, so a new key applies at once.
func New(store *settings.Store) *Client {
	return NewClient(Config{
		APIKey: func(ctx context.Context) (string, error) {
			conn, err := settings.TMDB.Get(ctx, store)
			return conn.APIKey, err
		},
	})
}

func NewClient(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.ImageBaseURL == "" {
		cfg.ImageBaseURL = DefaultImageBaseURL
	}
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: requestTimeout}
	}
	if cfg.Sleep == nil {
		cfg.Sleep = sleepContext
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.APIKey == nil {
		cfg.APIKey = func(context.Context) (string, error) { return "", nil }
	}
	return &Client{
		apiKey:    cfg.APIKey,
		baseURL:   strings.TrimSuffix(cfg.BaseURL, pathSeparator),
		imageBase: strings.TrimSuffix(cfg.ImageBaseURL, pathSeparator),
		http:      cfg.HTTP,
		sleep:     cfg.Sleep,
		now:       cfg.Now,
		bucket:    newTokenBucket(requestsPerSecond, cfg.Now, cfg.Sleep),
	}
}

func (c *Client) ImageURL(path string, size string) string {
	return c.imageBase + pathSeparator + size + path
}

// getJSON calls an API path and decodes the answer into out.
func (c *Client) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	key, err := c.apiKey(ctx)
	if err != nil {
		return err
	}
	if key == "" {
		return fmt.Errorf("%w: no TMDB key configured", catalog.ErrTmdbUnauthorized)
	}
	if query == nil {
		query = url.Values{}
	}
	header := http.Header{}
	header.Set(headerAccept, acceptJSON)
	if isReadAccessToken(key) {
		header.Set(headerAuth, bearerPrefix+key)
	} else {
		query.Set(paramAPIKey, key)
	}

	body, err := c.send(ctx, c.baseURL+path+"?"+query.Encode(), header)
	if err != nil {
		return fmt.Errorf("tmdb %s: %w", path, err)
	}
	defer func() { _ = body.Close() }()
	if err := json.NewDecoder(body).Decode(out); err != nil {
		return fmt.Errorf("tmdb %s: decoding: %w", path, err)
	}
	return nil
}

// send makes the request with rate limiting and retries, returning the body of a 2xx answer.
func (c *Client) send(ctx context.Context, rawURL string, header http.Header) (io.ReadCloser, error) {
	backoff := initialBackoff
	for retry := 0; ; retry++ {
		if err := c.bucket.Wait(ctx); err != nil {
			return nil, err
		}
		resp, err := c.do(ctx, rawURL, header)
		if err == nil && resp.StatusCode < http.StatusMultipleChoices {
			return resp.Body, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		var answered http.Header
		if err == nil {
			_ = resp.Body.Close()
			if !retryable(resp.StatusCode) {
				return nil, statusError(resp.StatusCode)
			}
			answered = resp.Header
		}
		if retry == maxRetries {
			return nil, fmt.Errorf("%w: gave up after %d retries", catalog.ErrTmdbUnavailable, maxRetries)
		}
		wait, asked := retryAfter(answered)
		if !asked {
			wait, backoff = backoff, nextBackoff(backoff)
		}
		if err := c.sleep(ctx, wait); err != nil {
			return nil, err
		}
	}
}

func (c *Client) do(ctx context.Context, rawURL string, header http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header = header.Clone()
	return c.http.Do(req)
}

func nextBackoff(d time.Duration) time.Duration {
	return min(d*2, maxBackoff)
}

// retryAfter reads Retry-After in seconds; asked is false when TMDB didn't send one.
func retryAfter(header http.Header) (wait time.Duration, asked bool) {
	seconds, err := strconv.Atoi(header.Get(headerRetryAfter))
	if err != nil || seconds < 0 {
		return 0, false
	}
	return time.Duration(seconds) * time.Second, true
}

func retryable(status int) bool {
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

func statusError(status int) error {
	switch status {
	case http.StatusUnauthorized:
		return catalog.ErrTmdbUnauthorized
	case http.StatusNotFound:
		return catalog.ErrTmdbNotFound
	default:
		return fmt.Errorf("tmdb: unexpected status %d", status)
	}
}

// isReadAccessToken tells a v4 read access token (a JWT) from a v3 API key.
func isReadAccessToken(key string) bool {
	return strings.Count(key, ".") == jwtPartSeparators
}

var errNoImagePath = errors.New("tmdb: empty image path")
