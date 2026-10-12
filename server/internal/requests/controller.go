package requests

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"slices"
	"time"

	"magpie/internal/domain"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/problem"
	"magpie/internal/requests/api"

	"github.com/go-fuego/fuego"
)

const (
	tagRequests = "requests"

	pathRequestID = "id"

	queryStatus = "status"
	queryUser   = "userId"
	queryTitle  = "titleId"
	queryCount  = "count"

	root          = "/requests"
	requestsRoute = httpserver.APIPrefix + root + "/"

	TypeAlreadyRequested problem.Type = "urn:magpie:problem:already-requested"
	TypeAlreadyMonitored problem.Type = "urn:magpie:problem:already-monitored"
	TypeRequestNotFound  problem.Type = "urn:magpie:problem:request-not-found"
	TypeWrongStatus      problem.Type = "urn:magpie:problem:request-status"
	TypeUnknownProfile   problem.Type = "urn:magpie:problem:unknown-profile"
)

type CreateRequestRequest struct {
	Kind   domain.MediaKind `json:"kind" validate:"required"`
	TmdbID domain.TmdbID    `json:"tmdbId" validate:"required"`
	// Scope is MOVIE for movies, ALL_SEASONS or SEASONS for series.
	Scope string `json:"scope" validate:"required"`
	// Seasons is set for SEASONS.
	Seasons []int `json:"seasons,omitempty"`
}

type ApproveRequestRequest struct {
	// ProfileID overrides the default profile.
	ProfileID *domain.ProfileID `json:"profileId,omitempty"`
}

type DeclineRequestRequest struct {
	Reason string `json:"reason,omitempty"`
}

type RequesterResponse struct {
	ID          domain.UserID `json:"id" validate:"required"`
	DisplayName string        `json:"displayName" validate:"required"`
}

type RequestResponse struct {
	ID        domain.RequestID  `json:"id" validate:"required"`
	Requester RequesterResponse `json:"requester" validate:"required"`
	Kind      domain.MediaKind  `json:"kind" validate:"required"`
	TmdbID    domain.TmdbID     `json:"tmdbId" validate:"required"`
	// TitleID is set once Catalog has the Title.
	TitleID   *domain.TitleID  `json:"titleId"`
	Scope     string           `json:"scope" validate:"required"`
	Seasons   []int            `json:"seasons" validate:"required"`
	ProfileID domain.ProfileID `json:"profileId" validate:"required"`
	Status    api.Status       `json:"status" validate:"required"`
	CreatedAt time.Time        `json:"createdAt" validate:"required"`
	DecidedBy *domain.UserID   `json:"decidedBy"`
	DecidedAt *time.Time       `json:"decidedAt"`
	// DeclineReason is set for DECLINED requests declined with a reason.
	DeclineReason string `json:"declineReason" validate:"required"`
	// FailureReason is a catalog.TitleAddFailed reason for FAILED requests.
	FailureReason  string `json:"failureReason" validate:"required"`
	AvailableItems int    `json:"availableItems" validate:"required"`
	TotalItems     int    `json:"totalItems" validate:"required"`
}

type StatusCountResponse struct {
	Status api.Status `json:"status" validate:"required"`
	Count  int        `json:"count" validate:"required"`
}

type RequestListResponse struct {
	// Items is empty when count=true.
	Items []RequestResponse `json:"items" validate:"required"`
	// Total is how many requests match every filter.
	Total int `json:"total" validate:"required"`
	// Counts has every status, counted over the visible requests without the status filter.
	Counts []StatusCountResponse `json:"counts" validate:"required"`
}

type Controller struct {
	service *Service
}

func NewController(s *Service) *Controller {
	return &Controller{service: s}
}

