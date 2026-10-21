package access

import (
	"net/http"

	"magpie/internal/domain"
	"magpie/internal/platform/problem"

	"github.com/go-fuego/fuego"
)

const (
	tagAccess   = "access"
	pathTitleID = "id"
)

type TitleAccessResponse struct {
	ManageFiles bool `json:"manageFiles"`
}

type Controller struct {
	gate *FileGate
}

func NewController(g *FileGate) *Controller {
	return &Controller{gate: g}
}

func (c *Controller) Register(s *fuego.Server) {
	fuego.Get(s, "/access/titles/{id}", c.title, fuego.OptionTags(tagAccess),
		fuego.OptionPath(pathTitleID, "Title ID"),
		fuego.OptionOperationID("getTitleAccess"),
		fuego.OptionSummary("What the signed-in user may do with a title's media files"))
}

func (c *Controller) title(ctx fuego.ContextNoBody) (TitleAccessResponse, error) {
	principal, ok := domain.PrincipalFrom(ctx.Context())
	if !ok {
		return TitleAccessResponse{}, problem.New(problem.TypeUnauthorized, http.StatusUnauthorized, "sign in first")
	}
	manage, err := c.gate.Allows(ctx.Context(), principal, domain.TitleID(ctx.PathParam(pathTitleID)))
	return TitleAccessResponse{ManageFiles: manage}, err
}
