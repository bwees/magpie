package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	platformapi "magpie/internal/platform/api"
	"magpie/internal/platform/health"
	"magpie/internal/platform/settings"
)

const (
	defaultBaseURL     = "https://api.themoviedb.org"
	pathAuthentication = "/3/authentication"
	apiKeyParam        = "api_key"
	keyValidDetail     = "API key valid"
)

type authenticationResponse struct {
	Success       bool   `json:"success"`
	StatusMessage string `json:"status_message"`
}

// HealthProbe checks that TMDB accepts the API key.
type HealthProbe struct {
	store   *settings.Store
	client  *http.Client
	baseURL string
}

var _ health.Probe = (*HealthProbe)(nil)

func NewHealthProbe(store *settings.Store) *HealthProbe {
	return &HealthProbe{store: store, client: &http.Client{}, baseURL: defaultBaseURL}
}

func (p *HealthProbe) Service() platformapi.Service { return platformapi.ServiceTMDB }

func (p *HealthProbe) Probe(ctx context.Context) (health.Result, error) {
	conn, err := settings.TMDB.Get(ctx, p.store)
	if err != nil {
		return health.Result{}, err
	}
	if conn.APIKey == "" {
		return health.Result{}, health.ErrNotConfigured
	}

	query := url.Values{apiKeyParam: {conn.APIKey}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+pathAuthentication+"?"+query.Encode(), nil)
	if err != nil {
		return health.Result{}, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return health.Result{}, fmt.Errorf("can't reach TMDB: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var body authenticationResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return health.Result{}, fmt.Errorf("unreadable TMDB response from %s (%s): %w", pathAuthentication, resp.Status, err)
	}
	if resp.StatusCode != http.StatusOK || !body.Success {
		return health.Result{}, fmt.Errorf("the TMDB API key was rejected: %s", body.StatusMessage)
	}
	return health.Result{Detail: keyValidDetail}, nil
}
