package httpserver

import (
	"github.com/go-fuego/fuego"
)

type HealthStatus string

const HealthStatusOK HealthStatus = "OK"

type HealthResponse struct {
	Status HealthStatus `json:"status" validate:"required"`
}

type HealthController struct{}

func NewHealthController() *HealthController {
	return &HealthController{}
}

func (c *HealthController) Register(api *fuego.Server) {
	fuego.Get(api, "/health", func(fuego.ContextNoBody) (HealthResponse, error) {
		return HealthResponse{Status: HealthStatusOK}, nil
	},
		fuego.OptionOperationID("getHealth"),
		fuego.OptionSummary("Report that the server is up"),
		fuego.OptionTags("platform"),
	)
}