func (c *Controller) Register(server *fuego.Server) {
	c.registerIssues(server)
	idPath := fuego.OptionPath(pathRequestID, "Request ID")
	fuego.Post(server, root, c.create,
		fuego.OptionOperationID("createRequest"),
		fuego.OptionSummary("Request a movie, a whole series or chosen seasons; admins and auto-approved users are approved at once"),
		fuego.OptionTags(tagRequests),
		fuego.OptionDefaultStatusCode(http.StatusCreated),
	)
	fuego.Get(server, root, c.list,
		fuego.OptionOperationID("listRequests"),
		fuego.OptionSummary("Requests the viewer may see, newest first: their own unless they may view others'"),
		fuego.OptionTags(tagRequests),
		fuego.OptionQueryArray(queryStatus, "Only these statuses", reflect.String),
		fuego.OptionQuery(queryUser, "Only this user's requests"),
		fuego.OptionQuery(queryTitle, "Only requests linked to this Title"),
		fuego.OptionQueryBool(queryCount, "Return only the total and counts, without items"),
	)
	fuego.Get(server, root+"/{id}", c.get,
		fuego.OptionOperationID("getRequest"),
		fuego.OptionSummary("One request with its item progress"),
		fuego.OptionTags(tagRequests),
		idPath,
	)
	fuego.Post(server, root+"/{id}/approve", c.approve,
		fuego.OptionOperationID("approveRequest"),
		fuego.OptionSummary("Approve a pending request, optionally with another profile (admin)"),
		fuego.OptionTags(tagRequests),
		idPath,
	)
	fuego.Post(server, root+"/{id}/decline", c.decline,
		fuego.OptionOperationID("declineRequest"),
		fuego.OptionSummary("Decline a pending request (admin)"),
		fuego.OptionTags(tagRequests),
		idPath,
	)
	fuego.Post(server, root+"/{id}/retry", c.retry,
		fuego.OptionOperationID("retryRequest"),
		fuego.OptionSummary("Approve a failed request again (admin)"),
		fuego.OptionTags(tagRequests),
		idPath,
	)
	fuego.Delete(server, root+"/{id}", c.cancel,
		fuego.OptionOperationID("cancelRequest"),
		fuego.OptionSummary("Cancel your own pending request"),
		fuego.OptionTags(tagRequests),
		idPath,
		fuego.OptionDefaultStatusCode(http.StatusNoContent),
	)
}

func (c *Controller) create(ctx fuego.ContextWithBody[CreateRequestRequest]) (RequestResponse, error) {
	principal, err := viewer(ctx.Context())
	if err != nil {
		return RequestResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return RequestResponse{}, err
	}
	view, err := c.service.Create(ctx.Context(), principal.User, NewRequest{Kind: body.Kind, Tmdb: body.TmdbID, Scope: body.Scope, Seasons: body.Seasons})
	if err != nil {
		return RequestResponse{}, requestProblem(err)
	}
	return requestResponse(view), nil
}

func (c *Controller) list(ctx fuego.ContextNoBody) (RequestListResponse, error) {
	principal, err := viewer(ctx.Context())
	if err != nil {
		return RequestListResponse{}, err
	}
	q, err := listQuery(ctx)
	if err != nil {
		return RequestListResponse{}, err
	}
	result, err := c.service.List(ctx.Context(), principal, q)
	if err != nil {
		return RequestListResponse{}, requestProblem(err)
	}
	response := RequestListResponse{
		Items:  make([]RequestResponse, 0, len(result.Items)),
		Total:  result.Total,
		Counts: make([]StatusCountResponse, 0, len(AllStatuses)),
	}
	for _, view := range result.Items {
		response.Items = append(response.Items, requestResponse(view))
	}
	for _, status := range AllStatuses {
		response.Counts = append(response.Counts, StatusCountResponse{Status: status, Count: result.Counts[status]})
	}
	return response, nil
}

func listQuery(ctx fuego.ContextNoBody) (ListQuery, error) {
	q := ListQuery{}
	for _, raw := range ctx.QueryParamArr(queryStatus) {
		status := api.Status(raw)
		if !slices.Contains(AllStatuses, status) {
			return q, badRequest("unknown status " + raw)
		}
		q.Statuses = append(q.Statuses, status)
	}
	if raw := ctx.QueryParam(queryUser); raw != "" {
		user := domain.UserID(raw)
		q.User = &user
	}
	if raw := ctx.QueryParam(queryTitle); raw != "" {
		title := domain.TitleID(raw)
		q.Title = &title
	}
	if ctx.QueryParam(queryCount) != "" {
		count, err := ctx.QueryParamBoolErr(queryCount)
		if err != nil {
			return q, badRequest("count must be true or false")
		}
		q.CountOnly = count
	}
	return q, nil
}

func (c *Controller) get(ctx fuego.ContextNoBody) (RequestResponse, error) {
	principal, err := viewer(ctx.Context())
	if err != nil {
		return RequestResponse{}, err
	}
	view, err := c.service.Get(ctx.Context(), principal, domain.RequestID(ctx.PathParam(pathRequestID)))
	if err != nil {
		return RequestResponse{}, requestProblem(err)
	}
	return requestResponse(view), nil
}

