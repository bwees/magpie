package transcode

import (
	"errors"
	"net/http"

	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/problem"
	"magpie/internal/transcode/api"

	"github.com/go-fuego/fuego"
)

const (
	pathScope = "scope"
	pathRef   = "ref"

	exclusionRoute = "/transcode/exclusions/{" + pathScope + "}/{" + pathRef + "}"
)

type BacklogResponse struct {
	// Backlog is the queued jobs of files found by a library scan or a migration.
	Backlog int `json:"backlog" validate:"required"`
	// Eligible is every file that met its cutoff and isn't transcoded yet.
	Eligible int `json:"eligible" validate:"required"`
	// Skipped counts eligible files kept as they are, per reason; reasons without files are absent.
	Skipped map[api.SkipReason]int `json:"skipped" validate:"required"`
}

type ExclusionResponse struct {
	Scope ExclusionScope `json:"scope" validate:"required"` // TITLE | ROOT_FOLDER
	Ref   string         `json:"ref" validate:"required"`   // a title ID or a root folder path
}

func (c *Controller) registerBacklog(s *fuego.Server) {
	fuego.Get(s, "/transcode/backlog", c.backlog,
		fuego.OptionOperationID("getTranscodeBacklog"),
		fuego.OptionSummary("Queued backlog jobs, eligible files and skips per reason (admin)"),
		fuego.OptionTags(tagTranscode),
	)
	fuego.Post(s, "/transcode/backlog/sweep", c.sweep,
		fuego.OptionOperationID("sweepTranscodeBacklog"),
		fuego.OptionSummary("Re-evaluate every eligible and skipped file against the current policy, in the background (admin)"),
		fuego.OptionTags(tagTranscode),
		fuego.OptionDefaultStatusCode(http.StatusAccepted),
	)
	fuego.Get(s, "/transcode/exclusions", c.listExclusions,
		fuego.OptionOperationID("listTranscodeExclusions"),
		fuego.OptionSummary("Titles and root folders kept out of transcoding (admin)"),
		fuego.OptionTags(tagTranscode),
	)
	fuego.Put(s, exclusionRoute, c.exclude,
		fuego.OptionOperationID("excludeFromTranscoding"),
		fuego.OptionSummary("Keep a title or root folder out of transcoding; its queued jobs are cancelled (admin)"),
		fuego.OptionTags(tagTranscode),
		fuego.OptionPath(pathScope, "TITLE or ROOT_FOLDER"),
		fuego.OptionPath(pathRef, "Title ID, or the root folder path URL-encoded"),
	)
	fuego.Delete(s, exclusionRoute, c.include,
		fuego.OptionOperationID("includeInTranscoding"),
		fuego.OptionSummary("End an exclusion and evaluate its files again (admin)"),
		fuego.OptionTags(tagTranscode),
		fuego.OptionPath(pathScope, "TITLE or ROOT_FOLDER"),
		fuego.OptionPath(pathRef, "Title ID, or the root folder path URL-encoded"),
		fuego.OptionDefaultStatusCode(http.StatusNoContent),
	)
}

func (c *Controller) backlog(ctx fuego.ContextNoBody) (BacklogResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return BacklogResponse{}, err
	}
	summary, err := c.jobs.Backlog(ctx.Context())
	return BacklogResponse(summary), err
}

func (c *Controller) sweep(ctx fuego.ContextNoBody) (BacklogResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return BacklogResponse{}, err
	}
	if err := c.jobs.RequestSweep(ctx.Context()); err != nil {
		return BacklogResponse{}, err
	}
	summary, err := c.jobs.Backlog(ctx.Context())
	return BacklogResponse(summary), err
}

func (c *Controller) listExclusions(ctx fuego.ContextNoBody) ([]ExclusionResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return nil, err
	}
	exclusions, err := c.jobs.Exclusions(ctx.Context())
	response := make([]ExclusionResponse, 0, len(exclusions))
	for _, e := range exclusions {
		response = append(response, ExclusionResponse(e))
	}
	return response, err
}

func (c *Controller) exclude(ctx fuego.ContextNoBody) (ExclusionResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return ExclusionResponse{}, err
	}
	e := exclusionOf(ctx)
	if err := exclusionProblem(c.jobs.Exclude(ctx.Context(), e)); err != nil {
		return ExclusionResponse{}, err
	}
	row, _ := e.row()
	return ExclusionResponse(row), nil
}

func (c *Controller) include(ctx fuego.ContextNoBody) (any, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return nil, err
	}
	return nil, exclusionProblem(c.jobs.Include(ctx.Context(), exclusionOf(ctx)))
}

func exclusionOf(ctx fuego.ContextNoBody) Exclusion {
	return Exclusion{Scope: ExclusionScope(ctx.PathParam(pathScope)), Ref: ctx.PathParam(pathRef)}
}

func exclusionProblem(err error) error {
	if errors.Is(err, ErrUnknownScope) {
		return problem.New(problem.TypeBadRequest, http.StatusBadRequest, "scope must be TITLE or ROOT_FOLDER")
	}
	return err
}
