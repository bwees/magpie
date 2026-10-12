package requests

import (
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
	tagIssues   = "issues"
	pathIssueID = "id"
	issuesRoot  = root + "/issues"

	// importsRoute is Library's imports queue, where a waiting download is mapped or removed.
	importsRoute = httpserver.APIPrefix + "/library/imports/"

	TypeIssueNotFound problem.Type = "urn:magpie:problem:issue-not-found"
	TypeIssueStatus   problem.Type = "urn:magpie:problem:issue-status"
	TypeImportWaiting problem.Type = "urn:magpie:problem:import-waiting"
)

type ReportIssueRequest struct {
	TitleID domain.TitleID `json:"titleId" validate:"required"`
	// EpisodeID narrows the issue to one episode of a series.
	EpisodeID *domain.EpisodeID `json:"episodeId,omitempty"`
	// Kind is VIDEO, AUDIO, SUBTITLE or OTHER.
	Kind string `json:"kind" validate:"required"`
	Text string `json:"text" validate:"required"`
}

type CommentIssueRequest struct {
	Body string `json:"body" validate:"required"`
}

type IssueActionRequest struct {
	// Action is REGRAB or RERUN_SUBTITLES.
	Action string `json:"action" validate:"required"`
	// WaitingRemoved tells a REGRAB that the admin removed the download waiting for its files to be mapped
	// (Library's Remove & blocklist), which otherwise answers 409 import-waiting.
	WaitingRemoved bool `json:"waitingRemoved,omitempty"`
}

type IssueEpisodeResponse struct {
	ID     domain.EpisodeID `json:"id" validate:"required"`
	Season int              `json:"season" validate:"required"`
	Number int              `json:"number" validate:"required"`
	Name   string           `json:"name" validate:"required"`
}

type IssueResponse struct {
	ID       domain.IssueID    `json:"id" validate:"required"`
	Reporter RequesterResponse `json:"reporter" validate:"required"`
	TitleID  domain.TitleID    `json:"titleId" validate:"required"`
	// TitleName is "" when Catalog no longer has the Title.
	TitleName string `json:"titleName" validate:"required"`
	// EpisodeID is set for an issue on one episode; Episode has its numbering while Catalog knows it.
	EpisodeID *domain.EpisodeID     `json:"episodeId"`
	Episode   *IssueEpisodeResponse `json:"episode,omitempty"`
	Kind      string                `json:"kind" validate:"required"`
	Text      string                `json:"text" validate:"required"`
	// Status is OPEN, ACTION_PENDING or RESOLVED.
	Status string `json:"status" validate:"required"`
	// Action is the fix last asked for (REGRAB or RERUN_SUBTITLES), "" when none was.
	Action     string     `json:"action" validate:"required"`
	Comments   int        `json:"comments" validate:"required"`
	CreatedAt  time.Time  `json:"createdAt" validate:"required"`
	ResolvedAt *time.Time `json:"resolvedAt"`
	// ResolvedBy is ACTION, TITLE_REMOVED or the user ID who closed it; "" while open.
	ResolvedBy string `json:"resolvedBy" validate:"required"`
}

type IssueCommentResponse struct {
	ID        CommentID         `json:"id" validate:"required"`
	Author    RequesterResponse `json:"author" validate:"required"`
	Body      string            `json:"body" validate:"required"`
	CreatedAt time.Time         `json:"createdAt" validate:"required"`
}

type IssueThreadResponse struct {
	Issue IssueResponse `json:"issue" validate:"required"`
	// Comments are oldest first.
	Comments []IssueCommentResponse `json:"comments" validate:"required"`
}

type IssueListResponse struct {
	// Items is empty when count=true.
	Items []IssueResponse `json:"items" validate:"required"`
	Total int             `json:"total" validate:"required"`
}

