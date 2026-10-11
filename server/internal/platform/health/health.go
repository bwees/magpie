// Package health probes each external connection and publishes platform.ServiceHealthChanged when one flips.
package health

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sync"
	"time"

	platformapi "magpie/internal/platform/api"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/db"
	"magpie/internal/platform/owner"
	"magpie/internal/platform/settings"

	"go.uber.org/fx"
	"gorm.io/gorm"
)

type Status string

const (
	StatusUnknown       Status = "UNKNOWN"
	StatusHealthy       Status = "HEALTHY"
	StatusUnhealthy     Status = "UNHEALTHY"
	StatusNotConfigured Status = "NOT_CONFIGURED"
)

const (
	probeTimeout = 10 * time.Second
	probesGroup  = `group:"health_probes"`
)

var (
	// ErrNotConfigured is what a probe returns when its connection has no URL or credentials yet.
	ErrNotConfigured  = errors.New("health: connection is not configured")
	ErrUnknownService = errors.New("health: no probe for that service")
)

// Services lists every connection in the order the UI shows them.
var Services = []platformapi.Service{
	platformapi.ServiceTMDB,
	platformapi.ServiceQBittorrent,
	platformapi.ServiceProwlarr,
	platformapi.ServiceJellyfin,
	platformapi.ServiceOpenSubtitles,
}

// Result is what a successful probe found, for the Test toast.
type Result struct {
	Version string
	Detail  string
}

// Probe is a minimal health method on one adapter. It reads its connection config itself.
type Probe interface {
	Service() platformapi.Service
	Probe(ctx context.Context) (Result, error)
}

// AsProbe provides constructor's Probe to the checker.
func AsProbe(constructor any) any {
	return fx.Annotate(constructor, fx.As(new(Probe)), fx.ResultTags(probesGroup))
}

// Check is the outcome of one probe. CheckedAt is zero until the service is probed in this process.
type Check struct {
	Service   platformapi.Service
	Status    Status
	Version   string
	Detail    string
	Error     string
	CheckedAt time.Time
}

// statusKey keeps each service's last status, so a flip is detected across restarts.
var statusKey = settings.NewKey(owner.Platform, "health", map[platformapi.Service]Status{})

type Checker struct {
	db     *db.DB
	bus    *bus.Bus
	store  *settings.Store
	probes map[platformapi.Service]Probe
	now    func() time.Time

	mu     sync.Mutex
	latest map[platformapi.Service]Check
}

type CheckerParams struct {
	fx.In

	DB     *db.DB
	Bus    *bus.Bus
	Store  *settings.Store
	Probes []Probe `group:"health_probes"`
}

func NewChecker(p CheckerParams) *Checker {
	probes := make(map[platformapi.Service]Probe, len(p.Probes))
	for _, probe := range p.Probes {
		probes[probe.Service()] = probe
	}
	return &Checker{db: p.DB, bus: p.Bus, store: p.Store, probes: probes, now: time.Now, latest: map[platformapi.Service]Check{}}
}

// Check probes one service, records its status and publishes ServiceHealthChanged if it flipped.
func (c *Checker) Check(ctx context.Context, service platformapi.Service) (Check, error) {
	probe, ok := c.probes[service]
	if !ok {
		return Check{}, fmt.Errorf("%w: %s", ErrUnknownService, service)
	}
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	result, err := probe.Probe(probeCtx)

	check := Check{Service: service, CheckedAt: c.now()}
	switch {
	case errors.Is(err, ErrNotConfigured):
		check.Status = StatusNotConfigured
	case err != nil:
		check.Status = StatusUnhealthy
		check.Error = err.Error()
	default:
		check.Status = StatusHealthy
		check.Version = result.Version
		check.Detail = result.Detail
	}

	if err := c.record(ctx, check); err != nil {
		return Check{}, err
	}
	c.mu.Lock()
	c.latest[service] = check
	c.mu.Unlock()
	return check, nil
}

// CheckAll probes every service at once.
func (c *Checker) CheckAll(ctx context.Context) error {
	var wg sync.WaitGroup
	errs := make([]error, len(Services))
	for i, service := range Services {
		if _, ok := c.probes[service]; !ok {
			continue
		}
		wg.Go(func() {
			_, errs[i] = c.Check(ctx, service)
		})
	}
	wg.Wait()
	return errors.Join(errs...)
}

// Latest returns every service's most recent check, falling back to its stored status before the first probe.
func (c *Checker) Latest(ctx context.Context) ([]Check, error) {
	stored, err := statusKey.Get(ctx, c.store)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	checks := make([]Check, 0, len(Services))
	for _, service := range Services {
		check, ok := c.latest[service]
		if !ok {
			check = Check{Service: service, Status: StatusUnknown}
			if status, found := stored[service]; found {
				check.Status = status
			}
		}
		checks = append(checks, check)
	}
	return checks, nil
}

func (c *Checker) record(ctx context.Context, check Check) error {
	stored, err := statusKey.Get(ctx, c.store)
	if err != nil {
		return err
	}
	if stored[check.Service] == check.Status {
		return nil
	}
	return c.db.Write(ctx, func(tx *gorm.DB) error {
		stored, err := statusKey.GetTx(tx)
		if err != nil {
			return err
		}
		previous := stored[check.Service]
		if previous == check.Status {
			return nil
		}
		next := maps.Clone(stored)
		next[check.Service] = check.Status
		if err := statusKey.SetTx(tx, next); err != nil {
			return err
		}
		if changed, flipped := flip(previous, check); flipped {
			return c.bus.Publish(tx, bus.EventOf(changed))
		}
		return nil
	})
}

// flip treats a service as healthy until a probe says otherwise: the first failure publishes, the first
// success after a failure publishes, and an unconfigured service never does.
func flip(previous Status, check Check) (platformapi.ServiceHealthChanged, bool) {
	changed := platformapi.ServiceHealthChanged{Service: check.Service, Healthy: check.Status == StatusHealthy, Error: check.Error}
	switch check.Status {
	case StatusUnhealthy:
		return changed, previous != StatusUnhealthy
	case StatusHealthy:
		return changed, previous == StatusUnhealthy
	default:
		return changed, false
	}
}
