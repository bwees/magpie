package library

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/problem"
	"magpie/internal/platform/settings"

	"github.com/go-fuego/fuego"
)

// LibrarySchedulesResponse is Library's part of Settings → General → Schedules.
type LibrarySchedulesResponse struct {
	JellyfinReconcileHours int `json:"jellyfinReconcileHours"`
}

type LibrarySchedulesRequest struct {
	JellyfinReconcileHours int `json:"jellyfinReconcileHours"`
}

// AvailabilityController serves the settings of following Jellyfin.
type AvailabilityController struct {
	library *Service
}

func NewAvailabilityController(s *Service) *AvailabilityController {
	return &AvailabilityController{library: s}
}

func (c *AvailabilityController) Register(s *fuego.Server) {
	tags := fuego.OptionTags(tagLibrary)
	fuego.Get(s, "/library/schedules", c.schedules, tags,
		fuego.OptionOperationID("getLibrarySchedules"),
		fuego.OptionSummary("How often the Jellyfin full reconcile runs (admin)"))
	fuego.Put(s, "/library/schedules", c.setSchedules, tags,
		fuego.OptionOperationID("setLibrarySchedules"),
		fuego.OptionSummary("Change how often the Jellyfin full reconcile runs (admin)"))
}

func (c *AvailabilityController) schedules(ctx fuego.ContextNoBody) (LibrarySchedulesResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return LibrarySchedulesResponse{}, err
	}
	schedules, err := c.library.Schedules(ctx.Context())
	if err != nil {
		return LibrarySchedulesResponse{}, err
	}
	return schedulesResponse(schedules), nil
}

func (c *AvailabilityController) setSchedules(ctx fuego.ContextWithBody[LibrarySchedulesRequest]) (LibrarySchedulesResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return LibrarySchedulesResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return LibrarySchedulesResponse{}, err
	}
	schedules := Schedules{JellyfinReconcile: settings.Duration(time.Duration(body.JellyfinReconcileHours) * time.Hour)}
	err = c.library.SetSchedules(ctx.Context(), schedules)
	if errors.Is(err, ErrInvalidSchedules) {
		return LibrarySchedulesResponse{}, problem.New(problem.TypeBadRequest, http.StatusBadRequest, strings.TrimPrefix(err.Error(), ErrInvalidSchedules.Error()+": "))
	}
	if err != nil {
		return LibrarySchedulesResponse{}, err
	}
	return schedulesResponse(schedules), nil
}

func schedulesResponse(s Schedules) LibrarySchedulesResponse {
	return LibrarySchedulesResponse{JellyfinReconcileHours: int(s.JellyfinReconcile.Std() / time.Hour)}
}