func (c *Controller) registerIssues(server *fuego.Server) {
	idPath := fuego.OptionPath(pathIssueID, "Issue ID")
	fuego.Post(server, issuesRoot, c.reportIssue,
		fuego.OptionOperationID("reportIssue"),
		fuego.OptionSummary("Report a problem with a Title or one episode (needs the report-issues toggle)"),
		fuego.OptionTags(tagIssues),
		fuego.OptionDefaultStatusCode(http.StatusCreated),
	)
	fuego.Get(server, issuesRoot, c.listIssues,
		fuego.OptionOperationID("listIssues"),
		fuego.OptionSummary("Issues the viewer may see, newest first: their own unless they may view others' requests"),
		fuego.OptionTags(tagIssues),
		fuego.OptionQueryArray(queryStatus, "Only these statuses", reflect.String),
		fuego.OptionQuery(queryUser, "Only this reporter's issues"),
		fuego.OptionQuery(queryTitle, "Only issues on this Title"),
		fuego.OptionQueryBool(queryCount, "Return only the total, without items"),
	)
	fuego.Get(server, issuesRoot+"/{id}", c.issueThread,
		fuego.OptionOperationID("getIssue"),
		fuego.OptionSummary("One issue with its comment thread"),
		fuego.OptionTags(tagIssues),
		idPath,
	)
	fuego.Post(server, issuesRoot+"/{id}/comments", c.commentIssue,
		fuego.OptionOperationID("commentIssue"),
		fuego.OptionSummary("Reply on an issue (its reporter or an admin)"),
		fuego.OptionTags(tagIssues),
		fuego.OptionDefaultStatusCode(http.StatusCreated),
		idPath,
	)
	fuego.Post(server, issuesRoot+"/{id}/actions", c.issueAction,
		fuego.OptionOperationID("requestIssueAction"),
		fuego.OptionSummary("Fix an open issue: blocklist and re-grab, or re-run subtitles (admin)"),
		fuego.OptionTags(tagIssues),
		idPath,
	)
	fuego.Post(server, issuesRoot+"/{id}/resolve", c.resolveIssue,
		fuego.OptionOperationID("resolveIssue"),
		fuego.OptionSummary("Close an issue by hand (its reporter or an admin)"),
		fuego.OptionTags(tagIssues),
		idPath,
	)
}

func (c *Controller) reportIssue(ctx fuego.ContextWithBody[ReportIssueRequest]) (IssueResponse, error) {
	principal, err := viewer(ctx.Context())
	if err != nil {
		return IssueResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return IssueResponse{}, err
	}
	view, err := c.service.Report(ctx.Context(), principal, NewIssue{Title: body.TitleID, Episode: body.EpisodeID, Kind: body.Kind, Text: body.Text})
	if err != nil {
		return IssueResponse{}, issueProblem(err)
	}
	return issueResponse(view), nil
}

func (c *Controller) listIssues(ctx fuego.ContextNoBody) (IssueListResponse, error) {
	principal, err := viewer(ctx.Context())
	if err != nil {
		return IssueListResponse{}, err
	}
	q, err := issueQuery(ctx)
	if err != nil {
		return IssueListResponse{}, err
	}
	result, err := c.service.Issues(ctx.Context(), principal, q)
	if err != nil {
		return IssueListResponse{}, issueProblem(err)
	}
	response := IssueListResponse{Items: make([]IssueResponse, 0, len(result.Items)), Total: result.Total}
	for _, view := range result.Items {
		response.Items = append(response.Items, issueResponse(view))
	}
	return response, nil
}

