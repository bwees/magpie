package acquisition

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"magpie/internal/acquisition/api"
	"magpie/internal/platform/owner"
	"magpie/internal/platform/settings"
)

// Defaults and ranges from 12-settings › Downloads and 01-architecture › Background work.
const (
	DefaultMetadataTimeout = 5 * time.Minute
	MinMetadataTimeout     = time.Minute
	MaxMetadataTimeout     = time.Hour

	DefaultStalledLimit = 6 * time.Hour
	MinStalledLimit     = 30 * time.Minute
	MaxStalledLimit     = 72 * time.Hour

	// TemporaryBlockFor is how long a metadata timeout or a stall keeps a release blocklisted.
	TemporaryBlockFor = 24 * time.Hour

	ActivePollInterval = 2 * time.Second
	IdlePollInterval   = 15 * time.Second

	// RecentGrabWindow is how far back the History spec looks for grabs, as Sonarr's does.
	RecentGrabWindow = 12 * time.Hour
)

var ErrInvalidDownloadOptions = errors.New("acquisition: invalid download options")

// DownloadOptions are Settings → Downloads: the blocked-file check and the download timeouts.
type DownloadOptions struct {
	BlockedFilesEnabled bool              `json:"blockedFilesEnabled"`
	BlockedPatterns     []string          `json:"blockedPatterns"`
	MetadataTimeout     settings.Duration `json:"metadataTimeout"`
	StalledLimit        settings.Duration `json:"stalledLimit"`
}

var DownloadOptionsKey = settings.NewKey(owner.Acquisition, "downloads", DownloadOptions{
	BlockedFilesEnabled: true,
	BlockedPatterns:     slices.Clone(DefaultBlockedPatterns),
	MetadataTimeout:     settings.Duration(DefaultMetadataTimeout),
	StalledLimit:        settings.Duration(DefaultStalledLimit),
})

// SeedingModeKey is the global "after download" mode; an indexer's override wins over it.
var SeedingModeKey = settings.NewKey(owner.Acquisition, "seedingMode", api.SeedUntilGoals)

func (o DownloadOptions) Validate() error {
	var errs []error
	if _, err := compileBlockedPatterns(o.BlockedPatterns); err != nil {
		errs = append(errs, err)
	}
	if !within(o.MetadataTimeout.Std(), MinMetadataTimeout, MaxMetadataTimeout) {
		errs = append(errs, fmt.Errorf("metadata timeout must be %s-%s", MinMetadataTimeout, MaxMetadataTimeout))
	}
	if !within(o.StalledLimit.Std(), MinStalledLimit, MaxStalledLimit) {
		errs = append(errs, fmt.Errorf("stalled limit must be %s-%s", MinStalledLimit, MaxStalledLimit))
	}
	if len(errs) > 0 {
		return fmt.Errorf("%w: %w", ErrInvalidDownloadOptions, errors.Join(errs...))
	}
	return nil
}

// blockedFiles is the check to run, or none when the user turned it off.
func (o DownloadOptions) blockedFiles() (blockedFiles, error) {
	if !o.BlockedFilesEnabled {
		return blockedFiles{}, nil
	}
	return compileBlockedPatterns(o.BlockedPatterns)
}
