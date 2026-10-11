package opensubtitles

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	platformapi "magpie/internal/platform/api"
	"magpie/internal/platform/health"
	"magpie/internal/platform/settings"
)

// AppKey is Magpie's OpenSubtitles consumer key, set at build time with
// -ldflags "-X magpie/internal/adapters/opensubtitles.AppKey=…". Users never enter it.
var AppKey string

const (
	defaultBaseURL = "https://api.opensubtitles.com"
	pathLogin      = "/api/v1/login"
	pathUserInfo   = "/api/v1/infos/user"

	apiKeyHeader        = "Api-Key"
	userAgentHeader     = "User-Agent"
	userAgent           = "Magpie v1"
	contentTypeHeader   = "Content-Type"
	acceptHeader        = "Accept"
	jsonContentType     = "application/json"
	authorizationHeader = "Authorization"
	bearerPrefix        = "Bearer "

	vipDetail     = " · VIP"
	quotaDetail   = "%d downloads/day%s"
	missingAppKey = "this build has no OpenSubtitles app key"
)

var errTokenExpired = errors.New("opensubtitles: token expired")

type account struct {
	AllowedDownloads int    `json:"allowed_downloads"`
	Level            string `json:"level"`
	VIP              bool   `json:"vip"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	User  account `json:"user"`
	Token string  `json:"token"`
}

type userInfoResponse struct {
	Data account `json:"data"`
}

// HealthProbe logs in once and reuses the token, since OpenSubtitles limits logins. A rejected token
// triggers one fresh login.
type HealthProbe struct {
	store   *settings.Store
	client  *http.Client
	baseURL string
	appKey  string

	mu       sync.Mutex
	token    string
	tokenFor settings.OpenSubtitlesConnection
}

var _ health.Probe = (*HealthProbe)(nil)

func NewHealthProbe(store *settings.Store) *HealthProbe {
	return &HealthProbe{store: store, client: &http.Client{}, baseURL: defaultBaseURL, appKey: AppKey}
}

func (p *HealthProbe) Service() platformapi.Service { return platformapi.ServiceOpenSubtitles }

func (p *HealthProbe) Probe(ctx context.Context) (health.Result, error) {
	conn, err := settings.OpenSubtitles.Get(ctx, p.store)
	if err != nil {
		return health.Result{}, err
	}
	if conn.Username == "" || conn.Password == "" {
		return health.Result{}, health.ErrNotConfigured
	}
	if p.appKey == "" {
		return health.Result{}, errors.New(missingAppKey)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.token != "" && p.tokenFor == conn {
		info, err := p.userInfo(ctx)
		if !errors.Is(err, errTokenExpired) {
			return result(info), err
		}
	}
	info, err := p.login(ctx, conn)
	if err != nil {
		p.token = ""
		return health.Result{}, err
	}
	return result(info), nil
}

func (p *HealthProbe) login(ctx context.Context, conn settings.OpenSubtitlesConnection) (account, error) {
	body, err := json.Marshal(loginRequest{Username: conn.Username, Password: conn.Password})
	if err != nil {
		return account{}, err
	}
	req, err := p.request(ctx, http.MethodPost, pathLogin, bytes.NewReader(body))
	if err != nil {
		return account{}, err
	}
	req.Header.Set(contentTypeHeader, jsonContentType)
	var resp loginResponse
	status, err := p.do(req, &resp)
	if err != nil {
		return account{}, err
	}
	if status == http.StatusUnauthorized {
		return account{}, errors.New("the OpenSubtitles username or password was rejected")
	}
	p.token, p.tokenFor = resp.Token, conn
	return resp.User, nil
}

func (p *HealthProbe) userInfo(ctx context.Context) (account, error) {
	req, err := p.request(ctx, http.MethodGet, pathUserInfo, nil)
	if err != nil {
		return account{}, err
	}
	req.Header.Set(authorizationHeader, bearerPrefix+p.token)
	var resp userInfoResponse
	status, err := p.do(req, &resp)
	if err != nil {
		return account{}, err
	}
	if status == http.StatusUnauthorized {
		return account{}, errTokenExpired
	}
	return resp.Data, nil
}

func (p *HealthProbe) request(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set(apiKeyHeader, p.appKey)
	req.Header.Set(userAgentHeader, userAgent)
	req.Header.Set(acceptHeader, jsonContentType)
	return req, nil
}

// do returns 401 as a status for the caller to interpret and every other non-200 as an error.
func (p *HealthProbe) do(req *http.Request, into any) (int, error) {
	resp, err := p.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("can't reach OpenSubtitles: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
			return resp.StatusCode, fmt.Errorf("unreadable OpenSubtitles response from %s: %w", req.URL.Path, err)
		}
		return resp.StatusCode, nil
	case http.StatusUnauthorized:
		return resp.StatusCode, nil
	default:
		return resp.StatusCode, fmt.Errorf("unexpected OpenSubtitles response from %s: %s", req.URL.Path, resp.Status)
	}
}

func result(a account) health.Result {
	vip := ""
	if a.VIP {
		vip = vipDetail
	}
	return health.Result{Version: a.Level, Detail: fmt.Sprintf(quotaDetail, a.AllowedDownloads, vip)}
}
