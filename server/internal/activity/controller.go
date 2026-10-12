package activity

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"slices"
	"time"

	"magpie/internal/activity/api"
	"magpie/internal/domain"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/problem"

	"github.com/go-fuego/fuego"
)

const (
	tagActivity = "activity"

	queryTitle    = "title"
	queryTitles   = "titles"
	queryActive   = "active"
	queryBefore   = "before"
	queryLimit    = "limit"
	querySeverity = "severity"

	queryRecent = "recent"

	// maxStageTitles bounds one library-grid stage lookup.
	maxStageTitles = 500
	// recentTitles fills Home's "Recently active" grid.
	recentTitles = 24
)

type TimelineEntryResponse struct {
	EventID  string    `json:"eventId" validate:"required"`
	Type     string    `json:"type" validate:"required"`
	Severity string    `json:"severity" validate:"required"`
	Episodes []string  `json:"episodes" validate:"required"`
	Text     string    `json:"text" validate:"required"`
	Detail   string    `json:"detail" validate:"required"`
	At       time.Time `json:"at" validate:"required"`
}

type HistoryEntryResponse struct {
	TimelineEntryResponse
	TitleID   *string `json:"titleId"`
	TitleName string  `json:"titleName" validate:"required"`
}

type SubsChipResponse struct {
	State     string   `json:"state" validate:"required"`
	Satisfied []string `json:"satisfied" validate:"required"`
	Missing   []string `json:"missing" validate:"required"`
}

type TranscodeChipResponse struct {
	State        string  `json:"state" validate:"required"`
	SavedPercent *int    `json:"savedPercent"`
	Reason       string  `json:"reason" validate:"required"`
	JobID        *string `json:"jobId"`
}

type PipelineRowResponse struct {
	TitleID    string                 `json:"titleId" validate:"required"`
	EpisodeID  *string                `json:"episodeId"`
	Season     int                    `json:"season" validate:"required"`
	Number     int                    `json:"number" validate:"required"`
	Stage      string                 `json:"stage" validate:"required"`
	Failed     bool                   `json:"failed" validate:"required"`
	Detail     string                 `json:"detail" validate:"required"`
	DownloadID *string                `json:"downloadId"`
	Subs       *SubsChipResponse      `json:"subs,omitempty"`
	Transcode  *TranscodeChipResponse `json:"transcode,omitempty"`
	UpdatedAt  time.Time              `json:"updatedAt" validate:"required"`
}

type AttentionItemResponse struct {
	ID       string    `json:"id" validate:"required"`
	Kind     string    `json:"kind" validate:"required"`
	Ref      string    `json:"ref" validate:"required"`
	TitleID  *string   `json:"titleId"`
	Text     string    `json:"text" validate:"required"`
	Action   string    `json:"action" validate:"required"`
	OpenedAt time.Time `json:"openedAt" validate:"required"`
}

type Controller struct {
	service *Service
}

func NewController(s *Service) *Controller {
	return &Controller{service: s}
}

