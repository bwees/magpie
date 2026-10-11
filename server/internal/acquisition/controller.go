package acquisition

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"magpie/internal/acquisition/api"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/problem"
	"magpie/internal/platform/settings"

	"github.com/go-fuego/fuego"
)

const (
	tagAcquisition = "acquisition"
	pathIndexerID  = "id"

	TypeIndexerNotFound      problem.Type = "urn:magpie:problem:indexer-not-found"
	TypeProwlarrUnauthorized problem.Type = "urn:magpie:problem:prowlarr-unauthorized"
	TypeProwlarrUnavailable  problem.Type = "urn:magpie:problem:prowlarr-unavailable"
)

type IndexerCapsResponse struct {
	MovieSearch bool           `json:"movieSearch"`
	MovieParams []TorznabParam `json:"movieParams" validate:"required"`
	TVSearch    bool           `json:"tvSearch"`
	TVParams    []TorznabParam `json:"tvParams" validate:"required"`
	Categories  []int          `json:"categories" validate:"required"`
}

type IndexerResponse struct {
	ID                int                 `json:"id"`
	Name              string              `json:"name"`
	Privacy           IndexerPrivacy      `json:"privacy"`
	Status            IndexerStatus       `json:"status"`
	LastError         string              `json:"lastError"`
	FailingSince      *time.Time          `json:"failingSince,omitempty"`
	AverageResponseMs int64               `json:"averageResponseMs"`
	LastOKAt          *time.Time          `json:"lastOkAt,omitempty"`
	Caps              IndexerCapsResponse `json:"caps"`
	Priority          int                 `json:"priority"`
	MinSeeders        int                 `json:"minSeeders"`
	SeedingOverride   SeedingOverride     `json:"seedingOverride"`
	SeedRatio         *float64            `json:"seedRatio,omitempty"`
	SeedDays          *int                `json:"seedDays,omitempty"`
}

type IndexerListResponse struct {
	Indexers []IndexerResponse `json:"indexers" validate:"required"`
	// ProwlarrURL is "" when no Prowlarr connection is set up.
	ProwlarrURL string `json:"prowlarrUrl"`
	// LastSyncedAt is the latest sync attempt since the server started.
	LastSyncedAt *time.Time `json:"lastSyncedAt,omitempty"`
	// SyncError is set when that attempt failed, e.g. Prowlarr was unreachable.
	SyncError string `json:"syncError,omitempty"`
}

// UpdateIndexerRequest replaces Magpie's settings for one indexer. Omitted seeding goals use the global goal.
type UpdateIndexerRequest struct {
	Priority        int             `json:"priority"`
	MinSeeders      int             `json:"minSeeders"`
	SeedingOverride SeedingOverride `json:"seedingOverride"`
	SeedRatio       *float64        `json:"seedRatio,omitempty"`
	SeedDays        *int            `json:"seedDays,omitempty"`
}

type IndexerOptionsResponse struct {
	SyncIntervalMinutes  int `json:"syncIntervalMinutes"`
	SearchTimeoutSeconds int `json:"searchTimeoutSeconds"`
	ResultCacheMinutes   int `json:"resultCacheMinutes"`
}

type IndexerOptionsRequest struct {
	SyncIntervalMinutes  int `json:"syncIntervalMinutes"`
	SearchTimeoutSeconds int `json:"searchTimeoutSeconds"`
	ResultCacheMinutes   int `json:"resultCacheMinutes"`
}

type Controller struct {
	service *Service
}

func NewController(s *Service) *Controller {
	return &Controller{service: s}
}

