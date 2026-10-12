// Package opensubtitles implements Subtitles' OpenSubtitles port and the connection's health probe against
// the OpenSubtitles.com REST API.
package opensubtitles

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"magpie/internal/domain"
	"magpie/internal/platform/settings"
	"magpie/internal/subtitles"
)

const (
	pathSubtitles = "/api/v1/subtitles"
	pathDownload  = "/api/v1/download"

	// requestGap spaces every call to keep under OpenSubtitles' 5 requests per second.
	requestGap     = 200 * time.Millisecond
	requestTimeout = 30 * time.Second
	// tokenLifetime is how long OpenSubtitles keeps a login token valid.
	tokenLifetime = 24 * time.Hour
	// defaultRetryAfter is the wait after a 429 without a usable Retry-After header.
	defaultRetryAfter = time.Second

	retryAfterHeader = "Retry-After"
	httpsScheme      = "https://"
	imdbPrefix       = "tt"
	languageSep      = ","
	regionSep        = "-"
	featureEpisode   = "Episode"

	paramLanguages     = "languages"
	paramMovieHash     = "moviehash"
	paramTmdbID        = "tmdb_id"
	paramParentTmdbID  = "parent_tmdb_id"
	paramImdbID        = "imdb_id"
	paramParentImdbID  = "parent_imdb_id"
	paramSeasonNumber  = "season_number"
	paramEpisodeNumber = "episode_number"

	noLogin = "no OpenSubtitles login is set under Settings → Connections"
)

// regionalLanguages are the languages OpenSubtitles only knows by region.
var regionalLanguages = map[domain.Language][]string{
	"pt": {"pt-pt", "pt-br"},
	"zh": {"zh-cn", "zh-tw"},
}

type searchResponse struct {
	Data []struct {
		Attributes subtitleAttributes `json:"attributes"`
	} `json:"data"`
}

type subtitleAttributes struct {
	Language          string         `json:"language"`
	HearingImpaired   bool           `json:"hearing_impaired"`
	ForeignPartsOnly  bool           `json:"foreign_parts_only"`
	MachineTranslated bool           `json:"machine_translated"`
	AITranslated      bool           `json:"ai_translated"`
	FPS               float64        `json:"fps"`
	Release           string         `json:"release"`
	MovieHashMatch    bool           `json:"moviehash_match"`
	FeatureDetails    featureDetails `json:"feature_details"`
	Files             []struct {
		FileID int64 `json:"file_id"`
	} `json:"files"`
}

type featureDetails struct {
	FeatureType   string `json:"feature_type"`
	Year          int    `json:"year"`
	ImdbID        int64  `json:"imdb_id"`
	TmdbID        int64  `json:"tmdb_id"`
	SeasonNumber  int    `json:"season_number"`
	EpisodeNumber int    `json:"episode_number"`
	ParentImdbID  int64  `json:"parent_imdb_id"`
	ParentTmdbID  int64  `json:"parent_tmdb_id"`
}

type downloadRequest struct {
	FileID int64 `json:"file_id"`
}

type downloadResponse struct {
	Link         string    `json:"link"`
	Remaining    int       `json:"remaining"`
	ResetTimeUTC time.Time `json:"reset_time_utc"`
}

// Config wires the client. Zero fields take production defaults.
type Config struct {
	// Login returns the configured OpenSubtitles login.
	Login   func(ctx context.Context) (settings.OpenSubtitlesConnection, error)
	BaseURL string
	AppKey  string
	HTTP    *http.Client
	Now     func() time.Time
	Sleep   func(ctx context.Context, d time.Duration) error
}

// Client keeps the login token in memory. Every request, logins included, waits its turn on one spacer.
type Client struct {
	login   func(ctx context.Context) (settings.OpenSubtitlesConnection, error)
	baseURL string
	appKey  string
	http    *http.Client
	now     func() time.Time
	sleep   func(ctx context.Context, d time.Duration) error

	mu      sync.Mutex
	token   string
	apiBase string
	next    time.Time
}

var _ subtitles.OpenSubtitles = (*Client)(nil)

