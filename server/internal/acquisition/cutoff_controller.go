package acquisition

import (
	"net/http"

	"magpie/internal/domain"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/problem"

	"github.com/go-fuego/fuego"
)

const (
	queryTitle = "title"

	TypeTitleRequired problem.Type = "urn:magpie:problem:title-required"
)

// CutoffStatusResponse says whether a library file is good enough yet.
type CutoffStatusResponse struct {
	File    domain.MediaFileID `json:"file"`
	Quality domain.Quality     `json:"quality"`
	CFScore int                `json:"cfScore"`
	// CutoffMet is quality at or above the profile's cutoff and CF score at or above its upgrade-until.
	CutoffMet bool `json:"cutoffMet"`
	// UpgradesAllowed is false once the file is transcoded or when its profile doesn't upgrade.
	UpgradesAllowed bool `json:"upgradesAllowed"`
}

// CutoffController serves the cutoff state of a title's files for the title page.
type CutoffController struct {
	cutoff *CutoffService
}

func NewCutoffController(s *CutoffService) *CutoffController {
	return &CutoffController{cutoff: s}
}

func (c *CutoffController) Register(s *fuego.Server) {
	fuego.Get(s, "/acquisition/cutoff", c.list, fuego.OptionTags(tagAcquisition),
		fuego.OptionOperationID("listCutoffStatus"),
		fuego.OptionSummary("Whether each of a title's files meets its profile's cutoff and may still be upgraded (admin)"),
		fuego.OptionQuery(queryTitle, "Title ID", fuego.ParamRequired()))
}

func (c *CutoffController) list(ctx fuego.ContextNoBody) ([]CutoffStatusResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return nil, err
	}
	title := domain.TitleID(ctx.QueryParam(queryTitle))
	if title == "" {
		return nil, problem.New(TypeTitleRequired, http.StatusBadRequest, "Pass the title")
	}
	statuses, err := c.cutoff.CutoffStatuses(ctx.Context(), title)
	if err != nil {
		return nil, err
	}
	out := make([]CutoffStatusResponse, 0, len(statuses))
	for _, s := range statuses {
		out = append(out, CutoffStatusResponse{
			File: s.File, Quality: s.Quality, CFScore: s.CFScore, CutoffMet: s.CutoffMet, UpgradesAllowed: s.UpgradesAllowed,
		})
	}
	return out, nil
}
