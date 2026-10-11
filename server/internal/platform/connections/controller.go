package connections

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	platformapi "magpie/internal/platform/api"
	"magpie/internal/platform/health"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/problem"
	"magpie/internal/platform/settings"

	"github.com/go-fuego/fuego"
	"go.uber.org/fx"
)

const (
	tagPlatform  = "platform"
	pathService  = "service"
	routeAll     = "/platform/connections"
	routeTestOne = routeAll + "/{" + pathService + "}/test"

	TypeInvalidConnection problem.Type = "urn:magpie:problem:invalid-connection"
	TypeUnknownService    problem.Type = "urn:magpie:problem:unknown-service"
)

var Module = fx.Module("connections",
	fx.Provide(
		NewService,
		httpserver.AsController(NewController),
	),
)

type ServiceHealthResponse struct {
	Service platformapi.Service `json:"service" validate:"required"`
	Status  health.Status       `json:"status" validate:"required"`
	Version string              `json:"version" validate:"required"`
	Detail  string              `json:"detail" validate:"required"`
	Error   string              `json:"error" validate:"required"`
	// CheckedAt is absent until the service is probed after a restart.
	CheckedAt *time.Time `json:"checkedAt,omitempty"`
}

type ConnectionsResponse struct {
	TMDB          settings.TMDBConnection          `json:"tmdb" validate:"required"`
	QBittorrent   settings.QBittorrentConnection   `json:"qbittorrent" validate:"required"`
	Prowlarr      settings.ProwlarrConnection      `json:"prowlarr" validate:"required"`
	Jellyfin      settings.JellyfinConnection      `json:"jellyfin" validate:"required"`
	OpenSubtitles settings.OpenSubtitlesConnection `json:"openSubtitles" validate:"required"`
	Health        []ServiceHealthResponse          `json:"health" validate:"required"`
}

// UpdateConnectionsRequest changes only the services it names. A secret equal to the mask keeps the stored one.
type UpdateConnectionsRequest struct {
	TMDB          *settings.TMDBConnection          `json:"tmdb,omitempty"`
	QBittorrent   *settings.QBittorrentConnection   `json:"qbittorrent,omitempty"`
	Prowlarr      *settings.ProwlarrConnection      `json:"prowlarr,omitempty"`
	Jellyfin      *settings.JellyfinConnection      `json:"jellyfin,omitempty"`
	OpenSubtitles *settings.OpenSubtitlesConnection `json:"openSubtitles,omitempty"`
}

type Controller struct {
	service *Service
}

func NewController(s *Service) *Controller {
	return &Controller{service: s}
}

func (c *Controller) Register(api *fuego.Server) {
	fuego.Get(api, routeAll, c.get,
		fuego.OptionOperationID("getConnections"),
		fuego.OptionSummary("External service connections with masked secrets and health (admin)"),
		fuego.OptionTags(tagPlatform),
	)
	fuego.Put(api, routeAll, c.save,
		fuego.OptionOperationID("updateConnections"),
		fuego.OptionSummary("Change the connections named in the body and re-check them (admin)"),
		fuego.OptionTags(tagPlatform),
	)
	fuego.Post(api, routeTestOne, c.test,
		fuego.OptionOperationID("testConnection"),
		fuego.OptionSummary("Probe one stored connection now (admin)"),
		fuego.OptionTags(tagPlatform),
		fuego.OptionPath(pathService, "tmdb, qbittorrent, prowlarr, jellyfin or opensubtitles"),
	)
}

func (c *Controller) get(ctx fuego.ContextNoBody) (ConnectionsResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return ConnectionsResponse{}, err
	}
	connections, err := c.service.Get(ctx.Context())
	if err != nil {
		return ConnectionsResponse{}, err
	}
	return c.response(ctx.Context(), connections)
}

func (c *Controller) save(ctx fuego.ContextWithBody[UpdateConnectionsRequest]) (ConnectionsResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return ConnectionsResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return ConnectionsResponse{}, err
	}
	connections, err := c.service.Save(ctx.Context(), Update(body))
	if errors.Is(err, ErrInvalidConnection) {
		return ConnectionsResponse{}, problem.New(TypeInvalidConnection, http.StatusBadRequest, err.Error())
	}
	if err != nil {
		return ConnectionsResponse{}, err
	}
	return c.response(ctx.Context(), connections)
}

func (c *Controller) test(ctx fuego.ContextNoBody) (ServiceHealthResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return ServiceHealthResponse{}, err
	}
	name := ctx.PathParam(pathService)
	service := platformapi.Service(strings.ToUpper(name))
	if !slices.Contains(health.Services, service) {
		return ServiceHealthResponse{}, problem.New(TypeUnknownService, http.StatusNotFound, "no connection named "+name)
	}
	check, err := c.service.Test(ctx.Context(), service)
	if err != nil {
		return ServiceHealthResponse{}, err
	}
	return healthResponse(check), nil
}

func (c *Controller) response(ctx context.Context, connections Connections) (ConnectionsResponse, error) {
	checks, err := c.service.Health(ctx)
	if err != nil {
		return ConnectionsResponse{}, err
	}
	statuses := make([]ServiceHealthResponse, 0, len(checks))
	for _, check := range checks {
		statuses = append(statuses, healthResponse(check))
	}
	return ConnectionsResponse{
		TMDB:          connections.TMDB,
		QBittorrent:   connections.QBittorrent,
		Prowlarr:      connections.Prowlarr,
		Jellyfin:      connections.Jellyfin,
		OpenSubtitles: connections.OpenSubtitles,
		Health:        statuses,
	}, nil
}

func healthResponse(check health.Check) ServiceHealthResponse {
	response := ServiceHealthResponse{
		Service: check.Service,
		Status:  check.Status,
		Version: check.Version,
		Detail:  check.Detail,
		Error:   check.Error,
	}
	if !check.CheckedAt.IsZero() {
		checkedAt := check.CheckedAt
		response.CheckedAt = &checkedAt
	}
	return response
}
