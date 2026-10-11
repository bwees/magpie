package httpserver

import (
	"github.com/go-fuego/fuego"
	"go.uber.org/fx"
)

const controllersGroup = `group:"controllers"`

// Controller registers its routes under the /api group.
type Controller interface {
	Register(api *fuego.Server)
}

// AsController provides constructor's result as a Controller the server registers at startup.
func AsController(constructor any) any {
	return fx.Annotate(constructor, fx.As(new(Controller)), fx.ResultTags(controllersGroup))
}

type controllers struct {
	fx.In
	All []Controller `group:"controllers"`
}