// New reads the login from the platform's OpenSubtitles connection on every Login, so a new password
// applies at the next login.
func New(store *settings.Store) *Client {
	return NewClient(Config{
		Login: func(ctx context.Context) (settings.OpenSubtitlesConnection, error) {
			return settings.OpenSubtitles.Get(ctx, store)
		},
	})
}

func NewClient(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = BaseURL
	}
	if cfg.AppKey == "" {
		cfg.AppKey = AppKey
	}
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: requestTimeout}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Sleep == nil {
		cfg.Sleep = sleepContext
	}
	return &Client{
		login: cfg.Login, baseURL: strings.TrimSuffix(cfg.BaseURL, "/"), apiBase: strings.TrimSuffix(cfg.BaseURL, "/"),
		appKey: cfg.AppKey, http: cfg.HTTP, now: cfg.Now, sleep: cfg.Sleep,
	}
}

func (c *Client) Login(ctx context.Context) (subtitles.Token, error) {
	conn, err := c.login(ctx)
	if err != nil {
		return subtitles.Token{}, err
	}
	if conn.Username == "" || conn.Password == "" {
		return subtitles.Token{}, fmt.Errorf("%w: %s", subtitles.ErrAuth, noLogin)
	}
	if c.appKey == "" {
		return subtitles.Token{}, fmt.Errorf("%w: %s", subtitles.ErrAuth, missingAppKey)
	}
	body, err := json.Marshal(loginRequest{Username: conn.Username, Password: conn.Password})
	if err != nil {
		return subtitles.Token{}, err
	}
	var resp loginResponse
	if err := c.call(ctx, http.MethodPost, c.baseURL+pathLogin, "", body, &resp); err != nil {
		return subtitles.Token{}, err
	}
	c.mu.Lock()
	c.token = resp.Token
	if resp.BaseURL != "" && c.baseURL == defaultBaseURL {
		c.apiBase = httpsScheme + resp.BaseURL
	}
	c.mu.Unlock()
	return subtitles.Token{
		Value: resp.Token, ExpiresAt: c.now().Add(tokenLifetime), VIP: resp.User.VIP,
		AllowedDownloads: resp.User.AllowedDownloads,
	}, nil
}

func (c *Client) Search(ctx context.Context, q subtitles.SearchQuery) ([]subtitles.Candidate, error) {
	token, base := c.session()
	var resp searchResponse
	if err := c.call(ctx, http.MethodGet, base+pathSubtitles+"?"+searchParams(q).Encode(), token, nil, &resp); err != nil {
		return nil, err
	}
	var out []subtitles.Candidate
	for _, item := range resp.Data {
		a := item.Attributes
		for _, f := range a.Files {
			out = append(out, candidate(a, f.FileID))
		}
	}
	return out, nil
}

func (c *Client) Download(ctx context.Context, fileID int64) ([]byte, int, time.Time, error) {
	token, base := c.session()
	if token == "" {
		return nil, 0, time.Time{}, fmt.Errorf("%w: not logged in", subtitles.ErrAuth)
	}
	body, err := json.Marshal(downloadRequest{FileID: fileID})
	if err != nil {
		return nil, 0, time.Time{}, err
	}
	var resp downloadResponse
	err = c.call(ctx, http.MethodPost, base+pathDownload, token, body, &resp)
	if errors.Is(err, subtitles.ErrAuth) {
		c.mu.Lock()
		c.token = ""
		c.mu.Unlock()
	}
	var quota *quotaError
	if errors.As(err, &quota) {
		return nil, 0, quota.ResetTimeUTC, subtitles.ErrQuotaExceeded
	}
	if err != nil {
		return nil, 0, time.Time{}, err
	}
	content, err := c.fetch(ctx, resp.Link)
	if err != nil {
		return nil, 0, time.Time{}, err
	}
	return content, max(resp.Remaining, 0), resp.ResetTimeUTC, nil
}

func (c *Client) session() (token, base string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.token, c.apiBase
}

