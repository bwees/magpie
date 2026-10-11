// Copyright (C) Magpie contributors.
// Portions ported from Sonarr (https://github.com/Sonarr/Sonarr) and
// Radarr (https://github.com/Radarr/Radarr), copyright their contributors.
// SPDX-License-Identifier: GPL-3.0-or-later

// Package parse turns a release name or a file path into a Release: title,
// year, season and episodes, quality, release group, languages and edition.
//
// It is a Go port of the release-title parsers in Sonarr (series) and Radarr
// (movies), both licensed GPL-3.0-or-later. Series names follow Sonarr's rules
// and movie names follow Radarr's, so the caller passes the media kind it is
// looking for. The vendored Sonarr and Radarr parser fixtures are the test suite.
//
// Anime and absolute episode numbering are out of scope for v1. Their regexes
// stay in the ordered pattern list so non-anime names match exactly as in Sonarr.
//
// The regexes run on dlclark/regexp2, a port of the .NET engine, so they are copied
// unchanged; one is split in two (release_group.go). Cheap gates (gates.go) skip regexes
// that cannot match a name; they change speed, not results.
//
// The package is pure: no I/O and no mutable state.
package parse
