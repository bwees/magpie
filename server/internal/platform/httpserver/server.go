// Package httpserver runs the fuego server that hosts every context's routes, the OpenAPI spec and the SPA.
package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"magpie/internal/platform/config"
	"magpie/internal/platform/problem"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-fuego/fuego"
	"go.uber.org/fx"
)

const (
	APIPrefix      = "/api"
	SpecURL        = APIPrefix + "/openapi.json"
	SpecFile       = "openapi.json"
	apiTitle       = "Magpie"
	apiVersion     = "1.0.0"
	apiDescription = "HTTP API of the Magpie server. Errors are RFC 9457 problem JSON."
	jsonContent    = "application/json"
	omitEmpty      = "omitempty"
	jsonTag        = "json"

	listenNetwork  = "tcp"
	listenAllHosts = ""
)

// Mode picks between serving HTTP and writing the OpenAPI spec to SpecFile and exiting.
type Mode string

const (
	ModeServe      Mode = "SERVE"
	ModeExportSpec Mode = "EXPORT_SPEC"
)

var Module = fx.Module("httpserver",
	fx.Provide(
		New,
		AsController(NewHealthController),
	),
	fx.Invoke(func(*fuego.Server) {}),
)

type Params struct {
	fx.In

	Lifecycle   fx.Lifecycle
	Shutdowner  fx.Shutdowner
	Env         config.Env
	Mode        Mode
	Controllers controllers
	Middlewares []Middleware `group:"http_middlewares"`
	Schemas     []Schemas    `group:"openapi_schemas"`
}

func New(p Params) *fuego.Server {
	server := Build(p.Env, p.Mode, p.Controllers.All, p.Middlewares...)
	for _, schemas := range p.Schemas {
		RegisterSchemas(server, schemas...)
	}

	p.Lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if p.Mode == ModeExportSpec {
				server.OutputOpenAPISpec()
				return p.Shutdowner.Shutdown()
			}
			return start(server, p.Env.Port)
		},
		OnStop: func(ctx context.Context) error {
			return server.Shutdown(ctx)
		},
	})
	return server
}

// Build creates the server with every controller's routes registered under /api. Middlewares wrap every
// route, the API not-found handler and the SPA, in the order given.
func Build(env config.Env, mode Mode, all []Controller, middlewares ...Middleware) *fuego.Server {
	server := fuego.NewServer(
		fuego.WithLoggingMiddleware(fuego.LoggingConfig{DisableRequest: true, DisableResponse: true}),
		fuego.WithSerializer(fuego.SendJSON),
		fuego.WithErrorSerializer(problem.Send),
		fuego.WithoutStartupMessages(),
		fuego.WithEngineOptions(
			fuego.WithRequestContentType(jsonContent),
			fuego.WithResponseContentType(jsonContent),
			fuego.WithErrorHandler(problem.Handle),
			fuego.WithOpenAPIConfig(fuego.OpenAPIConfig{
				JSONFilePath:         SpecFile,
				SpecURL:              SpecURL,
				DisableSwaggerUI:     true,
				DisableDefaultServer: true,
				DisableMessages:      true,
				DisableLocalSave:     mode != ModeExportSpec,
				PrettyFormatJSON:     true,
				Info:                 &openapi3.Info{Title: apiTitle, Version: apiVersion, Description: apiDescription},
			}),
		),
	)

	server.OpenAPI.SetGeneratorSchemaCustomizer(func(_ string, _ reflect.Type, tag reflect.StructTag, schema *openapi3.Schema) error {
		if strings.Contains(tag.Get(jsonTag), omitEmpty) {
			schema.Nullable = false
		}
		return nil
	})

	for _, middleware := range middlewares {
		fuego.Use(server, middleware)
	}

	api := fuego.Group(server, APIPrefix)
	for _, controller := range all {
		controller.Register(api)
	}

	registerAPINotFound(server)
	registerStaticWeb(server, env.WebDir)
	return server
}

// Binding before returning makes a port clash fail startup instead of only logging.
func start(server *fuego.Server, port int) error {
	listener, err := net.Listen(listenNetwork, net.JoinHostPort(listenAllHosts, strconv.Itoa(port)))
	if err != nil {
		return fmt.Errorf("listening on port %d: %w", port, err)
	}
	fuego.WithListener(listener)(server)

	go func() {
		if err := server.Run(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server stopped", "error", err)
		}
	}()
	slog.Info("magpie listening", "port", port)
	return nil
}
