// Package restart stops the app gracefully and starts the same binary again in its place.
package restart

import (
	"sync/atomic"

	"go.uber.org/fx"
)

type Restarter struct {
	shutdowner fx.Shutdowner
	requested  atomic.Bool
}

var Module = fx.Module("restart", fx.Provide(New))

func New(shutdowner fx.Shutdowner) *Restarter {
	return &Restarter{shutdowner: shutdowner}
}

// Restart stops the app; main then re-executes the binary once every stop hook has run.
func (r *Restarter) Restart() error {
	r.requested.Store(true)
	return r.shutdowner.Shutdown()
}

func (r *Restarter) Requested() bool {
	return r != nil && r.requested.Load()
}
