// Copyright (C) Magpie contributors.
// SPDX-License-Identifier: GPL-3.0-or-later

// Package fixtures holds Sonarr's and Radarr's parser test cases, vendored as Go
// data for the parse package's table tests. Each file names its source file and
// commit. Sonarr and Radarr are GPL-3.0-or-later; see their COPYRIGHT and LICENSE files.
//
// Files generated from [TestCase] attributes keep each case's NUnit method name
// and arguments unchanged, so a case can be traced back to its C# source line.
//
// Not vendored, because they test code outside the name parser: Sonarr's
// NormalizeSeriesTitle, NormalizeEpisodeTitle, SceneChecker, IsoLanguages,
// IsPossibleSpecialEpisode, ReleaseComparer, SeriesTitleInfo and
// ValidateParsedEpisodeInfo fixtures, and Radarr's NormalizeTitle, SlugParser,
// SceneChecker, IsoLanguages and ParsingServiceTests.
package fixtures

// Args are a test case's arguments in NUnit order: string, int, bool, float64,
// []int, []string, or nil for C# null.
type Args = []any

// Case is one NUnit [TestCase]: the test method it feeds and its arguments.
type Case struct {
	Line   int
	Method string
	Args   Args
}

// File is every [TestCase] in one Sonarr or Radarr fixture file.
type File struct {
	Source string
	Cases  []Case
}