func (c *Controller) Register(api *fuego.Server) {
	fuego.Get(api, "/acquisition/indexers", c.listIndexers,
		fuego.OptionOperationID("listIndexers"),
		fuego.OptionSummary("List Prowlarr's indexers with Magpie's settings (admin)"),
		fuego.OptionTags(tagAcquisition),
	)
	fuego.Put(api, "/acquisition/indexers/{id}", c.updateIndexer,
		fuego.OptionOperationID("updateIndexer"),
		fuego.OptionSummary("Replace Magpie's settings for one indexer (admin)"),
		fuego.OptionTags(tagAcquisition),
		fuego.OptionPath(pathIndexerID, "Prowlarr's indexer ID", fuego.ParamInteger()),
	)
	fuego.Post(api, "/acquisition/indexers/sync", c.syncIndexers,
		fuego.OptionOperationID("syncIndexers"),
		fuego.OptionSummary("Pull the indexer list from Prowlarr now (admin)"),
		fuego.OptionTags(tagAcquisition),
	)
	fuego.Get(api, "/acquisition/indexer-options", c.indexerOptions,
		fuego.OptionOperationID("getIndexerOptions"),
		fuego.OptionSummary("Sync interval, search timeout and result cache for all indexers (admin)"),
		fuego.OptionTags(tagAcquisition),
	)
	fuego.Put(api, "/acquisition/indexer-options", c.setIndexerOptions,
		fuego.OptionOperationID("setIndexerOptions"),
		fuego.OptionSummary("Change the options that apply to all indexers (admin)"),
		fuego.OptionTags(tagAcquisition),
	)
}

func (c *Controller) listIndexers(ctx fuego.ContextNoBody) (IndexerListResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return IndexerListResponse{}, err
	}
	list, err := c.service.Indexers(ctx.Context())
	if err != nil {
		return IndexerListResponse{}, err
	}
	return listResponse(list), nil
}

func (c *Controller) syncIndexers(ctx fuego.ContextNoBody) (IndexerListResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return IndexerListResponse{}, err
	}
	if err := c.service.SyncIndexers(ctx.Context()); err != nil {
		return IndexerListResponse{}, prowlarrProblem(err)
	}
	list, err := c.service.Indexers(ctx.Context())
	if err != nil {
		return IndexerListResponse{}, err
	}
	return listResponse(list), nil
}

func (c *Controller) updateIndexer(ctx fuego.ContextWithBody[UpdateIndexerRequest]) (IndexerResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return IndexerResponse{}, err
	}
	id, err := strconv.Atoi(ctx.PathParam(pathIndexerID))
	if err != nil {
		return IndexerResponse{}, problem.New(problem.TypeBadRequest, http.StatusBadRequest, "indexer ID must be a number")
	}
	body, err := ctx.Body()
	if err != nil {
		return IndexerResponse{}, err
	}
	ix, err := c.service.UpdateIndexer(ctx.Context(), id, IndexerSettings(body))
	switch {
	case errors.Is(err, ErrInvalidIndexerSettings):
		return IndexerResponse{}, problem.New(problem.TypeBadRequest, http.StatusBadRequest, err.Error())
	case errors.Is(err, api.ErrNotFound):
		return IndexerResponse{}, problem.New(TypeIndexerNotFound, http.StatusNotFound, "no indexer "+strconv.Itoa(id))
	case err != nil:
		return IndexerResponse{}, err
	}
	return indexerResponse(ix), nil
}

func (c *Controller) indexerOptions(ctx fuego.ContextNoBody) (IndexerOptionsResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return IndexerOptionsResponse{}, err
	}
	options, err := c.service.IndexerOptions(ctx.Context())
	if err != nil {
		return IndexerOptionsResponse{}, err
	}
	return optionsResponse(options), nil
}

