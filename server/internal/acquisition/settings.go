package acquisition

import (
	"errors"
	"fmt"
	"time"

	"magpie/internal/platform/owner"
	"magpie/internal/platform/settings"
)

// Defaults and ranges from 12-settings › Indexers and 01-architecture › Background work.
const (
	DefaultIndexerSyncInterval = time.Hour
	MinIndexerSyncInterval     = 15 * time.Minute
	MaxIndexerSyncInterval     = 24 * time.Hour

	DefaultSearchTimeout = 60 * time.Second
	MinSearchTimeout     = 10 * time.Second
	MaxSearchTimeout     = 120 * time.Second

	DefaultResultCache = 30 * time.Minute
	MaxResultCache     = 120 * time.Minute
)

var ErrInvalidIndexerOptions = errors.New("acquisition: invalid indexer options")

// IndexerOptions apply to every indexer.
type IndexerOptions struct {
	SyncInterval  settings.Duration `json:"syncInterval"`
	SearchTimeout settings.Duration `json:"searchTimeout"` // per indexer
	ResultCache   settings.Duration `json:"resultCache"`   // per (indexer, query); 0 turns the cache off
}

var IndexerOptionsKey = settings.NewKey(owner.Acquisition, "indexers", IndexerOptions{
	SyncInterval:  settings.Duration(DefaultIndexerSyncInterval),
	SearchTimeout: settings.Duration(DefaultSearchTimeout),
	ResultCache:   settings.Duration(DefaultResultCache),
})

func (o IndexerOptions) Validate() error {
	var errs []error
	if !within(o.SyncInterval.Std(), MinIndexerSyncInterval, MaxIndexerSyncInterval) {
		errs = append(errs, fmt.Errorf("sync interval must be %s-%s", MinIndexerSyncInterval, MaxIndexerSyncInterval))
	}
	if !within(o.SearchTimeout.Std(), MinSearchTimeout, MaxSearchTimeout) {
		errs = append(errs, fmt.Errorf("search timeout must be %s-%s", MinSearchTimeout, MaxSearchTimeout))
	}
	if !within(o.ResultCache.Std(), 0, MaxResultCache) {
		errs = append(errs, fmt.Errorf("result cache must be 0-%s", MaxResultCache))
	}
	if len(errs) > 0 {
		return fmt.Errorf("%w: %w", ErrInvalidIndexerOptions, errors.Join(errs...))
	}
	return nil
}

func within(d, lowest, highest time.Duration) bool {
	return d >= lowest && d <= highest
}
