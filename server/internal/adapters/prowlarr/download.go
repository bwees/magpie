package prowlarr

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"magpie/internal/acquisition"
)

const (
	magnetScheme   = "magnet"
	locationHeader = "Location"
	maxRedirects   = 10
)

var errTooManyRedirects = errors.New("too many redirects")

// stopAtMagnet keeps the client from following a redirect it can't fetch: the 30x answer itself
// carries the magnet in its Location header.
func stopAtMagnet(req *http.Request, via []*http.Request) error {
	if req.URL.Scheme == magnetScheme {
		return http.ErrUseLastResponse
	}
	if len(via) >= maxRedirects {
		return errTooManyRedirects
	}
	return nil
}

// Download fetches a release link (Prowlarr's /{id}/download?…, which carries its own apikey).
// A redirect to a magnet comes back as MagnetRedirect.
func (c *Client) Download(ctx context.Context, link string) (acquisition.TorrentPayload, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return acquisition.TorrentPayload{}, fmt.Errorf("%w: invalid download link: %w", acquisition.ErrUnavailable, err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return acquisition.TorrentPayload{}, transportError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if location := resp.Header.Get(locationHeader); isRedirect(resp.StatusCode) && strings.HasPrefix(location, magnetPrefix) {
		return acquisition.TorrentPayload{MagnetRedirect: location}, nil
	}
	if err := statusError(resp.StatusCode); err != nil {
		return acquisition.TorrentPayload{}, fmt.Errorf("download: %w", err)
	}
	torrent, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return acquisition.TorrentPayload{}, transportError(ctx, err)
	}
	return acquisition.TorrentPayload{Torrent: torrent}, nil
}

func isRedirect(status int) bool {
	return status >= http.StatusMultipleChoices && status < http.StatusBadRequest
}
