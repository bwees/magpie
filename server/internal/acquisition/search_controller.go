package acquisition

import (
	"context"
	"errors"
	"net/http"
	"time"

	"magpie/internal/acquisition/api"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	"magpie/internal/platform/access"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/problem"

	"github.com/go-fuego/fuego"
)

const (
	pathSearchID = "id"

	TypeSearchNotFound         problem.Type = "urn:magpie:problem:search-not-found"
	TypeReleaseNotFound        problem.Type = "urn:magpie:problem:release-not-found"
	TypeReleaseRejected        problem.Type = "urn:magpie:problem:release-rejected"
	TypeReleaseAlreadyGrabbed  problem.Type = "urn:magpie:problem:release-already-grabbed"
	TypeReleaseUnavailable     problem.Type = "urn:magpie:problem:release-unavailable"
	TypeTitleNotFound          problem.Type = "urn:magpie:problem:title-not-found"
	TypeQBittorrentUnavailable problem.Type = "urn:magpie:problem:qbittorrent-unavailable"
	TypeQBittorrentUnsupported problem.Type = "urn:magpie:problem:qbittorrent-unsupported"
)

// StartSearchRequest searches a title, or some of a series' episodes. A non-interactive search is the title
// page's Search button: it grabs on its own and ignores the delay.
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

// GrabResponse says where the grab stands. CHECKING: a magnet waits for its file list before any content
// downloads. REJECTED: the release held a Blocked file and Next, if set, is the release grabbed instead.
type GrabResponse struct {
	Status      GrabStatus         `json:"status"`
	Download    *domain.DownloadID `json:"download,omitempty"`
	BlockedFile *string            `json:"blockedFile,omitempty"`
	Next        *domain.DownloadID `json:"next,omitempty"`
}

type SearchController struct {
	searches  *SearchService
	downloads *DownloadService
	gate      *access.FileGate
}

func NewSearchController(s *SearchService, d *DownloadService, g *access.FileGate) *SearchController {
	return &SearchController{searches: s, downloads: d, gate: g}
}

func (c *SearchController) Register(s *fuego.Server) {
	tags := fuego.OptionTags(tagAcquisition)
	searchPath := fuego.OptionPath(pathSearchID, "Search ID")

	fuego.Post(s, "/acquisition/searches", c.start, tags,
		fuego.OptionOperationID("startSearch"),
		fuego.OptionSummary("Start a search; results stream on the live topic search:<id>, and a non-interactive one grabs on its own (file managers)"),
		fuego.OptionDefaultStatusCode(http.StatusCreated))
	fuego.Get(s, "/acquisition/searches/{id}", c.get, tags, searchPath,
		fuego.OptionOperationID("getSearch"),
		fuego.OptionSummary("A search's cached state, for reconnect and refresh (file managers)"))
	fuego.Delete(s, "/acquisition/searches/{id}", c.cancel, tags, searchPath,
		fuego.OptionOperationID("cancelSearch"),
		fuego.OptionSummary("Cancel a search's in-flight indexer queries (file managers)"),
		fuego.OptionDefaultStatusCode(http.StatusNoContent))
	fuego.Post(s, "/acquisition/searches/{id}/grab", c.grab, tags, searchPath,
		fuego.OptionOperationID("grabRelease"),
		fuego.OptionSummary("Grab a release from the search's cached results; override grabs a rejected one (file managers; override is admin)"),
		fuego.OptionDefaultStatusCode(http.StatusAccepted))
}

func (c *SearchController) start(ctx fuego.ContextWithBody[StartSearchRequest]) (SearchResponse, error) {
	body, err := ctx.Body()
	if err != nil {
		return SearchResponse{}, err
	}
	if err := c.gate.Require(ctx.Context(), body.Title); err != nil {
		return SearchResponse{}, err
	}
	trigger := api.TriggerUserAuto
	if body.Interactive {
		trigger = api.TriggerInteractive
	}
	snapshot, err := c.searches.Start(ctx.Context(), SearchRequest{Title: body.Title, Episodes: body.Episodes, Trigger: trigger})
	if err != nil {
		return SearchResponse{}, searchProblem(err)
	}
	return searchResponse(snapshot), nil
}

