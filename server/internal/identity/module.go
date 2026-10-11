package identity

import (
	"magpie/internal/identity/api"
	"magpie/internal/platform/db"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/owner"

	"go.uber.org/fx"
)

var Module = fx.Module(string(owner.Identity),
	db.ProvideMigrations(owner.Identity, Migrations().FS),
	fx.Provide(
		fx.Annotate(NewRepository, fx.As(fx.Self()), fx.As(new(api.Reader))),
		fx.Annotate(NewService, fx.ParamTags("", "", "", "", "", httpserver.SelfAuthenticatedTag)),
		httpserver.AsController(NewController),
		httpserver.AsMiddleware(func(s *Service) httpserver.Middleware { return s.Middleware }),
	),
)
