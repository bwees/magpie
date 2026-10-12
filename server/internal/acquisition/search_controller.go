package acquisition

import (
	"errors"
	"net/http"
	"time"

	"magpie/internal/acquisition/api"
	"magpie/internal/domain"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/problem"

	"github.com/go-fuego/fuego"
)

const (
	pathSearchID = "id"

	TypeSearchNotFound  problem.Type = "urn:magpie:problem:search-not-found"
	TypeReleaseNotFound problem.Type = "urn:magpie:problem:release-not-found"
	TypeReleaseRejected problem.Type = "urn:magpie:problem:release-rejected"
	TypeGrabUnavailable problem.Type = "urn:magpie:problem:grab-unavailable"
	TypeTitleNotFound   problem.Type = "urn:magpie:problem:title-not-found"
)

// StartSearchRequest searches a title, or some of a series' episodes. Only interactive searches can be
// started from the API for now.
type StartSearchRequest struct {
	Title       domain.TitleID     `json:"title"`
	Episodes    []domain.EpisodeID `json:"episodes,omitempty"`
	Interactive bool               `json:"interactive"`
}

// QualityBucketResponse is one allowed profile item, in profile order.
type QualityBucketResponse struct {
	Name       string `json:"name"`
	QualityIDs []int  `json:"qualityIds" validate:"required"`
	// MeetsCutoff is set on the cutoff item and every item ranked above it.
	MeetsCutoff bool `json:"meetsCutoff"`
}

type SearchProfileResponse struct {
	ID      domain.ProfileID        `json:"id"`
	Name    string                  `json:"name"`
	Cutoff  string                  `json:"cutoff"`
	Buckets []QualityBucketResponse `json:"buckets" validate:"required"`
	// PenalizedFormats are the custom formats the profile scores below zero, such as LQ.
	PenalizedFormats []string `json:"penalizedFormats" validate:"required"`
}

type SearchIndexerResponse struct {
	Name string `json:"name"`
	// Status is PENDING until the indexer answers, then DONE, FAILED or SKIPPED.
	Status  api.IndexerRunStatus `json:"status"`
	Message string               `json:"message"`
}

type SearchResponse struct {
	SearchID  domain.SearchID         `json:"searchId"`
	Title     domain.TitleID          `json:"title"`
	Trigger   api.SearchTrigger       `json:"trigger"`
	StartedAt time.Time               `json:"startedAt"`
	Completed bool                    `json:"completed"`
	Cancelled bool                    `json:"cancelled"`
	Profile   SearchProfileResponse   `json:"profile"`
	Indexers  []SearchIndexerResponse `json:"indexers" validate:"required"`
	// Releases are in arrival order; Rank is current.
	Releases []api.ReleaseDecision `json:"releases" validate:"required"`
}

type GrabRequest struct {
	ReleaseID domain.ReleaseID `json:"releaseId"`
	// Override grabs a rejected release, skipping the decision engine.
	Override bool `json:"override"`
}

type SearchController struct {
	searches *SearchService
}

func NewSearchController(s *SearchService) *SearchController {
	return &SearchController{searches: s}
}

func (c *SearchController) Register(s *fuego.Server) {
	tags := fuego.OptionTags(tagAcquisition)
	searchPath := fuego.OptionPath(pathSearchID, "Search ID")

	// 200, not 201: fuego writes a non-200 default status before the JSON content type, so the body
	// would go out as text/plain.
	fuego.Post(s, "/acquisition/searches", c.start, tags,
		fuego.OptionOperationID("startSearch"),
		fuego.OptionSummary("Start an interactive search; results stream on the live topic search:<id> (admin)"))
	fuego.Get(s, "/acquisition/searches/{id}", c.get, tags, searchPath,
		fuego.OptionOperationID("getSearch"),
		fuego.OptionSummary("A search's cached state, for reconnect and refresh (admin)"))
	fuego.Delete(s, "/acquisition/searches/{id}", c.cancel, tags, searchPath,
		fuego.OptionOperationID("cancelSearch"),
		fuego.OptionSummary("Cancel a search's in-flight indexer queries (admin)"),
		fuego.OptionDefaultStatusCode(http.StatusNoContent))
	fuego.Post(s, "/acquisition/searches/{id}/grab", c.grab, tags, searchPath,
		fuego.OptionOperationID("grabRelease"),
		fuego.OptionSummary("Grab a release from the search's cached results; override grabs a rejected one (admin)"),
		fuego.OptionDefaultStatusCode(http.StatusAccepted))
}

