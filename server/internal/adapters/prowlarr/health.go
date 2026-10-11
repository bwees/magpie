package prowlarr

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
	pathSystemStatus   = "/api/v1/system/status"
	healthPathIndexers = "/" + pathIndexers
)

type systemStatus struct {
	Version string `json:"version"`
}

type indexer struct {
	ID int `json:"id"`
}

// HealthProbe checks the API key and lists the indexers Prowlarr has.
type HealthProbe struct {
	store  *settings.Store
	client *http.Client
}

var _ health.Probe = (*HealthProbe)(nil)

func NewHealthProbe(store *settings.Store) *HealthProbe {
	return &HealthProbe{store: store, client: &http.Client{}}
}

func (p *HealthProbe) Service() platformapi.Service { return platformapi.ServiceProwlarr }

func (p *HealthProbe) Probe(ctx context.Context) (health.Result, error) {
	conn, err := settings.Prowlarr.Get(ctx, p.store)
	if err != nil {
		return health.Result{}, err
	}
	if conn.URL == "" || conn.APIKey == "" {
		return health.Result{}, health.ErrNotConfigured
	}
	var status systemStatus
	if err := p.get(ctx, conn, pathSystemStatus, &status); err != nil {
		return health.Result{}, err
	}
	var indexers []indexer
	if err := p.get(ctx, conn, healthPathIndexers, &indexers); err != nil {
		return health.Result{}, err
	}
	return health.Result{Version: status.Version, Detail: fmt.Sprintf("%d indexers found", len(indexers))}, nil
}

func (p *HealthProbe) get(ctx context.Context, conn settings.ProwlarrConnection, path string, into any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, conn.URL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set(apiKeyHeader, conn.APIKey)
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("can't reach Prowlarr: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return fmt.Errorf("the Prowlarr API key was rejected (%s)", resp.Status)
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("unexpected Prowlarr response from %s: %s", path, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		return fmt.Errorf("unreadable Prowlarr response from %s: %w", path, err)
	}
	return nil
}
