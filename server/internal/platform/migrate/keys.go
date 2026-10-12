package migrate

import (
	"fmt"
	"strings"

	"magpie/internal/domain"
)

// Record kinds inside a source key, e.g. "sonarr:episodefile:12".
const (
	recordEpisode        = "episode"
	recordEpisodeFile    = "episodefile"
	recordMovieFile      = "moviefile"
	recordCustomFormat   = "customformat"
	recordQualityProfile = "qualityprofile"
	recordReleaseProfile = "releaseprofile"
	recordDelayProfile   = "delayprofile"
	recordRootFolder     = "rootfolder"
	recordBlocklist      = "blocklist"
	recordHistory        = "history"

	keySeparator   = ":"
	splitSeparator = "/tmdb:"

	releaseProfileIgnored  = "ignored"
	releaseProfileRequired = "required"
	profileLanguage        = "language"
)

func (a App) prefix() string {
	return strings.ToLower(string(a))
}

// titleKey is a series or movie: "sonarr:12", "radarr:40".
func titleKey(app App, id int) domain.SourceKey {
	return domain.SourceKey(fmt.Sprintf("%s%s%d", app.prefix(), keySeparator, id))
}

// splitKey is one of the Magpie titles a split series became.
func splitKey(series domain.SourceKey, tmdb domain.TmdbID) domain.SourceKey {
	return domain.SourceKey(fmt.Sprintf("%s%s%d", series, splitSeparator, tmdb))
}

// recordKey is any other record: "sonarr:episode:812".
func recordKey(app App, record string, id int) domain.SourceKey {
	return domain.SourceKey(fmt.Sprintf("%s%s%s%s%d", app.prefix(), keySeparator, record, keySeparator, id))
}

// derivedKey is a record the migration makes from another, e.g. the format a release profile becomes.
func derivedKey(from domain.SourceKey, part string) domain.SourceKey {
	return domain.SourceKey(string(from) + keySeparator + part)
}