// searchParams are OpenSubtitles' filters. url.Values encodes them sorted by name, as OpenSubtitles asks.
func searchParams(q subtitles.SearchQuery) url.Values {
	v := url.Values{}
	languages := []string{string(q.Language)}
	if regional, ok := regionalLanguages[q.Language]; ok {
		languages = regional
	}
	v.Set(paramLanguages, strings.Join(languages, languageSep))
	if q.MovieHash != "" {
		v.Set(paramMovieHash, q.MovieHash)
	}
	tmdbParam, imdbParam := paramTmdbID, paramImdbID
	if q.Season > 0 {
		tmdbParam, imdbParam = paramParentTmdbID, paramParentImdbID
		v.Set(paramSeasonNumber, strconv.Itoa(q.Season))
		v.Set(paramEpisodeNumber, strconv.Itoa(q.Episode))
	}
	switch {
	case q.Tmdb > 0:
		v.Set(tmdbParam, strconv.FormatInt(int64(q.Tmdb), 10))
	case q.Imdb != "":
		v.Set(imdbParam, strings.TrimLeft(strings.TrimPrefix(strings.ToLower(q.Imdb), imdbPrefix), "0"))
	}
	return v
}

func candidate(a subtitleAttributes, fileID int64) subtitles.Candidate {
	d := a.FeatureDetails
	tmdb, imdb := d.TmdbID, d.ImdbID
	if d.FeatureType == featureEpisode {
		tmdb, imdb = d.ParentTmdbID, d.ParentImdbID
	}
	imdbID := ""
	if imdb > 0 {
		imdbID = imdbPrefix + strconv.FormatInt(imdb, 10)
	}
	lang, _, _ := strings.Cut(strings.ToLower(a.Language), regionSep)
	return subtitles.Candidate{
		FileID: fileID, Language: domain.Language(lang), ReleaseName: a.Release, HashMatch: a.MovieHashMatch,
		HearingImpaired: a.HearingImpaired, Forced: a.ForeignPartsOnly, MachineTranslated: a.MachineTranslated,
		AITranslated: a.AITranslated, Tmdb: domain.TmdbID(tmdb), Imdb: imdbID, Year: d.Year,
		Season: d.SeasonNumber, Episode: d.EpisodeNumber, FPS: a.FPS,
	}
}

// quotaError is OpenSubtitles' 406 answer to a download over the daily quota.
type quotaError struct {
	ResetTimeUTC time.Time `json:"reset_time_utc"`
}

func (e *quotaError) Error() string { return subtitles.ErrQuotaExceeded.Error() }

// call sends one API request and decodes a 200 answer into out. Other answers map to the port's errors.
func (c *Client) call(ctx context.Context, method, rawURL, token string, body []byte, out any) error {
	if err := c.wait(ctx); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set(apiKeyHeader, c.appKey)
	req.Header.Set(userAgentHeader, userAgent)
	req.Header.Set(acceptHeader, jsonContentType)
	if body != nil {
		req.Header.Set(contentTypeHeader, jsonContentType)
	}
	if token != "" {
		req.Header.Set(authorizationHeader, bearerPrefix+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%w: %w", subtitles.ErrUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("%w: unreadable answer from %s: %w", subtitles.ErrUnavailable, req.URL.Path, err)
		}
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%w: %s answered %s", subtitles.ErrAuth, req.URL.Path, resp.Status)
	case http.StatusNotAcceptable:
		quota := &quotaError{}
		_ = json.NewDecoder(resp.Body).Decode(quota)
		return quota
	case http.StatusTooManyRequests:
		return &subtitles.RetryAfterError{Wait: retryAfter(resp.Header.Get(retryAfterHeader))}
	}
	return fmt.Errorf("%w: %s answered %s", subtitles.ErrUnavailable, req.URL.Path, resp.Status)
}

// fetch downloads the subtitle file from the temporary link /download returned.
func (c *Client) fetch(ctx context.Context, link string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(userAgentHeader, userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %w", subtitles.ErrUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: the subtitle link answered %s", subtitles.ErrUnavailable, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// wait books the next request slot, requestGap after the last one, and sleeps until it.
func (c *Client) wait(ctx context.Context) error {
	c.mu.Lock()
	now := c.now()
	slot := c.next
	if slot.Before(now) {
		slot = now
	}
	c.next = slot.Add(requestGap)
	c.mu.Unlock()
	if delay := slot.Sub(now); delay > 0 {
		return c.sleep(ctx, delay)
	}
	return nil
}

func retryAfter(header string) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return defaultRetryAfter
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
