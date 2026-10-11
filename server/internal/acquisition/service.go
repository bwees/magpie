package acquisition

import (
	"context"
	"sync"
	"time"

	"magpie/internal/platform/settings"
)

// Service holds Acquisition's business logic.
type Service struct {
	repo     *Repository
	prowlarr Prowlarr
	store    *settings.Store
	now      func() time.Time

	mu       sync.Mutex
	lastSync IndexerSync
}

func NewService(repo *Repository, prowlarr Prowlarr, store *settings.Store) *Service {
	return &Service{repo: repo, prowlarr: prowlarr, store: store, now: time.Now}
}

// IndexerSync is the outcome of the latest sync since the server started.
type IndexerSync struct {
	At  time.Time // zero until the first sync finishes
	Err error
}

// IndexerList is the Settings → Indexers view.
type IndexerList struct {
	Indexers []Indexer // gone indexers left out
	WebURL   string    // Prowlarr's UI; "" when no connection is set up
	LastSync IndexerSync
}

// SyncIndexers pulls Prowlarr's indexer list and applies it. A failed pull changes no rows.
func (s *Service) SyncIndexers(ctx context.Context) error {
	listed, err := s.prowlarr.Indexers(ctx)
	if err == nil {
		now := s.now().UTC()
		err = s.repo.ApplyIndexerSync(ctx, func(stored []Indexer) []Indexer {
			return mergeIndexerSync(stored, listed, now)
		})
	}
	s.mu.Lock()
	s.lastSync = IndexerSync{At: s.now().UTC(), Err: err}
	s.mu.Unlock()
	return err
}

func (s *Service) LastIndexerSync() IndexerSync {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastSync
}

func (s *Service) Indexers(ctx context.Context) (IndexerList, error) {
	indexers, err := s.repo.Indexers(ctx, false)
	if err != nil {
		return IndexerList{}, err
	}
	webURL, err := s.prowlarr.WebURL(ctx)
	if err != nil {
		return IndexerList{}, err
	}
	return IndexerList{Indexers: indexers, WebURL: webURL, LastSync: s.LastIndexerSync()}, nil
}

// UpdateIndexer replaces Magpie's settings for one indexer. Returns api.ErrNotFound for an unknown ID.
func (s *Service) UpdateIndexer(ctx context.Context, prowlarrID int, update IndexerSettings) (Indexer, error) {
	if err := update.Validate(); err != nil {
		return Indexer{}, err
	}
	return s.repo.UpdateIndexerSettings(ctx, prowlarrID, update)
}

func (s *Service) IndexerOptions(ctx context.Context) (IndexerOptions, error) {
	return IndexerOptionsKey.Get(ctx, s.store)
}

func (s *Service) SetIndexerOptions(ctx context.Context, o IndexerOptions) error {
	if err := o.Validate(); err != nil {
		return err
	}
	return IndexerOptionsKey.Set(ctx, s.store, o)
}
