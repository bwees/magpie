// Copyright (C) Magpie contributors.
// SPDX-License-Identifier: GPL-3.0-or-later

package parse

import (
	"slices"
	"testing"

	"magpie/internal/platform/parse/fixtures"
)

func mustParseMovie(t *testing.T, title string, isDir bool) *movieInfo {
	t.Helper()
	r := parseMovieTitle(title, isDir)
	if r == nil {
		t.Fatalf("parseMovieTitle(%q) = nil", title)
	}
	return r
}

func requireMovieUnparsed(t *testing.T, args fixtures.Args) {
	if r := parseMovieTitle(argString(args, 0), false); r != nil {
		t.Errorf("parseMovieTitle(%q) = %+v, want nil", argString(args, 0), r)
	}
}

func movieSuites() []fixtureSuite {
	releaseGroup := checkReleaseGroup(movieReleaseGroupRules)
	return []fixtureSuite{
		{
			file: fixtures.RadarrParserFixture,
			checks: map[string]checker{
				"should_parse_movie_title": func(t *testing.T, args fixtures.Args) {
					requireEqual(t, "title", mustParseMovie(t, argString(args, 0), false).primaryTitle(), argString(args, 1))
				},
				"should_parse_anime_movie_title": func(t *testing.T, args fixtures.Args) {
					r := mustParseMovie(t, argString(args, 0), false)
					requireEqual(t, "title", r.primaryTitle(), argString(args, 1))
					requireEqual(t, "release group", r.releaseGroup, argString(args, 2))
					requireEqual(t, "year", r.year, argInt(args, 3))
				},
				"should_parse_anime_movie_title_without_year": func(t *testing.T, args fixtures.Args) {
					r := mustParseMovie(t, argString(args, 0), false)
					requireEqual(t, "title", r.primaryTitle(), argString(args, 1))
					requireEqual(t, "release group", r.releaseGroup, argString(args, 2))
				},
				"should_parse_german_movie": func(t *testing.T, args fixtures.Args) {
					r := mustParseMovie(t, argString(args, 0), false)
					requireEqual(t, "title", r.primaryTitle(), argString(args, 1))
					requireEqual(t, "edition", r.edition, argString(args, 2))
					requireEqual(t, "year", r.year, argInt(args, 3))
				},
				"should_parse_movie_alternative_titles": func(t *testing.T, args fixtures.Args) {
					got := mustParseMovie(t, argString(args, 0), true).titles
					if want := argStrings(args, 1); !slices.Equal(got, want) {
						t.Errorf("titles = %q, want %q", got, want)
					}
				},
				"should_parse_movie_folder_name": func(t *testing.T, args fixtures.Args) {
					requireEqual(t, "title", mustParseMovie(t, argString(args, 0), true).primaryTitle(), argString(args, 1))
				},
				"should_parse_movie_year": func(t *testing.T, args fixtures.Args) {
					requireEqual(t, "year", mustParseMovie(t, argString(args, 0), false).year, argInt(args, 1))
				},
				"should_not_parse_wrong_language_in_title": func(t *testing.T, args fixtures.Args) {
					requireEqual(t, "unknown language", isUnknownLanguage(mustParseMovie(t, argString(args, 0), true).languages), true)
				},
			},
			skipped: map[string]string{
				"should_parse_tmdb_id":        reasonNotAField,
				"should_parse_imdb_in_title":  reasonNotAField,
				"should_parse_hardcoded_subs": reasonNotAField,
				"should_normalize_imdbid":     reasonNotPorted,
			},
		},
		{
			file: fixtures.RadarrEditionParserFixture,
			checks: map[string]checker{
				"should_parse_edition": func(t *testing.T, args fixtures.Args) {
					requireEqual(t, "edition", mustParseMovie(t, argString(args, 0), false).edition, argString(args, 1))
				},
				"should_not_parse_edition": func(t *testing.T, args fixtures.Args) {
					requireEqual(t, "edition", mustParseMovie(t, argString(args, 0), false).edition, "")
				},
			},
		},
		{
			file: fixtures.RadarrReleaseGroupParserFixture,
			checks: map[string]checker{
				"should_parse_release_group":                     releaseGroup,
				"should_parse_exception_release_group":           releaseGroup,
				"should_not_include_extension_in_release_group":  releaseGroup,
				"should_not_include_language_in_release_group":   releaseGroup,
				"should_not_include_bad_suffix_in_release_group": releaseGroup,
				"should_parse_anime_release_groups":              releaseGroup,
				"should_not_parse_anime_hash_as_release_group":   releaseGroup,
			},
		},
		{
			file: fixtures.RadarrCrapParserFixture,
			checks: map[string]checker{
				"should_not_parse_crap": requireMovieUnparsed,
				"should_not_parse_random": func(t *testing.T, args fixtures.Args) {
					requireRandomUnparsed(t, argInt(args, 0), func(s string) bool { return parseMovieTitle(s, false) == nil })
				},
				"should_not_parse_file_name_without_proper_spacing": requireMovieUnparsed,
				"should_not_parse_invalid_release_name":             requireMovieUnparsed,
			},
		},
		{
			file: fixtures.RadarrUrlFixture,
			checks: map[string]checker{
				"should_not_parse_url_in_name": func(t *testing.T, args fixtures.Args) {
					got := cleanMovieTitle(mustParseMovie(t, argString(args, 0), false).primaryTitle())
					requireEqual(t, "clean title", got, cleanMovieTitle(argString(args, 1)))
				},
				"should_not_parse_url_in_group": releaseGroup,
			},
		},
		{file: fixtures.RadarrAnimeVersionFixture, checks: map[string]checker{"should_be_able_to_parse_repack": versionCheck(movieQualityRules)}},
	}
}

func TestRadarrFixtures(t *testing.T) {
	runSuites(t, movieSuites())
}

func TestRadarrHashedReleases(t *testing.T) {
	for _, c := range fixtures.RadarrHashedReleases {
		t.Run(c.Path, func(t *testing.T) {
			r := parseMoviePath(osAgnostic(c.Path))
			if r == nil {
				t.Fatalf("parseMoviePath(%q) = nil", c.Path)
			}
			requireEqual(t, "title", r.primaryTitle(), c.Title)
			requireEqual(t, "quality", r.quality.id, radarrQuality(t, c.Quality))
			requireEqual(t, "release group", r.releaseGroup, c.Group)
		})
	}
}

func TestRadarrDoesNotParseMD5(t *testing.T) {
	requireMD5Unparsed(t, func(s string) bool { return parseMovieTitle(s, false) == nil })
}
