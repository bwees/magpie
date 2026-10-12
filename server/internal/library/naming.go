package library

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
)

// The fixed Jellyfin naming, from library › Behaviour step 4.
const (
	titleFolderShape = "%s [tmdbid-%d]"
	yearShape        = "%s (%d)"
	seasonShape      = "Season %02d"
	episodeShape     = "%s S%02dE%02d"
	lastEpisodeShape = "-E%02d"

	colonBeforeSpace = ": "
	dashBeforeSpace  = " - "
	colon            = ":"
	dash             = "-"
	// forbiddenInNames are the characters Windows, macOS or Linux shares reject in a file name.
	forbiddenInNames = `<>"/\|?*`
	trailingTrim     = ". "
)

// sanitiseName makes a title safe as one path segment: colons become dashes, forbidden characters and
// control characters are dropped, and trailing dots and spaces are trimmed.
func sanitiseName(name string) string {
	name = strings.ReplaceAll(name, colonBeforeSpace, dashBeforeSpace)
	name = strings.ReplaceAll(name, colon, dash)
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(forbiddenInNames, r) {
			return -1
		}
		return r
	}, name)
	return strings.TrimRight(strings.TrimSpace(name), trailingTrim)
}

// titleName is "Title (Year)", or "Title" when the year is unknown.
func titleName(t catalogapi.Title) string {
	name := sanitiseName(t.Name)
	if t.Year == 0 {
		return name
	}
	return fmt.Sprintf(yearShape, name, t.Year)
}

func titleFolder(root string, t catalogapi.Title) string {
	return filepath.Join(root, fmt.Sprintf(titleFolderShape, titleName(t), t.External.Tmdb))
}

// moviePath is <root>/<Title> (<Year>) [tmdbid-<id>]/<Title> (<Year>).<ext>.
func moviePath(root string, t catalogapi.Title, ext string) string {
	return filepath.Join(titleFolder(root, t), titleName(t)+ext)
}

// episodePath is <root>/<Show> (<Year>) [tmdbid-<id>]/Season <NN>/<Show> S<NN>E<NN>[-E<NN>].<ext>, numbered
// by the title's current episode group. episodes are in (season, number) order.
func episodePath(root string, t catalogapi.Title, episodes []domain.EpisodeRef, ext string) string {
	return filepath.Join(titleFolder(root, t), episodeInShow(t, episodes, ext))
}

// episodeInShow is Season <NN>/<Show> S<NN>E<NN>[-E<NN>].<ext>, the part of an episode's path inside its show's folder.
func episodeInShow(t catalogapi.Title, episodes []domain.EpisodeRef, ext string) string {
	first, last := episodes[0], episodes[len(episodes)-1]
	name := fmt.Sprintf(episodeShape, sanitiseName(t.Name), first.Season, first.Number)
	if last.Number != first.Number || last.Season != first.Season {
		name += fmt.Sprintf(lastEpisodeShape, last.Number)
	}
	return filepath.Join(fmt.Sprintf(seasonShape, first.Season), name+ext)
}