func (c *SearchController) get(ctx fuego.ContextNoBody) (SearchResponse, error) {
	snapshot, err := c.managedSearch(ctx.Context(), domain.SearchID(ctx.PathParam(pathSearchID)))
	if err != nil {
		return SearchResponse{}, err
	}
	return searchResponse(snapshot), nil
}

func (c *SearchController) cancel(ctx fuego.ContextNoBody) (any, error) {
	snapshot, err := c.managedSearch(ctx.Context(), domain.SearchID(ctx.PathParam(pathSearchID)))
	if err != nil {
		return nil, err
	}
	return nil, searchProblem(c.searches.Cancel(snapshot.ID))
}

func (c *SearchController) grab(ctx fuego.ContextWithBody[GrabRequest]) (GrabResponse, error) {
	body, err := ctx.Body()
	if err != nil {
		return GrabResponse{}, err
	}
	if body.Override {
		if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
			return GrabResponse{}, err
		}
	}
	snapshot, err := c.managedSearch(ctx.Context(), domain.SearchID(ctx.PathParam(pathSearchID)))
	if err != nil {
		return GrabResponse{}, err
	}
	result, err := c.downloads.Grab(ctx.Context(), snapshot.ID, body.ReleaseID, body.Override)
	if err != nil {
		return GrabResponse{}, searchProblem(err)
	}
	return GrabResponse(result), nil
}

func (c *SearchController) managedSearch(ctx context.Context, id domain.SearchID) (SearchSnapshot, error) {
	if err := access.RequireManager(ctx); err != nil {
		return SearchSnapshot{}, err
	}
	snapshot, err := c.searches.Search(id)
	if err != nil {
		return SearchSnapshot{}, searchProblem(err)
	}
	return snapshot, c.gate.Require(ctx, snapshot.Title)
}

func searchProblem(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrSearchNotFound):
		return problem.New(TypeSearchNotFound, http.StatusNotFound, "The search expired or never existed")
	case errors.Is(err, ErrTitleNotFound), errors.Is(err, catalogapi.ErrNotFound):
		return problem.New(TypeTitleNotFound, http.StatusNotFound, "No such title")
	case errors.Is(err, api.ErrNotFound):
		// The search found neither the title's quality profile nor the default one.
		return problem.New(TypeProfileNotFound, http.StatusNotFound, "The title's quality profile is gone and so is the default profile; pick a default profile in Settings → Quality")
	case errors.Is(err, ErrInvalidSearch):
		return problem.New(problem.TypeBadRequest, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrReleaseNotFound):
		return problem.New(TypeReleaseNotFound, http.StatusNotFound, "The release isn't in this search's results")
	case errors.Is(err, ErrReleaseRejected):
		return problem.New(TypeReleaseRejected, http.StatusConflict, "The release was rejected; use Override to grab it anyway")
	case errors.Is(err, ErrAlreadyGrabbed):
		return problem.New(TypeReleaseAlreadyGrabbed, http.StatusConflict, "This release was already grabbed from this search")
	case errors.Is(err, ErrReleaseUnavailable), errors.Is(err, ErrInvalidTorrent):
		return problem.New(TypeReleaseUnavailable, http.StatusBadGateway, "The release couldn't be fetched from its indexer: "+rootCause(err).Error())
	}
	return qbittorrentProblem(err)
}

func qbittorrentProblem(err error) error {
	switch {
	case errors.Is(err, ErrUnsupportedVersion):
		return problem.New(TypeQBittorrentUnsupported, http.StatusBadGateway, "Magpie needs qBittorrent 5.2 or newer")
	case errors.Is(err, ErrUnauthorized):
		return problem.New(TypeQBittorrentUnavailable, http.StatusBadGateway, "qBittorrent rejected the API key")
	case errors.Is(err, ErrUnavailable), errors.Is(err, ErrTimeout):
		return problem.New(TypeQBittorrentUnavailable, http.StatusBadGateway, "qBittorrent is unreachable: "+rootCause(err).Error())
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