func (c *Controller) setIndexerOptions(ctx fuego.ContextWithBody[IndexerOptionsRequest]) (IndexerOptionsResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return IndexerOptionsResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return IndexerOptionsResponse{}, err
	}
	options := IndexerOptions{
		SyncInterval:  settings.Duration(time.Duration(body.SyncIntervalMinutes) * time.Minute),
		SearchTimeout: settings.Duration(time.Duration(body.SearchTimeoutSeconds) * time.Second),
		ResultCache:   settings.Duration(time.Duration(body.ResultCacheMinutes) * time.Minute),
	}
	err = c.service.SetIndexerOptions(ctx.Context(), options)
	if errors.Is(err, ErrInvalidIndexerOptions) {
		return IndexerOptionsResponse{}, problem.New(problem.TypeBadRequest, http.StatusBadRequest, err.Error())
	}
	if err != nil {
		return IndexerOptionsResponse{}, err
	}
	return optionsResponse(options), nil
}

func prowlarrProblem(err error) error {
	switch {
	case errors.Is(err, ErrUnauthorized):
		return problem.New(TypeProwlarrUnauthorized, http.StatusBadGateway, "Prowlarr rejected the API key")
	case errors.Is(err, ErrUnavailable), errors.Is(err, ErrTimeout):
		return problem.New(TypeProwlarrUnavailable, http.StatusBadGateway, syncErrorMessage(err))
	}
	return err
}

// syncErrorMessage is what the Indexers page shows when a sync failed.
func syncErrorMessage(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrUnauthorized):
		return "Prowlarr rejected the API key"
	case errors.Is(err, ErrTimeout):
		return "Prowlarr didn't answer in time"
	case errors.Is(err, ErrUnavailable):
		return "Prowlarr is unreachable: " + rootCause(err).Error()
	}
	return "Indexer sync failed"
}

// rootCause follows the last wrapped error down, so "acquisition: service unavailable: Get …: dial tcp …:
// connection refused" becomes "connection refused".
func rootCause(err error) error {
	for {
		var next error
		switch wrapped := err.(type) {
		case interface{ Unwrap() error }:
			next = wrapped.Unwrap()
		case interface{ Unwrap() []error }:
			if all := wrapped.Unwrap(); len(all) > 0 {
				next = all[len(all)-1]
			}
		}
		if next == nil {
			return err
		}
		err = next
	}
}

func listResponse(list IndexerList) IndexerListResponse {
	response := IndexerListResponse{
		Indexers:    make([]IndexerResponse, 0, len(list.Indexers)),
		ProwlarrURL: list.WebURL,
		SyncError:   syncErrorMessage(list.LastSync.Err),
	}
	if !list.LastSync.At.IsZero() {
		response.LastSyncedAt = &list.LastSync.At
	}
	for _, ix := range list.Indexers {
		response.Indexers = append(response.Indexers, indexerResponse(ix))
	}
	return response
}

func indexerResponse(ix Indexer) IndexerResponse {
	return IndexerResponse{
		ID:                ix.ProwlarrID,
		Name:              ix.Name,
		Privacy:           ix.Privacy,
		Status:            ix.Status,
		LastError:         ix.LastError,
		FailingSince:      ix.FailingSince,
		AverageResponseMs: ix.AverageResponse.Milliseconds(),
		LastOKAt:          ix.LastOKAt,
		Caps: IndexerCapsResponse{
			MovieSearch: ix.Caps.MovieSearch,
			MovieParams: nonNil(ix.Caps.MovieParams),
			TVSearch:    ix.Caps.TVSearch,
			TVParams:    nonNil(ix.Caps.TVParams),
			Categories:  nonNil(ix.Caps.Categories),
		},
		Priority:        ix.Settings.Priority,
		MinSeeders:      ix.Settings.MinSeeders,
		SeedingOverride: ix.Settings.SeedingOverride,
		SeedRatio:       ix.Settings.SeedRatio,
		SeedDays:        ix.Settings.SeedDays,
	}
}

func optionsResponse(o IndexerOptions) IndexerOptionsResponse {
	return IndexerOptionsResponse{
		SyncIntervalMinutes:  int(o.SyncInterval.Std() / time.Minute),
		SearchTimeoutSeconds: int(o.SearchTimeout.Std() / time.Second),
		ResultCacheMinutes:   int(o.ResultCache.Std() / time.Minute),
	}
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
