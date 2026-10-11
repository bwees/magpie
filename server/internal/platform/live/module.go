package live

import (
	"magpie/internal/platform/httpserver"

	"github.com/go-fuego/fuego"
	"go.uber.org/fx"
)

const Path = "/live"

var Module = fx.Module("live",
	fx.Provide(
		NewHub,
		httpserver.AsController(NewController),
	),
)

type Controller struct {
	hub *Hub
}

func NewController(h *Hub) *Controller {
	return &Controller{hub: h}
}

// Register mounts the socket at /api/live. It is a WebSocket, so it stays out of the OpenAPI spec.
func (c *Controller) Register(api *fuego.Server) {
	fuego.Handle(api, Path, c.hub, fuego.OptionHide())
}
