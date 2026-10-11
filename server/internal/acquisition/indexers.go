package acquisition

import (
	"errors"
	"fmt"
	"time"

	"magpie/internal/acquisition/api"
)

// IndexerStatus is how an indexer looked at the last sync with Prowlarr.
type IndexerStatus string

const (
	IndexerOK      IndexerStatus = "OK"
	IndexerFailing IndexerStatus = "FAILING"
	// IndexerGone was removed or disabled in Prowlarr. The row stays so downloads and history keep their indexer.
	IndexerGone IndexerStatus = "GONE"
)

// SeedingOverride replaces the global after-download mode for one indexer's releases.
type SeedingOverride string

const (
	OverrideUseGlobal      SeedingOverride = "USE_GLOBAL"
	OverrideSeedUntilGoals                 = SeedingOverride(api.SeedUntilGoals)
	OverrideDontSeed                       = SeedingOverride(api.DontSeed)
)

func (o SeedingOverride) Valid() bool {
	switch o {
	case OverrideUseGlobal, OverrideSeedUntilGoals, OverrideDontSeed:
		return true
	}
	return false
}

// Ranges from 12-settings › Indexers.
const (
	DefaultIndexerPriority = 25
	MinIndexerPriority     = 1
	MaxIndexerPriority     = 50

	DefaultMinSeeders = 1
	MaxMinSeeders     = 100

	MaxSeedRatio = 100
	MaxSeedDays  = 365
)

var ErrInvalidIndexerSettings = errors.New("acquisition: invalid indexer settings")

// IndexerSettings is Magpie's layer on top of an indexer configured in Prowlarr.
type IndexerSettings struct {
	Priority        int // lower is preferred
	MinSeeders      int
	SeedingOverride SeedingOverride
	SeedRatio       *float64 // nil = the global goal
	SeedDays        *int     // nil = the global goal
}

func DefaultIndexerSettings() IndexerSettings {
	return IndexerSettings{Priority: DefaultIndexerPriority, MinSeeders: DefaultMinSeeders, SeedingOverride: OverrideUseGlobal}
}

func (s IndexerSettings) Validate() error {
	var errs []error
	if s.Priority < MinIndexerPriority || s.Priority > MaxIndexerPriority {
		errs = append(errs, fmt.Errorf("priority must be %d-%d", MinIndexerPriority, MaxIndexerPriority))
	}
	if s.MinSeeders < 0 || s.MinSeeders > MaxMinSeeders {
		errs = append(errs, fmt.Errorf("minimum seeders must be 0-%d", MaxMinSeeders))
	}
	if !s.SeedingOverride.Valid() {
		errs = append(errs, fmt.Errorf("unknown seeding override %q", s.SeedingOverride))
	}
	if s.SeedRatio != nil && (*s.SeedRatio < 0 || *s.SeedRatio > MaxSeedRatio) {
		errs = append(errs, fmt.Errorf("seed ratio must be 0-%d", MaxSeedRatio))
	}
	if s.SeedDays != nil && (*s.SeedDays < 0 || *s.SeedDays > MaxSeedDays) {
		errs = append(errs, fmt.Errorf("seed days must be 0-%d", MaxSeedDays))
	}
	if len(errs) > 0 {
		return fmt.Errorf("%w: %w", ErrInvalidIndexerSettings, errors.Join(errs...))
	}
	return nil
}

// Indexer is one Prowlarr indexer as Acquisition stores it.
type Indexer struct {
	ProwlarrID      int
	Name            string
	Privacy         IndexerPrivacy
	Caps            TorznabCaps
	Status          IndexerStatus
	LastError       string
	FailingSince    *time.Time
	AverageResponse time.Duration
	LastOKAt        *time.Time
	SyncedAt        time.Time
	Settings        IndexerSettings
}

// mergeIndexerSync applies Prowlarr's list to the stored indexers and returns the rows to save.
// New indexers get default settings, listed ones get fresh caps and health, and unlisted ones are
// marked gone without losing their settings.
func mergeIndexerSync(stored []Indexer, listed []ProwlarrIndexer, now time.Time) []Indexer {
	byID := make(map[int]Indexer, len(stored))
	for _, ix := range stored {
		byID[ix.ProwlarrID] = ix
	}

	changed := make([]Indexer, 0, len(listed))
	for _, remote := range listed {
		ix, known := byID[remote.ID]
		if !known {
			ix = Indexer{ProwlarrID: remote.ID, Settings: DefaultIndexerSettings()}
		}
		delete(byID, remote.ID)
		changed = append(changed, ix.refresh(remote, now))
	}

	for _, ix := range byID {
		if ix.Status == IndexerGone {
			continue
		}
		ix.Status = IndexerGone
		ix.SyncedAt = now
		changed = append(changed, ix)
	}
	return changed
}

func (ix Indexer) refresh(remote ProwlarrIndexer, now time.Time) Indexer {
	ix.Name = remote.Name
	ix.Privacy = remote.Privacy
	ix.AverageResponse = remote.Health.AverageResponse
	ix.SyncedAt = now
	if remote.Caps != nil {
		ix.Caps = *remote.Caps
	}

	if remote.Health.Error == "" {
		ix.Status = IndexerOK
		ix.LastError = ""
		ix.FailingSince = nil
		ix.LastOKAt = &now
		return ix
	}

	if ix.Status != IndexerFailing || ix.FailingSince == nil {
		ix.FailingSince = &now
	}
	if remote.Health.FailingSince != nil {
		ix.FailingSince = remote.Health.FailingSince
	}
	ix.Status = IndexerFailing
	ix.LastError = remote.Health.Error
	return ix
}
