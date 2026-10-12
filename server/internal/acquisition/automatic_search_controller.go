package acquisition

import (
	"errors"
	"net/http"
	"time"

	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/problem"
	"magpie/internal/platform/settings"

	"github.com/go-fuego/fuego"
)

// SchedulesResponse is Acquisition's part of Settings → General → Schedules.
type SchedulesResponse struct {
	RSSIntervalMinutes int `json:"rssIntervalMinutes"`
	MissingSweepDays   int `json:"missingSweepDays"`
	CutoffSweepDays    int `json:"cutoffSweepDays"`
	// PendingCheckMinutes is fixed.
	PendingCheckMinutes int `json:"pendingCheckMinutes"`
}

type SchedulesRequest struct {
	RSSIntervalMinutes int `json:"rssIntervalMinutes"`
	MissingSweepDays   int `json:"missingSweepDays"`
	CutoffSweepDays    int `json:"cutoffSweepDays"`
}

// SearchOptionsResponse is Settings → Downloads "Search when a title is added".
type SearchOptionsResponse struct {
	SearchOnAdd bool `json:"searchOnAdd"`
}

type SearchOptionsRequest struct {
	SearchOnAdd bool `json:"searchOnAdd"`
}

// AutomaticSearchController serves the settings of automatic search, RSS and the backlog sweeps.
type AutomaticSearchController struct {
	searches *SearchService
}

func NewAutomaticSearchController(s *SearchService) *AutomaticSearchController {
	return &AutomaticSearchController{searches: s}
}

func (c *AutomaticSearchController) Register(s *fuego.Server) {
	tags := fuego.OptionTags(tagAcquisition)
	fuego.Get(s, "/acquisition/schedules", c.schedules, tags,
		fuego.OptionOperationID("getAcquisitionSchedules"),
		fuego.OptionSummary("RSS sync, backlog sweep and pending check intervals (admin)"))
	fuego.Put(s, "/acquisition/schedules", c.setSchedules, tags,
		fuego.OptionOperationID("setAcquisitionSchedules"),
		fuego.OptionSummary("Change the RSS sync and backlog sweep intervals (admin)"))
	fuego.Get(s, "/acquisition/search-options", c.searchOptions, tags,
		fuego.OptionOperationID("getSearchOptions"),
		fuego.OptionSummary("Whether a requested or added title is searched at once (admin)"))
	fuego.Put(s, "/acquisition/search-options", c.setSearchOptions, tags,
		fuego.OptionOperationID("setSearchOptions"),
		fuego.OptionSummary("Turn searching on title add on or off (admin)"))
}

func (c *AutomaticSearchController) schedules(ctx fuego.ContextNoBody) (SchedulesResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return SchedulesResponse{}, err
	}
	schedules, err := c.searches.Schedules(ctx.Context())
	if err != nil {
		return SchedulesResponse{}, err
	}
	return schedulesResponse(schedules), nil
}

func (c *AutomaticSearchController) setSchedules(ctx fuego.ContextWithBody[SchedulesRequest]) (SchedulesResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return SchedulesResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return SchedulesResponse{}, err
	}
	schedules := Schedules{
		RSSInterval:          settings.Duration(time.Duration(body.RSSIntervalMinutes) * time.Minute),
		MissingSweepInterval: settings.Duration(time.Duration(body.MissingSweepDays) * day),
		CutoffSweepInterval:  settings.Duration(time.Duration(body.CutoffSweepDays) * day),
	}
	err = c.searches.SetSchedules(ctx.Context(), schedules)
	if errors.Is(err, ErrInvalidSchedules) {
		return SchedulesResponse{}, problem.New(problem.TypeBadRequest, http.StatusBadRequest, err.Error())
	}
	if err != nil {
		return SchedulesResponse{}, err
	}
	return schedulesResponse(schedules), nil
}

func schedulesResponse(s Schedules) SchedulesResponse {
	return SchedulesResponse{
		RSSIntervalMinutes:  int(s.RSSInterval.Std() / time.Minute),
		MissingSweepDays:    int(s.MissingSweepInterval.Std() / day),
		CutoffSweepDays:     int(s.CutoffSweepInterval.Std() / day),
		PendingCheckMinutes: int(PendingCheckInterval / time.Minute),
	}
}

func (c *AutomaticSearchController) searchOptions(ctx fuego.ContextNoBody) (SearchOptionsResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return SearchOptionsResponse{}, err
	}
	on, err := c.searches.SearchOnAdd(ctx.Context())
	return SearchOptionsResponse{SearchOnAdd: on}, err
}

func (c *AutomaticSearchController) setSearchOptions(ctx fuego.ContextWithBody[SearchOptionsRequest]) (SearchOptionsResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return SearchOptionsResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return SearchOptionsResponse{}, err
	}
	if err := c.searches.SetSearchOnAdd(ctx.Context(), body.SearchOnAdd); err != nil {
		return SearchOptionsResponse{}, err
	}
	return SearchOptionsResponse(body), nil
}
