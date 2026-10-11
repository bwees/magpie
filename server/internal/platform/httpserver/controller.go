package httpserver

import (
	"net/http"

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

const middlewaresGroup = `group:"http_middlewares"`

// Middleware wraps every request the server handles, such as Identity's session middleware.
type Middleware func(http.Handler) http.Handler

// AsMiddleware provides constructor's Middleware to the server.
func AsMiddleware(constructor any) any {
	return fx.Annotate(constructor, fx.ResultTags(middlewaresGroup))
}

const schemasGroupName = "openapi_schemas"

// Schemas are types the OpenAPI spec lists as components without a route, such as live event payloads,
// so the web client gets their TypeScript types.
type Schemas []any

// AsSchemas adds s to the OpenAPI components.
func AsSchemas(s Schemas) fx.Option {
	return fx.Supply(fx.Annotated{Group: schemasGroupName, Target: s})
}

// RegisterSchemas adds each value's type, and the types it contains, to the server's OpenAPI components.
func RegisterSchemas(server *fuego.Server, values ...any) {
	for _, v := range values {
		fuego.SchemaTagFromType(server.OpenAPI, v)
	}
}
