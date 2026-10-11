package httpserver

import (
	"net/http"
	"slices"
	"strings"

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

const selfAuthenticatedGroup = "self_authenticated_paths"

// SelfAuthenticatedTag injects every path declared with AsSelfAuthenticated.
const SelfAuthenticatedTag = `group:"` + selfAuthenticatedGroup + `"`

// SelfAuthenticated is a path whose routes check their own credentials, such as the transcode agent
// socket. The session gate lets it, and everything under it, through untouched.
type SelfAuthenticated string

// AsSelfAuthenticated declares that path and everything under it check their own credentials.
func AsSelfAuthenticated(path string) fx.Option {
	return fx.Supply(fx.Annotated{Group: selfAuthenticatedGroup, Target: SelfAuthenticated(path)})
}

// Covers reports whether path is p or under it.
func (p SelfAuthenticated) Covers(path string) bool {
	return path == string(p) || strings.HasPrefix(path, string(p)+"/")
}

// AnyCovers reports whether any of paths covers path.
func AnyCovers(paths []SelfAuthenticated, path string) bool {
	return slices.ContainsFunc(paths, func(p SelfAuthenticated) bool { return p.Covers(path) })
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
