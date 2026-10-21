package access

import (
	"magpie/internal/platform/httpserver"

	"go.uber.org/fx"
)

var Module = fx.Module("access",
	fx.Provide(
		NewFileGate,
		httpserver.AsController(NewController),
	),
)
