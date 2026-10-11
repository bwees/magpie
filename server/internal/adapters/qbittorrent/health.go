package qbittorrent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	platformapi "magpie/internal/platform/api"
	"magpie/internal/platform/health"
	"magpie/internal/platform/settings"
)

const (
	pathWebAPIVersion  = "/api/v2/app/webapiVersion"
	pathAppVersion     = "/api/v2/app/version"
	pathCategories     = "/api/v2/torrents/categories"
	pathCreateCategory = "/api/v2/torrents/createCategory"

	authorizationHeader = "Authorization"
	bearerPrefix        = "Bearer "
	contentTypeHeader   = "Content-Type"
	formContentType     = "application/x-www-form-urlencoded"
	formCategory        = "category"
	formSavePath        = "savePath"
	appVersionPrefix    = "v"
)

// API keys and Basic auth arrived in WebAPI 2.15, which shipped with qBittorrent 5.2.
var minWebAPIVersion = []int{2, 15}

// HealthProbe checks the WebAPI version and makes sure Magpie's category exists.
type HealthProbe struct {
	store  *settings.Store
	client *http.Client
}

var _ health.Probe = (*HealthProbe)(nil)

func NewHealthProbe(store *settings.Store) *HealthProbe {
	return &HealthProbe{store: store, client: &http.Client{}}
}

func (p *HealthProbe) Service() platformapi.Service { return platformapi.ServiceQBittorrent }

func (p *HealthProbe) Probe(ctx context.Context) (health.Result, error) {
	conn, err := settings.QBittorrent.Get(ctx, p.store)
	if err != nil {
		return health.Result{}, err
	}
	if conn.URL == "" || conn.APIKey == "" {
		return health.Result{}, health.ErrNotConfigured
	}

	webAPI, err := p.text(ctx, conn, pathWebAPIVersion)
	if err != nil {
		return health.Result{}, err
	}
	recent, err := health.VersionAtLeast(webAPI, minWebAPIVersion...)
	if err != nil {
		return health.Result{}, err
	}
	if !recent {
		return health.Result{}, fmt.Errorf("qBittorrent WebAPI %s is too old; Magpie needs qBittorrent 5.2 or newer", webAPI)
	}
	version, err := p.text(ctx, conn, pathAppVersion)
	if err != nil {
		return health.Result{}, err
	}
	if err := p.ensureCategory(ctx, conn); err != nil {
		return health.Result{}, err
	}
	return health.Result{
		Version: strings.TrimPrefix(version, appVersionPrefix),
		Detail:  fmt.Sprintf("WebAPI %s · category %q ready", webAPI, conn.Category),
	}, nil
}

func (p *HealthProbe) ensureCategory(ctx context.Context, conn settings.QBittorrentConnection) error {
	body, err := p.call(ctx, conn, http.MethodGet, pathCategories, nil)
	if err != nil {
		return err
	}
	var categories map[string]json.RawMessage
	if err := json.Unmarshal(body, &categories); err != nil {
		return fmt.Errorf("unreadable qBittorrent categories: %w", err)
	}
	if _, ok := categories[conn.Category]; ok {
		return nil
	}
	form := url.Values{formCategory: {conn.Category}, formSavePath: {""}}
	_, err = p.call(ctx, conn, http.MethodPost, pathCreateCategory, strings.NewReader(form.Encode()))
	return err
}

func (p *HealthProbe) text(ctx context.Context, conn settings.QBittorrentConnection, path string) (string, error) {
	body, err := p.call(ctx, conn, http.MethodGet, path, nil)
	return strings.TrimSpace(string(body)), err
}

func (p *HealthProbe) call(ctx context.Context, conn settings.QBittorrentConnection, method, path string, form io.Reader) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, conn.URL+path, form)
	if err != nil {
		return nil, err
	}
	req.Header.Set(authorizationHeader, bearerPrefix+conn.APIKey)
	if form != nil {
		req.Header.Set(contentTypeHeader, formContentType)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("can't reach qBittorrent: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("the qBittorrent API key was rejected (%s)", resp.Status)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("unexpected qBittorrent response to %s %s: %s", method, path, resp.Status)
	}
	return body, nil
}