func (c *Controller) Register(server *fuego.Server) {
	pageOptions := []func(*fuego.BaseRoute){
		fuego.OptionQuery(queryBefore, "Only entries strictly older than this RFC 3339 time"),
		fuego.OptionQueryInt(queryLimit, "At most this many entries (default 50, max 200)"),
	}
	fuego.Get(server, "/activity/timeline", c.timeline, append([]func(*fuego.BaseRoute){
		fuego.OptionOperationID("getTimeline"),
		fuego.OptionSummary("A title's timeline, newest first"),
		fuego.OptionTags(tagActivity),
		fuego.OptionQuery(queryTitle, "Title ID", fuego.ParamRequired()),
	}, pageOptions...)...)
	fuego.Get(server, "/activity/pipeline", c.pipeline,
		fuego.OptionOperationID("getPipeline"),
		fuego.OptionSummary("Pipeline rows: one title's rows, each listed title's summary row, the active rows, or the recently active titles' summary rows"),
		fuego.OptionTags(tagActivity),
		fuego.OptionQuery(queryTitle, "One title's rows, movies first then episodes in season/number order"),
		fuego.OptionQueryArray(queryTitles, "The least-advanced row of each title, for library grids", reflect.String),
		fuego.OptionQueryBool(queryActive, "Rows searching, downloading, importing or transcoding"),
		fuego.OptionQueryBool(queryRecent, "The summary row of the 24 most recently changed titles, newest first"),
	)
	fuego.Get(server, "/activity/history", c.history, append([]func(*fuego.BaseRoute){
		fuego.OptionOperationID("getHistory"),
		fuego.OptionSummary("The global history, newest first (admin)"),
		fuego.OptionTags(tagActivity),
		fuego.OptionQuery(querySeverity, "Only entries at this severity or above: INFO, WARN or ERROR"),
	}, pageOptions...)...)
	fuego.Get(server, "/activity/attention", c.attention,
		fuego.OptionOperationID("getAttention"),
		fuego.OptionSummary("Open attention items, oldest first (admin)"),
		fuego.OptionTags(tagActivity),
	)
	c.registerNotifications(server)
}

func viewer(ctx context.Context) (domain.Principal, error) {
	principal, ok := domain.PrincipalFrom(ctx)
	if !ok {
		return domain.Principal{}, problem.New(problem.TypeUnauthorized, http.StatusUnauthorized, "sign in first")
	}
	return principal, nil
}

func badRequest(detail string) error {
	return problem.New(problem.TypeBadRequest, http.StatusBadRequest, detail)
}

func hidden(err error) error {
	if errors.Is(err, ErrTitleHidden) {
		return problem.New(problem.TypeNotFound, http.StatusNotFound, "no activity for this title")
	}
	return err
}

func page(c fuego.ContextNoBody) (api.Page, error) {
	var p api.Page
	if raw := c.QueryParam(queryBefore); raw != "" {
		before, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return p, badRequest("before must be an RFC 3339 time")
		}
		p.Before = &before
	}
	if raw := c.QueryParam(queryLimit); raw != "" {
		limit, err := c.QueryParamIntErr(queryLimit)
		if err != nil {
			return p, badRequest("limit must be a number")
		}
		p.Limit = limit
	}
	return p, nil
}

func (c *Controller) timeline(ctx fuego.ContextNoBody) ([]TimelineEntryResponse, error) {
	principal, err := viewer(ctx.Context())
	if err != nil {
		return nil, err
	}
	title := domain.TitleID(ctx.QueryParam(queryTitle))
	if title == "" {
		return nil, badRequest("title is required")
	}
	p, err := page(ctx)
	if err != nil {
		return nil, err
	}
	entries, err := c.service.Timeline(ctx.Context(), principal, title, p)
	if err != nil {
		return nil, hidden(err)
	}
	return mapAll(entries, timelineEntryResponse), nil
}

func (c *Controller) pipeline(ctx fuego.ContextNoBody) ([]PipelineRowResponse, error) {
	principal, err := viewer(ctx.Context())
	if err != nil {
		return nil, err
	}
	title := ctx.QueryParam(queryTitle)
	titles := ctx.QueryParamArr(queryTitles)
	active := ctx.QueryParamBool(queryActive)
	recent := ctx.QueryParamBool(queryRecent)
	chosen := slices.DeleteFunc([]bool{title != "", len(titles) > 0, active, recent}, func(set bool) bool { return !set })
	if len(chosen) != 1 {
		return nil, badRequest("pass exactly one of title, titles, active=true or recent=true")
	}

	var rows []api.PipelineRow
	switch {
	case title != "":
		rows, err = c.service.Pipeline(ctx.Context(), principal, domain.TitleID(title))
	case active:
		rows, err = c.service.ActivePipeline(ctx.Context(), principal)
	case recent:
		rows, err = c.service.RecentStages(ctx.Context(), principal, recentTitles, maxStageTitles)
	default:
		rows, err = c.titleStages(ctx.Context(), principal, titles)
	}
	if err != nil {
		return nil, hidden(err)
	}
	return mapAll(rows, pipelineRowResponse), nil
}

