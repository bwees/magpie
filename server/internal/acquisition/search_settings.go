package acquisition

import (
	"context"
	"errors"
	"fmt"
	"time"

	"magpie/internal/platform/owner"
	"magpie/internal/platform/settings"
)

const day = 24 * time.Hour

// Defaults and ranges from 12-settings › Schedules.
const (
	DefaultRSSInterval = 15 * time.Minute
	MinRSSInterval     = 10 * time.Minute
	MaxRSSInterval     = 120 * time.Minute

	DefaultMissingSweepInterval = day
	MinMissingSweepInterval     = day
	MaxMissingSweepInterval     = 7 * day

	DefaultCutoffSweepInterval = 7 * day
	MinCutoffSweepInterval     = day
	MaxCutoffSweepInterval     = 30 * day

	// PendingCheckInterval is how often held releases are looked at. It isn't a setting.
	PendingCheckInterval = time.Minute

	// IndexerQueryGap is the least time between two backlog queries to one indexer.
	IndexerQueryGap = 2 * time.Second
	// MinSweepGap spaces backlog searches: a search can send an indexer an ID query and then a title query.
	MinSweepGap   = maxQueryTiers * IndexerQueryGap
	maxQueryTiers = 2
)

var ErrInvalidSchedules = errors.New("acquisition: invalid schedules")

// Schedules are the Settings → General → Schedules rows Acquisition owns.
type Schedules struct {
	RSSInterval          settings.Duration `json:"rssInterval"`
	MissingSweepInterval settings.Duration `json:"missingSweepInterval"`
	CutoffSweepInterval  settings.Duration `json:"cutoffSweepInterval"`
}

var SchedulesKey = settings.NewKey(owner.Acquisition, "schedules", Schedules{
	RSSInterval:          settings.Duration(DefaultRSSInterval),
	MissingSweepInterval: settings.Duration(DefaultMissingSweepInterval),
	CutoffSweepInterval:  settings.Duration(DefaultCutoffSweepInterval),
})

// SearchOnAddKey is Settings → Downloads "Search when a title is added".
var SearchOnAddKey = settings.NewKey(owner.Acquisition, "searchOnAdd", true)

func (s *SearchService) Schedules(ctx context.Context) (Schedules, error) {
	return SchedulesKey.Get(ctx, s.deps.Store)
}

func (s *SearchService) SetSchedules(ctx context.Context, schedules Schedules) error {
	if err := schedules.Validate(); err != nil {
		return err
	}
	return SchedulesKey.Set(ctx, s.deps.Store, schedules)
}

func (s *SearchService) SearchOnAdd(ctx context.Context) (bool, error) {
	return SearchOnAddKey.Get(ctx, s.deps.Store)
}

func (s *SearchService) SetSearchOnAdd(ctx context.Context, on bool) error {
	return SearchOnAddKey.Set(ctx, s.deps.Store, on)
}

func (s Schedules) Validate() error {
	var errs []error
	if !within(s.RSSInterval.Std(), MinRSSInterval, MaxRSSInterval) {
		errs = append(errs, fmt.Errorf("RSS sync must be %s-%s", MinRSSInterval, MaxRSSInterval))
	}
	if !within(s.MissingSweepInterval.Std(), MinMissingSweepInterval, MaxMissingSweepInterval) {
		errs = append(errs, fmt.Errorf("the missing sweep must run every %s-%s", MinMissingSweepInterval, MaxMissingSweepInterval))
	}
	if !within(s.CutoffSweepInterval.Std(), MinCutoffSweepInterval, MaxCutoffSweepInterval) {
		errs = append(errs, fmt.Errorf("the cutoff-unmet sweep must run every %s-%s", MinCutoffSweepInterval, MaxCutoffSweepInterval))
	}
	if len(errs) > 0 {
		return fmt.Errorf("%w: %w", ErrInvalidSchedules, errors.Join(errs...))
	}
	return nil
}
