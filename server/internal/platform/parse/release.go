// Copyright (C) Magpie contributors.
// SPDX-License-Identifier: GPL-3.0-or-later

package parse

import (
	"fmt"
	"strings"
	"time"

	"magpie/internal/domain"
)

// Release is what a release name or file name says about its content.
type Release struct {
	// Name is the input as given.
	Name string
	Kind domain.MediaKind
	// Title is the parsed title without its year; Aliases are the alternative
	// titles found in it ("A AKA B", "A (B)", "A / B").
	Title   string
	Aliases []string
	// Year is the year in the name, or 0. Matching with a ±1 year tolerance is the caller's job.
	Year int

	// Season is the first season; Seasons lists every season in a multi-season pack.
	Season  int
	Seasons []int
	// Episodes is empty for season packs and daily episodes.
	Episodes []int
	// AirDate is set for daily episodes, formatted with AirDateLayout.
	AirDate       string
	DailyPart     int
	FullSeason    bool
	MultiSeason   bool
	PartialSeason bool
	SeasonExtra   bool
	Special       bool
	SplitEpisode  bool

	// Sample is true when the name says "sample". Sonarr also checks the size; that is the caller's job.
	Sample bool

	Quality  domain.Quality
	Revision Revision
	Group    string
	// Languages are the audio languages named in the release; empty means unknown.
	// OriginalLanguage is set by tags such as "orig" or German "DL".
	Languages        []domain.Language
	OriginalLanguage bool
	// Edition is a movie edition such as "Director's Cut" or "IMAX".
	Edition string
}

// UnableToParse is returned when a name matches none of the parser's patterns.
type UnableToParse struct {
	Name string
	Kind domain.MediaKind
}

func (e *UnableToParse) Error() string {
	return fmt.Sprintf("unable to parse %s release %q", strings.ToLower(string(e.Kind)), e.Name)
}

const sampleMarker = "sample"

// ParseRelease parses a release name as a movie (Radarr's rules) or an episode or pack (Sonarr's rules).
func ParseRelease(name string, kind domain.MediaKind) (Release, error) {
	return parseRelease(name, kind, time.Now())
}

// ParseFile parses a file path, falling back to its folder name as Sonarr and Radarr do.
func ParseFile(path string, kind domain.MediaKind) (Release, error) {
	return parseFile(path, kind, time.Now())
}

func parseRelease(name string, kind domain.MediaKind, now time.Time) (Release, error) {
	switch kind {
	case domain.MediaKindSeries:
		return episodeRelease(name, kind, parseEpisodeTitle(name, now))
	case domain.MediaKindMovie:
		return movieRelease(name, kind, parseMovieTitle(name, false))
	}
	return Release{}, fmt.Errorf("parse: unknown media kind %q", kind)
}

func parseFile(path string, kind domain.MediaKind, now time.Time) (Release, error) {
	switch kind {
	case domain.MediaKindSeries:
		return episodeRelease(path, kind, parseEpisodePath(path, now))
	case domain.MediaKindMovie:
		return movieRelease(path, kind, parseMoviePath(path))
	}
	return Release{}, fmt.Errorf("parse: unknown media kind %q", kind)
}

func episodeRelease(name string, kind domain.MediaKind, info *episodeInfo) (Release, error) {
	if info == nil {
		return Release{}, &UnableToParse{Name: name, Kind: kind}
	}
	return Release{
		Name:             name,
		Kind:             kind,
		Title:            info.titleInfo.titleWithoutYear,
		Aliases:          info.titleInfo.allTitles,
		Year:             info.titleInfo.year,
		Season:           info.seasonNumber(),
		Seasons:          info.seasonNumbers,
		Episodes:         info.episodeNumbers,
		AirDate:          info.airDate,
		DailyPart:        info.dailyPart,
		FullSeason:       info.fullSeason,
		MultiSeason:      len(info.seasonNumbers) > 1,
		PartialSeason:    info.partialSeason,
		SeasonExtra:      info.seasonExtra,
		Special:          info.special,
		SplitEpisode:     info.splitEpisode,
		Sample:           containsIgnoreCase(name, sampleMarker),
		Quality:          info.quality.domain(),
		Revision:         info.quality.revision,
		Group:            info.releaseGroup,
		Languages:        info.languages.list,
		OriginalLanguage: info.languages.original,
	}, nil
}

func movieRelease(name string, kind domain.MediaKind, info *movieInfo) (Release, error) {
	if info == nil {
		return Release{}, &UnableToParse{Name: name, Kind: kind}
	}
	return Release{
		Name:             name,
		Kind:             kind,
		Title:            info.primaryTitle(),
		Aliases:          info.titles[1:],
		Year:             info.year,
		Sample:           containsIgnoreCase(name, sampleMarker),
		Quality:          info.quality.domain(),
		Revision:         info.quality.revision,
		Group:            info.releaseGroup,
		Languages:        info.languages.list,
		OriginalLanguage: info.languages.original,
		Edition:          info.edition,
	}, nil
}