func (c *Controller) approve(ctx fuego.ContextWithBody[ApproveRequestRequest]) (RequestResponse, error) {
	principal, err := admin(ctx.Context())
	if err != nil {
		return RequestResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return RequestResponse{}, err
	}
	view, err := c.service.Approve(ctx.Context(), principal.User, domain.RequestID(ctx.PathParam(pathRequestID)), body.ProfileID)
	if err != nil {
		return RequestResponse{}, requestProblem(err)
	}
	return requestResponse(view), nil
}

func (c *Controller) decline(ctx fuego.ContextWithBody[DeclineRequestRequest]) (RequestResponse, error) {
	principal, err := admin(ctx.Context())
	if err != nil {
		return RequestResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return RequestResponse{}, err
	}
	view, err := c.service.Decline(ctx.Context(), principal.User, domain.RequestID(ctx.PathParam(pathRequestID)), body.Reason)
	if err != nil {
		return RequestResponse{}, requestProblem(err)
	}
	return requestResponse(view), nil
}

func (c *Controller) retry(ctx fuego.ContextNoBody) (RequestResponse, error) {
	principal, err := admin(ctx.Context())
	if err != nil {
		return RequestResponse{}, err
	}
	view, err := c.service.Retry(ctx.Context(), principal.User, domain.RequestID(ctx.PathParam(pathRequestID)))
	if err != nil {
		return RequestResponse{}, requestProblem(err)
	}
	return requestResponse(view), nil
}

func (c *Controller) cancel(ctx fuego.ContextNoBody) (any, error) {
	principal, err := viewer(ctx.Context())
	if err != nil {
		return nil, err
	}
	return nil, requestProblem(c.service.Cancel(ctx.Context(), principal.User, domain.RequestID(ctx.PathParam(pathRequestID))))
}

func viewer(ctx context.Context) (domain.Principal, error) {
	principal, ok := domain.PrincipalFrom(ctx)
	if !ok {
		return domain.Principal{}, problem.New(problem.TypeUnauthorized, http.StatusUnauthorized, "sign in first")
	}
	return principal, nil
}

func admin(ctx context.Context) (domain.Principal, error) {
	if err := httpserver.RequireAdmin(ctx); err != nil {
		return domain.Principal{}, err
	}
	return viewer(ctx)
}

func requestResponse(v RequestView) RequestResponse {
	seasons := v.Seasons
	if seasons == nil {
		seasons = []int{}
	}
	return RequestResponse{
		ID:             v.ID,
		Requester:      RequesterResponse{ID: v.User, DisplayName: v.RequesterName},
		Kind:           v.Kind,
		TmdbID:         v.Tmdb,
		TitleID:        v.Title,
		Scope:          v.Scope,
		Seasons:        seasons,
		ProfileID:      v.Profile,
		Status:         v.Status,
		CreatedAt:      v.CreatedAt,
		DecidedBy:      v.DecidedBy,
		DecidedAt:      v.DecidedAt,
		DeclineReason:  v.DeclineReason,
		FailureReason:  v.FailureReason,
		AvailableItems: v.AvailableItems,
		TotalItems:     v.TotalItems,
	}
}

// requestProblem maps service errors to problems. A duplicate names the request that covers it in instance.
func requestProblem(err error) error {
	var duplicate DuplicateError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &duplicate):
		conflict := problem.New(TypeAlreadyRequested, http.StatusConflict, "already requested")
		conflict.Instance = requestsRoute + string(duplicate.Existing)
		return conflict
	case errors.Is(err, ErrAlreadyMonitored):
		return problem.New(TypeAlreadyMonitored, http.StatusConflict, "already in the library and monitored")
	case errors.Is(err, api.ErrNotFound):
		return problem.New(TypeRequestNotFound, http.StatusNotFound, "no such request")
	case errors.Is(err, ErrWrongStatus):
		return problem.New(TypeWrongStatus, http.StatusConflict, err.Error())
	case errors.Is(err, ErrNotYours):
		return problem.New(problem.TypeForbidden, http.StatusForbidden, "you can only cancel your own requests")
	case errors.Is(err, ErrHiddenUser):
		return problem.New(problem.TypeForbidden, http.StatusForbidden, "you can only see your own requests")
	case errors.Is(err, ErrInvalidRequest):
		return badRequest(err.Error())
	case errors.Is(err, ErrUnknownProfile):
		return problem.New(TypeUnknownProfile, http.StatusBadRequest, err.Error())
	default:
		return err
	}
}

func badRequest(detail string) error {
	return problem.New(problem.TypeBadRequest, http.StatusBadRequest, detail)
}