func (c *SearchController) start(ctx fuego.ContextWithBody[StartSearchRequest]) (SearchResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return SearchResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return SearchResponse{}, err
	}
	if !body.Interactive {
		return SearchResponse{}, problem.New(problem.TypeBadRequest, http.StatusBadRequest, "only interactive searches can be started for now")
	}
	snapshot, err := c.searches.Start(ctx.Context(), SearchRequest{Title: body.Title, Episodes: body.Episodes, Trigger: api.TriggerInteractive})
	if err != nil {
		return SearchResponse{}, searchProblem(err)
	}
	return searchResponse(snapshot), nil
}

func (c *SearchController) get(ctx fuego.ContextNoBody) (SearchResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return SearchResponse{}, err
	}
	snapshot, err := c.searches.Search(domain.SearchID(ctx.PathParam(pathSearchID)))
	if err != nil {
		return SearchResponse{}, searchProblem(err)
	}
	return searchResponse(snapshot), nil
}

func (c *SearchController) cancel(ctx fuego.ContextNoBody) (any, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return nil, err
	}
	return nil, searchProblem(c.searches.Cancel(domain.SearchID(ctx.PathParam(pathSearchID))))
}

func (c *SearchController) grab(ctx fuego.ContextWithBody[GrabRequest]) (any, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return nil, err
	}
	body, err := ctx.Body()
	if err != nil {
		return nil, err
	}
	return nil, searchProblem(c.searches.Grab(domain.SearchID(ctx.PathParam(pathSearchID)), body.ReleaseID, body.Override))
}

func searchProblem(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrSearchNotFound):
		return problem.New(TypeSearchNotFound, http.StatusNotFound, "The search expired or never existed")
	case errors.Is(err, ErrTitleNotFound):
		return problem.New(TypeTitleNotFound, http.StatusNotFound, "No such title")
	case errors.Is(err, ErrInvalidSearch):
		return problem.New(problem.TypeBadRequest, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrReleaseNotFound):
		return problem.New(TypeReleaseNotFound, http.StatusNotFound, "The release isn't in this search's results")
	case errors.Is(err, ErrReleaseRejected):
		return problem.New(TypeReleaseRejected, http.StatusConflict, "The release was rejected; use Override to grab it anyway")
	case errors.Is(err, ErrGrabUnavailable):
		return problem.New(TypeGrabUnavailable, http.StatusNotImplemented, "Grabbing isn't available yet")
	}
	return err
}

func searchResponse(s SearchSnapshot) SearchResponse {
	indexers := make([]SearchIndexerResponse, 0, len(s.Indexers))
	for _, run := range s.Indexers {
		indexers = append(indexers, SearchIndexerResponse{Name: run.Indexer.Name, Status: run.Status, Message: run.Message})
	}
	return SearchResponse{
		SearchID:  s.ID,
		Title:     s.Title,
		Trigger:   s.Trigger,
		StartedAt: s.StartedAt,
		Completed: s.Completed,
		Cancelled: s.Cancelled,
		Profile:   searchProfileResponse(s.Profile),
		Indexers:  indexers,
		Releases:  nonNil(s.Releases),
	}
}

func searchProfileResponse(p api.Profile) SearchProfileResponse {
	response := SearchProfileResponse{ID: p.ID, Name: p.Name, Buckets: []QualityBucketResponse{}, PenalizedFormats: []string{}}
	if p.Cutoff >= 0 && p.Cutoff < len(p.Items) {
		response.Cutoff = itemName(p.Items[p.Cutoff])
	}
	for i, item := range p.Items {
		if !item.Allowed {
			continue
		}
		ids := make([]int, 0, len(item.Qualities))
		for _, q := range item.Qualities {
			ids = append(ids, q.ID)
		}
		response.Buckets = append(response.Buckets, QualityBucketResponse{Name: itemName(item), QualityIDs: ids, MeetsCutoff: i <= p.Cutoff})
	}
	for _, score := range p.FormatScores {
		if score.Score < 0 {
			response.PenalizedFormats = append(response.PenalizedFormats, score.Name)
		}
	}
	return response
}

func itemName(item api.ProfileItem) string {
	if item.Group != nil {
		return item.Group.Name
	}
	if len(item.Qualities) > 0 {
		return item.Qualities[0].Name
	}
	return ""
}
