package acquisition

import (
	"magpie/internal/platform/owner"

	"go.uber.org/fx"
)

var Module = fx.Module(string(owner.Acquisition))