func (c *Controller) titleStages(ctx context.Context, principal domain.Principal, raw []string) ([]api.PipelineRow, error) {
	if len(raw) > maxStageTitles {
		return nil, badRequest("too many titles")
	}
	titles := make([]domain.TitleID, 0, len(raw))
	for _, id := range raw {
		titles = append(titles, domain.TitleID(id))
	}
	stages, err := c.service.TitleStages(ctx, principal, titles)
	if err != nil {
		return nil, err
	}
	rows := make([]api.PipelineRow, 0, len(stages))
	for _, title := range titles {
		if row, ok := stages[title]; ok {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func (c *Controller) history(ctx fuego.ContextNoBody) ([]HistoryEntryResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return nil, err
	}
	severity := ctx.QueryParam(querySeverity)
	if severity != "" && !slices.Contains(severities, severity) {
		return nil, badRequest("severity must be INFO, WARN or ERROR")
	}
	p, err := page(ctx)
	if err != nil {
		return nil, err
	}
	entries, err := c.service.History(ctx.Context(), severity, p)
	if err != nil {
		return nil, err
	}
	return mapAll(entries, historyEntryResponse), nil
}

func (c *Controller) attention(ctx fuego.ContextNoBody) ([]AttentionItemResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return nil, err
	}
	items, err := c.service.Attention(ctx.Context())
	if err != nil {
		return nil, err
	}
	return mapAll(items, attentionItemResponse), nil
}

func mapAll[T, R any](items []T, fn func(T) R) []R {
	out := make([]R, 0, len(items))
	for _, item := range items {
		out = append(out, fn(item))
	}
	return out
}

func stringOf[T ~string](id T) string { return string(id) }

func optional[T ~string](id *T) *string {
	if id == nil {
		return nil
	}
	value := string(*id)
	return &value
}

func timelineEntryResponse(e api.TimelineEntry) TimelineEntryResponse {
	return TimelineEntryResponse{
		EventID: e.EventID, Type: e.Type, Severity: e.Severity, Episodes: mapAll(e.Episodes, stringOf[domain.EpisodeID]),
		Text: e.Text, Detail: e.Detail, At: e.At,
	}
}

func historyEntryResponse(e api.TimelineEntry) HistoryEntryResponse {
	return HistoryEntryResponse{TimelineEntryResponse: timelineEntryResponse(e), TitleID: optional(e.Title), TitleName: e.TitleName}
}

func pipelineRowResponse(r api.PipelineRow) PipelineRowResponse {
	response := PipelineRowResponse{
		TitleID: string(r.Title), EpisodeID: optional(r.Episode), Season: r.Season, Number: r.Number, Stage: string(r.Stage),
		Failed: r.Failed, Detail: r.Detail, DownloadID: optional(r.Download), UpdatedAt: r.Updated,
	}
	if r.Subs != nil {
		response.Subs = &SubsChipResponse{
			State:     r.Subs.State,
			Satisfied: mapAll(r.Subs.Satisfied, stringOf[domain.Language]),
			Missing:   mapAll(r.Subs.Missing, stringOf[domain.Language]),
		}
	}
	if r.Transcode != nil {
		response.Transcode = &TranscodeChipResponse{
			State: r.Transcode.State, SavedPercent: r.Transcode.Saved, Reason: r.Transcode.Reason, JobID: optional(r.Transcode.Job),
		}
	}
	return response
}

func attentionItemResponse(a api.AttentionItem) AttentionItemResponse {
	return AttentionItemResponse{
		ID: a.ID, Kind: a.Kind, Ref: a.Ref, TitleID: optional(a.Title), Text: a.Text, Action: a.Action, OpenedAt: a.OpenedAt,
	}
}
