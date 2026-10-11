package catalog

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"magpie/internal/platform/owner"
	"magpie/internal/platform/settings"
)

const (
	DefaultMetadataLanguage = "en-US"
	DefaultReleaseRegion    = "US"

	defaultMovieRefresh = 7 * 24 * time.Hour
	defaultReleaseClock = time.Hour
)

var ErrInvalidSettings = errors.New("catalog: invalid settings")

// MetadataSettings are the TMDB language for names and overviews, and the region whose release dates count.
type MetadataSettings struct {
	Language      string `json:"language"`
	ReleaseRegion string `json:"releaseRegion"`
}

var (
	// languageTag is TMDB's language form: ISO 639-1, optionally with an ISO 3166-1 region ("en", "pt-BR").
	languageTag = regexp.MustCompile(`^[a-z]{2}(-[A-Z]{2})?$`)
	// regionCode is ISO 3166-1 alpha-2.
	regionCode = regexp.MustCompile(`^[A-Z]{2}$`)
)

func (m MetadataSettings) Validate() error {
	if !languageTag.MatchString(m.Language) {
		return fmt.Errorf("%w: language %q is not like en or en-US", ErrInvalidSettings, m.Language)
	}
	if !regionCode.MatchString(m.ReleaseRegion) {
		return fmt.Errorf("%w: release region %q is not a two-letter country code", ErrInvalidSettings, m.ReleaseRegion)
	}
	return nil
}

var (
	metadataSetting = settings.NewKey(owner.Catalog, "metadata", MetadataSettings{
		Language:      DefaultMetadataLanguage,
		ReleaseRegion: DefaultReleaseRegion,
	})
	movieRefreshInterval = settings.NewKey(owner.Catalog, "schedule.movieRefresh", settings.Duration(defaultMovieRefresh))
	releaseClockInterval = settings.NewKey(owner.Catalog, "schedule.releaseClock", settings.Duration(defaultReleaseClock))
	// refreshPaused is set when TMDB rejects the key and cleared when the TMDB connection is healthy again.
	refreshPaused = settings.NewKey(owner.Catalog, "refresh.paused", false)
)
