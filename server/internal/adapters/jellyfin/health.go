package jellyfin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	platformapi "magpie/internal/platform/api"
	"magpie/internal/platform/health"
	"magpie/internal/platform/settings"
)

const (
	pathSystemInfo      = "/System/Info"
	authorizationHeader = "Authorization"
	// Jellyfin 12 accepts the API key only in this header; X-Emby-Token and api_key are disabled.
	tokenAuthorization = `MediaBrowser Token="%s"`
)

var minServerVersion = []int{12}

type systemInfo struct {
	Version    string `json:"Version"`
	ServerName string `json:"ServerName"`
}

// HealthProbe checks the API key and that the server runs Jellyfin 12 or newer.
type HealthProbe struct {
	store  *settings.Store
	client *http.Client
}

var _ health.Probe = (*HealthProbe)(nil)

func NewHealthProbe(store *settings.Store) *HealthProbe {
	return &HealthProbe{store: store, client: &http.Client{}}
}

func (p *HealthProbe) Service() platformapi.Service { return platformapi.ServiceJellyfin }

func (p *HealthProbe) Probe(ctx context.Context) (health.Result, error) {
	conn, err := settings.Jellyfin.Get(ctx, p.store)
	if err != nil {
		return health.Result{}, err
	}
	if conn.URL == "" || conn.APIKey == "" {
		return health.Result{}, health.ErrNotConfigured
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, conn.URL+pathSystemInfo, nil)
	if err != nil {
		return health.Result{}, err
	}
	req.Header.Set(authorizationHeader, fmt.Sprintf(tokenAuthorization, conn.APIKey))
	resp, err := p.client.Do(req)
	if err != nil {
		return health.Result{}, fmt.Errorf("can't reach Jellyfin: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return health.Result{}, fmt.Errorf("the Jellyfin API key was rejected (%s)", resp.Status)
	case resp.StatusCode != http.StatusOK:
		return health.Result{}, fmt.Errorf("unexpected Jellyfin response from %s: %s", pathSystemInfo, resp.Status)
	}
	var info systemInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return health.Result{}, fmt.Errorf("unreadable Jellyfin response from %s: %w", pathSystemInfo, err)
	}

	recent, err := health.VersionAtLeast(info.Version, minServerVersion...)
	if err != nil {
		return health.Result{}, err
	}
	if !recent {
		return health.Result{}, fmt.Errorf("this server runs Jellyfin %s; Magpie needs Jellyfin 12 or newer", info.Version)
	}
	return health.Result{Version: info.Version, Detail: info.ServerName}, nil
}
