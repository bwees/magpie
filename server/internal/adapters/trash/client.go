// Package trash implements Acquisition's TrashGuides port. It reads TRaSH Guides' public JSON
// (github.com/TRaSH-Guides/Guides, docs/json, MIT): GitHub's contents API lists a directory and
// raw.githubusercontent.com serves each file, so a sync costs six API calls.
package trash

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"magpie/internal/acquisition"
)

const (
	DefaultContentsURL = "https://api.github.com/repos/TRaSH-Guides/Guides/contents/docs/json"
	DefaultRawURL      = "https://raw.githubusercontent.com/TRaSH-Guides/Guides/master/docs/json"

	guideBranch = "master"
	refParam    = "ref"

	dirCustomFormats   = "cf"
	dirQualityProfiles = "quality-profiles"
	dirQualitySizes    = "quality-size"
	entryTypeFile      = "file"
	jsonExtension      = ".json"

	requestTimeout = 30 * time.Second
	// parallelFetches bounds the raw file downloads in flight for one directory.
	parallelFetches = 8
	maxBodyBytes    = 4 << 20
)

// sizeTables is the quality-size file each app's guide recommends by default.
var sizeTables = map[acquisition.TrashApp]string{
	acquisition.TrashRadarr: "movie.json",
	acquisition.TrashSonarr: "series.json",
}

type Client struct {
	contentsURL string
	rawURL      string
	http        *http.Client
}

var _ acquisition.TrashGuides = (*Client)(nil)

// New reads the live guide on GitHub.
func New() *Client {
	return NewClient(DefaultContentsURL, DefaultRawURL, &http.Client{Timeout: requestTimeout})
}

func NewClient(contentsURL, rawURL string, httpClient *http.Client) *Client {
	return &Client{contentsURL: contentsURL, rawURL: rawURL, http: httpClient}
}

func (c *Client) CustomFormats(ctx context.Context, app acquisition.TrashApp) ([]acquisition.TrashCustomFormat, error) {
	files, err := fetchDir[customFormatJSON](ctx, c, app, dirCustomFormats)
	if err != nil {
		return nil, err
	}
	formats := make([]acquisition.TrashCustomFormat, 0, len(files))
	for _, f := range files {
		if format, ok := f.toPort(app); ok {
			formats = append(formats, format)
		}
	}
	return formats, nil
}

func (c *Client) QualityProfiles(ctx context.Context, app acquisition.TrashApp) ([]acquisition.TrashQualityProfile, error) {
	files, err := fetchDir[qualityProfileJSON](ctx, c, app, dirQualityProfiles)
	if err != nil {
		return nil, err
	}
	profiles := make([]acquisition.TrashQualityProfile, 0, len(files))
	for _, p := range files {
		profiles = append(profiles, p.toPort())
	}
	return profiles, nil
}

func (c *Client) QualitySizes(ctx context.Context, app acquisition.TrashApp) ([]acquisition.TrashQualitySize, error) {
	var table qualitySizeJSON
	if err := c.getJSON(ctx, c.rawFile(app, dirQualitySizes, sizeTables[app]), &table); err != nil {
		return nil, err
	}
	return table.toPort(), nil
}

type dirEntry struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// fetchDir lists a guide directory and decodes every JSON file in it.
func fetchDir[T any](ctx context.Context, c *Client, app acquisition.TrashApp, dir string) ([]T, error) {
	listing := c.contentsURL + "/" + path.Join(appDir(app), dir) + "?" + url.Values{refParam: {guideBranch}}.Encode()
	var entries []dirEntry
	if err := c.getJSON(ctx, listing, &entries); err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.Type == entryTypeFile && strings.HasSuffix(e.Name, jsonExtension) {
			names = append(names, e.Name)
		}
	}

	files := make([]T, len(names))
	errs := make([]error, len(names))
	slots := make(chan struct{}, parallelFetches)
	var wg sync.WaitGroup
	for i, name := range names {
		wg.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()
			errs[i] = c.getJSON(ctx, c.rawFile(app, dir, name), &files[i])
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return files, nil
}

func appDir(app acquisition.TrashApp) string {
	return strings.ToLower(string(app))
}

func (c *Client) rawFile(app acquisition.TrashApp, dir, name string) string {
	return c.rawURL + "/" + path.Join(appDir(app), dir, name)
}

func (c *Client) getJSON(ctx context.Context, target string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%w: %w", acquisition.ErrUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%w: GET %s: HTTP %d", acquisition.ErrUnavailable, target, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return fmt.Errorf("%w: %w", acquisition.ErrUnavailable, err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%w: decoding %s: %w", acquisition.ErrUnavailable, target, err)
	}
	return nil
}
