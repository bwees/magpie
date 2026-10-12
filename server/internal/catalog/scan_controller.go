package catalog

import (
	"errors"
	"net/http"
	"time"

	"magpie/internal/domain"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/problem"

	"github.com/go-fuego/fuego"
)

const (
	pathReviewID = "id"

	TypeScanReviewNotFound problem.Type = "urn:magpie:problem:scan-review-not-found"
	TypeScanReviewResolved problem.Type = "urn:magpie:problem:scan-review-resolved"
)

type ScanCandidateResponse struct {
	TmdbID    domain.TmdbID    `json:"tmdbId" validate:"required"`
	Kind      domain.MediaKind `json:"kind" validate:"required"`
	Name      string           `json:"name" validate:"required"`
	Year      int              `json:"year" validate:"required"`
	Overview  string           `json:"overview" validate:"required"`
	PosterURL *string          `json:"posterUrl"`
	// Score is 0–1: how alike the folder's title and year are to this result's.
	Score float64 `json:"score" validate:"required"`
}

// ScanReviewResponse is one library folder Catalog couldn't match with certainty.
type ScanReviewResponse struct {
	ID          string           `json:"id" validate:"required"`
	FolderPath  string           `json:"folderPath" validate:"required"`
	Kind        domain.MediaKind `json:"kind" validate:"required"`
	ParsedTitle string           `json:"parsedTitle" validate:"required"`
	ParsedYear  *int             `json:"parsedYear"`
	TmdbHint    *domain.TmdbID   `json:"tmdbHint"`
	// State is PENDING, MATCHED or IGNORED.
	State      ScanReviewState         `json:"state" validate:"required"`
	TitleID    *domain.TitleID         `json:"titleId"`
	Candidates []ScanCandidateResponse `json:"candidates" validate:"required"`
	CreatedAt  time.Time               `json:"createdAt" validate:"required"`
}

type MatchScanReviewRequest struct {
	TmdbID domain.TmdbID `json:"tmdbId" validate:"required"`
}

// ScanReviewController serves the library-scan review queue.
type ScanReviewController struct {
	service *Service
}

func NewScanReviewController(s *Service) *ScanReviewController {
	return &ScanReviewController{service: s}
}

func (c *ScanReviewController) Register(server *fuego.Server) {
	group := fuego.Group(server, "/catalog")
	tags := fuego.OptionTags(tagCatalog)
	reviewPath := fuego.OptionPath(pathReviewID, "Scan review ID")
	fuego.Get(group, "/scan-review", c.list, tags,
		fuego.OptionOperationID("listScanReview"),
		fuego.OptionSummary("Library folders a scan couldn't match with certainty, oldest first (admin)"))
	fuego.Post(group, "/scan-review/{id}/match", c.match, tags, reviewPath,
		fuego.OptionOperationID("matchScanReview"),
		fuego.OptionSummary("Match a folder to a TMDB title; creates the title if it's new (admin)"))
	fuego.Post(group, "/scan-review/{id}/ignore", c.ignore, tags, reviewPath,
		fuego.OptionOperationID("ignoreScanReview"),
		fuego.OptionSummary("Leave a folder out of the library; later scans don't ask again (admin)"))
}

func (c *ScanReviewController) list(ctx fuego.ContextNoBody) ([]ScanReviewResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return nil, err
	}
	reviews, err := c.service.ScanReviews(ctx.Context())
	if err != nil {
		return nil, err
	}
	out := make([]ScanReviewResponse, 0, len(reviews))
	for _, r := range reviews {
		out = append(out, scanReviewResponse(r))
	}
	return out, nil
}

func (c *ScanReviewController) match(ctx fuego.ContextWithBody[MatchScanReviewRequest]) (ScanReviewResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return ScanReviewResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return ScanReviewResponse{}, err
	}
	if body.TmdbID <= 0 {
		return ScanReviewResponse{}, badRequest("tmdbId must be a positive number")
	}
	review, err := c.service.MatchScanReview(ctx.Context(), ctx.PathParam(pathReviewID), body.TmdbID)
	if err != nil {
		return ScanReviewResponse{}, scanReviewProblem(err)
	}
	return scanReviewResponse(review), nil
}

func (c *ScanReviewController) ignore(ctx fuego.ContextNoBody) (ScanReviewResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return ScanReviewResponse{}, err
	}
	review, err := c.service.IgnoreScanReview(ctx.Context(), ctx.PathParam(pathReviewID))
	if err != nil {
		return ScanReviewResponse{}, scanReviewProblem(err)
	}
	return scanReviewResponse(review), nil
}

func scanReviewProblem(err error) error {
	switch {
	case errors.Is(err, ErrScanReviewNotFound):
		return problem.New(TypeScanReviewNotFound, http.StatusNotFound, "No such folder in the review queue")
	case errors.Is(err, ErrScanReviewResolved):
		return problem.New(TypeScanReviewResolved, http.StatusConflict, "The folder was already matched or ignored")
	case errors.Is(err, ErrTmdbNotFound):
		return problem.New(TypeTitleNotFound, http.StatusNotFound, "TMDB has no such title")
	case isTmdbError(err):
		return problem.New(TypeTmdbUnavailable, http.StatusServiceUnavailable, "TMDB is unavailable: "+err.Error())
	}
	return err
}

func scanReviewResponse(r ScanReview) ScanReviewResponse {
	candidates := make([]ScanCandidateResponse, 0, len(r.Candidates))
	for _, c := range r.Candidates {
		candidate := ScanCandidateResponse{TmdbID: c.Tmdb, Kind: c.Kind, Name: c.Name, Year: c.Year, Overview: c.Overview, Score: c.Score}
		if c.PosterURL != "" {
			candidate.PosterURL = &c.PosterURL
		}
		candidates = append(candidates, candidate)
	}
	return ScanReviewResponse{
		ID: r.ID, FolderPath: r.Folder, Kind: r.Kind, ParsedTitle: r.ParsedTitle, ParsedYear: r.ParsedYear,
		TmdbHint: r.TmdbHint, State: r.State, TitleID: r.Title, Candidates: candidates, CreatedAt: r.CreatedAt,
	}
}