func issueQuery(ctx fuego.ContextNoBody) (IssueQuery, error) {
	q := IssueQuery{}
	for _, status := range ctx.QueryParamArr(queryStatus) {
		if !slices.Contains(AllIssueStatuses, status) {
			return q, badRequest("unknown status " + status)
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

func (c *Controller) issueThread(ctx fuego.ContextNoBody) (IssueThreadResponse, error) {
	principal, err := viewer(ctx.Context())
	if err != nil {
		return IssueThreadResponse{}, err
	}
	view, comments, err := c.service.IssueThread(ctx.Context(), principal, domain.IssueID(ctx.PathParam(pathIssueID)))
	if err != nil {
		return IssueThreadResponse{}, issueProblem(err)
	}
	response := IssueThreadResponse{Issue: issueResponse(view), Comments: make([]IssueCommentResponse, 0, len(comments))}
	for _, comment := range comments {
		response.Comments = append(response.Comments, commentResponse(comment))
	}
	return response, nil
}

func (c *Controller) commentIssue(ctx fuego.ContextWithBody[CommentIssueRequest]) (IssueCommentResponse, error) {
	principal, err := viewer(ctx.Context())
	if err != nil {
		return IssueCommentResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return IssueCommentResponse{}, err
	}
	comment, err := c.service.Comment(ctx.Context(), principal, domain.IssueID(ctx.PathParam(pathIssueID)), body.Body)
	if err != nil {
		return IssueCommentResponse{}, issueProblem(err)
	}
	return commentResponse(comment), nil
}

func (c *Controller) issueAction(ctx fuego.ContextWithBody[IssueActionRequest]) (IssueResponse, error) {
	if _, err := admin(ctx.Context()); err != nil {
		return IssueResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return IssueResponse{}, err
	}
	view, err := c.service.RequestAction(ctx.Context(), domain.IssueID(ctx.PathParam(pathIssueID)), body.Action, body.WaitingRemoved)
	if err != nil {
		return IssueResponse{}, issueProblem(err)
	}
	return issueResponse(view), nil
}

func (c *Controller) resolveIssue(ctx fuego.ContextNoBody) (IssueResponse, error) {
	principal, err := viewer(ctx.Context())
	if err != nil {
		return IssueResponse{}, err
	}
	view, err := c.service.Resolve(ctx.Context(), principal, domain.IssueID(ctx.PathParam(pathIssueID)))
	if err != nil {
		return IssueResponse{}, issueProblem(err)
	}
	return issueResponse(view), nil
}

func issueResponse(v IssueView) IssueResponse {
	response := IssueResponse{
		ID:         v.ID,
		Reporter:   RequesterResponse{ID: v.User, DisplayName: v.ReporterName},
		TitleID:    v.Title,
		TitleName:  v.TitleName,
		EpisodeID:  v.Episode,
		Kind:       v.Kind,
		Text:       v.Text,
		Status:     v.Status,
		Action:     v.PendingAction,
		Comments:   v.Comments,
		CreatedAt:  v.CreatedAt,
		ResolvedAt: v.ResolvedAt,
		ResolvedBy: v.ResolvedBy,
	}
	if e := v.EpisodeInfo; e != nil {
		response.Episode = &IssueEpisodeResponse{ID: e.ID, Season: e.Season, Number: e.Number, Name: e.Name}
	}
	return response
}

func commentResponse(c CommentView) IssueCommentResponse {
	return IssueCommentResponse{ID: c.ID, Author: RequesterResponse{ID: c.Author, DisplayName: c.AuthorName}, Body: c.Body, CreatedAt: c.CreatedAt}
}

// issueProblem maps issue errors to problems. A waiting download is named in detail, and instance is its
// import in Library's queue.
func issueProblem(err error) error {
	var waiting ImportWaitingError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &waiting):
		conflict := problem.New(TypeImportWaiting, http.StatusConflict, waiting.ReleaseName+" is waiting for its files to be mapped")
		conflict.Instance = importsRoute + string(waiting.Download)
		return conflict
	case errors.Is(err, api.ErrNotFound):
		return problem.New(TypeIssueNotFound, http.StatusNotFound, "no such issue")
	case errors.Is(err, ErrIssueStatus):
		return problem.New(TypeIssueStatus, http.StatusConflict, err.Error())
	case errors.Is(err, ErrCannotReport):
		return problem.New(problem.TypeForbidden, http.StatusForbidden, "reporting issues is turned off for you")
	case errors.Is(err, ErrNotReporter):
		return problem.New(problem.TypeForbidden, http.StatusForbidden, "only the reporter or an admin can do this")
	case errors.Is(err, ErrHiddenReporter):
		return problem.New(problem.TypeForbidden, http.StatusForbidden, "you can only see your own issues")
	case errors.Is(err, ErrInvalidIssue), errors.Is(err, ErrUnknownAction):
		return badRequest(err.Error())
	default:
		return err
	}
}
